package graph

import (
	"sort"

	"github.com/glthr/DeMystify/common"

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

// bfsComponent performs BFS from a start node
// NOTE: uses slices instead of maps for deterministic behavior
func (t *traverser) bfsComponent(startID int64, nodeIDs []int64) []common.NodeInfo {
	idMap := make(map[int64]int)
	for i, id := range nodeIDs {
		idMap[id] = i
	}

	maxID := 0
	for _, id := range nodeIDs {
		if int(id) > maxID {
			maxID = int(id)
		}
	}
	visited := make([]bool, maxID+1)

	var component []common.NodeInfo
	queue := []int64{startID}
	visited[startID] = true
	component = append(component, common.NodeInfo{
		ID:   startID,
		Name: t.g.GetNameForID(startID),
	})

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// get sorted outgoing neighbors
		outNeighbors := t.getSortedNeighbors(current, true)

		// process outgoing neighbors in sorted order
		for _, neighborID := range outNeighbors {
			if neighborID < int64(len(visited)) && !visited[neighborID] {
				visited[neighborID] = true
				component = append(component, common.NodeInfo{
					ID:   neighborID,
					Name: t.g.GetNameForID(neighborID),
				})
				queue = append(queue, neighborID)
			}
		}

		// get sorted incoming neighbors
		inNeighbors := t.getSortedNeighbors(current, false)

		// process incoming neighbors in sorted order
		for _, neighborID := range inNeighbors {
			if neighborID < int64(len(visited)) && !visited[neighborID] {
				visited[neighborID] = true
				component = append(component, common.NodeInfo{
					ID:   neighborID,
					Name: t.g.GetNameForID(neighborID),
				})
				queue = append(queue, neighborID)
			}
		}
	}

	// ensure deterministic ordering by sorting the final component
	sort.Slice(component, func(i, j int) bool {
		return component[i].ID < component[j].ID
	})

	return component
}

// findComponents identifies all connected components in the graph
func (t *traverser) findComponents() [][]common.NodeInfo {
	nodeIDs := t.getAllNodeIDs()

	maxID := int64(0)
	for _, id := range nodeIDs {
		if id > maxID {
			maxID = id
		}
	}
	visited := make([]bool, maxID+1)

	var components [][]common.NodeInfo

	for _, id := range nodeIDs {
		if id < int64(len(visited)) && !visited[id] {
			component := t.bfsComponent(id, nodeIDs)

			for _, node := range component {
				if node.ID < int64(len(visited)) {
					visited[node.ID] = true
				}
			}

			components = append(components, component)
		}
	}

	// sort the components deterministically:
	// 1. by size (largest first)
	// 2. for equal sizes, by the smallest node ID in each component
	sort.Slice(components, func(i, j int) bool {
		if len(components[i]) != len(components[j]) {
			return len(components[i]) > len(components[j])
		}

		minLen := len(components[i])
		if len(components[j]) < minLen {
			minLen = len(components[j])
		}

		for k := 0; k < minLen; k++ {
			if components[i][k].ID != components[j][k].ID {
				return components[i][k].ID < components[j][k].ID
			}
		}

		// if all nodes are the same up to the minimum length,
		// the shortest component comes first
		return len(components[i]) < len(components[j])
	})

	return components
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
	allNodeIDs := make([]int64, 0, g.Graph.Nodes().Len())
	nodes := g.Graph.Nodes()
	for nodes.Next() {
		allNodeIDs = append(allNodeIDs, nodes.Node().ID())
	}

	sort.Slice(allNodeIDs, func(i, j int) bool {
		return allNodeIDs[i] < allNodeIDs[j]
	})

	maxID := int64(0)
	for _, id := range allNodeIDs {
		if id > maxID {
			maxID = id
		}
	}

	visited := make([]bool, maxID+1)

	var components [][]int64

	// process nodes in deterministic order
	for _, startID := range allNodeIDs {
		if int(startID) >= len(visited) || visited[startID] {
			continue // skip if already visited or out of bounds
		}

		// start a new component with BFS from this node
		var component []int64
		queue := []int64{startID}
		visited[startID] = true
		component = append(component, startID)

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]

			outNeighbors := make([]int64, 0)
			from := g.From(current)
			for from.Next() {
				outNeighbors = append(outNeighbors, from.Node().ID())
			}

			// sort neighbors for deterministic processing
			sort.Slice(outNeighbors, func(i, j int) bool {
				return outNeighbors[i] < outNeighbors[j]
			})

			for _, neighbor := range outNeighbors {
				if int(neighbor) < len(visited) && !visited[neighbor] {
					visited[neighbor] = true
					component = append(component, neighbor)
					queue = append(queue, neighbor)
				}
			}

			inNeighbors := make([]int64, 0)
			to := g.To(current)
			for to.Next() {
				inNeighbors = append(inNeighbors, to.Node().ID())
			}

			// sort neighbors for deterministic processing
			sort.Slice(inNeighbors, func(i, j int) bool {
				return inNeighbors[i] < inNeighbors[j]
			})

			for _, neighbor := range inNeighbors {
				if int(neighbor) < len(visited) && !visited[neighbor] {
					visited[neighbor] = true
					component = append(component, neighbor)
					queue = append(queue, neighbor)
				}
			}
		}

		// sort component nodes for deterministic order
		sort.Slice(component, func(i, j int) bool {
			return component[i] < component[j]
		})

		components = append(components, component)
	}

	// sort components deterministically
	sort.Slice(components, func(i, j int) bool {
		// first by size (largest first)
		if len(components[i]) != len(components[j]) {
			return len(components[i]) > len(components[j])
		}

		// for components of equal size, compare individual node IDs
		minLen := len(components[i])
		if len(components[j]) < minLen {
			minLen = len(components[j])
		}

		for k := 0; k < minLen; k++ {
			if components[i][k] != components[j][k] {
				return components[i][k] < components[j][k]
			}
		}

		// if all common elements are equal, the shorter component comes first
		return len(components[i]) < len(components[j])
	})

	return components
}
