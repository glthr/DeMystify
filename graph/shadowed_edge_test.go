package graph

import (
	"testing"

	"github.com/glthr/DeMystify/common"
)

// A shadowed record is kept for display like a disabled one: it is no line of
// the graph, so no path follows it and it gives its target no incoming edge.
func TestShadowedEdgesAreRecordedButNotTraversed(t *testing.T) {
	nodes := []*common.Node{{Name: "A"}, {Name: "B"}, {Name: "C"}}
	metadata := &common.Metadata{Nodes: nodes, Edges: []*common.Edge{
		{Source: nodes[0], Target: nodes[1], Attributes: []common.EdgeAttribute{common.IntraAge, common.Shadowed}},
		{Source: nodes[0], Target: nodes[2], Attributes: []common.EdgeAttribute{common.IntraAge}},
	}}
	g, err := NewGraph(metadata)
	if err != nil {
		t.Fatal(err)
	}
	g.Process()
	if metadata.TotalEdges != 1 || len(metadata.Edges) != 2 || g.HasEdgeFromTo(0, 1) || !g.HasEdge("A", "B") {
		t.Fatalf("shadowed record: active pairs=%d records=%d line=%v kept=%v", metadata.TotalEdges, len(metadata.Edges), g.HasEdgeFromTo(0, 1), g.HasEdge("A", "B"))
	}
	if _, err := g.ComputeShortestPath(0, 1, nil); err == nil {
		t.Error("a path followed a shadowed record")
	}
	if !nodes[1].IsOfType(common.IsSource) || !nodes[1].IsOfType(common.IsIsolated) {
		t.Errorf("the target of a shadowed record alone must stay unreachable: %v", nodes[1].Attributes)
	}
	// Marking an active record afterwards removes its line as well.
	if !g.AddEdgeAttribute(0, 2, common.Shadowed) || g.HasEdgeFromTo(0, 2) || metadata.TotalEdges != 0 {
		t.Errorf("an edge marked shadowed stayed in the graph: pairs=%d", metadata.TotalEdges)
	}
}
