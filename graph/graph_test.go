package graph

import (
	"github.com/glthr/DeMystify/common"
	"math"
	"reflect"
	"testing"
)

func newTestGraph(t *testing.T, count int) (*MystGraph, []*common.Node) {
	t.Helper()
	nodes := make([]*common.Node, count)
	for i := range nodes {
		nodes[i] = &common.Node{Name: string(rune('A' + i)), StackName: "Test Age"}
	}
	g, err := NewGraph(&common.Metadata{Nodes: nodes})
	if err != nil {
		t.Fatal(err)
	}
	return g, nodes
}

func TestShortestPathDoesNotEnumerateEqualRoutes(t *testing.T) {
	const layers = 30 // More than a billion equally short routes.
	g, nodes := newTestGraph(t, 2*layers+2)
	add := func(from, to int, weight float64) {
		t.Helper()
		if err := g.AddEdge(&common.Edge{Source: nodes[from], Target: nodes[to]}, weight); err != nil {
			t.Fatal(err)
		}
	}
	add(0, 1, 1)
	add(0, 2, 1)
	want := []int64{0}
	for layer := 0; layer < layers; layer++ {
		a, b := 1+2*layer, 2+2*layer
		want = append(want, int64(a))
		if layer+1 == layers {
			add(a, len(nodes)-1, 1)
			add(b, len(nodes)-1, 1)
		} else {
			add(a, a+2, 1)
			add(a, b+2, 1)
			add(b, a+2, 1)
			add(b, b+2, 1)
		}
	}
	want = append(want, int64(len(nodes)-1))
	got, err := g.ComputeShortestPath(0, int64(len(nodes)-1), nil)
	if err != nil || got.Distance != layers+1 || !reflect.DeepEqual(got.Path, want) {
		t.Fatalf("layered shortest path = %+v, %v; want %v", got, err, want)
	}
}

func TestGraphRejectsInvalidInput(t *testing.T) {
	for _, metadata := range []*common.Metadata{
		{Nodes: []*common.Node{nil}},
		{Edges: []*common.Edge{nil}},
		{Nodes: []*common.Node{{Name: "A"}}, Edges: []*common.Edge{{Source: &common.Node{Name: "A"}}}},
		{Nodes: []*common.Node{{Name: "A"}}, Edges: []*common.Edge{{Source: &common.Node{Name: "A"}, Target: &common.Node{Name: "unknown"}}}},
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("invalid metadata panicked instead of returning an error: %v", r)
				}
			}()
			if _, err := NewGraph(metadata); err == nil {
				t.Error("invalid metadata accepted")
			}
		}()
	}
	g, nodes := newTestGraph(t, 2)
	for _, weight := range []float64{-1, math.NaN(), math.Inf(-1)} {
		if err := g.AddEdge(&common.Edge{Source: nodes[0], Target: nodes[1]}, weight); err == nil {
			t.Errorf("invalid weight %v accepted", weight)
		}
	}
}

func TestReaddingEdgeUpdatesItsWeight(t *testing.T) {
	g, nodes := newTestGraph(t, 2)
	edge := &common.Edge{Source: nodes[0], Target: nodes[1]}
	for _, weight := range []float64{8, 3, 7} {
		if err := g.AddEdge(edge, weight); err != nil {
			t.Fatal(err)
		}
		got, err := g.ComputeShortestPath(0, 1, nil)
		if err != nil || got.Distance != weight {
			t.Fatalf("readded edge: %+v, %v; want updated weight %v", got, err, weight)
		}
		if got := g.Graph.WeightedLines(0, 1).Len(); got != 1 {
			t.Fatalf("duplicate edge created %d parallel lines", got)
		}
	}
}

func TestWeightedShortestPath(t *testing.T) {
	g, nodes := newTestGraph(t, 3)
	for _, e := range []struct {
		from, to int
		weight   float64
	}{{0, 2, 9}, {0, 1, 2}, {1, 2, 3}} {
		if err := g.AddEdge(&common.Edge{Source: nodes[e.from], Target: nodes[e.to]}, e.weight); err != nil {
			t.Fatal(err)
		}
	}
	got, err := g.ComputeShortestPath(0, 2, nil)
	if err != nil || got.Distance != 5 || !reflect.DeepEqual(got.Path, []int64{0, 1, 2}) {
		t.Fatalf("weighted path = %+v, %v; want distance 5 through B", got, err)
	}
}

func TestBacktrackingOnlyPathIsUnreachable(t *testing.T) {
	g, nodes := newTestGraph(t, 2)
	if err := g.addEdgeWithAttributes(&common.Edge{Source: nodes[0], Target: nodes[1], Attributes: []common.EdgeAttribute{common.Backtracking}}); err != nil {
		t.Fatal(err)
	}
	if got, err := g.ComputeShortestPath(0, 1, nil); err == nil {
		t.Fatalf("backtracking-only route accepted: %+v", got)
	}
	if _, ok := g.ComputeAllShortestPaths()[0][1]; ok {
		t.Fatal("backtracking-only route included in all shortest paths")
	}
}

func TestShortestPathsExcludeBacktracking(t *testing.T) {
	g, nodes := newTestGraph(t, 3)
	for _, e := range []*common.Edge{
		{Source: nodes[0], Target: nodes[2], Attributes: []common.EdgeAttribute{common.Backtracking}},
		{Source: nodes[0], Target: nodes[1]},
		{Source: nodes[1], Target: nodes[2]},
	} {
		if err := g.addEdgeWithAttributes(e); err != nil {
			t.Fatal(err)
		}
	}
	got, err := g.ComputeShortestPath(0, 2, nil)
	if err != nil || got.Distance != 2 || !reflect.DeepEqual(got.Path, []int64{0, 1, 2}) {
		t.Fatalf("shortest path = %+v, %v; want A -> B -> C, distance 2", got, err)
	}
	all := g.ComputeAllShortestPaths()
	if path, ok := all[0][2]; !ok || path.Distance != 2 {
		t.Fatalf("all shortest paths lost the valid alternative route: %+v", all)
	}
	separated := g.FindMostSeparatedNodes()
	if separated.Distance != 2 || !reflect.DeepEqual(separated.Path, []int64{0, 1, 2}) {
		t.Fatalf("most separated = %+v; want A -> B -> C, distance 2", separated)
	}
}

func TestParallelEdgesDoNotHideUsableRoutes(t *testing.T) {
	for _, attrs := range []common.EdgeAttribute{common.Backtracking, common.Disabled} {
		for _, reverse := range []bool{false, true} {
			g, nodes := newTestGraph(t, 3)
			edges := []*common.Edge{
				{Source: nodes[0], Target: nodes[1]},
				{Source: nodes[0], Target: nodes[1], Attributes: []common.EdgeAttribute{attrs}},
			}
			if reverse {
				edges[0], edges[1] = edges[1], edges[0]
			}
			edges = append(edges, &common.Edge{Source: nodes[1], Target: nodes[2]})
			for _, e := range edges {
				if err := g.addEdgeWithAttributes(e); err != nil {
					t.Fatal(err)
				}
			}
			got, err := g.ComputeShortestPath(0, 2, nil)
			if err != nil || got.Distance != 2 {
				t.Fatalf("attribute %v, reverse %v: got %+v, %v; want distance 2", attrs, reverse, got, err)
			}
			if _, ok := g.ComputeAllShortestPaths()[0][2]; !ok {
				t.Fatalf("attribute %v, reverse %v: valid parallel route excluded", attrs, reverse)
			}
		}
	}
}
