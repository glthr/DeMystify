package hypercard

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These are synthetic format-10 records, not extracts from a game. Offsets are
// independently derived from the pinned stackimport format notes and decoder.
// Headers, declared counts, padding and checksums are deliberately populated so
// malformed-record tests reach their intended layer rather than a bad envelope.
type validationBlock struct {
	kind string
	id   int32
	data []byte
}

func validationU16(b []byte, off, value int) {
	binary.BigEndian.PutUint16(b[off:], uint16(value))
}

func validationU32(b []byte, off int, value uint32) {
	binary.BigEndian.PutUint32(b[off:], value)
}

func validationEncode(blocks []validationBlock) []byte {
	var data []byte
	for _, block := range blocks {
		payload := block.data
		if (block.kind == "TAIL" || block.kind == "FREE") && len(payload) < 4 {
			payload = append(bytes.Clone(payload), make([]byte, 4-len(payload))...)
		}
		header := make([]byte, 12)
		validationU32(header, 0, uint32(12+len(payload)))
		copy(header[4:8], block.kind)
		validationU32(header, 8, uint32(block.id))
		data = append(data, header...)
		data = append(data, payload...)
	}
	// Ordinary format-10 files place the master index immediately after STAK.
	// Populate that metadata independently of checksum repair so tests can also
	// exercise this importer's explicit tolerance of an unused index pointer.
	if len(data) >= 28 && string(data[4:8]) == "STAK" {
		validationU32(data, 24, binary.BigEndian.Uint32(data))
	}
	validationSealEnvelope(data)
	return data
}

// Repair only envelope fields, allowing fuzz mutations to reach record parsing.
// Unit cases separately exercise checksum/size failures without this repair.
func validationSealEnvelope(data []byte) {
	if len(data) < 1536 || string(data[4:8]) != "STAK" {
		return
	}
	stackSize := binary.BigEndian.Uint32(data)
	if stackSize < 1536 || uint64(stackSize) > uint64(len(data)) {
		return
	}
	validationU32(data, 20, uint32(len(data)))
	validationU32(data, 112, 0)
	var sum uint32
	for i := 0; i < 1536; i += 4 {
		sum += binary.BigEndian.Uint32(data[i:])
	}
	validationU32(data, 112, -sum)
}

func validationPart(id, kind int, name string, script []byte) []byte {
	b := make([]byte, 30)
	validationU16(b, 2, id)
	validationU16(b, 4, kind<<8)
	b = append(b, name...)
	b = append(b, 0, 0) // Name terminator and the part's separator byte.
	b = append(b, script...)
	b = append(b, 0)
	validationU16(b, 0, len(b))
	return b
}

func validationContent(id int, payload []byte) []byte {
	b := make([]byte, 4)
	validationU16(b, 0, id)
	validationU16(b, 2, len(payload))
	return append(b, payload...)
}

func validationAppendAligned(b, record []byte) []byte {
	b = append(b, record...)
	if len(b)%2 != 0 {
		b = append(b, 0xa5) // Padding is not text or a string terminator.
	}
	return b
}

func validationLayer(card bool, name string, script []byte) []byte {
	header, partsAt, contentsAt := 38, 24, 32
	if card {
		header, partsAt, contentsAt = 42, 28, 36
	}
	b := make([]byte, header)
	parts := [][]byte{
		validationPart(1, 2, "picture field", nil),
		validationPart(2, 1, "shared button", nil),
	}
	contents := [][]byte{validationContent(1, append([]byte{0}, "shared field"...))}
	if card {
		validationU32(b, 20, 200) // Owning PAGE.
		validationU32(b, 24, 300) // Owning BKGD.
		parts = [][]byte{
			validationPart(5, 1, "odd", []byte("on mouseUp\rgo this card\rend mouseUp\r")),
			validationPart(6, 2, "a longer field name", []byte("on openField\r\npass openField\rend openField\r")),
			validationPart(7, 2, "empty field", nil),
		}
		// Styled text has a 6-byte style header/run, then its plain marker.
		styled := append([]byte{0x80, 6, 0, 0, 0, 7, 0}, []byte("caf\x8e\rline two")...)
		contents = [][]byte{
			validationContent(1, []byte{0, 'V', 'i', 'e', 'w', ' ', 0xf0}),
			validationContent(2, []byte{0, '1'}),  // Background-button highlight.
			validationContent(-5, []byte{0, '1'}), // Actual card-button text.
			validationContent(-6, styled),
			validationContent(-7, []byte{0}), // A valid empty plain field.
		}
	}
	validationU16(b, partsAt, len(parts))
	validationU16(b, contentsAt, len(contents))
	for _, part := range parts {
		b = validationAppendAligned(b, part)
	}
	for _, content := range contents {
		b = validationAppendAligned(b, content)
	}
	b = append(b, name...)
	b = append(b, 0)
	b = append(b, script...)
	return append(b, 0)
}

