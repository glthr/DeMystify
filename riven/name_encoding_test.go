package riven

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRivenNAMEDecodesMacRomanWithoutChangingResourceBytes(t *testing.T) {
	// Riven's named Zip places use MacRoman 0xbd (Omega) as a delimiter.
	// Other extended characters must survive, too; replacing invalid UTF-8
	// with U+FFFD loses the stored identity even when the display looks similar.
	first := []byte{0xbd, 'd', 'o', 'm', 'e', 0xbd, 0}
	second := []byte{'c', 'a', 'f', 0x8e, ' ', 0xad, 0}
	b := make([]byte, 10)
	binary.BigEndian.PutUint16(b, 2)
	binary.BigEndian.PutUint16(b[4:], uint16(len(first)))
	b = append(b, first...)
	b = append(b, second...)
	before := bytes.Clone(b)
	names, err := ParseNames(b)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"ΩdomeΩ", "café ≠"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("NAME strings = %#v, want %#v", names, want)
	}
	if !bytes.Equal(b, before) {
		t.Fatal("NAME decoding mutated resource bytes")
	}
	encoded, err := json.Marshal(names)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(encoded) || strings.Contains(string(encoded), `\ufffd`) || bytes.Contains(encoded, []byte("�")) {
		t.Fatalf("serialized names lost MacRoman identity: %s", encoded)
	}
}

func TestRivenArchiveResourceNameDecodesMacRoman(t *testing.T) {
	b := mohFixture("tBMP", 3)
	copy(b[52:60], []byte{0xbd, 's', 'c', 'o', 'p', 'e', 0xbd, 0})
	before := bytes.Clone(b)
	p := putArchiveFixture(t, "macroman.mhk", b)
	a, err := OpenArchive(FileSlice{Path: p, Name: "J_DATA1.MHK", Size: int64(len(b))})
	if err != nil {
		t.Fatal(err)
	}
	if got := a.Resources["tBMP"][9].Name; got != "ΩscopeΩ" || !utf8.ValidString(got) {
		t.Fatalf("archive resource name = %q", got)
	}
	if !bytes.Equal(b, before) {
		t.Fatal("archive decoding mutated input fixture")
	}
	data, err := a.Read("tBMP", 9)
	if err != nil || string(data) != "abc" {
		t.Fatalf("resource payload changed while decoding its name: %q, %v", data, err)
	}
}
