// Package assets prepares user-supplied decoded pictures for a local graph view.
// It reads PNG, JPEG and GIF files; it does not extract or decode HyperCard media.
package assets

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/glthr/DeMystify/common"
)

// Image paths are filenames relative to Prepare's destination directory.
type Image struct {
	Path      string `json:"path"`
	Thumbnail string `json:"thumbnail"`
	Name      string `json:"name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type Report struct {
	Matched      int      `json:"matched"`
	Missing      int      `json:"missing"`
	UniqueImages int      `json:"uniqueImages"`
	Ambiguous    int      `json:"ambiguous"`
	Diagnostics  []string `json:"diagnostics,omitempty"`
}

// Entry maps an image to an exact node, a stack/card ID, or a stack/picture name.
// Paths are relative to the supplied image directory. A manifest, when present,
// is authoritative; unrelated image files are not inferred as extra entries.
type Entry struct {
	Path    string `json:"path"`
	Node    string `json:"node,omitempty"`
	Stack   string `json:"stack,omitempty"`
	Picture string `json:"picture,omitempty"`
	CardID  int    `json:"card_id,omitempty"`
}

type Manifest struct {
	Images []Entry `json:"images"`
}

const (
	maxImageBytes = 32 << 20
	maxPixels     = 32 << 20
	maxManifest   = 8 << 20
)

type indexedImage struct {
	entry Entry
	file  string
}

type imageInfo struct {
	hash   string
	format string
	width  int
	height int
}

// Prepare resolves pictures conservatively and copies only matched images to
// destDir. Matching priority is exact node, stack/card ID, stack/picture, then a
// unique unscoped picture filename. Different bytes at the same priority are
// ambiguous and remain unmapped. Identical source files are deduplicated.
//
// sourceDir may contain images.json (Manifest) or decoded image files. Without
// a manifest, stack directories (including *.xstk) scope card_N and picture
// filenames. Neither source files nor symlink targets are modified or followed.
func Prepare(sourceDir, destDir string, nodes []*common.Node) (map[int64]Image, Report, error) {
	result := make(map[int64]Image)
	var report Report
	for _, node := range nodes {
		if eligible(node) {
			report.Missing++
		}
	}
	if sourceDir == "" {
		return result, report, nil
	}
	root, err := directory(sourceDir, false)
	if err != nil {
		return nil, report, err
	}
	entries, err := index(root, nodes)
	if err != nil {
		return nil, report, err
	}
	if len(entries) == 0 {
		report.Diagnostics = append(report.Diagnostics, "no decoded PNG, JPEG or GIF images found")
		return result, report, nil
	}
	planned, err := plannedDirectory(destDir)
	if err != nil {
		return nil, report, err
	}
	if containsPath(root, planned) || containsPath(planned, root) {
		return nil, report, fmt.Errorf("image source and destination directories must not overlap")
	}
	dest, err := directory(planned, true)
	if err != nil {
		return nil, report, err
	}
	infos := make(map[string]imageInfo)
	prepared := make(map[string]Image)
	seenIDs := make(map[int64]bool)
	for _, node := range nodes {
		if !eligible(node) {
			continue
		}
		if seenIDs[node.GraphID] {
			return nil, report, fmt.Errorf("duplicate graph node ID %d", node.GraphID)
		}
		seenIDs[node.GraphID] = true
		best := 0
		var candidates []indexedImage
		for _, entry := range entries {
			priority := match(entry.entry, node)
			if priority > best {
				best, candidates = priority, []indexedImage{entry}
			} else if priority != 0 && priority == best {
				candidates = append(candidates, entry)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		unique := make(map[string]indexedImage)
		for _, candidate := range candidates {
			info, ok := infos[candidate.file]
			if !ok {
				data, err := readImage(root, candidate.file)
				if err != nil {
					return nil, report, err
				}
				info, err = inspect(data)
				if err != nil {
					return nil, report, fmt.Errorf("%s: %w", candidate.entry.Path, err)
				}
				infos[candidate.file] = info
			}
			unique[info.hash] = candidate
		}
		if len(unique) != 1 {
			report.Ambiguous++
			report.Diagnostics = append(report.Diagnostics, fmt.Sprintf("ambiguous image for %s: %d different pictures at the same matching priority", node.Name, len(unique)))
			continue
		}
		for digest, candidate := range unique {
			value, ok := prepared[digest]
			if !ok {
				data, err := readImage(root, candidate.file)
				if err != nil {
					return nil, report, err
				}
				if hash(data) != digest {
					return nil, report, fmt.Errorf("image changed while preparing %s", candidate.entry.Path)
				}
				value, err = copyImage(dest, data, infos[candidate.file])
				if err != nil {
					return nil, report, err
				}
				prepared[digest] = value
			}
			value.Name = candidate.entry.Picture
			if value.Name == "" && node.SecondaryName != nil {
				value.Name = *node.SecondaryName
			}
			if value.Name == "" {
				value.Name = filepath.Base(candidate.entry.Path)
			}
			result[node.GraphID] = value
		}
		report.Matched++
		report.Missing--
	}
	report.UniqueImages = len(prepared)
	return result, report, nil
}

func eligible(node *common.Node) bool {
	return node != nil && !node.IsOfType(common.IsStack) && !node.IsOfType(common.IsVirtual)
}

func directory(path string, create bool) (string, error) {
	if create {
		if err := os.MkdirAll(path, 0700); err != nil {
			return "", err
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("%s: expected a directory, not a symlink or file", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(resolved)
}

// Resolve existing ancestors before creating anything, including aliases such
// as a destination reached through a symlink to the original image directory.
func plannedDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := absolute
	var missing []string
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if current == absolute && info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("image destination must not be a symlink")
			}
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			info, err = os.Stat(resolved)
			if err != nil || !info.IsDir() {
				return "", fmt.Errorf("image destination ancestor is not a directory: %s", current)
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		current = parent
	}
}

func containsPath(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func safePath(root, relative string) (string, error) {
	if strings.Contains(relative, "\\") || !filepath.IsLocal(relative) || relative == "." {
		return "", fmt.Errorf("image path must be local to the source directory: %q", relative)
	}
	path := root
	for _, component := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		path = filepath.Join(path, component)
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink images are not supported: %s", relative)
		}
	}
	return path, nil
}

func index(root string, nodes []*common.Node) ([]indexedImage, error) {
	manifestPath := filepath.Join(root, "images.json")
	if _, err := os.Lstat(manifestPath); err == nil {
		path, err := safePath(root, "images.json")
		if err != nil {
			return nil, err
		}
		data, err := readBounded(path, maxManifest)
		if err != nil {
			return nil, err
		}
		var manifest Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("images.json: %w", err)
		}
		var result []indexedImage
		for _, entry := range manifest.Images {
			if entry.Path == "" || (entry.Node == "" && entry.Picture == "" && entry.CardID == 0) || entry.CardID < 0 || entry.CardID != 0 && entry.Stack == "" {
				return nil, fmt.Errorf("images.json contains an invalid image selector or path")
			}
			path, err := safePath(root, entry.Path)
			if err != nil {
				return nil, err
			}
			if !supported(path) {
				return nil, fmt.Errorf("%s: expected a decoded PNG, JPEG or GIF", entry.Path)
			}
			result = append(result, indexedImage{entry: entry, file: path})
		}
		return result, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	stacks := make(map[string]string)
	for _, node := range nodes {
		if node != nil && node.StackName != "" {
			stacks[strings.ToLower(node.StackName)] = node.StackName
		}
	}
	var result []indexedImage
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink images are not supported: %s", path)
		}
		if entry.IsDir() || !supported(path) {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		picture := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		value := Entry{Path: relative, Picture: picture}
		components := strings.Split(relative, string(filepath.Separator))
		for i, component := range components[:len(components)-1] {
			name := component
			if strings.HasSuffix(strings.ToLower(name), ".xstk") {
				name = name[:len(name)-5]
			}
			if stack, ok := stacks[strings.ToLower(name)]; ok {
				value.Stack = stack
				if id, err := strconv.Atoi(strings.TrimPrefix(picture, "card_")); err == nil && id > 0 {
					value.CardID, value.Picture = id, ""
				}
				if value.CardID == 0 && i+2 < len(components) {
					full := filepath.Join(components[i+1:]...)
					full = strings.TrimSuffix(filepath.ToSlash(full), filepath.Ext(full))
					result = append(result, indexedImage{entry: Entry{Path: relative, Stack: stack, Picture: full}, file: path})
				}
				break
			}
		}
		result = append(result, indexedImage{entry: value, file: path})
		return nil
	})
	return result, err
}

func match(entry Entry, node *common.Node) int {
	if entry.Stack != "" && !strings.EqualFold(entry.Stack, node.StackName) {
		return 0
	}
	if entry.Node != "" {
		if strings.EqualFold(entry.Node, node.Name) {
			return 4
		}
		return 0
	}
	if entry.CardID != 0 {
		position := strings.LastIndexByte(node.Name, ':')
		if position >= 0 {
			id, err := strconv.Atoi(node.Name[position+1:])
			if err == nil && id == entry.CardID {
				return 3
			}
		}
		return 0
	}
	if node.SecondaryName != nil && strings.EqualFold(entry.Picture, *node.SecondaryName) {
		if entry.Stack != "" {
			return 2
		}
		return 1
	}
	return 0
}

func supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif":
		return true
	}
	return false
}

func readBounded(path string, maximum int64) ([]byte, error) {
	initial, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !initial.Mode().IsRegular() || initial.Size() > maximum {
		return nil, fmt.Errorf("%s: expected a regular file no larger than %d bytes", path, maximum)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, fmt.Errorf("%s: expected a regular file no larger than %d bytes", path, maximum)
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("%s grew beyond size limit", path)
	}
	return data, nil
}

func readImage(root, path string) ([]byte, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return nil, err
	}
	checked, err := safePath(root, relative)
	if err != nil {
		return nil, err
	}
	return readBounded(checked, maxImageBytes)
}

func inspect(data []byte) (imageInfo, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return imageInfo{}, fmt.Errorf("invalid decoded image: %w", err)
	}
	if config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > maxPixels {
		return imageInfo{}, fmt.Errorf("unsupported image dimensions %d×%d", config.Width, config.Height)
	}
	return imageInfo{hash: hash(data), format: format, width: config.Width, height: config.Height}, nil
}

func hash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func copyImage(dest string, data []byte, info imageInfo) (Image, error) {
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Image{}, fmt.Errorf("image pixel data cannot be decoded: %w", err)
	}
	extension := "." + info.format
	if info.format == "jpeg" {
		extension = ".jpg"
	}
	value := Image{Path: info.hash + extension, Width: info.width, Height: info.height}
	if err := writeUnique(dest, value.Path, data); err != nil {
		return Image{}, err
	}
	value.Thumbnail = value.Path
	scale := math.Min(1, math.Min(320/float64(info.width), 240/float64(info.height)))
	if scale == 1 {
		return value, nil
	}
	width, height := max(1, int(math.Round(float64(info.width)*scale))), max(1, int(math.Round(float64(info.height)*scale)))
	thumbnail := resize(decoded, width, height)
	var encoded bytes.Buffer
	if thumbnail.Opaque() {
		err = jpeg.Encode(&encoded, thumbnail, &jpeg.Options{Quality: 82})
		extension = ".jpg"
	} else {
		err = png.Encode(&encoded, thumbnail)
		extension = ".png"
	}
	if err != nil {
		return Image{}, err
	}
	value.Thumbnail = hash(encoded.Bytes()) + extension
	if err := writeUnique(dest, value.Thumbnail, encoded.Bytes()); err != nil {
		return Image{}, err
	}
	return value, nil
}

func writeUnique(dest, name string, data []byte) error {
	path := filepath.Join(dest, name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe existing image output %s", name)
		}
		previous, readErr := readBounded(path, maxImageBytes)
		if readErr == nil && bytes.Equal(previous, data) {
			return nil
		}
		return fmt.Errorf("existing image output differs from content-addressed bytes: %s", name)
	}
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// Bilinear interpolation retains aspect ratio; full-resolution source bytes
// are copied separately and are never changed by thumbnail generation.
func resize(source image.Image, width, height int) *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := source.Bounds()
	for y := 0; y < height; y++ {
		sy := math.Max(0, (float64(y)+0.5)*float64(bounds.Dy())/float64(height)-0.5)
		y0 := int(sy)
		y1, fy := min(y0+1, bounds.Dy()-1), sy-float64(y0)
		for x := 0; x < width; x++ {
			sx := math.Max(0, (float64(x)+0.5)*float64(bounds.Dx())/float64(width)-0.5)
			x0 := int(sx)
			x1, fx := min(x0+1, bounds.Dx()-1), sx-float64(x0)
			r0, g0, b0, a0 := source.At(bounds.Min.X+x0, bounds.Min.Y+y0).RGBA()
			r1, g1, b1, a1 := source.At(bounds.Min.X+x1, bounds.Min.Y+y0).RGBA()
			r2, g2, b2, a2 := source.At(bounds.Min.X+x0, bounds.Min.Y+y1).RGBA()
			r3, g3, b3, a3 := source.At(bounds.Min.X+x1, bounds.Min.Y+y1).RGBA()
			channel := func(a, b, c, d uint32) uint8 {
				value := (1-fy)*((1-fx)*float64(a)+fx*float64(b)) + fy*((1-fx)*float64(c)+fx*float64(d))
				return uint8(math.Round(value / 257))
			}
			result.SetRGBA(x, y, color.RGBA{R: channel(r0, r1, r2, r3), G: channel(g0, g1, g2, g3), B: channel(b0, b1, b2, b3), A: channel(a0, a1, a2, a3)})
		}
	}
	return result
}