func validationFixture(ids []int32, sentinel bool) []validationBlock {
	stack := make([]byte, 1524)
	validationU32(stack, 4, 10)
	validationU32(stack, 24, 1)
	validationU32(stack, 28, 300)
	validationU32(stack, 32, uint32(len(ids)))
	validationU32(stack, 36, uint32(ids[len(ids)-1])) // Need not be order[0].
	validationU32(stack, 40, 100)
	stack = append(stack, []byte("on openStack\r-- caf\x8e \xad \xc2 \xf0\r\nend openStack\r\x00")...)
	list := make([]byte, 42)
	validationU32(list, 4, 1)
	validationU32(list, 12, uint32(len(ids)))
	validationU16(list, 16, 8)
	validationU32(list, 28, uint32(len(ids)))
	validationU32(list, 36, 200)
	validationU16(list, 40, len(ids))
	page := make([]byte, 12+8*len(ids))
	validationU32(page, 4, 100)
	for i, id := range ids {
		validationU32(page, 12+i*8, uint32(id))
	}
	if sentinel {
		page = append(page, make([]byte, 8)...)
	}
	blocks := []validationBlock{
		{"STAK", -1, stack}, {"LIST", 100, list}, {"PAGE", 200, page},
		{"BKGD", 300, validationLayer(false, "Shared", []byte("on openBackground\rpass openBackground\rend openBackground\r"))},
	}
	for _, id := range ids {
		name := fmt.Sprintf("Room %d caf", id) + "\x8e"
		blocks = append(blocks, validationBlock{"CARD", id, validationLayer(true, name, []byte("on openCard\r\npass openCard\rend openCard\r"))})
	}
	return append(blocks, validationBlock{"TAIL", -1, nil})
}

func validationFind(blocks []validationBlock, kind string) *validationBlock {
	for i := range blocks {
		if blocks[i].kind == kind {
			return &blocks[i]
		}
	}
	panic("fixture block not found: " + kind)
}

func validationFirstContent(b []byte) int {
	off := 42
	for i := 0; i < int(binary.BigEndian.Uint16(b[28:])); i++ {
		off += int(binary.BigEndian.Uint16(b[off:]))
		off += off % 2
	}
	return off
}

func validationCard(t *testing.T, stack Stack, id int) Card {
	t.Helper()
	for _, card := range stack.Cards {
		if card.ID == id {
			return card
		}
	}
	t.Fatalf("missing card %d", id)
	return Card{}
}

