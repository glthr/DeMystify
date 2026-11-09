package assets

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/glthr/DeMystify/common"
)

func picture(t *testing.T, root, name string, value color.Color) []byte {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 544, 333))
	for y := 0; y < 333; y++ {
		for x := 0; x < 544; x++ {
			img.Set(x, y, value)
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func manifest(t *testing.T, root string, entries ...Entry) {
	t.Helper()
	data, err := json.Marshal(Manifest{Images: entries})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "images.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func node(id int64, stack, name string) *common.Node {
	return &common.Node{GraphID: id, StackName: stack, Name: stack + ":8336", SecondaryName: &name}
}

func TestPrepareScopesPicturesAndCopiesOriginalBytes(t *testing.T) {
	source, dest := t.TempDir(), filepath.Join(t.TempDir(), "images")
	original := picture(t, source, "Myst/Dock.png", color.RGBA{R: 200, A: 255})
	picture(t, source, "Channelwood Age/Dock.png", color.RGBA{G: 200, A: 255})
	images, report, err := Prepare(source, dest, []*common.Node{node(1, "Myst", "Dock"), node(2, "Channelwood Age", "Dock")})
	if err != nil {
		t.Fatal(err)
	}
	if report.Matched != 2 || report.Missing != 0 || report.UniqueImages != 2 || images[1].Path == images[2].Path {
		t.Fatalf("incorrect scope/coverage: %+v, %+v", images, report)
	}
	got, err := os.ReadFile(filepath.Join(dest, images[1].Path))
	if err != nil || !bytes.Equal(got, original) {
		t.Fatal("full-resolution image bytes changed")
	}
	thumb, err := os.Open(filepath.Join(dest, images[1].Thumbnail))
	if err != nil {
		t.Fatal(err)
	}
	defer thumb.Close()
	config, _, err := image.DecodeConfig(thumb)
	if err != nil || config.Width != 320 || config.Height != 196 || images[1].Width != 544 || images[1].Height != 333 {
		t.Fatalf("thumbnail distorted source dimensions: %+v, %+v, %v", config, images[1], err)
	}
}

func TestPrepareManifestPriorityDedupeAndAmbiguity(t *testing.T) {
	source, dest := t.TempDir(), filepath.Join(t.TempDir(), "images")
	first := picture(t, source, "first.png", color.RGBA{R: 200, A: 255})
	if err := os.WriteFile(filepath.Join(source, "same.png"), first, 0600); err != nil {
		t.Fatal(err)
	}
	picture(t, source, "other.png", color.RGBA{G: 200, A: 255})
	manifest(t, source, Entry{Path: "first.png", Stack: "Myst", Picture: "Dock"}, Entry{Path: "same.png", Stack: "Myst", Picture: "Dock"})
	images, report, err := Prepare(source, dest, []*common.Node{node(1, "Myst", "Dock"), node(2, "Myst", "Dock")})
	if err != nil || report.Matched != 2 || report.UniqueImages != 1 || images[1].Path != images[2].Path {
		t.Fatalf("identical sources did not dedupe: %+v, %v", report, err)
	}
	manifest(t, source, Entry{Path: "first.png", Stack: "Myst", Picture: "Dock"}, Entry{Path: "other.png", Stack: "Myst", Picture: "Dock"})
	images, report, err = Prepare(source, dest, []*common.Node{node(1, "Myst", "Dock")})
	if err != nil || len(images) != 0 || report.Ambiguous != 1 || report.Missing != 1 {
		t.Fatalf("ambiguous image was guessed: %+v, %v", report, err)
	}
	manifest(t, source, Entry{Path: "first.png", Stack: "Myst", Picture: "Dock"}, Entry{Path: "other.png", Node: "Myst:8336"})
	images, report, err = Prepare(source, dest, []*common.Node{node(1, "Myst", "Dock")})
	if err != nil || report.Matched != 1 || images[1].Path == hash(first)+".png" {
		t.Fatalf("exact node mapping lost priority: %+v, %v", report, err)
	}
}

func TestPrepareRejectsEscapesAndOverlapBeforeMutation(t *testing.T) {
	source := t.TempDir()
	original := picture(t, source, "Dock.png", color.RGBA{A: 255})
	nodes := []*common.Node{node(1, "Myst", "Dock")}
	inside := filepath.Join(source, "must-not-exist", "images")
	if _, _, err := Prepare(source, inside, nodes); err == nil {
		t.Fatal("accepted source/destination overlap")
	}
	if _, err := os.Lstat(filepath.Dir(inside)); !os.IsNotExist(err) {
		t.Fatal("overlap rejection modified original directory")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(source, alias); err == nil {
		if _, _, err := Prepare(source, filepath.Join(alias, "must-not-exist"), nodes); err == nil {
			t.Fatal("accepted aliased destination inside originals")
		}
		if _, err := os.Lstat(filepath.Join(source, "must-not-exist")); !os.IsNotExist(err) {
			t.Fatal("alias rejection modified original directory")
		}
	}
	manifest(t, source, Entry{Path: "../escape.png", Picture: "Dock"})
	if _, _, err := Prepare(source, filepath.Join(t.TempDir(), "images"), nodes); err == nil {
		t.Fatal("accepted manifest traversal")
	}
	got, err := os.ReadFile(filepath.Join(source, "Dock.png"))
	if err != nil || !bytes.Equal(got, original) {
		t.Fatal("failure changed original picture")
	}
}

func TestPrepareDoesNotOverwriteExistingContentAddress(t *testing.T) {
	source, dest := t.TempDir(), t.TempDir()
	original := picture(t, source, "Dock.png", color.RGBA{A: 255})
	digest := sha256.Sum256(original)
	path := filepath.Join(dest, hex.EncodeToString(digest[:])+".png")
	if err := os.WriteFile(path, []byte("user data"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Prepare(source, dest, []*common.Node{node(1, "Myst", "Dock")}); err == nil {
		t.Fatal("overwrote an existing content-addressed path")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "user data" {
		t.Fatal("existing file was truncated")
	}
}

func TestPrepareValidatesSmallImagePixelData(t *testing.T) {
	source, dest := t.TempDir(), t.TempDir()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 64, 40))); err != nil {
		t.Fatal(err)
	}
	truncated := encoded.Bytes()[:encoded.Len()-12] // Remove IEND, retain a valid header.
	if _, _, err := image.DecodeConfig(bytes.NewReader(truncated)); err != nil {
		t.Fatal("fixture must retain a readable size header")
	}
	if err := os.WriteFile(filepath.Join(source, "Dock.png"), truncated, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Prepare(source, dest, []*common.Node{node(1, "Myst", "Dock")}); err == nil {
		t.Fatal("accepted truncated pixels merely because no resize was needed")
	}
	entries, err := os.ReadDir(dest)
	if err != nil || len(entries) != 0 {
		t.Fatal("published an undecodable small image")
	}
}
