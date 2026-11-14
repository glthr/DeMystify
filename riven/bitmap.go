package riven

// Stored Riven tBMP images use byte-oriented pixel histories and local BGR
// palettes. Format facts were checked against ScummVM's Mohawk bitmap reader
// at aed0208e772b128d3f953207a297fc521c32833a. This decoder does not simulate
// script state, picture compositing, movies, or the game's rendering effects.

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"

	"github.com/glthr/DeMystify/assets"
)

const maxBitmapBytes = 64 << 20

// DecodeBitmap decodes the stored raw or Riven-packed tBMP resource. Indexed
// images remain paletted, preserving the original colour table in local PNGs.
func DecodeBitmap(data []byte) (image.Image, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("riven bitmap: truncated header")
	}
	width := int(binary.BigEndian.Uint16(data[0:2]) & 0x3fff)
	height := int(binary.BigEndian.Uint16(data[2:4]) & 0x3fff)
	stride := int(binary.BigEndian.Uint16(data[4:6]) & 0x3ffe)
	format := binary.BigEndian.Uint16(data[6:8])
	channels := 0
	switch format & 7 {
	case 2:
		channels = 1
	case 4:
		channels = 3
	default:
		return nil, fmt.Errorf("riven bitmap: unsupported depth %d", format&7)
	}
	if width <= 0 || width > 8192 || height <= 0 || height > 8192 || stride < width*channels || stride*height > maxBitmapBytes {
		return nil, fmt.Errorf("riven bitmap: invalid dimensions or stride")
	}
	pack, draw := format&0x0f00, format&0x00f0
	if draw != 0 {
		return nil, fmt.Errorf("riven bitmap: unsupported draw format 0x%02x", draw)
	}
	pos := 8
	var palette color.Palette
	if format&8 != 0 || pack == 0x400 && channels == 1 {
		if len(data)-pos < 772 {
			return nil, fmt.Errorf("riven bitmap: truncated palette")
		}
		pos += 4 // Stored palette metadata; Riven's local tables have 256 entries.
		palette = make(color.Palette, 256)
		for i := range palette {
			bgr := data[pos+i*3 : pos+i*3+3]
			palette[i] = color.RGBA{R: bgr[2], G: bgr[1], B: bgr[0], A: 255}
		}
		pos += 768
	}
	if channels == 1 && palette == nil {
		return nil, fmt.Errorf("riven bitmap: indexed image has no local palette")
	}
	var pixels []byte
	switch pack {
	case 0:
		if len(data)-pos < stride*height {
			return nil, fmt.Errorf("riven bitmap: truncated raw pixels")
		}
		pixels = data[pos : pos+stride*height]
	case 0x400:
		if len(data)-pos < 4 {
			return nil, fmt.Errorf("riven bitmap: truncated packing header")
		}
		var err error
		pixels, err = unpackRiven(data[pos+4:], stride*height)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("riven bitmap: unsupported packing 0x%03x", pack)
	}
	if channels == 1 {
		result := image.NewPaletted(image.Rect(0, 0, width, height), palette)
		for y := 0; y < height; y++ {
			copy(result.Pix[y*result.Stride:y*result.Stride+width], pixels[y*stride:y*stride+width])
		}
		return result, nil
	}
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			p := y*stride + x*3
			result.SetNRGBA(x, y, color.NRGBA{R: pixels[p+2], G: pixels[p+1], B: pixels[p], A: 255})
		}
	}
	return result, nil
}

