package dot

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
)

func TestRepeatedEdgeRecordsSurviveMergeAndRegeneration(t *testing.T) {
	g, metadata := newTestGenerator(t)
	a, b := metadata.Nodes[0], metadata.Nodes[1]
	forward := &common.Edge{Source: a, Target: b, Attributes: []common.EdgeAttribute{common.IntraAge}}
	reverse := &common.Edge{Source: b, Target: a, Attributes: []common.EdgeAttribute{common.IntraAge}}
	disabled := &common.Edge{Source: a, Target: b, Attributes: []common.EdgeAttribute{common.Disabled}}
	metadata.Edges = []*common.Edge{disabled, forward, reverse, forward, reverse, forward}
	wantOrder := slices.Clone(metadata.Edges)
	for _, path := range [][]int64{nil, {a.GraphID, b.GraphID}, nil} {
		output, err := g.Generate(path)
		if err != nil {
			t.Fatal(err)
		}
		wantLines, wantMerged := 4, 2
		if len(path) > 0 {
			wantLines, wantMerged = 6, 0
			if strings.Count(output, "Path edge") != 1 {
				t.Fatal("path did not highlight exactly one repeated record")
			}
		}
		if len(edgeLines(output)) != wantLines || strings.Count(output, "dir=both") != wantMerged {
			t.Fatalf("path=%v: repeated records lost or merged twice: %v", path, edgeLines(output))
		}
		if !strings.Contains(output, "Disabled edge") || !reflect.DeepEqual(metadata.Edges, wantOrder) {
			t.Fatal("rendering changed metadata order or concealed the disabled record")
		}
	}
}

func TestMetadataEdgesKeepDrawingOrder(t *testing.T) {
	a := &common.Node{Name: "A", Attributes: []common.NodeAttribute{common.IsCard}}
	b := &common.Node{Name: "B", Attributes: []common.NodeAttribute{common.IsCard}}
	c := &common.Node{Name: "C", Attributes: []common.NodeAttribute{common.IsCard}}
	d := &common.Node{Name: "D", Attributes: []common.NodeAttribute{common.IsCard}}
	metadata := &common.Metadata{Nodes: []*common.Node{a, b, c, d}, Edges: []*common.Edge{
		{Source: c, Target: d, Attributes: []common.EdgeAttribute{common.Disabled}},
		{Source: d, Target: c, Attributes: []common.EdgeAttribute{common.IntraAge}},
		{Source: b, Target: c, Attributes: []common.EdgeAttribute{common.IntraAge}},
		{Source: b, Target: a, Attributes: []common.EdgeAttribute{common.IntraAge}},
		{Source: a, Target: a, Attributes: []common.EdgeAttribute{common.SelfReference}},
		{Source: a, Target: b, Attributes: []common.EdgeAttribute{common.IntraAge}},
		{Source: c, Target: d, Attributes: []common.EdgeAttribute{common.IntraAge}},
	}}
	model, err := graph.NewGraph(metadata)
	if err != nil {
		t.Fatal(err)
	}
	output, err := NewGenerator(model, metadata).Generate([]int64{c.GraphID, d.GraphID, 99, c.GraphID})
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, `  "`) && strings.Contains(line, " -> ") {
			lines = append(lines, line)
		}
	}
	// Parallel ordinary edges must precede the highlight; implied transitions
	// follow metadata highlights in the supplied path's order.
	want := []struct{ pair, style string }{
		{`"0" -> "0"`, "constraint=false"},
		{`"0" -> "1"`, "dir=both"},
		{`"1" -> "2"`, "[]"},
		{`"2" -> "3"`, "Disabled edge"},
		{`"3" -> "2"`, "[]"},
		{`"2" -> "3"`, "Path edge"},
		{`"3" -> "99"`, "Path edge (implied)"},
		{`"99" -> "2"`, "Path edge (implied)"},
	}
	if len(lines) != len(want) {
		t.Fatalf("edge records missing or duplicated: %v", lines)
	}
	for i, expected := range want {
		if !strings.Contains(lines[i], expected.pair) || !strings.Contains(lines[i], expected.style) {
			t.Errorf("edge %d: got %s; want %s with %s", i, lines[i], expected.pair, expected.style)
		}
	}
}
