package graph

import (
	"fmt"
	"github.com/glthr/DeMystify/common"
	"gonum.org/v1/gonum/graph/path"
	"math"
)

func (pa *pathAnalyzer) computeSimpleShortestPath(from, to int64) (*common.ShortestPathInfo, error) {
	if from == to {
		return &common.ShortestPathInfo{From: from, To: to, Path: []int64{from}}, nil
	}

	// Distances to the target identify the edges that can belong to a
	// shortest path. Select the smallest viable next node instead of
	// enumerating every equal-cost route (there can be exponentially many).
	distances := path.DijkstraFrom(pa.g.Node(to), reversedPathGraph{pa.g})
	weight := distances.WeightTo(from)
	info := &common.ShortestPathInfo{From: from, To: to, Distance: weight}
	if math.IsInf(weight, 1) {
		return info, fmt.Errorf("no valid path exists from %d to %d without backtracking", from, to)
	}

	info.Path = []int64{from}
	visited := map[int64]bool{from: true}
	for current := from; current != to; {
		// Zero-weight edges can form cycles. Restrict reachability to nodes
		// outside the prefix, so the greedy choice still yields a simple path.
		reachable := pa.shortestPathReachable(to, distances, visited)
		found := false
		for _, next := range pa.g.traverser.getSortedNeighbors(current, true) {
			w, ok := pa.g.Graph.Weight(current, next)
			if ok && !visited[next] && reachable[next] &&
				equalDistance(w+distances.WeightTo(next), distances.WeightTo(current)) {
				info.Path = append(info.Path, next)
				visited[next] = true
				current = next
				found = true
				break
			}
		}
		if !found {
			return info, fmt.Errorf("no valid shortest path exists from %d to %d", from, to)
		}
	}
	return info, nil
}
