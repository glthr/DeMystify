// Package riven imports the original Riven CD resource data into DeMystify.
package riven

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileSlice struct {
	Path   string `json:"container"`
	Name   string `json:"path"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
}

func (s FileSlice) Read(offset, size int64) ([]byte, error) {
	if offset < 0 || size < 0 || offset > s.Size || size > s.Size-offset || size > 1<<30 {
		return nil, fmt.Errorf("read outside %s: %d+%d/%d", s.Name, offset, size, s.Size)
	}
	f, e := os.Open(s.Path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b := make([]byte, int(size))
	_, e = f.ReadAt(b, s.Offset+offset)
	if e != nil {
		return nil, e
	}
	return b, nil
}

type Resource struct {
	ID     uint16 `json:"id"`
	Index  uint16 `json:"index"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
	Name   string `json:"name"`
}
type Archive struct {
	Source    FileSlice                      `json:"source"`
	Resources map[string]map[uint16]Resource `json:"resources"`
}
type ResourceRef struct {
	Archive  *Archive
	Resource Resource
}

func (a *Archive) Read(tag string, id uint16) ([]byte, error) {
	r, ok := a.Resources[tag][id]
	if !ok {
		return nil, fmt.Errorf("%s lacks %s:%d", a.Source.Name, tag, id)
	}
	return a.Source.Read(r.Offset, r.Size)
}
func be16(b []byte) uint16 { return binary.BigEndian.Uint16(b) }
func be32(b []byte) uint32 { return binary.BigEndian.Uint32(b) }

func OpenArchive(s FileSlice) (*Archive, error) {
	h, e := s.Read(0, 28)
	if e != nil {
		return nil, e
	}
	if string(h[:4]) != "MHWK" || string(h[8:12]) != "RSRC" || be16(h[12:14]) != 0x100 {
		return nil, fmt.Errorf("invalid Mohawk archive %s", s.Name)
	}
	dir := int64(be32(h[20:24]))
	ft := dir + int64(be16(h[24:26]))
	b, e := s.Read(ft, 4)
	if e != nil {
		return nil, e
	}
	count := int64(be32(b))
	b, e = s.Read(ft+4, count*10)
	if e != nil {
		return nil, e
	}
	files := make([]Resource, int(count))
	for i := range files {
		p := b[i*10:]
		files[i] = Resource{Offset: int64(be32(p)), Size: int64(be16(p[4:])) | int64(p[6])<<16 | int64(p[7]&7)<<24}
		if files[i].Offset > s.Size {
			return nil, fmt.Errorf("invalid file offset in %s", s.Name)
		}
	}
	b, e = s.Read(dir, 4)
	if e != nil {
		return nil, e
	}
	strBase := dir + int64(be16(b))
	types := int64(be16(b[2:]))
	table, e := s.Read(dir+4, types*8)
	if e != nil {
		return nil, e
	}
	a := &Archive{Source: s, Resources: map[string]map[uint16]Resource{}}
	for i := int64(0); i < types; i++ {
		p := table[i*8:]
		tag := string(p[:4])
		if _, ok := a.Resources[tag]; ok {
			return nil, fmt.Errorf("duplicate type %s", tag)
		}
		rt, nt := dir+int64(be16(p[4:])), dir+int64(be16(p[6:]))
		v, e := s.Read(nt, 2)
		if e != nil {
			return nil, e
		}
		nc := int64(be16(v))
		v, e = s.Read(nt+2, nc*4)
		if e != nil {
			return nil, e
		}
		names := map[uint16]string{}
		for j := int64(0); j < nc; j++ {
			q := v[j*4:]
			off := strBase + int64(be16(q))
			size := min(int64(65536), s.Size-off)
			str, e := s.Read(off, size)
			if e != nil {
				return nil, e
			}
			end := -1
			for k, c := range str {
				if c == 0 {
					end = k
					break
				}
			}
			if end < 0 {
				return nil, fmt.Errorf("unterminated resource name")
			}
			names[be16(q[2:])] = string(str[:end])
		}
		v, e = s.Read(rt, 2)
		if e != nil {
			return nil, e
		}
		rc := int64(be16(v))
		v, e = s.Read(rt+2, rc*4)
		if e != nil {
			return nil, e
		}
		a.Resources[tag] = map[uint16]Resource{}
		for j := int64(0); j < rc; j++ {
			q := v[j*4:]
			id, index := be16(q), be16(q[2:])
			if index == 0 || int(index) > len(files) {
				return nil, fmt.Errorf("invalid resource index")
			}
			if _, ok := a.Resources[tag][id]; ok {
				return nil, fmt.Errorf("duplicate resource %s:%d", tag, id)
			}
			r := files[int(index)-1]
			r.ID = id
			r.Index = index
			r.Name = names[index]
			if tag == "tMOV" {
				next := int(index)
				for next < len(files) && files[next].Offset == r.Offset {
					next++
				}
				end := s.Size
				if next < len(files) {
					end = files[next].Offset
				}
				r.Size = end - r.Offset
			}
			if r.Offset < 0 || r.Size < 0 || r.Offset > s.Size || r.Size > s.Size-r.Offset {
				return nil, fmt.Errorf("resource outside archive: %s:%d", tag, id)
			}
			a.Resources[tag][id] = r
		}
	}
	return a, nil
}

