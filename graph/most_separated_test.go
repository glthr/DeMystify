package graph_test

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
)

// referenceMostSeparated ranks the materialized all-pairs table: the largest
// finite distance, then the fewest connections at both ends, then node order.
func referenceMostSeparated(t *testing.T, g *graph.MystGraph, size int) common.NodePairInfo {
	t.Helper()
	degree := func(id int64) int {
		count := 0
		for to := g.To(id); to.Next(); {
			count++
		}
		for from := g.From(id); from.Next(); {
			count++
		}
		return count
	}
	all := g.ComputeAllShortestPaths()
	found, score := false, 0
	var want common.NodePairInfo
	for from := range int64(size) {
		for to := range int64(size) {
			route, exists := all[from][to]
			if !exists {
				continue
			}
			current := degree(from) + degree(to)
			if !found || route.Distance > want.Distance || route.Distance == want.Distance && current < score {
				found, score = true, current
				want = common.NodePairInfo{Source: common.NodeInfo{ID: from, Name: fmt.Sprint(from)},
					Target: common.NodeInfo{ID: to, Name: fmt.Sprint(to)}, Distance: route.Distance}
			}
		}
	}
	if !found {
		return common.NodePairInfo{}
	}
	route, err := g.ComputeShortestPath(want.Source.ID, want.Target.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	want.Path = route.Path
	return want
}

func sameSeparation(got, want common.NodePairInfo) bool {
	return got.Source == want.Source && got.Target == want.Target && got.Distance == want.Distance && slices.Equal(got.Path, want.Path)
}

func TestMostSeparatedNodesMatchAllPairsReference(t *testing.T) {
	rng := rand.New(rand.NewSource(124))
	for trial := range 200 {
		g, _ := buildReferenceGraph(t, rng)
		want := referenceMostSeparated(t, g, referenceSize)
		if got := g.FindMostSeparatedNodes(); !sameSeparation(got, want) {
			t.Fatalf("trial %d: got %+v, want %+v", trial, got, want)
		}
	}
}

func TestProcessReportsSeparationWithoutMaterializingAllPairs(t *testing.T) {
	metadata := &common.Metadata{}
	for i := range 5 {
		metadata.Nodes = append(metadata.Nodes, &common.Node{Name: fmt.Sprint(i)})
	}
	for _, pair := range [][2]int{{0, 1}, {1, 2}, {0, 3}, {3, 4}, {3, 2}} {
		metadata.Edges = append(metadata.Edges, &common.Edge{Source: metadata.Nodes[pair[0]], Target: metadata.Nodes[pair[1]],
			Attributes: []common.EdgeAttribute{common.IntraAge}})
	}
	g, err := graph.NewGraph(metadata)
	if err != nil {
		t.Fatal(err)
	}
	g.Process()
	if metadata.Stats.ShortestPaths != nil {
		t.Fatalf("Process materialized %d all-pairs sources", len(metadata.Stats.ShortestPaths))
	}
	// 0 -> 2 and 0 -> 4 are both two steps away. Node 2 has two incoming
	// connections and node 4 only one, so the later, more peripheral pair wins.
	want := referenceMostSeparated(t, g, len(metadata.Nodes))
	if got := metadata.Stats.MostSeparatedNodes; !sameSeparation(got, want) || got.Distance != 2 || !slices.Equal(got.Path, []int64{0, 3, 4}) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if all := g.ComputeAllShortestPaths(); len(all) != 5 || all[0][4].Distance != 2 || all[0][2].Distance != 2 {
		t.Fatalf("on-demand all-pairs table is incomplete: %+v", all)
	}
}
