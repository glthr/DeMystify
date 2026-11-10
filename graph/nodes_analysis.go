package graph

import (
	"slices"

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

// findNodesWithNoIncomingEdges identifies nodes with no incoming edge from another node.
func (na *nodesAnalyzer) findNodesWithNoIncomingEdges() []common.NodeInfo {
	na.g.clearNodeAttribute(common.IsSource)
	var result []common.NodeInfo

	for _, id := range na.g.traverser.getAllNodeIDs() {
		to := na.g.To(id)
		hasExternalIncoming := false
		for to.Next() {
			if to.Node().ID() != id {
				hasExternalIncoming = true
				break
			}
		}
		if !hasExternalIncoming {
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

// findNodesWithSelfLoops lists the nodes with an active reference to
// themselves. Every record from a node to itself is marked as a
// self-reference, disabled or not: marking a disabled one only when an active
// record sat beside it described the same kind of record in two ways.
func (na *nodesAnalyzer) findNodesWithSelfLoops() []common.NodeInfo {
	var result []common.NodeInfo

	mark := func(edge *common.Edge) {
		if edge.Source.GraphID == edge.Target.GraphID && !edge.IsOfType(common.SelfReference) {
			edge.Attributes = append(slices.Clone(edge.Attributes), common.SelfReference)
		}
	}
	for _, edge := range na.g.Metadata.Edges {
		mark(edge)
	}
	for id, targets := range na.g.EdgeMap {
		for i := range targets[id] {
			mark(&targets[id][i])
		}
	}
	for _, id := range na.g.traverser.getAllNodeIDs() {
		if na.g.HasEdgeFromTo(id, id) {
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

// FindNodesWithNoIncomingEdges returns sources, ignoring self-loops as incoming edges.
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