func TestNativeValidationPreservesTextRecordsAndOrder(t *testing.T) {
	data := validationEncode(validationFixture([]int32{20, 10}, true))
	before := bytes.Clone(data)
	stack, err := Read(data, "Synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, before) {
		t.Fatal("reader changed its source bytes")
	}
	if stack.Name != "Synthetic" || !stack.BackgroundsComplete || !reflect.DeepEqual(stack.CardOrder, []int{20, 10}) {
		t.Fatalf("stack identity/completeness/order: %+v", stack)
	}
	if stack.Script.Text != "on openStack\n-- café ≠ ¬ \uf8ff\nend openStack\n" {
		t.Fatalf("MacRoman/line endings = %q", stack.Script.Text)
	}
	if stack.Script.Source.Offset != 1536 || stack.Script.Source.Path != "Synthetic" {
		t.Fatalf("stack script provenance = %+v", stack.Script.Source)
	}
	if len(stack.Backgrounds) != 1 || stack.Backgrounds[0].ID != 300 || stack.Backgrounds[0].Name != "Shared" || stack.Backgrounds[0].Script.Text != "on openBackground\npass openBackground\nend openBackground\n" {
		t.Fatalf("background script metadata = %+v", stack.Backgrounds)
	}
	card := validationCard(t, stack, 20)
	if card.Name != "Room 20 café" || card.BackgroundID != 300 || card.Script.Text != "on openCard\npass openCard\nend openCard\n" {
		t.Fatalf("card metadata = %+v", card)
	}
	if len(card.Parts) != 3 || card.Parts[0].ID != 5 || card.Parts[0].Type != "button" || card.Parts[1].Type != "field" || card.Parts[1].Script.Text != "on openField\npass openField\nend openField\n" {
		t.Fatalf("variable part records = %+v", card.Parts)
	}
	cardOffset := int(card.Source.Offset)
	if cardOffset < 0 || cardOffset+12 > len(data) || string(data[cardOffset+4:cardOffset+8]) != "CARD" || int32(binary.BigEndian.Uint32(data[cardOffset+8:])) != 20 {
		t.Fatalf("card source does not point to its original block: %+v", card.Source)
	}
	for _, source := range []struct {
		script Script
		prefix string
	}{{card.Script, "on openCard\r\n"}, {card.Parts[1].Script, "on openField\r\n"}} {
		offset := source.script.Source.Offset
		if offset < 0 || offset >= int64(len(data)) || !bytes.HasPrefix(data[offset:], []byte(source.prefix)) {
			t.Fatalf("script provenance does not identify original CRLF bytes: %+v", source.script.Source)
		}
	}
	want := []Content{{Layer: "background", ID: 1, Text: "View \uf8ff"}, {Layer: "background", ID: 2},
		{Layer: "card", ID: 5, Text: "1"}, {Layer: "card", ID: 6, Text: "café\nline two"}, {Layer: "card", ID: 7}}
	if !reflect.DeepEqual(card.Contents, want) {
		t.Fatalf("content markers, styles, layers or highlight override: got %+v, want %+v", card.Contents, want)
	}
}

func TestNativeValidationFullPageNeedsNoSentinel(t *testing.T) {
	ids := make([]int32, 253)
	for i := range ids {
		ids[i] = int32(1000 + i)
	}
	blocks := validationFixture(ids, false)
	if got := len(validationFind(blocks, "PAGE").data); got != 2036 {
		t.Fatalf("fixture PAGE payload length = %d", got)
	}
	stack, err := Read(validationEncode(blocks), "Full page")
	if err != nil {
		t.Fatal(err)
	}
	if len(stack.Cards) != len(ids) || len(stack.CardOrder) != len(ids) || stack.CardOrder[252] != 1252 {
		t.Fatalf("full declared page lost cards: %d cards, order %v", len(stack.Cards), stack.CardOrder)
	}
}

func TestNativeValidationFreeIDsAreNotLiveObjectIDs(t *testing.T) {
	blocks := validationFixture([]int32{20, 10}, false)
	blocks = append(blocks[:len(blocks)-1], validationBlock{"FREE", -1, []byte{1, 2}}, validationBlock{"FREE", -1, []byte{3, 4}}, validationBlock{"TAIL", -1, nil})
	if _, err := Read(validationEncode(blocks), "Free blocks"); err != nil {
		t.Fatalf("repeated free-space identifiers rejected: %v", err)
	}
}

func TestNativeValidationIgnoresUnusedMasterIndexMetadata(t *testing.T) {
	// This is a tolerance test for direct block scanning, not a claim that an
	// invalid index pointer is a fully valid HyperCard file. The importer does
	// not dereference MAST and must use actual block lengths for framing.
	data := validationEncode(validationFixture([]int32{20, 10}, false))
	validationU32(data, 24, 0xffffffff)
	validationSealEnvelope(data)
	stack, err := Read(data, "Unused master index")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stack.CardOrder, []int{20, 10}) {
		t.Fatalf("unused metadata changed card order: %v", stack.CardOrder)
	}
}

