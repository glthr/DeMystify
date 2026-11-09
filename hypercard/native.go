package hypercard

// The block layout is based on the reverse-engineered HyperCard 2.x format
// documentation and cross-checked against stackimport by Rebecca Bettencourt
// and Uli Kusterer (https://github.com/uliwitness/stackimport). This reader is
// independently implemented and deliberately does not decode media or execute
// HyperTalk. Offsets below include each block's size/type/ID header.
// Format reference: https://github.com/PierreLorenzi/HyperCardPreview/blob/master/StackFormat.md

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const nativeFormat = "hypercard-10"

// ReadFile is bounded independently of untrusted sizes inside the file. Media
// banks are separate from the graph-bearing data forks this reader supports.
const maxNativeFileBytes int64 = 256 << 20

type nativeKey struct {
	kind string
	id   int
}

type nativeBlock struct {
	kind   string
	id     int
	offset int
	data   []byte
}

type nativeReader struct {
	path   string
	blocks map[nativeKey]nativeBlock
	order  []nativeBlock
}

// ReadFile imports a HyperCard format-10 data fork of at most 256 MiB. Resource
// forks and external artwork/sound banks are not needed for the static graph
// and are not read.
func ReadFile(path string) (Stack, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Stack{}, err
	}
	if !info.Mode().IsRegular() {
		return Stack{}, fmt.Errorf("%s: HyperCard input must be a regular file", path)
	}
	if info.Size() > maxNativeFileBytes {
		return Stack{}, fmt.Errorf("%s: HyperCard data fork exceeds supported size limit (%d bytes)", path, maxNativeFileBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return Stack{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxNativeFileBytes+1))
	if err != nil {
		return Stack{}, fmt.Errorf("%s: %w", path, err)
	}
	if int64(len(data)) > maxNativeFileBytes {
		return Stack{}, fmt.Errorf("%s: HyperCard data fork exceeds supported size limit (%d bytes)", path, maxNativeFileBytes)
	}
	stack, err := readNative(data, filepath.Base(path), path)
	if err != nil {
		return Stack{}, fmt.Errorf("%s: %w", path, err)
	}
	return stack, nil
}

// Read imports a HyperCard format-10 data fork without modifying data. Name is
// the stack's filename-derived identity, not a value stored in its STAK block.
// Cards and CardOrder retain the order declared by the LIST/PAGE tables.
func Read(data []byte, name string) (Stack, error) {
	stack, err := readNative(data, name, name)
	if err != nil {
		return Stack{}, fmt.Errorf("%s: %w", name, err)
	}
	return stack, nil
}

