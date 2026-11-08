package graph

import (
	"github.com/glthr/DeMystify/common"
	"testing"
)

func TestRepeatedAttributesDoNotMatchDifferentFlags(t *testing.T) {
	nodes := []*common.Node{{Name: "A"}, {Name: "B"}}
	disabled := &common.Edge{Source: nodes[0], Target: nodes[1], Attributes: []common.EdgeAttribute{common.CrossAge, common.Disabled}}
	g, err := NewGraph(&common.Metadata{Nodes: nodes, Edges: []*common.Edge{disabled}})
	if err != nil {
		t.Fatal(err)
	}
	enabled := &common.Edge{Source: nodes[0], Target: nodes[1], Attributes: []common.EdgeAttribute{common.CrossAge, common.CrossAge}}
	if err := g.AddEdge(enabled, 1); err != nil {
		t.Fatal(err)
	}
	edges, ok := g.GetAllEdges(0, 1)
	if !ok || len(edges) != 2 || len(g.Metadata.Edges) != 2 {
		t.Fatalf("different flag lists falsely merged: edges=%+v metadata=%+v", edges, g.Metadata.Edges)
	}
	if !edges[0].IsOfType(common.Disabled) || edges[1].IsOfType(common.Disabled) {
		t.Fatal("disabled/enabled attributes were not preserved")
	}

	reordered := &common.Edge{Source: nodes[0], Target: nodes[1], Attributes: []common.EdgeAttribute{common.Disabled, common.CrossAge}}
	if err := g.AddEdge(reordered, 1); err != nil {
		t.Fatal(err)
	}
	if len(g.Metadata.Edges) != 2 {
		t.Fatal("reordering the same flags created another edge")
	}
	lines := g.Graph.WeightedLines(0, 1)
	if lines.Len() != 1 || !lines.Next() || lines.WeightedLine().ID() != 1 {
		t.Fatal("disabled line identity was reused for the enabled edge")
	}
	if got, err := g.ComputeShortestPath(0, 1, nil); err != nil || got.Distance != 1 {
		t.Fatalf("enabled alternative missing: %+v %v", got, err)
	}
}
