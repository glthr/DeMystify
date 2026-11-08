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

	for _, id := range na.g.traverser.getAllNodeIDs() {
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

	for _, id := range na.g.traverser.getAllNodeIDs() {
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
	na.g.clearNodeAttribute(common.IsSource)
	var result []common.NodeInfo

	for _, id := range na.g.traverser.getAllNodeIDs() {
		to := na.g.To(id)
		if !to.Next() {
			na.g.addNodeAttribute(id, common.IsSource)
			result = append(result, common.NodeInfo{
				ID: id, Name: na.g.GetNameForID(id),
			})
		}
	}

	return result
}

// findNodesWithNoOutgoingEdges identifies all sink nodes
func (na *nodesAnalyzer) findNodesWithNoOutgoingEdges() []common.NodeInfo {
	na.g.clearNodeAttribute(common.IsSink)
	var result []common.NodeInfo

	for _, id := range na.g.traverser.getAllNodeIDs() {
		from := na.g.From(id)
		if !from.Next() {
			na.g.addNodeAttribute(id, common.IsSink)
			result = append(result, common.NodeInfo{
				ID: id, Name: na.g.GetNameForID(id),
			})
		}
	}

	return result
}

// findIsolatedNodes identifies nodes that have no incoming or outgoing edges
func (na *nodesAnalyzer) findIsolatedNodes() []common.NodeInfo {
	na.g.clearNodeAttribute(common.IsIsolated)
	var result []common.NodeInfo

	for _, id := range na.g.traverser.getAllNodeIDs() {
		to := na.g.To(id)
		from := na.g.From(id)
		if !to.Next() && !from.Next() {
			na.g.addNodeAttribute(id, common.IsIsolated)
			result = append(result, common.NodeInfo{
				ID: id, Name: na.g.GetNameForID(id),
			})
		}
	}

	return result
}

// findNodesWithSelfLoops identifies all nodes that have edges to themselves
func (na *nodesAnalyzer) findNodesWithSelfLoops() []common.NodeInfo {
	var result []common.NodeInfo

	for _, id := range na.g.traverser.getAllNodeIDs() {
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
