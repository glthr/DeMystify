package dot

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/glthr/DeMystify/common"
)

// writeEdges renders individual metadata edges without dropping parallel records.
func (g *Generator) writeEdges(buf *bytes.Buffer) {
	g.renderMetadataEdges(buf)
}

// processBidirectionalEdges handles bidirectional edges with matching attributes
// skipCustomPath determines whether to skip edges that are part of the custom path
func (g *Generator) processBidirectionalEdges(buf *bytes.Buffer, processedPairs, bidirectionalPairs, renderedNodePairs map[string]bool, skipCustomPath bool) {
	// get sorted source and target IDs
	sourceIDs := g.getSortedNodeIDs(g.edgesMap)

	// process bidirectional edges with matching attributes
	for _, fromID := range sourceIDs {
		targetIDs := g.getSortedTargetIDs(g.edgesMap[fromID], fromID, true)

		for _, toID := range targetIDs {
			// skip self-loops (already processed)
			if fromID == toID {
				continue
			}

			forwardPairID := g.getNodePairID(fromID, toID)
			reversePairID := g.getNodePairID(toID, fromID)
			biPairID := g.getBidirectionalPairID(fromID, toID)

			if processedPairs[forwardPairID] {
				continue
			}

			// skip edges that are part of the custom path if requested
			if skipCustomPath && (g.customPathEdgeSet[forwardPairID] || g.customPathEdgeSet[reversePairID]) {
				processedPairs[forwardPairID] = true
				processedPairs[reversePairID] = true
				continue
			}

			// check for bidirectional relationship with matching attributes
			if bidirectionalPairs[biPairID] && g.hasMatchingEdges(fromID, toID) {
				// find non-disabled, non-transitive edges
				var matchedEdge *common.Edge
				forwardEdges := g.edgesMap[fromID][toID]

				for _, edge := range forwardEdges {
					if !edge.IsOfType(common.Disabled) &&
						!edge.IsOfType(common.RestrictiveTransitivityTail) &&
						!edge.IsOfType(common.RestrictiveTransitivityHead) {
						matchedEdge = &edge
						break
					}
				}

				if matchedEdge != nil {
					// render the bidirectional edge
					g.renderBidirectionalEdge(buf, fromID, toID, matchedEdge)

					processedPairs[forwardPairID] = true
					processedPairs[reversePairID] = true
					renderedNodePairs[forwardPairID] = true
					renderedNodePairs[reversePairID] = true
				}
			}
		}
	}
}

// processRegularEdges processes regular (non-matched bidirectional, non-self-loop) edges
// skipCustomPath determines whether to skip edges that are part of the custom path
func (g *Generator) processRegularEdges(buf *bytes.Buffer, processedPairs, renderedNodePairs, transitiveEdgesProcessed map[string]bool, skipCustomPath bool) {
	// get sorted source IDs
	sourceIDs := g.getSortedNodeIDs(g.edgesMap)

	for _, fromID := range sourceIDs {
		// get sorted target IDs
		targetIDs := g.getSortedTargetIDs(g.edgesMap[fromID], fromID, false)

		for _, toID := range targetIDs {
			// skip self-loops (already processed)
			if fromID == toID {
				continue
			}

			nodePairID := g.getNodePairID(fromID, toID)

			if processedPairs[nodePairID] {
				continue
			}

			// skip custom path edges if requested
			if skipCustomPath && g.customPathEdgeSet[nodePairID] {
				processedPairs[nodePairID] = true
				continue
			}

			// sort edges for deterministic order
			edges := g.sortEdges(g.edgesMap[fromID][toID])

			for _, edge := range edges {
				isTransitive := edge.IsOfType(common.RestrictiveTransitivityTail) || edge.IsOfType(common.RestrictiveTransitivityHead)
				isBacktracking := edge.IsOfType(common.Backtracking)
				isDisabled := edge.IsOfType(common.Disabled)

				// for restrictive transitivity edges, create a unique key including the transitivity ID
				if isTransitive {
					transitiveKey := g.getTransitiveEdgeKey(nodePairID, &edge)

					if transitiveEdgesProcessed[transitiveKey] {
						continue
					}
					transitiveEdgesProcessed[transitiveKey] = true
				} else if !isBacktracking && !isDisabled {
					if renderedNodePairs[nodePairID] {
						continue
					}
				}

				// render the edge (not on the custom path)
				g.renderEdge(buf, fromID, toID, &edge, false)

				if !isTransitive && !isBacktracking && !isDisabled {
					renderedNodePairs[nodePairID] = true
				}
			}

			processedPairs[nodePairID] = true
		}
	}
}