func unpackRiven(data []byte, expected int) ([]byte, error) {
	if expected <= 0 || expected > maxBitmapBytes {
		return nil, fmt.Errorf("riven pixels: invalid unpacked size")
	}
	output, pos := make([]byte, 0, expected), 0
	next := func() (byte, error) {
		if pos >= len(data) {
			return 0, fmt.Errorf("riven pixels: truncated command stream")
		}
		value := data[pos]
		pos++
		return value, nil
	}
	literal := func(count int) error {
		if count < 0 || count > len(data)-pos {
			return fmt.Errorf("riven pixels: truncated literal")
		}
		if count > expected-len(output) {
			return fmt.Errorf("riven pixels: output exceeds image bounds")
		}
		output = append(output, data[pos:pos+count]...)
		pos += count
		return nil
	}
	previous := func(distance, delta int) error {
		if distance <= 0 || distance > len(output) {
			return fmt.Errorf("riven pixels: reference precedes output")
		}
		if len(output) >= expected {
			return fmt.Errorf("riven pixels: output exceeds image bounds")
		}
		output = append(output, byte(int(output[len(output)-distance])+delta))
		return nil
	}
	history := func(distance, count int) error {
		if distance <= 0 || distance > len(output) {
			return fmt.Errorf("riven pixels: history precedes output")
		}
		if count < 0 || count > expected-len(output) {
			return fmt.Errorf("riven pixels: output exceeds image bounds")
		}
		// The moving source window deliberately permits overlapping runs.
		for i := 0; i < count; i++ {
			output = append(output, output[len(output)-distance])
		}
		return nil
	}
	runs := map[byte]struct {
		count int
		tail  bool
	}{0xa4: {3, true}, 0xa8: {4, false}, 0xac: {5, true}, 0xb4: {6, false}, 0xb8: {7, true},
		0xbc: {8, false}, 0xe4: {9, true}, 0xe8: {10, false}, 0xec: {11, true}, 0xf4: {12, false}, 0xf8: {13, true}}
	for len(output) < expected {
		command, err := next()
		if err != nil {
			return nil, err
		}
		if command == 0 {
			break
		}
		switch {
		case command < 0x40:
			err = literal(int(command) * 2)
		case command < 0x80:
			count := int(command - 0x40)
			if count > 0 {
				if len(output) < 2 || count*2 > expected-len(output) {
					return nil, fmt.Errorf("riven pixels: invalid repeated duplet")
				}
				a, b := output[len(output)-2], output[len(output)-1]
				for i := 0; i < count; i++ {
					output = append(output, a, b)
				}
			}
		case command < 0xc0:
			count := int(command - 0x80)
			if count > 0 {
				if len(output) < 4 || count*4 > expected-len(output) {
					return nil, fmt.Errorf("riven pixels: invalid repeated four-pixel group")
				}
				var group [4]byte
				copy(group[:], output[len(output)-4:])
				for i := 0; i < count; i++ {
					output = append(output, group[:]...)
				}
			}
		default:
			for i := 0; i < int(command-0xc0); i++ {
				sub, readErr := next()
				if readErr != nil {
					return nil, readErr
				}
				low := int(sub & 15)
				// Each helper samples after the preceding append. Sampling both
				// bytes before writing would corrupt dependent duplet operations.
				switch {
				case sub >= 1 && sub <= 15:
					err = previous(low*2, 0)
					if err == nil {
						err = previous(low*2, 0)
					}
				case sub == 0x10:
					err = previous(2, 0)
					if err == nil {
						err = literal(1)
					}
				case sub >= 0x11 && sub <= 0x1f:
					err = previous(2, 0)
					if err == nil {
						err = previous(low, 0)
					}
				case sub >= 0x20 && sub <= 0x2f:
					err = previous(2, 0)
					if err == nil {
						err = previous(2, low)
					}
				case sub >= 0x30 && sub <= 0x3f:
					err = previous(2, 0)
					if err == nil {
						err = previous(2, -low)
					}
				case sub == 0x40:
					err = literal(1)
					if err == nil {
						err = previous(2, 0)
					}
				case sub >= 0x41 && sub <= 0x4f:
					err = previous(low, 0)
					if err == nil {
						err = previous(2, 0)
					}
				case sub == 0x50:
					err = literal(2)
				case sub >= 0x51 && sub <= 0x57:
					err = previous(int(sub&7), 0)
					if err == nil {
						err = literal(1)
					}
				case sub >= 0x59 && sub <= 0x5f:
					err = literal(1)
					if err == nil {
						err = previous(int(sub&7), 0)
					}
				case sub >= 0x60 && sub <= 0x6f:
					err = literal(1)
					if err == nil {
						err = previous(2, low)
					}
				case sub >= 0x70 && sub <= 0x7f:
					err = literal(1)
					if err == nil {
						err = previous(2, -low)
					}
				case sub >= 0x80 && sub <= 0x8f:
					err = previous(2, low)
					if err == nil {
						err = previous(2, 0)
					}
				case sub >= 0x90 && sub <= 0x9f:
					err = previous(2, low)
					if err == nil {
						err = literal(1)
					}
				case sub == 0xa0 || sub == 0xb0 || sub == 0xe0 || sub == 0xf0 || sub == 0xff:
					pattern, readErr := next()
					if readErr != nil {
						return nil, readErr
					}
					first, second := int(pattern>>4), int(pattern&15)
					if sub != 0xa0 && sub != 0xb0 {
						first = -first
					}
					if sub != 0xa0 && sub != 0xe0 {
						second = -second
					}
					err = previous(2, first)
					if err == nil {
						err = previous(2, second)
					}
				case sub >= 0xc0 && sub <= 0xcf:
					err = previous(2, -low)
					if err == nil {
						err = previous(2, 0)
					}
				case sub >= 0xd0 && sub <= 0xdf:
					err = previous(2, -low)
					if err == nil {
						err = literal(1)
					}
				case sub == 0xfc:
					control, readErr := next()
					if readErr != nil {
						return nil, readErr
					}
					lowDistance, readErr := next()
					if readErr != nil {
						return nil, readErr
					}
					distance := int(control&3)<<8 | int(lowDistance)
					err = history(distance, (int(control>>3)+1)*2+1)
					if err == nil {
						if control&4 != 0 {
							err = history(distance, 1)
						} else {
							err = literal(1)
						}
					}
				default:
					if run, ok := runs[sub&0xfc]; ok {
						lowDistance, readErr := next()
						if readErr != nil {
							return nil, readErr
						}
						err = history(int(sub&3)<<8|int(lowDistance), run.count)
						if err == nil && run.tail {
							err = literal(1)
						}
					} else {
						err = fmt.Errorf("riven pixels: unsupported subcommand 0x%02x", sub)
					}
				}
				if err != nil {
					return nil, err
				}
			}
		}
		if err != nil {
			return nil, err
		}
	}
	if len(output) != expected {
		return nil, fmt.Errorf("riven pixels: truncated output (%d of %d)", len(output), expected)
	}
	return output, nil
}