func TestNativeValidationPartTypeFlagsAndOmittedScripts(t *testing.T) {
	blocks := validationFixture([]int32{20, 10}, false)
	card := validationFind(blocks, "CARD")
	payload := bytes.Clone(card.data[:42])
	flags := []uint16{0, 0x0200, 0x0100, 0x0300, 0xffff, 0xfeff}
	validationU16(payload, 28, len(flags))
	validationU16(payload, 36, 0)
	for i, flag := range flags {
		part := make([]byte, 30)
		validationU16(part, 2, 40+i)
		validationU16(part, 4, int(flag))
		if i%2 == 1 {
			part = append(part, 'x') // 32-byte named record, with no script.
		}
		part = append(part, 0) // A 31-byte empty-name record is also complete.
		if i == 2 {
			part = append(part, 0) // Empty-name record with internal alignment.
		}
		validationU16(part, 0, len(part))
		payload = validationAppendAligned(payload, part)
	}
	payload = append(payload, []byte("Minimal parts\x00on openCard\rend openCard\r\x00")...)
	card.data = payload
	stack, err := Read(validationEncode(blocks), "Minimal parts")
	if err != nil {
		t.Fatal(err)
	}
	got := validationCard(t, stack, 20)
	if len(got.Parts) != len(flags) || got.Name != "Minimal parts" {
		t.Fatalf("record boundary lost: %+v", got)
	}
	for i, flag := range flags {
		want := "field"
		if flag&0x0100 != 0 {
			want = "button"
		}
		if got.Parts[i].Type != want || got.Parts[i].Script.Text != "" {
			t.Fatalf("flags %#04x produced %+v, want empty-script %s", flag, got.Parts[i], want)
		}
	}
}

func TestNativeValidationOmittedRootScripts(t *testing.T) {
	blocks := validationFixture([]int32{20, 10}, false)
	blocks[0].data = blocks[0].data[:1524] // Script starts at raw byte1536.
	for _, kind := range []string{"BKGD", "CARD"} {
		block := validationFind(blocks, kind)
		script := []byte("on openBackground\rpass openBackground\rend openBackground\r\x00")
		if kind == "CARD" {
			script = []byte("on openCard\r\npass openCard\rend openCard\r\x00")
		}
		if !bytes.HasSuffix(block.data, script) {
			t.Fatalf("%s fixture script mismatch", kind)
		}
		block.data = block.data[:len(block.data)-len(script)]
	}
	data := validationEncode(blocks)
	stack, err := Read(data, "No root scripts")
	if err != nil {
		t.Fatal(err)
	}
	if stack.Script.Text != "" || stack.Script.Source.Offset != 1536 || stack.Backgrounds[0].Script.Text != "" || validationCard(t, stack, 20).Script.Text != "" {
		t.Fatalf("omitted root script decoded as content: %+v", stack)
	}
}

func TestNativeValidationRejectsUnsupportedOSAScripts(t *testing.T) {
	for _, scope := range []string{"stack", "background", "card", "part"} {
		t.Run(scope, func(t *testing.T) {
			blocks := validationFixture([]int32{20, 10}, false)
			var script []byte
			switch scope {
			case "stack":
				script = blocks[0].data[1524:]
			case "background":
				b := validationFind(blocks, "BKGD").data
				script = b[bytes.Index(b, []byte("on openBackground\r")):]
			case "card":
				b := validationFind(blocks, "CARD").data
				script = b[bytes.Index(b, []byte("on openCard\r\n")):]
			case "part":
				b := validationFind(blocks, "CARD").data
				script = b[bytes.Index(b, []byte("on mouseUp\r")):]
			}
			// A leading NUL with nonzero following data is OSA, not verified
			// empty HyperTalk. This reader deliberately has no OSA decoder.
			script[0] = 0
			if _, err := Read(validationEncode(blocks), "OSA"); err == nil || !strings.Contains(err.Error(), "OSA") {
				t.Fatalf("unsupported %s script error = %v", scope, err)
			}
		})
	}
}