func (g *Generator) writeCustomPathInfo(buf *bytes.Buffer) {
	buf.WriteString("  // Custom path: ")
	for i, id := range g.customPath {
		if i > 0 {
			buf.WriteString(" → ")
		}
		buf.WriteString(fmt.Sprintf("%d", id))
	}
	buf.WriteString("\n\n")
}

// identifyBidirectionalEdges finds all bidirectional relationships
func (g *Generator) identifyBidirectionalEdges(bidirectionalPairs map[string]bool) {
	// track attribute-matched bidirectional edges
	attrMatchedEdges := make(map[string]bool)

	var sourceIDs []int64
	for fromID := range g.edgesMap {
		sourceIDs = append(sourceIDs, fromID)
	}
	sort.Slice(sourceIDs, func(i, j int) bool {
		return sourceIDs[i] < sourceIDs[j]
	})

	// examine each node pair once
	for _, fromID := range sourceIDs {
		targetMap := g.edgesMap[fromID]

		// for each target, check for reverse edges
		var targetIDs []int64
		for toID := range targetMap {
			if fromID < toID {
				targetIDs = append(targetIDs, toID)
			}
		}
		sort.Slice(targetIDs, func(i, j int) bool {
			return targetIDs[i] < targetIDs[j]
		})

		for _, toID := range targetIDs {
			forwardEdges := targetMap[toID]

			// check if reverse edges exist
			reverseEdges, reverseExists := g.edgesMap[toID][fromID]
			if !reverseExists {
				continue // no bidirectional relationship
			}

			// mark this pair as bidirectional
			pairID := g.getBidirectionalPairID(fromID, toID)
			bidirectionalPairs[pairID] = true

			// check if any pairs of edges have matching attributes
			for _, forwardEdge := range forwardEdges {
				// skip if this edge is disabled or transitive
				if forwardEdge.IsOfType(common.Disabled) ||
					forwardEdge.IsOfType(common.RestrictiveTransitivityTail) ||
					forwardEdge.IsOfType(common.RestrictiveTransitivityHead) {
					continue
				}

				for _, reverseEdge := range reverseEdges {
					// skip if reverse edge is disabled or transitive
					if reverseEdge.IsOfType(common.Disabled) ||
						reverseEdge.IsOfType(common.RestrictiveTransitivityTail) ||
						reverseEdge.IsOfType(common.RestrictiveTransitivityHead) {
						continue
					}

					// check if attributes match
					if g.edgeAttributesMatch(&forwardEdge, &reverseEdge) {
						attrMatchedEdges[pairID] = true
						break
					}
				}

				if attrMatchedEdges[pairID] {
					break
				}
			}
		}
	}
}

// identifySelfLoops identifies all self-loop edges
func (g *Generator) identifySelfLoops() []int64 {
	selfLoopSet := make(map[int64]bool)

	for fromID, targets := range g.edgesMap {
		if _, hasSelfLoop := targets[fromID]; hasSelfLoop {
			selfLoopSet[fromID] = true
		}
	}

	selfLoops := make([]int64, 0, len(selfLoopSet))
	for id := range selfLoopSet {
		selfLoops = append(selfLoops, id)
	}

	sort.Slice(selfLoops, func(i, j int) bool {
		return selfLoops[i] < selfLoops[j]
	})

	return selfLoops
}