func bitmapThumbnail(source image.Image, maxWidth int) image.Image {
	box := source.Bounds()
	if box.Dx() <= maxWidth {
		return source
	}
	width, height := maxWidth, max(1, (box.Dy()*maxWidth+box.Dx()/2)/box.Dx())
	if indexed, ok := source.(*image.Paletted); ok {
		result := image.NewPaletted(image.Rect(0, 0, width, height), indexed.Palette)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				result.SetColorIndex(x, y, indexed.ColorIndexAt(box.Min.X+x*box.Dx()/width, box.Min.Y+y*box.Dy()/height))
			}
		}
		return result
	}
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			result.Set(x, y, source.At(box.Min.X+x*box.Dx()/width, box.Min.Y+y*box.Dy()/height))
		}
	}
	return result
}

func writeBitmapPNG(path string, source image.Image) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".riven-png-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err = encoder.Encode(temporary, source); err == nil {
		err = temporary.Close()
	} else {
		temporary.Close()
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}

// ExtractImages writes one stored default PLST view per shipped card into
// output, returning the absolute directory containing images.json. This uses
// the same assets manifest consumed by --assets; game files remain untouched.
// Thumbnails are available locally as well; the shared asset pipeline can
// prepare its own display sizes from the lossless originals.
func (imported *Import) ExtractImages(output string) (string, error) {
	if imported == nil || imported.Metadata == nil {
		return "", fmt.Errorf("riven images: no imported graph")
	}
	root, err := filepath.Abs(output)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Join(root, "images"), 0700); err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Join(root, "thumbnails"), 0700); err != nil {
		return "", err
	}
	var manifest assets.Manifest
	decoded := make(map[string]string)
	stackNames := make([]string, 0, len(imported.Stacks))
	for name := range imported.Stacks {
		stackNames = append(stackNames, name)
	}
	sort.Strings(stackNames)
	for _, name := range stackNames {
		stack := imported.Stacks[name]
		cardIDs := make([]int, 0, len(stack.Cards))
		for id := range stack.Cards {
			cardIDs = append(cardIDs, int(id))
		}
		sort.Ints(cardIDs)
		for _, id := range cardIDs {
			card := stack.Cards[uint16(id)]
			pictures := append([]Picture(nil), card.Pictures...)
			if len(pictures) == 0 {
				return "", fmt.Errorf("riven images: %s:%d has no PLST picture", name, id)
			}
			sort.SliceStable(pictures, func(i, j int) bool {
				if (pictures[i].Index == 1) != (pictures[j].Index == 1) {
					return pictures[i].Index == 1
				}
				a, b := pictures[i].Rect, pictures[j].Rect
				return (a[2]-a[0])*(a[3]-a[1]) > (b[2]-b[0])*(b[3]-b[1])
			})
			picture := pictures[0]
			key := fmt.Sprintf("%s:%d", name, picture.ImageID)
			path, seen := decoded[key]
			if !seen {
				ref, ok := stack.Resources["tBMP"][picture.ImageID]
				if !ok {
					return "", fmt.Errorf("riven images: %s card %d has no tBMP:%d", name, id, picture.ImageID)
				}
				raw, readErr := ref.Archive.Read("tBMP", picture.ImageID)
				if readErr != nil {
					return "", fmt.Errorf("riven images %s: %w", key, readErr)
				}
				bitmap, decodeErr := DecodeBitmap(raw)
				if decodeErr != nil {
					return "", fmt.Errorf("riven images %s: %w", key, decodeErr)
				}
				filename := fmt.Sprintf("%s-%d.png", name, picture.ImageID)
				path = filepath.ToSlash(filepath.Join("images", filename))
				if err = writeBitmapPNG(filepath.Join(root, filepath.FromSlash(path)), bitmap); err != nil {
					return "", err
				}
				if err = writeBitmapPNG(filepath.Join(root, "thumbnails", filename), bitmapThumbnail(bitmap, 240)); err != nil {
					return "", err
				}
				decoded[key] = path
			}
			manifest.Images = append(manifest.Images, assets.Entry{Node: NodeName(name, uint16(id)), Path: path})
		}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(root, ".riven-images-*")
	if err != nil {
		return "", err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err = temporary.Write(append(data, '\n')); err == nil {
		err = temporary.Close()
	} else {
		temporary.Close()
	}
	if err != nil {
		return "", err
	}
	if err = os.Rename(name, filepath.Join(root, "images.json")); err != nil {
		return "", err
	}
	return root, nil
}
