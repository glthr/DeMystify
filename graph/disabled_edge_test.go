package graph

import (
	"github.com/glthr/DeMystify/common"
	"testing"
)

func TestAddingDisabledEdgesWithoutAllocatedLines(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		t.Run(map[bool]string{false: "first edge", true: "alongside active edge"}[parallel], func(t *testing.T) {
			g, nodes := newTestGraph(t, 2)
			wantEdges, wantRecords := uint(0), 1
			if parallel {
				if err := g.AddEdge(&common.Edge{Source: nodes[0], Target: nodes[1]}, 1); err != nil {
					t.Fatal(err)
				}
				wantEdges, wantRecords = 1, 2
			}
			edge := &common.Edge{Source: nodes[0], Target: nodes[1], Attributes: []common.EdgeAttribute{common.Disabled}}
			for range 2 {
				if err := g.AddEdge(edge, 1); err != nil {
					t.Fatal(err)
				}
			}
			if g.HasEdgeFromTo(0, 1) != parallel || g.Metadata.TotalEdges != wantEdges || len(g.Metadata.Edges) != wantRecords || !g.IsDisabledEdge(0, 1) {
				t.Fatalf("disabled insertion changed graph/metadata: physical=%v total=%d records=%d", g.HasEdgeFromTo(0, 1), g.Metadata.TotalEdges, len(g.Metadata.Edges))
			}
			if parallel {
				if path, err := g.ComputeShortestPath(0, 1, nil); err != nil || path.Distance != 1 {
					t.Fatalf("active alternative lost: %+v %v", path, err)
				}
			}
		})
	}
}

// A disabled record from a card to itself is a self-reference like any other.
// It used to be marked only when an active one sat beside it.
func TestEverySelfRecordIsMarkedAndOnlyActiveOnesListTheNode(t *testing.T) {
	g, nodes := newTestGraph(t, 3)
	for _, edge := range []*common.Edge{
		{Source: nodes[0], Target: nodes[0], Attributes: []common.EdgeAttribute{common.Disabled}},
		{Source: nodes[1], Target: nodes[1]},
		{Source: nodes[1], Target: nodes[1], Attributes: []common.EdgeAttribute{common.Disabled}},
		{Source: nodes[1], Target: nodes[2]},
	} {
		if err := g.AddEdge(edge, 1); err != nil {
			t.Fatal(err)
		}
	}
	listed := g.FindNodesWithSelfLoops()
	if len(listed) != 1 || listed[0].Name != "B" {
		t.Fatalf("nodes with an active self-reference = %+v, want only B", listed)
	}
	for _, edge := range g.Metadata.Edges {
		if self := edge.Source == edge.Target; edge.IsOfType(common.SelfReference) != self {
			t.Errorf("%s -> %s (disabled=%v): marked as self-reference = %v", edge.Source.Name, edge.Target.Name, edge.IsOfType(common.Disabled), !self)
		}
	}
	for id, targets := range g.EdgeMap {
		for target, records := range targets {
			for _, record := range records {
				if record.IsOfType(common.SelfReference) != (id == target) {
					t.Errorf("stored record %d -> %d disagrees with the metadata", id, target)
				}
			}
		}
	}
	// A second pass adds nothing.
	g.FindNodesWithSelfLoops()
	for _, edge := range g.Metadata.Edges {
		count := 0
		for _, attribute := range edge.Attributes {
			if attribute == common.SelfReference {
				count++
			}
		}
		if count > 1 {
			t.Fatalf("self-reference marked %d times on one record", count)
		}
	}
}
