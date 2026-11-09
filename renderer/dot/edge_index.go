package dot

import (
	"sort"

	"github.com/glthr/DeMystify/common"
)

type nodePair struct{ from, to int64 }

// Each metadata record has its own flag, including repeated edge pointers.
type pendingEdge struct {
	edge *common.Edge
	used bool
}

// indexEdges creates render-local groups in source/target order. Keeping the
// consumption flag beside each record avoids synchronizing parallel maps.
func indexEdges(edges []*common.Edge) ([]nodePair, map[nodePair][]*pendingEdge) {
	groups := make(map[nodePair][]*pendingEdge)
	var pairs []nodePair
	for _, edge := range edges {
		pair := nodePair{edge.Source.GraphID, edge.Target.GraphID}
		if _, exists := groups[pair]; !exists {
			pairs = append(pairs, pair)
		}
		groups[pair] = append(groups[pair], &pendingEdge{edge: edge})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].from != pairs[j].from {
			return pairs[i].from < pairs[j].from
		}
		return pairs[i].to < pairs[j].to
	})
	for _, records := range groups {
		sort.SliceStable(records, func(i, j int) bool {
			a, b := records[i].edge, records[j].edge
			// Prefer enabled, ordinary edges when choosing a path highlight.
			if a.IsOfType(common.Disabled) != b.IsOfType(common.Disabled) {
				return !a.IsOfType(common.Disabled)
			}
			if a.IsOfType(common.Backtracking) != b.IsOfType(common.Backtracking) {
				return !a.IsOfType(common.Backtracking)
			}
			return a.TransitivityID < b.TransitivityID
		})
	}
	return pairs, groups
}
