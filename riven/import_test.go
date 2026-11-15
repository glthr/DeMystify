package riven

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestSuppliedDiscCorpus(t *testing.T) {
	input, profile := fixtureCorpus(t)
	i, e := LoadInput(input)
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	md := i.Metadata
	if md.TotalCards != 2307 || len(md.Nodes) != 2312 || len(md.Edges) != 6722 || i.CommandCount != profile.commands || i.NavigationCount != 6980 || i.MissingReferences != 8 {
		t.Fatalf("corpus mismatch cards=%d nodes=%d edges=%d commands=%d navigation=%d missing=%d", md.TotalCards, len(md.Nodes), len(md.Edges), i.CommandCount, i.NavigationCount, i.MissingReferences)
	}
	if len(i.Archives) != profile.archives || len(i.Unresolved) != 3 {
		t.Fatalf("archive/return count %d/%d", len(i.Archives), len(i.Unresolved))
	}
	for _, script := range []struct {
		stack, tag        string
		id                uint16
		original, updated string
	}{
		{"bspit", "HSPT", 284, "B_DATA.MHK", "B_DATA1.MHK"},
		{"jspit", "CARD", 255, "J_DATA1.MHK", "J_DATA3.MHK"},
		{"jspit", "CARD", 538, "J_DATA2.MHK", "J_DATA3.MHK"},
		{"jspit", "HSPT", 255, "J_DATA1.MHK", "J_DATA3.MHK"},
	} {
		want := script.original
		if profile.edition == "1.02" {
			want = script.updated
		}
		ref := i.Stacks[script.stack].Resources[script.tag][script.id]
		if got := strings.ToUpper(filepath.Base(ref.Archive.Source.Name)); got != want {
			t.Errorf("edition %s: %s %s:%d selected %s, want %s", profile.edition, script.stack, script.tag, script.id, got, want)
		}
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
	baseKinds := map[string]int{}
	baseEndpoints := map[string]bool{}
	evidenceCounts := map[string]int{}
	disabled, conditional := 0, 0
	calls := 0
	for _, edge := range md.Edges {
		kinds[edge.Kind]++
		if edge.Source == nil || edge.Target == nil || len(edge.Evidence) == 0 {
			t.Fatal("edge lost provenance")
		}
		if edge.Kind == "script" {
			calls += len(edge.Evidence)
		}
		evidenceCounts[edge.Kind] += len(edge.Evidence)
		if edge.IsOfType(common.Disabled) {
			disabled++
		}
		if edge.IsOfType(common.Conditional) {
			conditional++
		}
		if edge.Kind == "sequence-continuation" {
			if !edge.IsOfType(common.SequenceContinuation) {
				t.Fatal("derived continuation lost its path exclusion")
			}
			continue
		}
		if edge.IsOfType(common.SequenceContinuation) {
			t.Fatalf("literal %s record was replaced by a continuation", edge.Kind)
		}
		key := edge.Source.Details.SourceID + ">" + edge.Target.Details.SourceID + ":" + edge.Kind
		if !baseEndpoints[key] {
			baseEndpoints[key] = true
			baseKinds[edge.Kind]++
		}
	}
	if want := (map[string]int{"script": 6077, "zip": 606, "native": 10, "native-timer": 3, "native-ending": 4, "sequence-continuation": 22}); !reflect.DeepEqual(kinds, want) {
		t.Errorf("classified records=%v want %v", kinds, want)
	}
	// Classification may split parallel records; it must preserve the original
	// source/target/kind union and every literal instruction's provenance.
	if want := (map[string]int{"script": 6063, "zip": 606, "native": 10, "native-timer": 3, "native-ending": 4}); len(baseEndpoints) != 6686 || !reflect.DeepEqual(baseKinds, want) {
		t.Errorf("original endpoint union=%d kinds=%v want 6686/%v", len(baseEndpoints), baseKinds, want)
	}
	if want := (map[string]int{"script": 6980, "zip": 606, "native": 13, "native-timer": 3, "native-ending": 7, "sequence-continuation": 22}); !reflect.DeepEqual(evidenceCounts, want) {
		t.Errorf("callsite evidence=%v want %v", evidenceCounts, want)
	}
	if disabled != 0 || conditional != 835 {
		t.Errorf("classification changed: disabled=%d conditional=%d", disabled, conditional)
	}
	if calls != 6980 {
		t.Fatalf("navigation callsite evidence %d", calls)
	}
	states, ok := i.Manifest["hotspotAvailability"].(map[string]map[int]HotspotState)
	if !ok {
		t.Fatal("hotspot availability audit missing")
	}
	statuses := map[string]int{}
	for _, hotspots := range states {
		for _, state := range hotspots {
			statuses[state.Status]++
			if state.Reason == "" {
				t.Fatal("availability classification lacks its proof limit")
			}
		}
	}
	if want := (map[string]int{"enabled": 6456, "conditional": 897, "unknown": 1145}); !reflect.DeepEqual(statuses, want) {
		t.Errorf("hotspot availability=%v want %v", statuses, want)
	}
	for _, node := range md.Nodes {
		for _, zone := range node.Details.Zones {
			if zone.AvailabilityReason == "" || (zone.Availability != "enabled" && zone.Availability != "disabled" && zone.Availability != "conditional" && zone.Availability != "unknown") {
				t.Fatalf("%s zone %d lost explicit availability: %+v", node.Details.SourceID, zone.ID, zone)
			}
			if zone.Enabled != (zone.Availability != "disabled") {
				t.Fatalf("%s zone %d compatibility flag contradicts availability", node.Details.SourceID, zone.ID)
			}
		}
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
