package graph

import (
	"github.com/glthr/DeMystify/common"
	"math"
	"testing"
)

func TestOptionalNavigationLayersDoNotBecomeOrdinaryPaths(t *testing.T) {
	for _, attribute := range []common.EdgeAttribute{common.Zip, common.EndingReset} {
		g, nodes := newTestGraph(t, 3)
		first := &common.Edge{Source: nodes[0], Target: nodes[1], Attributes: []common.EdgeAttribute{common.Native}, Kind: "native", Evidence: []map[string]any{{"offset": 12}}}
		second := &common.Edge{Source: nodes[1], Target: nodes[2], Attributes: []common.EdgeAttribute{attribute}}
		if err := g.addEdgeWithAttributes(first); err != nil {
			t.Fatal(err)
		}
		if err := g.addEdgeWithAttributes(second); err != nil {
			t.Fatal(err)
		}
		if got, err := g.ComputeShortestPath(0, 1, nil); err != nil || got.Distance != 1 {
			t.Fatalf("native movement was excluded: %+v, %v", got, err)
		}
		if _, err := g.ComputeShortestPath(0, 2, nil); err == nil {
			t.Fatal("optional shortcut/reset became an ordinary route")
		}
		if weight, ok := g.Weight(1, 2); !ok || !math.IsInf(weight, 1) {
			t.Fatalf("optional edge is not retained at infinite path cost: %v,%v", weight, ok)
		}
	}
}

func TestTypedNativeRecordsRetainSeparateEvidence(t *testing.T) {
	g, nodes := newTestGraph(t, 2)
	for _, kind := range []string{"native", "native-timer"} {
		edge := &common.Edge{Source: nodes[0], Target: nodes[1], Kind: kind, Attributes: []common.EdgeAttribute{common.Native}, Evidence: []map[string]any{{"kind": kind}}}
		if err := g.addEdgeWithAttributes(edge); err != nil {
			t.Fatal(err)
		}
	}
	edges, ok := g.GetAllEdges(0, 1)
	if !ok || len(edges) != 2 {
		t.Fatalf("typed transitions collapsed: %+v", edges)
	}
	for _, edge := range edges {
		if len(edge.Evidence) != 1 || edge.Evidence[0]["kind"] != edge.Kind {
			t.Fatalf("lost callsite evidence: %+v", edge)
		}
	}
}