func readNative(data []byte, name, path string) (Stack, error) {
	if strings.TrimSpace(name) == "" {
		return Stack{}, fmt.Errorf("HyperCard stack requires a name")
	}
	r := nativeReader{path: path, blocks: make(map[nativeKey]nativeBlock)}
	if err := r.scan(data); err != nil {
		return Stack{}, err
	}
	stak := r.blocks[nativeKey{"STAK", -1}]
	if err := stak.require(0, 1536); err != nil {
		return Stack{}, err
	}
	if format := stak.u32(16); format != 10 {
		return Stack{}, stak.errorAt(16, "unsupported HyperCard format %d (want 10)", format)
	}
	if uint64(stak.u32(20)) != uint64(len(data)) {
		return Stack{}, stak.errorAt(20, "declared data-fork size does not match input size")
	}
	// The header's master-index metadata is not needed here. In particular,
	// offset 24 must not be treated as a second STAK block-length field: the
	// actual block length was already bounded while scanning the data fork.
	var checksum uint32
	for off := 0; off < 1536; off += 4 {
		checksum += stak.u32(off)
	}
	if checksum != 0 {
		return Stack{}, stak.errorAt(112, "invalid STAK checksum")
	}
	stackScript, err := r.scriptAt(stak, 1536, len(stak.data))
	if err != nil {
		return Stack{}, err
	}
	stack := Stack{Name: name, Source: r.source(0), Script: stackScript}
	backgrounds := make(map[int]Background)
	for _, b := range r.order {
		if b.kind != "BKGD" {
			continue
		}
		layer, err := r.layer(b, false)
		if err != nil {
			return Stack{}, err
		}
		background := Background{ID: b.id, Name: layer.name, Parts: layer.parts,
			Contents: layer.contents, Script: layer.script, Source: r.source(b.offset)}
		if err := validateContents(background.Contents, background.Parts, nil); err != nil {
			return Stack{}, b.errorAt(0, "%v", err)
		}
		backgrounds[b.id] = background
		stack.Backgrounds = append(stack.Backgrounds, background)
	}
	if uint64(stak.u32(36)) != uint64(len(backgrounds)) {
		return Stack{}, stak.errorAt(36, "declared background count does not match BKGD inventory")
	}
	if len(backgrounds) != 0 {
		if _, ok := backgrounds[stak.idAt(40)]; !ok {
			return Stack{}, stak.errorAt(40, "first background refers to a missing BKGD")
		}
	}
	cardBlocks := 0
	for key := range r.blocks {
		if key.kind == "CARD" {
			cardBlocks++
		}
	}
	if uint64(stak.u32(44)) != uint64(cardBlocks) {
		return Stack{}, stak.errorAt(44, "declared card count does not match CARD inventory")
	}
	cardOrder, pages, err := r.cardOrder(stak.idAt(52), cardBlocks)
	if err != nil {
		return Stack{}, err
	}
	stack.CardOrder = cardOrder
	for _, id := range cardOrder {
		b := r.blocks[nativeKey{"CARD", id}]
		layer, err := r.layer(b, true)
		if err != nil {
			return Stack{}, err
		}
		background, ok := backgrounds[layer.owner]
		if !ok {
			return Stack{}, b.errorAt(36, "card refers to missing background %d", layer.owner)
		}
		if layer.page != pages[id] {
			return Stack{}, b.errorAt(32, "card PAGE reference does not match its LIST/PAGE entry")
		}
		if err := validateContents(layer.contents, background.Parts, layer.parts); err != nil {
			return Stack{}, b.errorAt(0, "%v", err)
		}
		// A per-card background button content is its highlight state, not text.
		// The neutral model has no highlight property; keep its identity and the
		// empty text exposed by the XML frontend.
		for i := range layer.contents {
			content := &layer.contents[i]
			if content.Layer == "background" && partType(background.Parts, content.ID) == "button" {
				content.Text = ""
			}
		}
		stack.Cards = append(stack.Cards, Card{ID: id, Name: layer.name,
			BackgroundID: layer.owner, Parts: layer.parts, Contents: layer.contents,
			Script: layer.script, Source: r.source(b.offset)})
	}
	if cardBlocks != 0 {
		if _, ok := pages[stak.idAt(48)]; !ok {
			return Stack{}, stak.errorAt(48, "first card refers to a missing CARD")
		}
	}
	stack.BackgroundsComplete = true
	for _, b := range r.order {
		switch b.kind {
		case "STAK", "LIST", "PAGE", "CARD", "BKGD", "TAIL":
			continue
		}
		message := "block is not used for static script analysis"
		switch b.kind {
		case "BMAP":
			message = "bitmap pixels are not decoded"
		case "FTBL", "STBL":
			message = "font and visual style data are not decoded"
		case "PRNT", "PRST", "PRFT":
			message = "print settings are not decoded"
		case "MAST":
			message = "master index is not used; data-fork blocks were indexed directly"
		case "FREE":
			message = "free block skipped"
		default:
			message = "unsupported block skipped"
		}
		stack.Diagnostics = append(stack.Diagnostics, Diagnostic{BlockType: b.kind,
			ID: b.id, Offset: int64(b.offset), Message: message})
	}
	return stack, nil
}

