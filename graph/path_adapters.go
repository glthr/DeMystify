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

type reversedPathGraph struct{ g *MystGraph }

func (g reversedPathGraph) From(id int64) gograph.Nodes { return g.g.To(id) }

func (g reversedPathGraph) Edge(from, to int64) gograph.Edge {
	edge := g.g.Edge(to, from)
	if edge == nil {
		return nil
	}
	return edge.ReversedEdge()
}

func (g reversedPathGraph) Weight(from, to int64) (float64, bool) { return g.g.Weight(to, from) }
