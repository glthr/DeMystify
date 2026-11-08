package graph

import (
	"github.com/glthr/DeMystify/common"
	"slices"
	"testing"
)

func TestRound2SingleAndAllPairsAccumulateCostsConsistently(t *testing.T) {
	g, nodes := newTestGraph(t, 4)
	for i, w := range []float64{1e16, 1, 1} {
		if err := g.AddEdge(&common.Edge{Source: nodes[i], Target: nodes[i+1]}, w); err != nil {
			t.Fatal(err)
		}
	}
	single, err := g.ComputeShortestPath(0, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	all := g.ComputeAllShortestPaths()[0][3]
	if single.Distance != all.Distance {
		t.Fatalf("single/all-pairs accumulation differs: %g vs %g", single.Distance, all.Distance)
	}
}

func TestShortestPathHandlesRoundingInducedTightCycles(t *testing.T) {
	g, nodes := newTestGraph(t, 4)
	for _, e := range []struct {
		from, to int
		weight   float64
	}{{0, 1, 1e16}, {1, 2, 1}, {2, 1, 1}, {2, 3, 4}, {0, 3, 1e16 + 4}} {
		if err := g.AddEdge(&common.Edge{Source: nodes[e.from], Target: nodes[e.to]}, e.weight); err != nil {
			t.Fatal(err)
		}
	}
	got, err := g.ComputeShortestPath(0, 3, nil)
	if err != nil || got.Distance != 1e16+4 || !slices.Equal(got.Path, []int64{0, 1, 2, 3}) {
		t.Fatalf("rounding cycle path: %+v, %v", got, err)
	}
}

func TestRound2MandatoryNodesDoNotChangeCostOfSameRoute(t *testing.T) {
	g, nodes := newTestGraph(t, 4)
	for i, w := range []float64{1e16, 1, 1} {
		if err := g.AddEdge(&common.Edge{Source: nodes[i], Target: nodes[i+1]}, w); err != nil {
			t.Fatal(err)
		}
	}
	single, err := g.ComputeShortestPath(0, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	mandatory, err := g.ComputeShortestPath(0, 3, []common.Node{*nodes[1]})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(single.Path, mandatory.Path) || g.ComputeAllShortestPaths()[0][3].Distance != mandatory.Distance {
		t.Fatalf("same route changed cost: %+v vs %+v", single, mandatory)
	}
}