func (r *nativeReader) scan(data []byte) error {
	off := 0
	for off < len(data) {
		if len(data)-off < 12 {
			return fmt.Errorf("truncated block header at byte %d", off)
		}
		size := binary.BigEndian.Uint32(data[off : off+4])
		kind := string(data[off+4 : off+8])
		id := int(int32(binary.BigEndian.Uint32(data[off+8 : off+12])))
		if size < 12 || uint64(size) > uint64(len(data)-off) {
			return fmt.Errorf("invalid %q block size %d at byte %d", kind, size, off)
		}
		b := nativeBlock{kind: kind, id: id, offset: off, data: data[off : off+int(size)]}
		if off == 0 && (kind != "STAK" || id != -1) {
			return b.errorAt(0, "first block must be STAK #-1")
		}
		if kind == "STAK" && (off != 0 || id != -1) {
			return b.errorAt(0, "unexpected additional STAK block")
		}
		if kind == "CARD" || kind == "BKGD" || kind == "LIST" || kind == "PAGE" {
			if id <= 0 {
				return b.errorAt(8, "block requires a positive ID")
			}
		}
		key := nativeKey{kind, id}
		if kind != "FREE" {
			if _, exists := r.blocks[key]; exists {
				return b.errorAt(0, "duplicate live block %s #%d", kind, id)
			}
			r.blocks[key] = b
		}
		r.order = append(r.order, b)
		off += int(size)
		if kind == "TAIL" {
			if id != -1 || off != len(data) {
				return b.errorAt(0, "invalid TAIL or trailing bytes after TAIL")
			}
			return nil
		}
	}
	return fmt.Errorf("missing TAIL block")
}

func (r *nativeReader) cardOrder(listID, expected int) ([]int, map[int]int, error) {
	list, ok := r.blocks[nativeKey{"LIST", listID}]
	if !ok {
		return nil, nil, fmt.Errorf("missing LIST #%d", listID)
	}
	if err := list.require(0, 48); err != nil {
		return nil, nil, err
	}
	if uint64(list.u32(24)) != uint64(expected) || uint64(list.u32(40)) != uint64(expected) {
		return nil, nil, list.errorAt(24, "LIST total card counts disagree with CARD inventory")
	}
	stride := int(list.u16(28))
	if stride < 5 {
		return nil, nil, list.errorAt(28, "invalid PAGE card-entry size %d", stride)
	}
	pageCount := uint64(list.u32(16))
	if pageCount > uint64((len(list.data)-48)/6) {
		return nil, nil, list.errorAt(16, "LIST page count exceeds block bounds")
	}
	var order []int
	pages := make(map[int]int)
	seenPages := make(map[int]bool)
	for i := 0; i < int(pageCount); i++ {
		pos := 48 + i*6
		id := list.idAt(pos)
		count := int(list.u16(pos + 4))
		page, ok := r.blocks[nativeKey{"PAGE", id}]
		if !ok || seenPages[id] {
			return nil, nil, list.errorAt(pos, "missing or repeated PAGE #%d", id)
		}
		seenPages[id] = true
		if err := page.require(0, 24); err != nil {
			return nil, nil, err
		}
		if page.idAt(16) != listID {
			return nil, nil, page.errorAt(16, "PAGE belongs to a different LIST")
		}
		if count > (len(page.data)-24)/stride {
			return nil, nil, page.errorAt(24, "declared card entries exceed PAGE bounds")
		}
		// A PAGE can fill its block exactly. LIST's count is authoritative;
		// there is no requirement for a trailing zero-ID sentinel.
		for j := 0; j < count; j++ {
			pos := 24 + j*stride
			cardID := page.idAt(pos)
			if _, ok := r.blocks[nativeKey{"CARD", cardID}]; !ok {
				return nil, nil, page.errorAt(pos, "PAGE refers to missing CARD #%d", cardID)
			}
			if _, repeated := pages[cardID]; repeated {
				return nil, nil, page.errorAt(pos, "repeated CARD #%d in LIST/PAGE order", cardID)
			}
			pages[cardID] = id
			order = append(order, cardID)
		}
	}
	if len(order) != expected {
		return nil, nil, list.errorAt(0, "LIST/PAGE inventory contains %d cards, expected %d", len(order), expected)
	}
	for key := range r.blocks {
		if key.kind == "PAGE" && !seenPages[key.id] {
			return nil, nil, list.errorAt(0, "unreferenced PAGE #%d", key.id)
		}
		if key.kind == "LIST" && key.id != listID {
			return nil, nil, list.errorAt(0, "unreferenced LIST #%d", key.id)
		}
	}
	return order, pages, nil
}

