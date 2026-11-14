package riven

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/glthr/DeMystify/assets"
	"github.com/glthr/DeMystify/common"
)

func bitmapFixture(width, height, stride, format uint16, pixels []byte) []byte {
	data := make([]byte, 8)
	for i, value := range []uint16{width, height, stride, format} {
		binary.BigEndian.PutUint16(data[i*2:], value)
	}
	if format&8 != 0 || format&0x0f00 == 0x400 && format&7 == 2 {
		data = append(data, make([]byte, 772)...)
		copy(data[12:15], []byte{11, 22, 33})
	}
	if format&0x0f00 == 0x400 {
		data = append(data, make([]byte, 4)...)
	}
	return append(data, pixels...)
}

func TestRivenPixelLiteralAndRepeatedGroups(t *testing.T) {
	stream := []byte{2, 3, 4, 5, 6, 0x42, 0x81}
	want := []byte{3, 4, 5, 6, 5, 6, 5, 6, 5, 6, 5, 6}
	got, err := unpackRiven(stream, len(want))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("got %v, %v; want %v", got, err, want)
	}
}

func TestRivenPixelUnsignedDeltaAndOverlappingHistory(t *testing.T) {
	cases := []struct{ stream, want []byte }{
		{[]byte{1, 254, 1, 0xc2, 0xa0, 0x32, 0xf0, 0x54}, []byte{254, 1, 1, 3, 252, 255}},
		{[]byte{1, 5, 9, 0xc2, 0xa8, 2, 0xfc, 4, 2}, []byte{5, 9, 5, 9, 5, 9, 5, 9, 5, 9}},
		{[]byte{2, 10, 20, 30, 40, 0xc2, 0x11, 0x41}, []byte{10, 20, 30, 40, 30, 30, 30, 30}},
	}
	for _, c := range cases {
		got, err := unpackRiven(c.stream, len(c.want))
		if err != nil || !bytes.Equal(got, c.want) {
			t.Fatalf("%v: got %v, %v; want %v", c.stream, got, err, c.want)
		}
	}
}

func TestRivenPixelArithmeticInstructionFamilies(t *testing.T) {
	cases := []struct{ command, pair []byte }{
		{[]byte{0x02}, []byte{10, 20}}, {[]byte{0x10, 99}, []byte{30, 99}}, {[]byte{0x13}, []byte{30, 30}},
		{[]byte{0x23}, []byte{30, 43}}, {[]byte{0x33}, []byte{30, 37}}, {[]byte{0x40, 99}, []byte{99, 40}},
		{[]byte{0x43}, []byte{20, 40}}, {[]byte{0x50, 99, 77}, []byte{99, 77}}, {[]byte{0x53, 99}, []byte{20, 99}},
		{[]byte{0x5b, 99}, []byte{99, 30}}, {[]byte{0x63, 99}, []byte{99, 43}}, {[]byte{0x73, 99}, []byte{99, 37}},
		{[]byte{0x83}, []byte{33, 40}}, {[]byte{0x93, 99}, []byte{33, 99}}, {[]byte{0xa0, 0x23}, []byte{32, 43}},
		{[]byte{0xb0, 0x23}, []byte{32, 37}}, {[]byte{0xc3}, []byte{27, 40}}, {[]byte{0xd3, 99}, []byte{27, 99}},
		{[]byte{0xe0, 0x23}, []byte{28, 43}}, {[]byte{0xf0, 0x23}, []byte{28, 37}}, {[]byte{0xff, 0x23}, []byte{28, 37}},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%x", c.command), func(t *testing.T) {
			stream := append([]byte{2, 10, 20, 30, 40, 0xc1}, c.command...)
			want := append([]byte{10, 20, 30, 40}, c.pair...)
			got, err := unpackRiven(stream, 6)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("got %v, %v; want %v", got, err, want)
			}
		})
	}
}

func TestRivenPixelStreamRejectsTruncationOverrunAndInvalidHistory(t *testing.T) {
	cases := []struct {
		stream   []byte
		expected int
	}{
		{[]byte{1, 1}, 2}, {[]byte{0x42}, 4}, {[]byte{1, 1, 2, 0x42}, 4}, {[]byte{0}, 2}, {[]byte{0xc1, 0x58}, 2},
		{[]byte{1, 1, 2, 0xc1, 0xa8, 0}, 6}, {[]byte{1, 1, 2, 0xc1, 0xfc}, 6}, {nil, maxBitmapBytes + 1},
	}
	for _, c := range cases {
		if _, err := unpackRiven(c.stream, c.expected); err == nil {
			t.Fatalf("accepted %x expected=%d", c.stream, c.expected)
		}
	}
}