func dual16(b []byte) (uint16, error) {
	if len(b) < 4 {
		return 0, io.ErrUnexpectedEOF
	}
	a := binary.LittleEndian.Uint16(b)
	if a != be16(b[2:]) {
		return 0, fmt.Errorf("ISO dual-endian mismatch")
	}
	return a, nil
}
func dual32(b []byte) (uint32, error) {
	if len(b) < 8 {
		return 0, io.ErrUnexpectedEOF
	}
	a := binary.LittleEndian.Uint32(b)
	if a != be32(b[4:]) {
		return 0, fmt.Errorf("ISO dual-endian mismatch")
	}
	return a, nil
}

type isoEntry struct {
	extent, size int64
	flags        byte
	name         string
}

func isoRecord(b []byte, total int64) (isoEntry, error) {
	if len(b) < 34 || int(b[0]) != len(b) || 33+int(b[32]) > len(b) {
		return isoEntry{}, fmt.Errorf("invalid ISO directory record")
	}
	ex, e := dual32(b[2:])
	if e != nil {
		return isoEntry{}, e
	}
	sz, e := dual32(b[10:])
	if e != nil {
		return isoEntry{}, e
	}
	if b[1] != 0 || b[26] != 0 || b[27] != 0 || b[25]&128 != 0 {
		return isoEntry{}, fmt.Errorf("unsupported ISO extents")
	}
	if int64(ex)*2048+int64(sz) > total {
		return isoEntry{}, fmt.Errorf("ISO extent outside image")
	}
	return isoEntry{int64(ex) * 2048, int64(sz), b[25], string(b[33 : 33+int(b[32])])}, nil
}
func ISOFiles(path string) ([]FileSlice, error) {
	info, e := os.Stat(path)
	if e != nil {
		return nil, e
	}
	s := FileSlice{Path: path, Name: filepath.Base(path), Size: info.Size()}
	var primary []byte
	for sector := int64(16); sector < min(int64(256), s.Size/2048); sector++ {
		b, e := s.Read(sector*2048, 2048)
		if e != nil {
			return nil, e
		}
		if string(b[1:6]) != "CD001" || b[6] != 1 {
			return nil, fmt.Errorf("invalid ISO volume")
		}
		if b[0] == 1 {
			primary = b
		}
		if b[0] == 255 {
			break
		}
	}
	if primary == nil {
		return nil, fmt.Errorf("missing ISO primary descriptor")
	}
	block, e := dual16(primary[128:])
	if e != nil || block != 2048 {
		return nil, fmt.Errorf("unsupported ISO logical block")
	}
	root, e := isoRecord(primary[156:156+int(primary[156])], s.Size)
	if e != nil {
		return nil, e
	}
	visited := map[int64]bool{}
	var result []FileSlice
	var walk func(isoEntry, string) error
	walk = func(entry isoEntry, prefix string) error {
		if visited[entry.extent] {
			return fmt.Errorf("cyclic ISO directory")
		}
		visited[entry.extent] = true
		b, e := s.Read(entry.extent, entry.size)
		if e != nil {
			return e
		}
		for pos := 0; pos < len(b); {
			n := int(b[pos])
			if n == 0 {
				pos = (pos/2048 + 1) * 2048
				continue
			}
			if pos+n > len(b) || pos%2048+n > 2048 {
				return fmt.Errorf("ISO record crosses sector")
			}
			child, e := isoRecord(b[pos:pos+n], s.Size)
			if e != nil {
				return e
			}
			pos += n
			if child.name == "\x00" || child.name == "\x01" || child.flags&4 != 0 {
				continue
			}
			name := strings.Split(child.name, ";")[0]
			full := name
			if prefix != "" {
				full = prefix + "/" + name
			}
			if child.flags&2 != 0 {
				if e = walk(child, full); e != nil {
					return e
				}
			} else {
				result = append(result, FileSlice{Path: path, Name: full, Offset: child.extent, Size: child.size})
			}
		}
		return nil
	}
	if e = walk(root, ""); e != nil {
		return nil, e
	}
	return result, nil
}
func DiscoverArchives(input string) ([]*Archive, error) {
	var paths []string
	info, e := os.Stat(input)
	if e != nil {
		return nil, e
	}
	if info.IsDir() {
		e = filepath.WalkDir(input, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != input && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".iso" || ext == ".mhk" {
				paths = append(paths, path)
			}
			return nil
		})
		if e != nil {
			return nil, e
		}
	} else {
		paths = []string{input}
	}
	sort.Strings(paths)
	var result []*Archive
	for _, p := range paths {
		var slices []FileSlice
		if strings.EqualFold(filepath.Ext(p), ".iso") {
			slices, e = ISOFiles(p)
		} else {
			in, err := os.Stat(p)
			if err != nil {
				return nil, err
			}
			slices = []FileSlice{{Path: p, Name: filepath.Base(p), Size: in.Size()}}
		}
		if e != nil {
			return nil, e
		}
		for _, s := range slices {
			if !strings.EqualFold(filepath.Ext(s.Name), ".mhk") {
				continue
			}
			a, err := OpenArchive(s)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", s.Name, err)
			}
			result = append(result, a)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no Mohawk archives found in %s", input)
	}
	return result, nil
}