type nativeLayer struct {
	name     string
	owner    int
	page     int
	parts    []Part
	contents []Content
	script   Script
}

func (r *nativeReader) layer(b nativeBlock, card bool) (nativeLayer, error) {
	var result nativeLayer
	partCountPos, contentCountPos, pos := 36, 44, 50
	if card {
		partCountPos, contentCountPos, pos = 40, 48, 54
	}
	if err := b.require(0, pos); err != nil {
		return result, err
	}
	if card {
		result.owner, result.page = b.idAt(36), b.idAt(32)
	}
	partCount := int(b.u16(partCountPos))
	contentCount := int(b.u16(contentCountPos))
	if partCount > (len(b.data)-pos)/31 {
		return result, b.errorAt(partCountPos, "part count exceeds block bounds")
	}
	seenParts := make(map[int]bool)
	for i := 0; i < partCount; i++ {
		if err := b.require(pos, 30); err != nil {
			return result, err
		}
		length := int(b.u16(pos))
		if length < 31 {
			return result, b.errorAt(pos, "part record is too short")
		}
		if err := b.require(pos, length); err != nil {
			return result, err
		}
		id := int(b.u16(pos + 2))
		if id == 0 || seenParts[id] {
			return result, b.errorAt(pos+2, "invalid or repeated part ID %d", id)
		}
		seenParts[id] = true
		kind := "field"
		if b.u16(pos+4)&0x0100 != 0 {
			kind = "button"
		}
		_, afterName, err := r.stringAt(b, pos+30, pos+length)
		if err != nil {
			return result, err
		}
		// A part without a script may stop at its name's terminator, with
		// at most one following zero alignment byte. Otherwise the script
		// follows a zero indicator and must terminate inside this record.
		scriptStart := pos + length
		if afterName < pos+length {
			if b.data[afterName] != 0 {
				return result, b.errorAt(afterName, "unsupported part script indicator")
			}
			scriptStart = afterName + 1
		}
		script, err := r.scriptAt(b, scriptStart, pos+length)
		if err != nil {
			return result, err
		}
		result.parts = append(result.parts, Part{ID: id, Type: kind,
			Script: script, Source: r.source(b.offset + pos)})
		pos += length
		pos += pos % 2
	}
	if contentCount > (len(b.data)-pos)/5 {
		return result, b.errorAt(contentCountPos, "content count exceeds block bounds")
	}
	seenContents := make(map[nativeKey]bool)
	for i := 0; i < contentCount; i++ {
		if err := b.require(pos, 4); err != nil {
			return result, err
		}
		id := int(int16(b.u16(pos)))
		length := int(b.u16(pos + 2))
		if id == 0 || length < 1 {
			return result, b.errorAt(pos, "invalid content ID or length")
		}
		if err := b.require(pos+4, length); err != nil {
			return result, err
		}
		layer := "background"
		if id < 0 {
			layer, id = "card", -id
		}
		if !card && layer == "card" {
			return result, b.errorAt(pos, "background contains card-layer content")
		}
		key := nativeKey{layer, id}
		if seenContents[key] {
			return result, b.errorAt(pos, "repeated content for %s part %d", layer, id)
		}
		seenContents[key] = true
		body := b.data[pos+4 : pos+4+length]
		textStart := 1 // The zero marker preceding unstyled text is not text.
		if body[0] != 0 {
			if len(body) < 2 || binary.BigEndian.Uint16(body[:2])&0x8000 == 0 {
				return result, b.errorAt(pos+4, "invalid text/style marker")
			}
			textStart = int(binary.BigEndian.Uint16(body[:2]) & 0x7fff)
			if textStart < 2 || textStart > len(body) || (textStart-2)%4 != 0 {
				return result, b.errorAt(pos+4, "invalid styled-text header length")
			}
			// Some styled records retain the zero text marker after the runs.
			// It is a format marker rather than a NUL in the field's contents.
			if textStart < len(body) && body[textStart] == 0 {
				textStart++
			}
		}
		result.contents = append(result.contents, Content{Layer: layer, ID: id,
			Text: decodeMacRoman(body[textStart:])})
		pos += 4 + length + length%2
	}
	name, afterName, err := r.stringAt(b, pos, len(b.data))
	if err != nil {
		return result, err
	}
	result.name = name.Text
	result.script, err = r.scriptAt(b, afterName, len(b.data))
	return result, err
}