func TestDecodeBitmapPaletteBGRAndStride(t *testing.T) {
	raw := bitmapFixture(1, 2, 2, 0xa, []byte{0, 99, 0, 88})
	got, err := DecodeBitmap(raw)
	if err != nil {
		t.Fatal(err)
	}
	indexed, ok := got.(*image.Paletted)
	if !ok {
		t.Fatalf("got %T, want paletted", got)
	}
	if !bytes.Equal(indexed.Pix, []byte{0, 0}) {
		t.Fatalf("stride padding retained: %v", indexed.Pix)
	}
	if color.NRGBAModel.Convert(got.At(0, 0)) != (color.NRGBA{R: 33, G: 22, B: 11, A: 255}) {
		t.Fatal("palette is not BGR")
	}
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, got); err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(&encoded)
	if err != nil || decoded.Bounds() != got.Bounds() {
		t.Fatalf("PNG round trip: %v", err)
	}
	if decoded.At(0, 1) != got.At(0, 1) {
		t.Fatal("PNG changed stored colour")
	}
}

func TestDecodeBitmapPackedPaletteAndRawRGB(t *testing.T) {
	packed, err := DecodeBitmap(bitmapFixture(2, 1, 2, 0x40a, []byte{1, 0, 0}))
	if err != nil || packed.Bounds() != image.Rect(0, 0, 2, 1) {
		t.Fatalf("packed: %v", err)
	}
	raw, err := DecodeBitmap(bitmapFixture(2, 1, 6, 4, []byte{3, 2, 1, 6, 5, 4}))
	if err != nil {
		t.Fatal(err)
	}
	if color.NRGBAModel.Convert(raw.At(0, 0)) != (color.NRGBA{1, 2, 3, 255}) || color.NRGBAModel.Convert(raw.At(1, 0)) != (color.NRGBA{4, 5, 6, 255}) {
		t.Fatal("raw RGB channels swapped")
	}
}

func TestDecodeBitmapRejectsEveryTruncatedPrefixAndUnsupportedHeaders(t *testing.T) {
	raw := bitmapFixture(1, 2, 2, 0xa, []byte{0, 99, 0, 88})
	for size := 0; size < len(raw); size++ {
		if _, err := DecodeBitmap(raw[:size]); err == nil {
			t.Fatalf("accepted truncated prefix of size %d", size)
		}
	}
	for _, data := range [][]byte{
		bitmapFixture(1, 1, 2, 2, []byte{0, 0}),    // Missing CLUT.
		bitmapFixture(1, 1, 2, 1, []byte{0, 0}),    // Unknown depth.
		bitmapFixture(1, 1, 2, 0x1a, []byte{0, 0}), // Unsupported draw method.
		bitmapFixture(8192, 8192, 16384, 0xa, nil), // Oversized row allocation.
		bitmapFixture(0, 1, 2, 0xa, nil),
	} {
		if _, err := DecodeBitmap(data); err == nil {
			t.Fatal("accepted unsupported/invalid bitmap")
		}
	}
}

func TestBitmapThumbnailUsesNearestStoredPixelAndKeepsPalette(t *testing.T) {
	source := image.NewPaletted(image.Rect(0, 0, 4, 2), color.Palette{color.Black, color.White})
	copy(source.Pix, []byte{0, 1, 1, 0, 1, 0, 0, 1})
	thumb := bitmapThumbnail(source, 2)
	indexed, ok := thumb.(*image.Paletted)
	if !ok || !reflect.DeepEqual(indexed.Palette, source.Palette) || !bytes.Equal(indexed.Pix, []byte{0, 1}) {
		t.Fatalf("bad thumbnail: %#v", thumb)
	}
	if source.Pix[0] != 0 || source.Bounds() != image.Rect(0, 0, 4, 2) {
		t.Fatal("thumbnail mutated original")
	}
}