func TestNativeValidationContentIsLengthDelimited(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload []byte
		want    string
	}{
		{"plain embedded NUL", []byte{0, 'a', 0, 'b'}, "a\x00b"},
		{"style runs without plain marker", []byte{0x80, 6, 0, 0, 0, 7, 'a', 'b'}, "ab"},
		{"empty decorated text", []byte{0x80, 2}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blocks := validationFixture([]int32{20, 10}, false)
			card := validationFind(blocks, "CARD")
			first := validationFirstContent(card.data)
			length := int(binary.BigEndian.Uint16(card.data[first+2:]))
			after := first + 4 + length + length%2
			var replacement []byte
			replacement = validationAppendAligned(replacement, validationContent(1, tc.payload))
			card.data = append(append(bytes.Clone(card.data[:first]), replacement...), card.data[after:]...)
			stack, err := Read(validationEncode(blocks), "Length-delimited content")
			if err != nil {
				t.Fatal(err)
			}
			if got := validationCard(t, stack, 20).Contents[0].Text; got != tc.want {
				t.Fatalf("text = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNativeValidationRejectsBrokenReferencesAndRecords(t *testing.T) {
	tests := []struct {
		name   string
		change func([]validationBlock) []validationBlock
	}{
		{"duplicate live card", func(b []validationBlock) []validationBlock {
			return append(b[:len(b)-1], *validationFind(b, "CARD"), validationBlock{"TAIL", -1, nil})
		}},
		{"missing list", func(b []validationBlock) []validationBlock { return append(b[:1], b[2:]...) }},
		{"missing page", func(b []validationBlock) []validationBlock { return append(b[:2], b[3:]...) }},
		{"missing background", func(b []validationBlock) []validationBlock { return append(b[:3], b[4:]...) }},
		{"missing card", func(b []validationBlock) []validationBlock { return append(b[:4], b[5:]...) }},
		{"unsupported format", func(b []validationBlock) []validationBlock { validationU32(b[0].data, 4, 9); return b }},
		{"page list owner", func(b []validationBlock) []validationBlock {
			validationU32(validationFind(b, "PAGE").data, 4, 999)
			return b
		}},
		{"card page owner", func(b []validationBlock) []validationBlock {
			validationU32(validationFind(b, "CARD").data, 20, 999)
			return b
		}},
		{"card background owner", func(b []validationBlock) []validationBlock {
			validationU32(validationFind(b, "CARD").data, 24, 999)
			return b
		}},
		{"missing page card", func(b []validationBlock) []validationBlock {
			validationU32(validationFind(b, "PAGE").data, 12, 999)
			return b
		}},
		{"missing first card", func(b []validationBlock) []validationBlock {
			validationU32(b[0].data, 36, 999)
			return b
		}},
		{"missing first background", func(b []validationBlock) []validationBlock {
			validationU32(b[0].data, 28, 999)
			return b
		}},
		{"duplicate ordered card", func(b []validationBlock) []validationBlock {
			p := validationFind(b, "PAGE").data
			validationU32(p, 20, 20)
			return b
		}},
		{"excess page count", func(b []validationBlock) []validationBlock {
			validationU16(validationFind(b, "LIST").data, 40, 0xffff)
			return b
		}},
		{"excess list count", func(b []validationBlock) []validationBlock {
			validationU32(validationFind(b, "LIST").data, 4, 0xffffffff)
			return b
		}},
		{"wrong declared total", func(b []validationBlock) []validationBlock {
			validationU32(validationFind(b, "LIST").data, 12, 3)
			return b
		}},
		{"short part", func(b []validationBlock) []validationBlock {
			validationU16(validationFind(b, "CARD").data, 42, 29)
			return b
		}},
		{"oversized part", func(b []validationBlock) []validationBlock {
			validationU16(validationFind(b, "CARD").data, 42, 0xffff)
			return b
		}},
		{"duplicate part ID", func(b []validationBlock) []validationBlock {
			p := validationFind(b, "CARD").data
			second := 42 + int(binary.BigEndian.Uint16(p[42:]))
			second += second % 2
			validationU16(p, second+2, 5)
			return b
		}},
		{"unterminated part string", func(b []validationBlock) []validationBlock {
			p := validationFind(b, "CARD").data
			n := int(binary.BigEndian.Uint16(p[42:]))
			for i := 72; i < 42+n; i++ {
				p[i] = 'x'
			}
			return b
		}},
		{"excess part count", func(b []validationBlock) []validationBlock {
			validationU16(validationFind(b, "CARD").data, 28, 0xffff)
			return b
		}},
		{"excess content count", func(b []validationBlock) []validationBlock {
			validationU16(validationFind(b, "CARD").data, 36, 0xffff)
			return b
		}},
		{"empty content payload", func(b []validationBlock) []validationBlock {
			p := validationFind(b, "CARD").data
			validationU16(p, validationFirstContent(p)+2, 0)
			return b
		}},
		{"oversized content", func(b []validationBlock) []validationBlock {
			p := validationFind(b, "CARD").data
			validationU16(p, validationFirstContent(p)+2, 0xffff)
			return b
		}},
		{"missing background part", func(b []validationBlock) []validationBlock {
			p := validationFind(b, "CARD").data
			validationU16(p, validationFirstContent(p), 99)
			return b
		}},
		{"missing own card part", func(b []validationBlock) []validationBlock {
			p := validationFind(b, "CARD").data
			validationU16(p, validationFirstContent(p), -99)
			return b
		}},
		{"duplicate content identity", func(b []validationBlock) []validationBlock {
			p := validationFind(b, "CARD").data
			first := validationFirstContent(p)
			length := int(binary.BigEndian.Uint16(p[first+2:]))
			validationU16(p, first+4+length+length%2, 1)
			return b
		}},
		{"short style header", func(b []validationBlock) []validationBlock {
			p := validationFind(b, "CARD").data
			validationU16(p, validationFirstContent(p)+4, 0x8001)
			return b
		}},
		{"style outside content", func(b []validationBlock) []validationBlock {
			p := validationFind(b, "CARD").data
			validationU16(p, validationFirstContent(p)+4, 0xffff)
			return b
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			blocks := validationFixture([]int32{20, 10}, true)
			if _, err := Read(validationEncode(tc.change(blocks)), "Invalid"); err == nil {
				t.Fatal("malformed stack accepted")
			}
		})
	}
}

func TestNativeValidationTruncatedAndInvalidBlockLengths(t *testing.T) {
	data := validationEncode(validationFixture([]int32{20, 10}, false))
	for _, cut := range []int{0, 1, 7, 11, 12, 100, 1535, 1536, len(data) - 1, len(data) - 12} {
		if _, err := Read(data[:cut], "Truncated"); err == nil {
			t.Errorf("accepted prefix of %d/%d bytes", cut, len(data))
		}
	}
	listAt := int(binary.BigEndian.Uint32(data))
	for _, length := range []uint32{0, 1, 11, 0xffffffff} {
		broken := bytes.Clone(data)
		validationU32(broken, listAt, length)
		if _, err := Read(broken, "Bad length"); err == nil {
			t.Errorf("accepted block length %d", length)
		}
	}
	broken := bytes.Clone(data)
	broken[112] ^= 1
	if _, err := Read(broken, "Bad checksum"); err == nil {
		t.Fatal("accepted incorrect stack-header checksum")
	}
}

func TestNativeValidationReadFilePreservesInput(t *testing.T) {
	data := validationEncode(validationFixture([]int32{20, 10}, true))
	path := filepath.Join(t.TempDir(), "Original Stack")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	stack, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if stack.Name != "Original Stack" || stack.Source.Path != path || stack.Source.Format != "hypercard-10" {
		t.Fatalf("file provenance = %+v", stack.Source)
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, data) {
		t.Fatalf("source file changed: %v", err)
	}
	if _, err := ReadFile(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing file accepted")
	}
}

func FuzzNativeValidationBounded(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("not a stack"))
	f.Add(validationEncode(validationFixture([]int32{20, 10}, false)))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			t.Skip()
		}
		inputs := [][]byte{data}
		repaired := bytes.Clone(data)
		validationSealEnvelope(repaired)
		if !bytes.Equal(data, repaired) {
			inputs = append(inputs, repaired)
		}
		for _, input := range inputs {
			before := bytes.Clone(input)
			stack, err := Read(input, "Fuzz")
			if !bytes.Equal(input, before) {
				t.Fatal("reader mutated input")
			}
			if err != nil {
				continue
			}
			backgrounds, cards, order := map[int]bool{}, map[int]bool{}, map[int]bool{}
			for _, b := range stack.Backgrounds {
				if b.ID <= 0 || backgrounds[b.ID] {
					t.Fatalf("invalid background identity %d", b.ID)
				}
				backgrounds[b.ID] = true
			}
			for _, card := range stack.Cards {
				if card.ID <= 0 || cards[card.ID] || !backgrounds[card.BackgroundID] {
					t.Fatalf("invalid card identity/owner: %+v", card)
				}
				cards[card.ID] = true
			}
			for _, id := range stack.CardOrder {
				if !cards[id] || order[id] {
					t.Fatalf("invalid ordered card %d", id)
				}
				order[id] = true
			}
			if len(order) != len(cards) {
				t.Fatal("card inventory and order disagree")
			}
		}
	})
}
