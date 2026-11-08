package graph

import (
	"gonum.org/v1/gonum/graph/path"
	"math"
)

func (pa *pathAnalyzer) shortestPathReachable(to int64, distances path.Shortest, visited map[int64]bool) map[int64]bool {
	reachable := map[int64]bool{to: true}
	queue := []int64{to}
	for i := 0; i < len(queue); i++ {
		current := queue[i]
		incoming := pa.g.To(current)
		for incoming.Next() {
			previous := incoming.Node().ID()
			if visited[previous] || reachable[previous] {
				continue
			}
			w, ok := pa.g.Weight(previous, current)
			if ok && equalDistance(distances.WeightTo(previous)+w, distances.WeightTo(current)) {
				reachable[previous] = true
				queue = append(queue, previous)
			}
		}
	}
	return reachable
}

func equalDistance(a, b float64) bool {
	if math.IsInf(a, 0) || math.IsInf(b, 0) {
		return false
	}
	return a == b
}
