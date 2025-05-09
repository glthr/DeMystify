package graph

import (
	"github.com/glthr/DeMystify/common"
)

// nodesAnalyzer is a utility for analyzing the node properties
type nodesAnalyzer struct {
	g *MystGraph
}

func newNodeAnalyzer(g *MystGraph) *nodesAnalyzer {
	return &nodesAnalyzer{g: g}
}

// findNodeWithMostIncomingEdges identifies the node with the highest number of incoming edges
func (na *nodesAnalyzer) findNodeWithMostIncomingEdges() common.NodeDegreeInfo {
	var maxNode common.NodeDegreeInfo
	maxInDegree := -1

	nodes := na.g.Graph.Nodes()
	for nodes.Next() {
		node := nodes.Node()
		id := node.ID()

		inDegree := 0
		to := na.g.To(id)
		for to.Next() {
			inDegree++
		}

		if inDegree > maxInDegree {
			maxInDegree = inDegree
			maxNode = common.NodeDegreeInfo{
				ID:     id,
				Name:   na.g.GetNameForID(id),
				Degree: inDegree,
			}
		}
	}

	return maxNode
}

// findNodeWithMostOutgoingEdges identifies the node with the highest number of outgoing edges
func (na *nodesAnalyzer) findNodeWithMostOutgoingEdges() common.NodeDegreeInfo {
	var maxNode common.NodeDegreeInfo
	maxOutDegree := -1

	nodes := na.g.Graph.Nodes()
	for nodes.Next() {
		node := nodes.Node()
		id := node.ID()

		outDegree := 0
		from := na.g.From(id)
		for from.Next() {
			outDegree++
		}

		if outDegree > maxOutDegree {
			maxOutDegree = outDegree
			maxNode = common.NodeDegreeInfo{
				ID:     id,
				Name:   na.g.GetNameForID(id),
				Degree: outDegree,
			}
		}
	}

	return maxNode
}

// findNodesWithNoIncomingEdges identifies all source nodes
func (na *nodesAnalyzer) findNodesWithNoIncomingEdges() []common.NodeInfo {
	var result []common.NodeInfo

	nodes := na.g.Graph.Nodes()
	for nodes.Next() {
		node := nodes.Node()
		id := node.ID()

		to := na.g.To(id)
		if !to.Next() {
			// no incoming edges in the graph structure, but we need to check
			// if there are any backtracking edges
			hasBacktrackingEdge := false

			// check all edges where this node is the target
			for _, edge := range na.g.Metadata.Edges {
				if edge.Target.GraphID == id && edge.IsOfType(common.Backtracking) {
					hasBacktrackingEdge = true
					break
				}
			}

			// only consider as source if there are no incoming edges and no backtracking edges
			if !hasBacktrackingEdge {
				na.g.addNodeAttribute(id, common.IsSource)
				result = append(result, common.NodeInfo{
					ID:   id,
					Name: na.g.GetNameForID(id),
				})
			}
		}
	}

	return result
}

// findNodesWithNoOutgoingEdges identifies all sink nodes
func (na *nodesAnalyzer) findNodesWithNoOutgoingEdges() []common.NodeInfo {
	var result []common.NodeInfo

	nodes := na.g.Graph.Nodes()
	for nodes.Next() {
		node := nodes.Node()
		id := node.ID()

		from := na.g.From(id)
		if !from.Next() {
			// no outgoing edges in the graph structure, but we need to check
			// if there are any backtracking edges in the metadata

			hasBacktrackingEdge := false

			// check all edges in metadata where this node is the origin
			for _, edge := range na.g.Metadata.Edges {
				if edge.Target.GraphID == id && edge.IsOfType(common.Backtracking) {
					hasBacktrackingEdge = true
					break
				}
			}

			// only consider as sink if there are no outgoing edges and no backtracking edges
			if !hasBacktrackingEdge {
				na.g.addNodeAttribute(id, common.IsSink)
				result = append(result, common.NodeInfo{
					ID:   id,
					Name: na.g.GetNameForID(id),
				})
			}
		}
	}

	return result
}

// findIsolatedNodes identifies nodes that have no incoming or outgoing edges
func (na *nodesAnalyzer) findIsolatedNodes() []common.NodeInfo {
	var result []common.NodeInfo

	nodes := na.g.Graph.Nodes()
	for nodes.Next() {
		node := nodes.Node()
		id := node.ID()

		to := na.g.To(id)
		from := na.g.From(id)
		if !to.Next() && !from.Next() {
			hasBacktrackingEdge := false
			for _, edge := range na.g.Metadata.Edges {
				if (edge.Source.GraphID == id || edge.Target.GraphID == id) &&
					edge.IsOfType(common.Backtracking) {
					hasBacktrackingEdge = true
					break
				}
			}

			// only consider as isolated if there are no backtracking edges
			if !hasBacktrackingEdge {
				na.g.addNodeAttribute(id, common.IsIsolated)
				result = append(result, common.NodeInfo{
					ID:   id,
					Name: na.g.GetNameForID(id),
				})
			}
		}
	}

	return result
}

// findNodesWithSelfLoops identifies all nodes that have edges to themselves
func (na *nodesAnalyzer) findNodesWithSelfLoops() []common.NodeInfo {
	var result []common.NodeInfo

	nodes := na.g.Graph.Nodes()
	for nodes.Next() {
		node := nodes.Node()
		id := node.ID()

		// check if the node has an edge to itself
		if na.g.HasEdgeFromTo(id, id) {
			na.g.AddEdgeAttribute(id, id, common.SelfReference)
			result = append(result, common.NodeInfo{
				ID:   id,
				Name: na.g.GetNameForID(id),
			})
		}
	}

	return result
}

func (g *MystGraph) FindNodeWithMostIncomingEdges() common.NodeDegreeInfo {
	result := g.nodeAnalyzer.findNodeWithMostIncomingEdges()

	return result
}

func (g *MystGraph) FindNodeWithMostOutgoingEdges() common.NodeDegreeInfo {
	result := g.nodeAnalyzer.findNodeWithMostOutgoingEdges()

	return result
}

func (g *MystGraph) FindNodesWithNoIncomingEdges() []common.NodeInfo {
	return g.nodeAnalyzer.findNodesWithNoIncomingEdges()
}

func (g *MystGraph) FindNodesWithNoOutgoingEdges() []common.NodeInfo {
	return g.nodeAnalyzer.findNodesWithNoOutgoingEdges()
}

func (g *MystGraph) FindIsolatedNodes() []common.NodeInfo {
	return g.nodeAnalyzer.findIsolatedNodes()
}

func (g *MystGraph) FindNodesWithSelfLoops() []common.NodeInfo {
	return g.nodeAnalyzer.findNodesWithSelfLoops()
}
