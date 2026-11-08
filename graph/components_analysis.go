package graph

import (
	"slices"
	"sort"

	gograph "gonum.org/v1/gonum/graph"
)

// traverser is a utility for the traversal operations
type traverser struct {
	g *MystGraph
}

func newTraverser(g *MystGraph) *traverser {
	return &traverser{g: g}
}

// getAllNodeIDs returns all node IDs in the graph
func (t *traverser) getAllNodeIDs() []int64 {
	var nodeIDs []int64
	nodes := t.g.Graph.Nodes()
	for nodes.Next() {
		nodeIDs = append(nodeIDs, nodes.Node().ID())
	}

	sort.Slice(nodeIDs, func(i, j int) bool {
		return nodeIDs[i] < nodeIDs[j]
	})

	return nodeIDs
}

// getSortedNeighbors returns a sorted slice of neighbor node IDs
// (outgoing set to `true` gets outgoing neighbors; outgoing set to `false` gets incoming neighbors)
func (t *traverser) getSortedNeighbors(nodeID int64, outgoing bool) []int64 {
	var neighbors []int64
	var iter gograph.Nodes

	if outgoing {
		iter = t.g.From(nodeID)
	} else {
		iter = t.g.To(nodeID)
	}

	for iter.Next() {
		neighbors = append(neighbors, iter.Node().ID())
	}

	sort.Slice(neighbors, func(i, j int) bool {
		return neighbors[i] < neighbors[j]
	})

	return neighbors
}

// formatPathAsNames converts a path of IDs to a path of names
func (t *traverser) formatPathAsNames(path []int64) []string {
	var result []string
	for _, id := range path {
		result = append(result, t.g.GetNameForID(id))
	}
	return result
}

func (g *MystGraph) FormatPathAsNames(path []int64) []string {
	return g.traverser.formatPathAsNames(path)
}

// FindConnectedComponents returns connected components in the graph as ID arrays
// with guaranteed deterministic ordering
func (g *MystGraph) FindConnectedComponents() [][]int64 {
	visited := make(map[int64]bool, g.Graph.Nodes().Len())
	var components [][]int64
	for _, start := range g.traverser.getAllNodeIDs() {
		if visited[start] {
			continue
		}
		component := []int64{start}
		visited[start] = true
		for i := 0; i < len(component); i++ {
			current := component[i]
			neighbors := append(g.traverser.getSortedNeighbors(current, true),
				g.traverser.getSortedNeighbors(current, false)...)
			for _, next := range neighbors {
				if !visited[next] {
					visited[next] = true
					component = append(component, next)
				}
			}
		}
		slices.Sort(component)
		components = append(components, component)
	}
	sort.Slice(components, func(i, j int) bool {
		if len(components[i]) != len(components[j]) {
			return len(components[i]) > len(components[j])
		}
		return slices.Compare(components[i], components[j]) < 0
	})
	return components
}
