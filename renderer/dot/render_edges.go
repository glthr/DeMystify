package dot

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/glthr/DeMystify/common"
)

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

// renderEdge renders an edge to the buffer
func (g *Generator) renderEdge(buf *bytes.Buffer, fromID, toID int64, edge *common.Edge, isOnPath bool) {
	style := g.applyEdgeStyle(edge, isOnPath)
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
	pairs, groups := indexEdges(g.metadata.Edges)
	pathPairs := make(map[nodePair]bool)
	for i := 0; i+1 < len(g.customPath); i++ {
		pathPairs[nodePair{g.customPath[i], g.customPath[i+1]}] = true
	}
	var pathEdges []*common.Edge
	highlighted := make(map[nodePair]bool)
	if len(g.customPath) > 1 {
		g.writeCustomPathInfo(buf)
	}

	for _, pair := range pairs {
		reversePair := nodePair{pair.to, pair.from}
		for _, record := range groups[pair] {
			if record.used {
				continue
			}
			record.used = true
			edge := record.edge

			if pathPairs[pair] && !highlighted[pair] {
				pathEdges = append(pathEdges, edge)
				highlighted[pair] = true
				continue
			}
			if pair.from == pair.to {
				g.renderSelfLoopEdge(&loops, pair.from, edge, false)
				continue
			}

			// Keep both directions explicit when either is highlighted.
			style := g.applyEdgeStyle(edge, false)
			matched := false
			if !pathPairs[pair] && !pathPairs[reversePair] {
				for _, reverse := range groups[reversePair] {
					if !reverse.used && g.edgeAttributesMatch(edge, reverse.edge) {
						reverse.used = true
						style.bidirectional = true
						g.shareStroke(&style, edge, groups[pair], groups[reversePair])
						fmt.Fprintf(&bidirectional, "  \"%d\" -> \"%d\" [%s];\n", pair.from, pair.to, g.buildEdgeStyleString(style))
						matched = true
						break
					}
				}
			}
			if !matched {
				if !pathPairs[pair] {
					g.shareStroke(&style, edge, groups[pair], nil)
				}
				fmt.Fprintf(&regular, "  \"%d\" -> \"%d\" [%s];\n", pair.from, pair.to, g.buildEdgeStyleString(style))
			}
		}
	}

	buf.Write(loops.Bytes())
	buf.Write(bidirectional.Bytes())
	buf.Write(regular.Bytes())

	// Render highlights last so ordinary parallel edges cannot obscure them.
	for _, edge := range pathEdges {
		fromID, toID := edge.Source.GraphID, edge.Target.GraphID
		if fromID == toID {
			g.renderSelfLoopEdge(buf, fromID, edge, true)
		} else {
			g.renderEdge(buf, fromID, toID, edge, true)
		}
	}
	// A supplied custom path may contain implied transitions absent from metadata.
	for i := 0; i+1 < len(g.customPath); i++ {
		fromID, toID := g.customPath[i], g.customPath[i+1]
		pair := nodePair{fromID, toID}
		if highlighted[pair] {
			continue
		}
		edge := common.Edge{Source: &common.Node{GraphID: fromID}, Target: &common.Node{GraphID: toID}}
		buf.WriteString(fmt.Sprintf("  // Adding implied path edge: %d -> %d\n", fromID, toID))
		style := g.applyEdgeStyle(&edge, true)
		style.tooltip += " (implied)"
		buf.WriteString(fmt.Sprintf("  \"%d\" -> \"%d\" [%s];\n", fromID, toID, g.buildEdgeStyleString(style)))
		highlighted[pair] = true
	}
	buf.WriteString("\n")
}

// shareStroke draws an automatic next step on the stroke of an ordinary
// reference that joins the same two cards in the same direction: the hollow
// head follows the filled one. Neato can route two strokes between the same
// cards along one line, which would hide a hollow head under a filled one.
func (g *Generator) shareStroke(style *edgeStyle, edge *common.Edge, forward, backward []*pendingEdge) {
	if style.arrowhead != "" || style.arrowtail != "" || edge.IsOfType(common.SequenceContinuation) {
		return
	}
	if g.takeHollowStep(forward) {
		style.arrowhead = sharedContinuationArrow
		style.tooltip = joinTooltip(style.tooltip, continuationTooltip)
	}
	if style.bidirectional && g.takeHollowStep(backward) {
		style.arrowtail = sharedContinuationArrow
		style.tooltip = joinTooltip(style.tooltip, continuationTooltip)
	}
}

// takeHollowStep consumes one pending automatic next step that would otherwise
// be a plain stroke with a hollow head.
func (g *Generator) takeHollowStep(records []*pendingEdge) bool {
	for _, record := range records {
		if record.used || !record.edge.IsOfType(common.SequenceContinuation) {
			continue
		}
		if own := g.applyEdgeStyle(record.edge, false); own != (edgeStyle{arrowhead: continuationArrow, tooltip: continuationTooltip}) {
			continue
		}
		record.used = true
		return true
	}
	return false
}

func joinTooltip(existing, addition string) string {
	if existing == "" {
		return addition
	}
	return existing + "; " + addition
}
