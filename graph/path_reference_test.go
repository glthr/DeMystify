package graph_test

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
	"testing"
)

func TestShortestPathsMatchSmallGraphReference(t *testing.T) {
	rng := rand.New(rand.NewSource(79))
	const size = referenceSize
	for trial := 0; trial < 30; trial++ {
		t.Run(fmt.Sprint(trial), func(t *testing.T) {
			g, costs := buildReferenceGraph(t, rng)
			distances := referenceDistances(costs)

			all := g.ComputeAllShortestPaths()
			for from := 0; from < size; from++ {
				for to := 0; to < size; to++ {
					result, err := g.ComputeShortestPath(int64(from), int64(to), nil)
					if math.IsInf(distances[from][to], 1) {
						if err == nil {
							t.Fatalf("%d -> %d: expected unreachable, got %+v", from, to, result)
						}
						if _, exists := all[int64(from)][int64(to)]; exists {
							t.Fatalf("all-pairs includes unreachable %d -> %d", from, to)
						}
						continue
					}
					if err != nil || result.Distance != distances[from][to] {
						t.Fatalf("%d -> %d: got %+v, %v; want distance %v", from, to, result, err, distances[from][to])
					}

					expected := referenceRoute(costs, from, to, distances[from][to])

					if !slices.Equal(result.Path, expected) {
						t.Fatalf("%d -> %d: got route %v, want %v", from, to, result.Path, expected)
					}
					if from != to {
						if route, exists := all[int64(from)][int64(to)]; !exists || route.Distance != distances[from][to] {
							t.Fatalf("all-pairs mismatch for %d -> %d: %+v", from, to, route)
						}
					}
				}
			}
		})
	}
}