func validateContents(contents []Content, background, card []Part) error {
	for _, content := range contents {
		parts := background
		if content.Layer == "card" {
			parts = card
		}
		if partType(parts, content.ID) == "" {
			return fmt.Errorf("content refers to missing %s part %d", content.Layer, content.ID)
		}
	}
	return nil
}

func partType(parts []Part, id int) string {
	for _, part := range parts {
		if part.ID == id {
			return part.Type
		}
	}
	return ""
}

func (r *nativeReader) source(offset int) Source {
	return Source{Path: r.path, Offset: int64(offset), Format: nativeFormat}
}

func (r *nativeReader) scriptAt(b nativeBlock, start, limit int) (Script, error) {
	if start < 0 || start > limit || limit > len(b.data) {
		return Script{}, b.errorAt(start, "script exceeds block bounds")
	}
	empty := Script{Source: r.source(b.offset + start)}
	if start == limit {
		return empty, nil
	}
	if b.data[start] == 0 {
		for _, value := range b.data[start+1 : limit] {
			if value != 0 {
				return Script{}, b.errorAt(start, "unsupported OSA/binary script")
			}
		}
		return empty, nil
	}
	script, _, err := r.stringAt(b, start, limit)
	return script, err
}

func (r *nativeReader) stringAt(b nativeBlock, start, limit int) (Script, int, error) {
	if start < 0 || start >= limit || limit > len(b.data) {
		return Script{}, 0, b.errorAt(start, "missing terminated string")
	}
	end := bytes.IndexByte(b.data[start:limit], 0)
	if end < 0 {
		return Script{}, 0, b.errorAt(start, "unterminated string")
	}
	return Script{Text: decodeMacRoman(b.data[start : start+end]),
		Source: r.source(b.offset + start)}, start + end + 1, nil
}

func (b nativeBlock) require(offset, length int) error {
	if offset < 0 || length < 0 || offset > len(b.data) || length > len(b.data)-offset {
		return b.errorAt(offset, "record exceeds block bounds")
	}
	return nil
}

func (b nativeBlock) u16(offset int) uint16 {
	return binary.BigEndian.Uint16(b.data[offset : offset+2])
}

func (b nativeBlock) u32(offset int) uint32 {
	return binary.BigEndian.Uint32(b.data[offset : offset+4])
}

func (b nativeBlock) idAt(offset int) int {
	return int(int32(b.u32(offset)))
}

func (b nativeBlock) errorAt(offset int, format string, args ...any) error {
	return fmt.Errorf("HyperCard %s #%d at byte %d: %s", b.kind, b.id,
		b.offset+offset, fmt.Sprintf(format, args...))
}

// MacRoman's upper half, in byte order. The command-key control character uses
// the same visible Unicode representation as the existing XML conversion.
var macRomanUpper = []rune("ÄÅÇÉÑÖÜáàâäãåçéèêëíìîïñóòôöõúùûü†°¢£§•¶ß®©™´¨≠ÆØ∞±≤≥¥µ∂∑∏π∫ªºΩæø¿¡¬√ƒ≈∆«»… ÀÃÕŒœ–—“”‘’÷◊ÿŸ⁄€‹›ﬁﬂ‡·‚„‰ÂÊÁËÈÍÎÏÌÓÔÒÚÛÙıˆ˜¯˘˙˚¸˝˛ˇ")

func decodeMacRoman(data []byte) string {
	var out strings.Builder
	out.Grow(len(data))
	for _, c := range data {
		switch {
		case c >= 128:
			out.WriteRune(macRomanUpper[c-128])
		case c == 0x11:
			out.WriteRune('⌘')
		default:
			out.WriteByte(c)
		}
	}
	text := strings.ReplaceAll(out.String(), "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}
