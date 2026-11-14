package riven

import (
	"fmt"
	"strings"

	"github.com/glthr/DeMystify/common"
)

type Branch struct {
	Value    uint16    `json:"value"`
	Commands []Command `json:"commands"`
}
type Command struct {
	Opcode       uint16   `json:"opcode"`
	Args         []uint16 `json:"args"`
	Offset       int      `json:"offset"`
	End          int      `json:"endOffset"`
	Variable     uint16   `json:"variable,omitempty"`
	Branches     []Branch `json:"branches,omitempty"`
	StackNameID  uint16   `json:"stackNameId,omitempty"`
	GlobalCardID uint32   `json:"globalCardId,omitempty"`
}
type Script struct {
	Type     uint16    `json:"type"`
	Offset   int       `json:"offset"`
	Commands []Command `json:"commands"`
}
type Hotspot struct {
	ID                   uint16 `json:"id"`
	NameID               int    `json:"nameId"`
	Name                 string `json:"name"`
	Rect                 [4]int `json:"rect"`
	Cursor, Index, Flags uint16
	TransitionOffset     int
	Scripts              []Script `json:"scripts"`
	Offset               int      `json:"offset"`
}
type Picture struct {
	Index   uint16 `json:"index"`
	ImageID uint16 `json:"imageId"`
	Rect    [4]int `json:"rect"`
}
type Card struct {
	ID           uint16                    `json:"id"`
	NameID       int                       `json:"nameId"`
	Name         string                    `json:"name"`
	GlobalID     uint32                    `json:"globalId"`
	ZipModePlace uint16                    `json:"zipModePlace"`
	Scripts      []Script                  `json:"scripts"`
	Hotspots     []Hotspot                 `json:"hotspots"`
	Pictures     []Picture                 `json:"pictures"`
	Sources      map[string]map[string]any `json:"provenance"`
}
type Stack struct {
	Name      string                            `json:"name"`
	Names     map[uint16][]string               `json:"names"`
	RMap      []uint32                          `json:"rmap"`
	Cards     map[uint16]*Card                  `json:"cards"`
	Resources map[string]map[uint16]ResourceRef `json:"-"`
}
type reader struct {
	b   []byte
	pos int
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return make([]byte, max(n, 0))
	}
	if n < 0 || r.pos > len(r.b) || n > len(r.b)-r.pos {
		r.err = fmt.Errorf("truncated resource at +0x%x", r.pos)
		return make([]byte, max(n, 0))
	}
	v := r.b[r.pos : r.pos+n]
	r.pos += n
	return v
}
func (r *reader) u16() uint16 { return be16(r.take(2)) }
func (r *reader) i16() int    { return int(int16(r.u16())) }
func (r *reader) u32() uint32 { return be32(r.take(4)) }
func (r *reader) finish() error {
	if r.err != nil {
		return r.err
	}
	if r.pos != len(r.b) {
		return fmt.Errorf("%d trailing resource bytes at +0x%x", len(r.b)-r.pos, r.pos)
	}
	return nil
}
func (r *reader) commands(depth int) []Command {
	if depth > 128 {
		r.err = fmt.Errorf("excessive script nesting")
		return nil
	}
	count := int(r.u16())
	if count > (len(r.b)-r.pos)/4 {
		r.err = fmt.Errorf("invalid command count")
		return nil
	}
	out := make([]Command, 0, count)
	for i := 0; i < count && r.err == nil; i++ {
		c := Command{Offset: r.pos}
		c.Opcode = r.u16()
		n := int(r.u16())
		if c.Opcode == 8 {
			c.Variable = r.u16()
			c.Args = []uint16{c.Variable}
			branches := int(r.u16())
			if n != 2 || branches > (len(r.b)-r.pos)/4 {
				r.err = fmt.Errorf("invalid switch header at %d", c.Offset)
				break
			}
			for j := 0; j < branches && r.err == nil; j++ {
				value := r.u16()
				c.Branches = append(c.Branches, Branch{value, r.commands(depth + 1)})
			}
		} else {
			if n > (len(r.b)-r.pos)/2 {
				r.err = fmt.Errorf("invalid argument count")
				break
			}
			for j := 0; j < n; j++ {
				c.Args = append(c.Args, r.u16())
			}
			if c.Opcode == 27 {
				if n != 3 {
					r.err = fmt.Errorf("stack change needs three words")
					break
				}
				c.StackNameID = c.Args[0]
				c.GlobalCardID = uint32(c.Args[1])<<16 | uint32(c.Args[2])
			}
		}
		c.End = r.pos
		out = append(out, c)
	}
	return out
}
func (r *reader) typed() []Script {
	n := int(r.u16())
	if n > (len(r.b)-r.pos)/4 {
		r.err = fmt.Errorf("invalid script count")
		return nil
	}
	out := make([]Script, 0, n)
	for i := 0; i < n && r.err == nil; i++ {
		s := Script{Offset: r.pos}
		s.Type = r.u16()
		s.Commands = r.commands(0)
		out = append(out, s)
	}
	return out
}
func ParseCard(b []byte) (*Card, error) {
	r := reader{b: b}
	c := &Card{NameID: r.i16(), ZipModePlace: r.u16()}
	c.Scripts = r.typed()
	return c, r.finish()
}
func ParseHotspots(b []byte) ([]Hotspot, error) {
	r := reader{b: b}
	n := int(r.u16())
	if n > (len(b)-r.pos)/24 {
		return nil, fmt.Errorf("invalid hotspot count")
	}
	hs := make([]Hotspot, 0, n)
	for i := 0; i < n && r.err == nil; i++ {
		h := Hotspot{Offset: r.pos}
		h.ID = r.u16()
		h.NameID = r.i16()
		for j := range h.Rect {
			h.Rect[j] = r.i16()
		}
		r.u16()
		h.Cursor = r.u16()
		h.Index = r.u16()
		h.TransitionOffset = r.i16()
		h.Flags = r.u16()
		h.Scripts = r.typed()
		hs = append(hs, h)
	}
	return hs, r.finish()
}
func ParseNames(b []byte) ([]string, error) {
	r := reader{b: b}
	n := int(r.u16())
	offsets := make([]uint16, n)
	for i := range offsets {
		offsets[i] = r.u16()
	}
	r.take(n * 2)
	if r.err != nil {
		return nil, r.err
	}
	base := r.pos
	names := make([]string, n)
	for i, o := range offsets {
		p := base + int(o)
		if p >= len(b) {
			return nil, fmt.Errorf("NAME offset outside resource")
		}
		end := p
		for end < len(b) && b[end] != 0 {
			end++
		}
		if end == len(b) {
			return nil, fmt.Errorf("unterminated NAME string")
		}
		names[i] = common.DecodeMacRoman(b[p:end])
	}
	return names, nil
}
func ParseRMap(b []byte) ([]uint32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("RMAP not word aligned")
	}
	r := reader{b: b}
	out := make([]uint32, len(b)/4)
	for i := range out {
		out[i] = r.u32()
	}
	return out, r.finish()
}
func ParsePictures(b []byte) ([]Picture, error) {
	r := reader{b: b}
	n := int(r.u16())
	if n > (len(b)-r.pos)/12 {
		return nil, fmt.Errorf("invalid picture count")
	}
	ps := make([]Picture, n)
	for i := range ps {
		ps[i].Index = r.u16()
		ps[i].ImageID = r.u16()
		for j := range ps[i].Rect {
			ps[i].Rect[j] = int(r.u16())
		}
	}
	return ps, r.finish()
}
func NameAt(names map[uint16][]string, table uint16, id int) string {
	if id < 0 {
		return ""
	}
	ns := names[table]
	if id >= len(ns) {
		return fmt.Sprintf("NAME%d[%d]", table, id)
	}
	return ns[id]
}