// getSortedNodeIDs returns a sorted slice of node IDs from a map
func (g *Generator) getSortedNodeIDs(nodeMap map[int64]map[int64][]common.Edge) []int64 {
	var nodeIDs []int64
	for nodeID := range nodeMap {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Slice(nodeIDs, func(i, j int) bool {
		return nodeIDs[i] < nodeIDs[j]
	})
	return nodeIDs
}

// getSortedTargetIDs returns a sorted slice of target node IDs
// If bidirectionalOnly is true, only returns targets where fromID < toID
func (g *Generator) getSortedTargetIDs(targetMap map[int64][]common.Edge, fromID int64, bidirectionalOnly bool) []int64 {
	var targetIDs []int64
	for toID := range targetMap {
		if !bidirectionalOnly || fromID < toID {
			targetIDs = append(targetIDs, toID)
		}
	}
	sort.Slice(targetIDs, func(i, j int) bool {
		return targetIDs[i] < targetIDs[j]
	})
	return targetIDs
}

// sortEdges sorts edges for deterministic order
func (g *Generator) sortEdges(edges []common.Edge) []common.Edge {
	sortedEdges := make([]common.Edge, len(edges))
	copy(sortedEdges, edges)

	sort.Slice(sortedEdges, func(i, j int) bool {
		// sort by type and attributes
		if sortedEdges[i].IsOfType(common.Backtracking) != sortedEdges[j].IsOfType(common.Backtracking) {
			return !sortedEdges[i].IsOfType(common.Backtracking) // non-backtracking first
		}
		if sortedEdges[i].IsOfType(common.Disabled) != sortedEdges[j].IsOfType(common.Disabled) {
			return !sortedEdges[i].IsOfType(common.Disabled) // non-disabled first
		}
		// default to transitivity ID
		return sortedEdges[i].TransitivityID < sortedEdges[j].TransitivityID
	})

	return sortedEdges
}

// getTransitiveEdgeKey creates a unique key for a transitive edge
func (g *Generator) getTransitiveEdgeKey(nodePairID string, edge *common.Edge) string {
	typeStr := "One"
	if edge.IsOfType(common.RestrictiveTransitivityHead) {
		typeStr = "Two"
	}
	return fmt.Sprintf("%s:trans%s:%d", nodePairID, typeStr, edge.TransitivityID)
}

// renderEdge renders an edge to the buffer
func (g *Generator) renderEdge(buf *bytes.Buffer, fromID, toID int64, edge *common.Edge, isOnPath bool) {
	style := g.applyEdgeStyle(edge, isOnPath)
	styleStr := g.buildEdgeStyleString(style)
	buf.WriteString(fmt.Sprintf("  \"%d\" -> \"%d\" [%s];\n", fromID, toID, styleStr))
}

// renderBidirectionalEdge renders a bidirectional edge to the buffer
func (g *Generator) renderBidirectionalEdge(buf *bytes.Buffer, fromID, toID int64, edge *common.Edge) {
	style := g.applyEdgeStyle(edge, false)
	style.bidirectional = true
	styleStr := g.buildEdgeStyleString(style)
	buf.WriteString(fmt.Sprintf("  \"%d\" -> \"%d\" [%s];\n", fromID, toID, styleStr))
}

// renderSelfLoopEdge renders a self-loop edge to the buffer
func (g *Generator) renderSelfLoopEdge(buf *bytes.Buffer, nodeID int64, edge *common.Edge, isOnPath bool) {
	style := g.applyEdgeStyle(edge, isOnPath)
	style.isSelfLoop = true
	styleStr := g.buildEdgeStyleString(style)
	buf.WriteString(fmt.Sprintf("  \"%d\" -> \"%d\" [%s];\n", nodeID, nodeID, styleStr))
}

// processSelfLoops handles all self-loop edges
func (g *Generator) processSelfLoops(buf *bytes.Buffer, selfLoops []int64) {
	for _, nodeID := range selfLoops {
		edges, ok := g.edgesMap[nodeID][nodeID]
		if !ok {
			continue
		}

		// sort edges for deterministic output
		sortedEdges := g.sortEdges(edges)

		for _, edge := range sortedEdges {
			isOnPath := g.isOnCustomPath(nodeID, nodeID)
			g.renderSelfLoopEdge(buf, nodeID, &edge, isOnPath)
		}
	}
}

// processCustomPathEdges handles edges on the custom path
func (g *Generator) processCustomPathEdges(buf *bytes.Buffer, processedPairs map[string]bool) {
	if len(g.customPath) <= 1 {
		return
	}

	buf.WriteString("\n  // Custom path edges\n")

	for i := 0; i < len(g.customPath)-1; i++ {
		fromID := g.customPath[i]
		toID := g.customPath[i+1]

		var edge *common.Edge
		isPlaceholder := false

		if edges, ok := g.edgesMap[fromID][toID]; ok && len(edges) > 0 {
			// use the first existing edge
			edge = &edges[0]
		} else {
			// create a placeholder edge
			edge = &common.Edge{
				Source:     &common.Node{GraphID: fromID},
				Target:     &common.Node{GraphID: toID},
				Attributes: []common.EdgeAttribute{},
			}
			isPlaceholder = true
			buf.WriteString(fmt.Sprintf("  // Adding implied path edge: %d -> %d\n", fromID, toID))
		}

		if isPlaceholder {
			style := g.applyEdgeStyle(edge, true)

			if style.tooltip != "" {
				style.tooltip += " (implied)"
			}

			styleStr := g.buildEdgeStyleString(style)
			buf.WriteString(fmt.Sprintf("  \"%d\" -> \"%d\" [%s];\n", fromID, toID, styleStr))
		} else {
			g.renderEdge(buf, fromID, toID, edge, true)
		}

		forwardPairID := g.getNodePairID(fromID, toID)
		processedPairs[forwardPairID] = true
	}
}

// hasMatchingEdges checks if there are matching bidirectional edges
func (g *Generator) hasMatchingEdges(fromID, toID int64) bool {
	forwardEdges, forwardOK := g.edgesMap[fromID][toID]
	reverseEdges, reverseOK := g.edgesMap[toID][fromID]

	if !forwardOK || !reverseOK {
		return false
	}

	// find non-disabled, non-transitive edges
	for _, fEdge := range forwardEdges {
		if fEdge.IsOfType(common.Disabled) ||
			fEdge.IsOfType(common.RestrictiveTransitivityTail) ||
			fEdge.IsOfType(common.RestrictiveTransitivityHead) {
			continue
		}

		for _, rEdge := range reverseEdges {
			if rEdge.IsOfType(common.Disabled) ||
				rEdge.IsOfType(common.RestrictiveTransitivityTail) ||
				rEdge.IsOfType(common.RestrictiveTransitivityHead) {
				continue
			}

			if g.edgeAttributesMatch(&fEdge, &rEdge) {
				return true
			}
		}
	}

	return false
}

// buildEdgeStyleString converts an edgeStyle to a DOT style attribute
func (g *Generator) buildEdgeStyleString(style edgeStyle) string {
	var parts []string

	appendIfNotEmpty := func(key, value string) {
		if value != "" {
			parts = append(parts, fmt.Sprintf("%s=\"%s\"", key, escapeForDOT(value)))
		}
	}

	// add properties in a fixed order
	appendIfNotEmpty("color", style.color)
	appendIfNotEmpty("style", style.style)

	if style.penWidth > 0 {
		parts = append(parts, fmt.Sprintf("penwidth=%.1f", style.penWidth))
	}

	if style.bidirectional {
		parts = append(parts, "dir=both")
	}

	if style.weight > 0 {
		parts = append(parts, fmt.Sprintf("weight=%.1f", style.weight))
	}

	if style.minLen > 0 {
		parts = append(parts, fmt.Sprintf("minlen=%d", style.minLen))
	}

	if style.tooltip != "" {
		parts = append(parts, fmt.Sprintf("tooltip=\"%s\"", escapeForDOT(style.tooltip)))
	}

	appendIfNotEmpty("arrowhead", style.arrowhead)
	appendIfNotEmpty("arrowtail", style.arrowtail)

	if style.arrowhead != "" && style.arrowtail != "" && !style.bidirectional {
		parts = append(parts, "dir=\"both\"")
	}

	// special handling for self-loops
	if style.isSelfLoop {
		if style.arrowhead == "" {
			parts = append(parts, "arrowhead=\"normal\"")
		}

		parts = append(parts, "minlen=2")
		parts = append(parts, "constraint=false")
	}

	sort.Strings(parts)

	return strings.Join(parts, ", ")
}

// renderMetadataEdges preserves each metadata edge, merging only individual reverse edges
// with matching attributes. Tracking whole node pairs would discard parallel edges.
func (g *Generator) renderMetadataEdges(buf *bytes.Buffer) {
	var loops, bidirectional, regular bytes.Buffer
	processed := make(map[string][]bool)
	for fromID, targets := range g.edgesMap {
		for toID, edges := range targets {
			targets[toID] = g.sortEdges(edges)
			processed[g.getNodePairID(fromID, toID)] = make([]bool, len(edges))
		}
	}

	type pathEdge struct {
		fromID, toID int64
		edge         common.Edge
	}
	var pathEdges []pathEdge
	highlighted := make(map[string]bool)
	if len(g.customPath) > 1 {
		g.writeCustomPathInfo(buf)
	}

	for _, fromID := range g.getSortedNodeIDs(g.edgesMap) {
		for _, toID := range g.getSortedTargetIDs(g.edgesMap[fromID], fromID, false) {
			pairID := g.getNodePairID(fromID, toID)
			reverseID := g.getNodePairID(toID, fromID)
			for i, edge := range g.edgesMap[fromID][toID] {
				if processed[pairID][i] {
					continue
				}
				processed[pairID][i] = true

				if g.isOnCustomPath(fromID, toID) && !highlighted[pairID] {
					pathEdges = append(pathEdges, pathEdge{fromID, toID, edge})
					highlighted[pairID] = true
					continue
				}
				if fromID == toID {
					g.renderSelfLoopEdge(&loops, fromID, &edge, false)
					continue
				}

				// Keep both directions explicit when either is highlighted.
				matched := false
				if !g.isOnCustomPath(fromID, toID) && !g.isOnCustomPath(toID, fromID) {
					for j, reverse := range g.edgesMap[toID][fromID] {
						if !processed[reverseID][j] && g.edgeAttributesMatch(&edge, &reverse) {
							processed[reverseID][j] = true
							g.renderBidirectionalEdge(&bidirectional, fromID, toID, &edge)
							matched = true
							break
						}
					}
				}
				if !matched {
					g.renderEdge(&regular, fromID, toID, &edge, false)
				}
			}
		}
	}

	buf.Write(loops.Bytes())
	buf.Write(bidirectional.Bytes())
	buf.Write(regular.Bytes())

	// Render highlights last so ordinary parallel edges cannot obscure them.
	for _, item := range pathEdges {
		if item.fromID == item.toID {
			g.renderSelfLoopEdge(buf, item.fromID, &item.edge, true)
		} else {
			g.renderEdge(buf, item.fromID, item.toID, &item.edge, true)
		}
	}
	// A supplied custom path may contain implied transitions absent from metadata.
	for i := 0; i+1 < len(g.customPath); i++ {
		fromID, toID := g.customPath[i], g.customPath[i+1]
		pairID := g.getNodePairID(fromID, toID)
		if highlighted[pairID] {
			continue
		}
		edge := common.Edge{Source: &common.Node{GraphID: fromID}, Target: &common.Node{GraphID: toID}}
		buf.WriteString(fmt.Sprintf("  // Adding implied path edge: %d -> %d\n", fromID, toID))
		style := g.applyEdgeStyle(&edge, true)
		style.tooltip += " (implied)"
		buf.WriteString(fmt.Sprintf("  \"%d\" -> \"%d\" [%s];\n", fromID, toID, g.buildEdgeStyleString(style)))
		highlighted[pairID] = true
	}
	buf.WriteString("\n")
}
