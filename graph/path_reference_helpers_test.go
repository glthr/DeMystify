package graph_test

import (
	"fmt"
	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
	"math"
	"math/rand"
	"slices"
	"testing"
)

const referenceSize = 6

type referenceCosts [referenceSize][referenceSize]float64

func buildReferenceGraph(t *testing.T, rng *rand.Rand) (*graph.MystGraph, referenceCosts) {
	t.Helper()
	const size = referenceSize
	metadata := &common.Metadata{}
	var costs referenceCosts
	for i := 0; i < size; i++ {
		metadata.Nodes = append(metadata.Nodes, &common.Node{Name: fmt.Sprint(i)})
		for j := 0; j < size; j++ {
			costs[i][j] = math.Inf(1)
		}
	}
	for i := 0; i < size; i++ {
		for j := 0; j < size; j++ {
			kind := rng.Intn(6)
			if kind > 2 {
				continue
			}
			attribute := []common.EdgeAttribute{common.IntraAge, common.Disabled, common.Backtracking}[kind]
			metadata.Edges = append(metadata.Edges, &common.Edge{
				Source: metadata.Nodes[i], Target: metadata.Nodes[j],
				Attributes: []common.EdgeAttribute{attribute},
			})
			if kind == 0 {
				costs[i][j] = 1
			}
		}
	}
	g, err := graph.NewGraph(metadata)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < size; i++ {
		for j := 0; j < size; j++ {
			if rng.Intn(3) != 0 {
				continue
			}
			weight := []float64{0, 0.25, 2, 1e9}[rng.Intn(4)]
			edge := &common.Edge{Source: metadata.Nodes[i], Target: metadata.Nodes[j], Attributes: []common.EdgeAttribute{common.CrossAge}}
			if err := g.AddEdge(edge, weight); err != nil {
				t.Fatal(err)
			}
			costs[i][j] = math.Min(costs[i][j], weight)
		}
	}
	return g, costs
}

func referenceDistances(costs referenceCosts) referenceCosts {
	const size = referenceSize
	distances := costs
	for i := 0; i < size; i++ {
		distances[i][i] = 0
	}
	for k := 0; k < size; k++ {
		for i := 0; i < size; i++ {
			for j := 0; j < size; j++ {
				distances[i][j] = math.Min(distances[i][j], distances[i][k]+distances[k][j])
			}
		}
	}
	return distances
}

func referenceRoute(costs referenceCosts, from, to int, target float64) []int64 {
	const size = referenceSize
	var expected []int64
	var search func(int, float64, []int64, uint) bool
	search = func(current int, cost float64, route []int64, visited uint) bool {
		if current == to {
			if cost == target {
				expected = slices.Clone(route)
				return true
			}
			return false
		}
		for next := 0; next < size; next++ {
			if visited&(1<<next) == 0 && !math.IsInf(costs[current][next], 1) &&
				search(next, cost+costs[current][next], append(route, int64(next)), visited|1<<next) {
				return true
			}
		}
		return false
	}
	search(from, 0, []int64{int64(from)}, 1<<from)
	return expected
}
