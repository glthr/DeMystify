package hypercard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeTextDecoding(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"newlines", []byte("one\rtwo\r\nthree\nfour"), "one\ntwo\nthree\nfour"},
		{"HyperTalk symbols", []byte{0xad, 0xb2, 0xb3, 0xc2, 0xc6}, "≠≤≥¬∆"},
		{"Macintosh glyphs", []byte{0x11, 0xf0, 0x8e, 0xdb}, "⌘é€"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := decodeMacRoman(tc.data); got != tc.want {
				t.Fatalf("decoded text = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNativeReadFileRejectsNonRegularInput(t *testing.T) {
	path := t.TempDir()
	if _, err := ReadFile(path); err == nil || !strings.Contains(err.Error(), "regular file") || !strings.Contains(err.Error(), path) {
		t.Fatalf("directory input error = %v", err)
	}
}

func TestNativeReadFileBoundsLoading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Oversized Stack")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// A sparse file exercises the pre-read size guard without allocating the
	// payload in the test process or populating it on disk.
	if err := file.Truncate(maxNativeFileBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(path); err == nil || !strings.Contains(err.Error(), "size limit") || !strings.Contains(err.Error(), path) {
		t.Fatalf("oversized input error = %v", err)
	}
}

func TestNativeReadFileDecodeErrorIdentifiesInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Broken Stack")
	if err := os.WriteFile(path, []byte("not a HyperCard stack"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(path); err == nil || strings.Count(err.Error(), path) != 1 {
		t.Fatalf("decode error should identify its input exactly once: %v", err)
	}
}
