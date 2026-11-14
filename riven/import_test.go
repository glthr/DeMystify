package riven

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
)

func words(values ...uint16) []byte {
	out := make([]byte, len(values)*2)
	for i, v := range values {
		binary.BigEndian.PutUint16(out[i*2:], v)
	}
	return out
}
func TestNestedSwitchAndRMapWidth(t *testing.T) {
	b := words(65535, 0, 1, 6, 1, 8, 2, 3, 2, 0, 1, 2, 1, 123, 65535, 1, 27, 3, 0, 0x1234, 0x5678)
	c, e := ParseCard(b)
	if e != nil {
		t.Fatal(e)
	}
	cmd := c.Scripts[0].Commands[0]
	if cmd.Offset != 10 || cmd.Variable != 3 || len(cmd.Branches) != 2 || cmd.Branches[1].Value != 65535 {
		t.Fatalf("switch lost: %+v", cmd)
	}
	target := cmd.Branches[1].Commands[0]
	if target.GlobalCardID != 0x12345678 || target.StackNameID != 0 {
		t.Fatalf("RMAP truncated: %+v", target)
	}
	for n := 0; n < len(b); n++ {
		if _, e := ParseCard(b[:n]); e == nil {
			t.Fatalf("accepted truncated prefix %d", n)
		}
	}
	if _, e := ParseCard(append(b, 0)); e == nil {
		t.Fatal("accepted trailing byte")
	}
}
func TestHotspotSignedGeometryAndAlignment(t *testing.T) {
	b := words(2, 1, 65535, 65534, 3, 608, 392, 0, 4, 5, 65534, 1, 0, 2, 0, 0, 0, 20, 30, 0, 2, 9, 0, 0, 1, 2, 1, 2, 1, 50)
	hs, e := ParseHotspots(b)
	if e != nil {
		t.Fatal(e)
	}
	if len(hs) != 2 || hs[0].NameID != -1 || hs[0].Rect[0] != -2 || hs[0].TransitionOffset != -2 || hs[1].Scripts[0].Commands[0].Args[0] != 50 {
		t.Fatalf("misaligned hotspots: %+v", hs)
	}
}
func TestNameOffsetsNotSortIndex(t *testing.T) {
	b := append(words(2, 6, 0, 1, 0), []byte("first\x00second\x00")...)
	names, e := ParseNames(b)
	if e != nil {
		t.Fatal(e)
	}
	if names[0] != "second" || names[1] != "first" {
		t.Fatalf("wrong NAME indexing %v", names)
	}
	if _, e := ParseNames(words(1, 65535, 0)); e == nil {
		t.Fatal("accepted bad name pointer")
	}
	if _, e := ParseRMap([]byte{0}); e == nil {
		t.Fatal("accepted partial RMAP")
	}
}
func TestFileSliceBounds(t *testing.T) {
	p := filepath.Join(t.TempDir(), "input")
	if e := os.WriteFile(p, []byte("0123456789"), 0600); e != nil {
		t.Fatal(e)
	}
	s := FileSlice{Path: p, Name: "part", Offset: 2, Size: 4}
	b, e := s.Read(0, 4)
	if e != nil || string(b) != "2345" {
		t.Fatalf("slice %s %v", b, e)
	}
	for _, v := range [][2]int64{{-1, 1}, {0, 5}, {5, 0}, {0, 1 << 31}} {
		if _, e = s.Read(v[0], v[1]); e == nil {
			t.Fatalf("accepted bounds %v", v)
		}
	}
}

func fixtureInput(t *testing.T) string {
	t.Helper()
	p := os.Getenv("RIVEN_TEST_DATA")
	if p == "" {
		t.Skip("set RIVEN_TEST_DATA to the original five-disc directory for corpus checks")
	}
	return p
}
func TestSuppliedDiscCorpus(t *testing.T) {
	i, e := LoadInput(fixtureInput(t))
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	md := i.Metadata
	if md.TotalCards != 2307 || len(md.Nodes) != 2312 || len(md.Edges) != 6686 || i.CommandCount != 45491 || i.NavigationCount != 6980 || i.MissingReferences != 8 {
		t.Fatalf("corpus mismatch cards=%d nodes=%d edges=%d commands=%d navigation=%d missing=%d", md.TotalCards, len(md.Nodes), len(md.Edges), i.CommandCount, i.NavigationCount, i.MissingReferences)
	}
	if len(i.Archives) != 28 || len(i.Unresolved) != 3 {
		t.Fatalf("archive/return count %d/%d", len(i.Archives), len(i.Unresolved))
	}
	expected := map[string]int{"aspit": 8, "bspit": 498, "gspit": 267, "jspit": 814, "tspit": 547, "ospit": 89, "pspit": 47, "rspit": 37}
	for s, n := range expected {
		if len(i.Stacks[s].Cards) != n {
			t.Errorf("%s has %d cards", s, len(i.Stacks[s].Cards))
		}
	}
	virtual := map[string]bool{}
	for _, n := range md.Nodes {
		if n.IsOfType(common.IsVirtual) {
			virtual[n.Details.SourceID] = true
		}
	}
	for _, id := range []string{"bspit:466", "jspit:740", "tspit:361", "tspit:477", "tspit:478"} {
		if !virtual[id] {
			t.Errorf("missing virtual %s", id)
		}
	}
	kinds := map[string]int{}
	calls := 0
	for _, edge := range md.Edges {
		kinds[edge.Kind]++
		if edge.Source == nil || edge.Target == nil || len(edge.Evidence) == 0 {
			t.Fatal("edge lost provenance")
		}
		if edge.Kind == "script" {
			calls += len(edge.Evidence)
		}
	}
	for k, n := range map[string]int{"script": 6063, "zip": 606, "native": 10, "native-timer": 3, "native-ending": 4} {
		if kinds[k] != n {
			t.Errorf("%s=%d want %d", k, kinds[k], n)
		}
	}
	if calls != 6980 {
		t.Fatalf("navigation callsite evidence %d", calls)
	}
	g, e := graph.NewGraph(md)
	if e != nil {
		t.Fatal(e)
	}
	if md.Nodes[md.StartNodeID].Details.SourceID != "tspit:155" {
		t.Fatal("lost gameplay entry")
	}
	if g.Metadata.TotalNodes != 2312 {
		t.Fatal("shared graph lost nodes")
	}
}