func TestExtractImagesUsesExistingAssetsManifestAndDeduplicatesDefaultBitmap(t *testing.T) {
	root := t.TempDir()
	raw := bitmapFixture(1, 2, 2, 0xa, []byte{0, 99, 0, 88})
	input := filepath.Join(root, "original-resource.bin")
	if err := os.WriteFile(input, raw, 0600); err != nil {
		t.Fatal(err)
	}
	resource := Resource{ID: 4, Offset: 0, Size: int64(len(raw))}
	archive := &Archive{Source: FileSlice{Path: input, Size: int64(len(raw))}, Resources: map[string]map[uint16]Resource{"tBMP": {4: resource}}}
	imported := &Import{Metadata: &common.Metadata{}, Stacks: map[string]*Stack{"tspit": {
		Name: "tspit", Resources: map[string]map[uint16]ResourceRef{"tBMP": {4: {Archive: archive, Resource: resource}}},
		Cards: map[uint16]*Card{
			155: {ID: 155, Pictures: []Picture{{Index: 2, ImageID: 99, Rect: [4]int{0, 0, 608, 392}}, {Index: 1, ImageID: 4, Rect: [4]int{0, 0, 1, 2}}}},
			156: {ID: 156, Pictures: []Picture{{Index: 1, ImageID: 4, Rect: [4]int{0, 0, 1, 2}}}},
		},
	}}}
	dir, err := imported.ExtractImages(filepath.Join(root, "decoded"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "images.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest assets.Manifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Images) != 2 || manifest.Images[0].Node != "Temple:155" || manifest.Images[1].Node != "Temple:156" || manifest.Images[0].Path != manifest.Images[1].Path {
		t.Fatalf("bad asset manifest: %s", data)
	}
	paths, err := filepath.Glob(filepath.Join(dir, "images", "*.png"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("deduplication: %v, %v", paths, err)
	}
	file, err := os.Open(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(file)
	file.Close()
	if err != nil || config.Width != 1 || config.Height != 2 {
		t.Fatalf("wrong default picture: %v, %v", config, err)
	}
	after, err := os.ReadFile(input)
	if err != nil || !bytes.Equal(after, raw) {
		t.Fatal("original resource changed")
	}
	nodes := []*common.Node{
		{GraphID: 1, Name: "Temple:155", StackName: "Temple", Attributes: []common.NodeAttribute{common.IsCard}},
		{GraphID: 2, Name: "Temple:156", StackName: "Temple", Attributes: []common.NodeAttribute{common.IsCard}},
	}
	matched, report, err := assets.Prepare(dir, filepath.Join(root, "prepared"), nodes)
	if err != nil || len(matched) != 2 || report.Matched != 2 || report.Missing != 0 || report.UniqueImages != 1 {
		t.Fatalf("shared assets pipeline: matched=%d report=%+v err=%v", len(matched), report, err)
	}
}

// No game artwork is in the repository. With the user's five CD images, this
// exercises every default view and compares representative decoded bytes to
// the independently verified extraction, one card from each stack.
func TestSuppliedDiscBitmapViews(t *testing.T) {
	input, _ := fixtureCorpus(t)
	imported, err := LoadInput(input)
	if err != nil {
		t.Fatal(err)
	}
	defer imported.Close()
	want := map[string]string{
		"aspit:1":   "7735f53e00cbb7189a02b3c5c316431e9e17275ff0572477a9d65f7509cd95c2",
		"bspit:1":   "45e9dc94f5275928b51444b720da3eb7791c25f7f31aa7f3878b700db9a66d62",
		"gspit:1":   "f85a4f89bc1f0de93aa4c6a20ac528a4eea4286fc2431b28090df088c75e5482",
		"jspit:621": "0fa8c6a148c444e09dad7ebed02bf81139514ffbff9b4736d4eec7901df719ba",
		"ospit:1":   "2775b568d32d39b3e5600e7653bfb6515906b6eac68677075ef241a1336f6ab1",
		"pspit:1":   "3c1d8a0d7a3bc048e69c2b4fb3b0f7c86269ae159dd5da36d656e418a25238b3",
		"rspit:1":   "8005a7573ab749a7f06d35212bb3dc286fd0ca0e0985ac16faf2728cae1a4ed1",
		"tspit:155": "e92c5637f66e10c319c386a45596de9262fedbfbf2516b4fe6ccea265e08f291",
	}
	count, hashed := 0, 0
	formats := map[uint16]int{}
	for name, stack := range imported.Stacks {
		for id, card := range stack.Cards {
			var picture *Picture
			for i := range card.Pictures {
				if card.Pictures[i].Index == 1 {
					picture = &card.Pictures[i]
					break
				}
			}
			if picture == nil {
				t.Fatalf("%s:%d lacks default PLST record", name, id)
			}
			ref, ok := stack.Resources["tBMP"][picture.ImageID]
			if !ok {
				t.Fatalf("%s:%d lacks tBMP:%d", name, id, picture.ImageID)
			}
			raw, err := ref.Archive.Read("tBMP", picture.ImageID)
			if err != nil {
				t.Fatal(err)
			}
			formats[binary.BigEndian.Uint16(raw[6:8])]++
			bitmap, err := DecodeBitmap(raw)
			if err != nil {
				t.Fatalf("%s:%d tBMP:%d: %v", name, id, picture.ImageID, err)
			}
			if bitmap.Bounds() != image.Rect(0, 0, 608, 392) {
				t.Fatalf("%s:%d unexpected dimensions: %v", name, id, bitmap.Bounds())
			}
			if expected, ok := want[SourceID(name, id)]; ok {
				indexed, ok := bitmap.(*image.Paletted)
				if !ok {
					t.Fatalf("%s:%d not indexed", name, id)
				}
				hash := sha256.New()
				hash.Write(indexed.Pix)
				for _, c := range indexed.Palette {
					r, g, b, _ := c.RGBA()
					hash.Write([]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8)})
				}
				if got := fmt.Sprintf("%x", hash.Sum(nil)); got != expected {
					t.Fatalf("%s:%d decoded pixel/palette hash %s, want %s", name, id, got, expected)
				}
				hashed++
			}
			count++
		}
	}
	if count != 2307 || hashed != 8 || formats[0x40a] != 2235 || formats[0xa] != 72 {
		t.Fatalf("decoded=%d representative=%d formats=%v", count, hashed, formats)
	}
	t.Logf("decoded %d default 608x392 views, %d exact representative pixel/palette hashes; packed=%d raw=%d", count, hashed, formats[0x40a], formats[0xa])
}
