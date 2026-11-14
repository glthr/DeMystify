package riven

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func both16(value uint16) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint16(b, value)
	binary.BigEndian.PutUint16(b[2:], value)
	return b
}
func both32(value uint32) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b, value)
	binary.BigEndian.PutUint32(b[4:], value)
	return b
}
func isoFixtureRecord(name []byte, extent, size uint32, flags byte) []byte {
	n := 33 + len(name)
	if len(name)%2 == 0 {
		n++
	}
	b := make([]byte, n)
	b[0], b[25], b[32] = byte(n), flags, byte(len(name))
	copy(b[2:10], both32(extent))
	copy(b[10:18], both32(size))
	copy(b[28:32], both16(1))
	copy(b[33:], name)
	return b
}
func mohFixture(tag string, size uint16) []byte {
	b := make([]byte, 83)
	copy(b, "MHWK")
	binary.BigEndian.PutUint32(b[4:], uint32(len(b)))
	copy(b[8:], "RSRC")
	binary.BigEndian.PutUint16(b[12:], 0x100)
	binary.BigEndian.PutUint32(b[16:], 55)
	binary.BigEndian.PutUint32(b[20:], 28)
	binary.BigEndian.PutUint16(b[24:], 32)
	binary.BigEndian.PutUint16(b[26:], 14)
	binary.BigEndian.PutUint16(b[28:], 24)
	binary.BigEndian.PutUint16(b[30:], 1)
	copy(b[32:], tag)
	binary.BigEndian.PutUint16(b[36:], 12)
	binary.BigEndian.PutUint16(b[38:], 18)
	binary.BigEndian.PutUint16(b[40:], 1)
	binary.BigEndian.PutUint16(b[42:], 9)
	binary.BigEndian.PutUint16(b[44:], 1)
	binary.BigEndian.PutUint16(b[46:], 1)
	binary.BigEndian.PutUint16(b[48:], 0)
	binary.BigEndian.PutUint16(b[50:], 1)
	copy(b[52:], "entry\x00")
	binary.BigEndian.PutUint32(b[60:], 1)
	binary.BigEndian.PutUint32(b[64:], 80)
	binary.BigEndian.PutUint16(b[68:], size)
	copy(b[80:], "abc")
	return b
}
func isoFixture() []byte {
	b := make([]byte, 24*2048)
	p := b[16*2048 : 17*2048]
	copy(p, []byte{1, 'C', 'D', '0', '0', '1', 1})
	copy(p[128:], both16(2048))
	copy(p[156:], isoFixtureRecord([]byte{0}, 20, 2048, 2))
	copy(b[17*2048:], []byte{255, 'C', 'D', '0', '0', '1', 1})
	directory := bytes.Join([][]byte{
		isoFixtureRecord([]byte{0}, 20, 2048, 2), isoFixtureRecord([]byte{1}, 20, 2048, 2),
		isoFixtureRecord([]byte("A_DATA.MHK;1"), 21, 4, 4), // Hybrid Mac associated resource fork.
		isoFixtureRecord([]byte("A_DATA.MHK;1"), 22, 83, 0),
	}, nil)
	copy(b[20*2048:], directory)
	copy(b[21*2048:], "FORK")
	copy(b[22*2048:], mohFixture("CARD", 3))
	return b
}
func putArchiveFixture(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestISOSelectsDataForkAndBoundedEmbeddedArchive(t *testing.T) {
	p := putArchiveFixture(t, "disc.iso", isoFixture())
	files, err := ISOFiles(p)
	if err != nil || len(files) != 1 || files[0].Name != "A_DATA.MHK" || files[0].Offset != 22*2048 || files[0].Size != 83 {
		t.Fatalf("wrong ISO fork/extents: %+v %v", files, err)
	}
	archives, err := DiscoverArchives(p)
	if err != nil || len(archives) != 1 {
		t.Fatalf("associated fork was not skipped: %+v %v", archives, err)
	}
	data, err := archives[0].Read("CARD", 9)
	if err != nil || string(data) != "abc" {
		t.Fatalf("read escaped archive extent: %q %v", data, err)
	}
}

func TestISORejectsDualEndianMismatchAndOutsideExtent(t *testing.T) {
	for name, mutate := range map[string]func([]byte){
		"block-endian":      func(b []byte) { binary.BigEndian.PutUint16(b[16*2048+130:], 1024) },
		"root-endian":       func(b []byte) { binary.BigEndian.PutUint32(b[16*2048+156+6:], 19) },
		"outside-root":      func(b []byte) { copy(b[16*2048+156+2:], both32(24)) },
		"multi-extent":      func(b []byte) { b[20*2048+2*34+25] = 128 },
		"short-root-record": func(b []byte) { b[16*2048+156] = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			b := isoFixture()
			mutate(b)
			if _, err := ISOFiles(putArchiveFixture(t, "bad.iso", b)); err == nil {
				t.Fatal("accepted malformed ISO metadata")
			}
		})
	}
}

func TestMohawkEmbeddedResourceAndName(t *testing.T) {
	b := mohFixture("CARD", 3)
	p := putArchiveFixture(t, "embedded.bin", append(append([]byte("PREFIX"), b...), []byte("SUFFIX")...))
	a, err := OpenArchive(FileSlice{Path: p, Name: "A_DATA.MHK", Offset: 6, Size: int64(len(b))})
	if err != nil {
		t.Fatal(err)
	}
	r := a.Resources["CARD"][9]
	if r.Name != "entry" || r.Offset != 80 || r.Size != 3 || r.Index != 1 {
		t.Fatalf("wrong resource table: %+v", r)
	}
	data, err := a.Read("CARD", 9)
	if err != nil || string(data) != "abc" {
		t.Fatalf("wrong resource bytes: %q %v", data, err)
	}
}

func TestMohawkRejectsIndexResourceAndDirectoryBounds(t *testing.T) {
	for name, mutate := range map[string]func([]byte){
		"zero-index":       func(b []byte) { binary.BigEndian.PutUint16(b[44:], 0) },
		"large-index":      func(b []byte) { binary.BigEndian.PutUint16(b[44:], 2) },
		"resource-size":    func(b []byte) { binary.BigEndian.PutUint16(b[68:], 4) },
		"resource-offset":  func(b []byte) { binary.BigEndian.PutUint32(b[64:], 84) },
		"directory-offset": func(b []byte) { binary.BigEndian.PutUint32(b[20:], 1000) },
		"file-count":       func(b []byte) { binary.BigEndian.PutUint32(b[60:], 0xffffffff) },
		"name-offset":      func(b []byte) { binary.BigEndian.PutUint16(b[48:], 0xffff) },
	} {
		t.Run(name, func(t *testing.T) {
			b := mohFixture("CARD", 3)
			mutate(b)
			p := putArchiveFixture(t, "bad.mhk", b)
			if _, err := OpenArchive(FileSlice{Path: p, Name: "bad.mhk", Size: int64(len(b))}); err == nil {
				t.Fatal("accepted malformed Mohawk metadata")
			}
		})
	}
}

func TestMohawkMovieUsesExtentInsteadOfStoredSize(t *testing.T) {
	b := mohFixture("tMOV", 0)
	p := putArchiveFixture(t, "movie.mhk", b)
	a, err := OpenArchive(FileSlice{Path: p, Name: "movie.mhk", Size: int64(len(b))})
	if err != nil {
		t.Fatal(err)
	}
	data, err := a.Read("tMOV", 9)
	if err != nil || string(data) != "abc" || a.Resources["tMOV"][9].Size != 3 {
		t.Fatalf("bad movie extent: %q %v", data, err)
	}
}
