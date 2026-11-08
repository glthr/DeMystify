package graph

import (
	gograph "gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/iterator"
)

// orderedPathGraph makes equal-distance traversal reproducible.
type orderedPathGraph struct{ *MystGraph }

func (g orderedPathGraph) From(id int64) gograph.Nodes {
	ids := g.traverser.getSortedNeighbors(id, true)
	nodes := make([]gograph.Node, len(ids))
	for i, id := range ids {
		nodes[i] = g.Node(id)
	}
	return iterator.NewOrderedNodes(nodes)
}
