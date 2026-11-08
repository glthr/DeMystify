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
