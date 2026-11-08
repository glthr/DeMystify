package graph_test

import (
	"fmt"
	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
	"math"
	"math/rand"
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