var events = []string{"mouse_down", "mouse_drag", "mouse_up", "mouse_enter", "mouse_inside", "mouse_leave", "card_load", "card_leave", "card_frame", "card_enter", "card_update"}

func EventName(t uint16) string {
	if int(t) < len(events) {
		return events[t]
	}
	return fmt.Sprintf("event_%d", t)
}

var opNames = map[uint16]string{1: "draw_bitmap", 2: "change_card", 3: "play_script_slst", 4: "play_sound", 7: "set_variable", 8: "switch", 9: "enable_hotspot", 10: "disable_hotspot", 12: "stop_sound", 13: "change_cursor", 14: "delay", 17: "run_external", 18: "transition", 19: "refresh_card", 20: "begin_screen_update", 21: "apply_screen_update", 24: "increment_variable", 27: "change_stack", 28: "disable_movie", 29: "disable_all_movies", 31: "enable_movie", 32: "play_movie_blocking", 33: "play_movie", 34: "stop_movie", 36: "unknown_36", 37: "fade_ambient_sounds", 38: "store_movie_opcode", 39: "activate_plst", 40: "activate_slst", 41: "activate_mlst_and_play", 43: "activate_blst", 44: "activate_flst", 45: "zip_mode", 46: "activate_mlst"}

func FormatScripts(ss []Script, names map[uint16][]string) string {
	var b strings.Builder
	var emit func([]Command, int)
	emit = func(cs []Command, depth int) {
		pad := strings.Repeat("  ", depth)
		for _, c := range cs {
			fmt.Fprintf(&b, "%s/* +0x%04x */ ", pad, c.Offset)
			if c.Opcode == 8 {
				fmt.Fprintf(&b, "switch (%s) {\n", NameAt(names, 4, int(c.Variable)))
				for _, br := range c.Branches {
					label := fmt.Sprintf("case %d", br.Value)
					if br.Value == 65535 {
						label = "default"
					}
					fmt.Fprintf(&b, "%s  %s:\n", pad, label)
					emit(br.Commands, depth+2)
					fmt.Fprintf(&b, "%s    break;\n", pad)
				}
				fmt.Fprintf(&b, "%s}\n", pad)
				continue
			}
			if c.Opcode == 17 && len(c.Args) >= 2 {
				fmt.Fprintf(&b, "%s(%v);\n", NameAt(names, 3, int(c.Args[0])), c.Args[2:])
			} else if (c.Opcode == 7 || c.Opcode == 24) && len(c.Args) == 2 {
				symbol := "="
				if c.Opcode == 24 {
					symbol = "+="
				}
				fmt.Fprintf(&b, "%s %s %d;\n", NameAt(names, 4, int(c.Args[0])), symbol, c.Args[1])
			} else if c.Opcode == 27 {
				fmt.Fprintf(&b, "change_stack(%q, rmap=0x%x);\n", NameAt(names, 5, int(c.StackNameID)), c.GlobalCardID)
			} else {
				name, ok := opNames[c.Opcode]
				if !ok {
					name = fmt.Sprintf("opcode_%d", c.Opcode)
				}
				fmt.Fprintf(&b, "%s(%v);\n", name, c.Args)
			}
		}
	}
	for _, s := range ss {
		fmt.Fprintf(&b, "on %s {\n", EventName(s.Type))
		emit(s.Commands, 1)
		b.WriteString("}\n")
	}
	return b.String()
}
