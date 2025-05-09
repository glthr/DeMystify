package dot

import (
	"fmt"

	"github.com/glthr/DeMystify/common"
)

func enrichTransitiveEdgeIDTooltip(tooltip *string, edge *common.Edge) {
	transitivityType := "Head"
	if edge.IsOfType(common.RestrictiveTransitivityTail) {
		transitivityType = "Tail"
	}

	*tooltip += fmt.Sprintf(" (Restrictive Transitivity %s ID %d)", transitivityType, edge.TransitivityID)
}

// applyEdgeStyle applies styling to an edge based on its attributes
func (g *Generator) applyEdgeStyle(edge *common.Edge, isOnCustomPath bool) edgeStyle {
	style := edgeStyle{}

	// handle custom path styling first (the highest priority)
	if isOnCustomPath {
		style.color = "#E74C3C" // bright red
		style.penWidth = 2.5
		style.tooltip = "Path edge"

		// modify based on edge attributes
		if edge.IsOfType(common.CrossAge) {
			style.style = "dashed"
			style.tooltip = "CrossAge connection on path"
		}

		if edge.IsOfType(common.Backtracking) {
			style.color = "#6a1cea"
			style.style = "dotted"
			style.arrowtail = "inv"
			style.arrowhead = "inv"
			style.tooltip = "Pop back on path"
		}

		// set arrowhead to none for restrictive transitivity edges on the custom path
		if edge.IsOfType(common.RestrictiveTransitivityTail) {
			style.arrowhead = "none"
			enrichTransitiveEdgeIDTooltip(&style.tooltip, edge)
		} else if edge.IsOfType(common.RestrictiveTransitivityHead) {
			enrichTransitiveEdgeIDTooltip(&style.tooltip, edge)
		}

		return style
	}

	// Handle restrictive transitive edges
	if edge.IsOfType(common.RestrictiveTransitivityTail) {
		g.applyBaseEdgeStyle(edge, &style)

		enrichTransitiveEdgeIDTooltip(&style.tooltip, edge)

		// set colors based on transitivity ID
		if style.color == "" {
			colorIndex := int(edge.TransitivityID) % len(nodesColors)
			style.color = darkenColor(nodesColors[colorIndex])
		}

		style.arrowhead = "none"

		if style.penWidth == 0 {
			style.penWidth = 1.2
		}

		return style
	}

	if edge.IsOfType(common.RestrictiveTransitivityHead) {
		for _, existingEdge := range g.metadata.Edges {
			if existingEdge.IsOfType(common.RestrictiveTransitivityTail) &&
				existingEdge.TransitivityID == edge.TransitivityID {
				// found matching restrictive transitivity tail: inherit its style
				g.applyBaseEdgeStyle(existingEdge, &style)

				style.arrowhead = "normal"

				enrichTransitiveEdgeIDTooltip(&style.tooltip, edge)

				return style
			}
		}
	}

	// for regular edges, apply base styling
	g.applyBaseEdgeStyle(edge, &style)
	return style
}

// applyBaseEdgeStyle applies basic styling to an edge
func (g *Generator) applyBaseEdgeStyle(edge *common.Edge, style *edgeStyle) {
	if edge.IsOfType(common.CrossAge) {
		style.color = g.stackColors[edge.Source.StackName].borderColor
		style.tooltip = "CrossAge connection"
		style.penWidth = 3.0
	}

	if edge.IsOfType(common.Backtracking) {
		style.arrowtail = "inv"
		style.arrowhead = "inv"
		if style.tooltip != "" {
			style.tooltip += " + PopBack"
		} else {
			style.tooltip = "pop card"
		}

		if style.color == "" {
			style.color = "#5375b9"
		}
	}

	if edge.IsOfType(common.Disabled) {
		if !edge.IsOfType(common.RestrictiveTransitivityTail) && !edge.IsOfType(common.RestrictiveTransitivityHead) {
			style.style = "dashed"
		}

		style.arrowhead = "tee"

		if style.color == "" {
			style.color = "#CCCCCC"
		}

		if style.tooltip != "" {
			style.tooltip += " (disabled)"
		} else {
			style.tooltip = "Disabled edge"
		}
	}
}

// edgeAttributesMatch checks if two edges match in terms of their attributes
func (g *Generator) edgeAttributesMatch(edge1, edge2 *common.Edge) bool {
	isEdge1Transitive := edge1.IsOfType(common.RestrictiveTransitivityTail) || edge1.IsOfType(common.RestrictiveTransitivityHead)
	isEdge2Transitive := edge2.IsOfType(common.RestrictiveTransitivityTail) || edge2.IsOfType(common.RestrictiveTransitivityHead)

	if isEdge1Transitive || isEdge2Transitive {
		return false // restrictive transitivity edges are never merged
	}

	if edge1.IsOfType(common.Disabled) || edge2.IsOfType(common.Disabled) {
		return false // disabled edges are never merged
	}

	isEdge1Backtracking := edge1.IsOfType(common.Backtracking)
	isEdge2Backtracking := edge2.IsOfType(common.Backtracking)

	if isEdge1Backtracking != isEdge2Backtracking {
		return false
	}

	if len(edge1.Attributes) != len(edge2.Attributes) {
		return false
	}

	attrMap1 := make(map[common.EdgeAttribute]bool, len(edge1.Attributes))
	for _, attr := range edge1.Attributes {
		attrMap1[attr] = true
	}

	// ensure that all attributes match
	for _, attr := range edge2.Attributes {
		if !attrMap1[attr] {
			return false
		}
	}

	return true
}

func (g *Generator) getNodePairID(fromID, toID int64) string {
	return fmt.Sprintf("%d->%d", fromID, toID)
}

func (g *Generator) getBidirectionalPairID(id1, id2 int64) string {
	if id1 < id2 {
		return fmt.Sprintf("%d<->%d", id1, id2)
	}
	return fmt.Sprintf("%d<->%d", id2, id1)
}
