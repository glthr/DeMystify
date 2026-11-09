package dot

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/glthr/DeMystify/common"
)

// writeNodes writes all nodes with their styles to the DOT output
func (g *Generator) writeNodes(buf *bytes.Buffer) {
	g.writeNodesWithPositions(buf, nil)
}

// Positions are already validated and encoded as DOT coordinate pairs.
func (g *Generator) writeNodesWithPositions(buf *bytes.Buffer, positions map[int64]string) {
	customPathNodes := make(map[int64]bool)
	for _, id := range g.customPath {
		customPathNodes[id] = true
	}

	// collect all node IDs first
	var nodeIDs []int64
	nodes := g.graph.Nodes()
	for nodes.Next() {
		nodeIDs = append(nodeIDs, nodes.Node().ID())
	}

	// sort node IDs for deterministic ordering
	sort.Slice(nodeIDs, func(i, j int) bool {
		return nodeIDs[i] < nodeIDs[j]
	})

	written := make(map[int64]bool)
	// process nodes in sorted order
	for _, id := range nodeIDs {
		nodeObj, nodeExists := g.nodeByID(id)
		if !nodeExists {
			continue
		}

		// check if node is on custom path
		isOnCustomPath := customPathNodes[id]

		style := g.getNodeStyle(id, nodeObj)
		if isOnCustomPath {
			style = g.applyCustomPathStylingToNode(style, nodeObj)
		}
		if style.label == "" {
			if g.config.UseNodeNames {
				style.label = g.getDisplayName(nodeObj)
			} else {
				style.label = fmt.Sprintf("%d", id)
			}
		}
		attributes := g.buildNodeStyleString(style)
		if position, exists := positions[id]; exists {
			if attributes != "" {
				attributes += ", "
			}
			attributes += "pos=\"" + position + "!\""
		}
		buf.WriteString(fmt.Sprintf("  \"%d\" [%s];\n", id, attributes))
		written[id] = true
	}
	// Generate permits implied path endpoints outside the canonical inventory.
	// Explicit pos-only declarations retain their inherited default node style.
	var implicit []int64
	for id := range positions {
		if !written[id] {
			implicit = append(implicit, id)
		}
	}
	sort.Slice(implicit, func(i, j int) bool { return implicit[i] < implicit[j] })
	for _, id := range implicit {
		buf.WriteString(fmt.Sprintf("  \"%d\" [pos=\"%s!\"];\n", id, positions[id]))
	}
}

// Optional name mappings must not hide or replace canonical graph records.
func (g *Generator) nodeByID(id int64) (common.Node, bool) {
	if name, exists := g.graph.GetNodeName(id); exists {
		if node, found := g.graph.NodeMap[name]; found && node.GraphID == id {
			return node, true
		}
	}
	node, err := g.graph.GetNodeFromID(id)
	if err != nil {
		return common.Node{}, false
	}
	return *node, true
}

// getDisplayName creates a readable display name for a node
func (g *Generator) getDisplayName(node common.Node) string {
	name := node.Name

	if node.OriginalName != nil {
		name = fmt.Sprintf("%s (%s)", name, *node.OriginalName)
	}

	if node.IsOfType(common.IsStack) {
		name = fmt.Sprintf("[Stack] %s", name)
	}

	if node.IsOfType(common.IsVirtual) {
		name = fmt.Sprintf("%s (virtual)", name)
	}

	return name
}

// getNodeStyle gets the style for a node.
func (g *Generator) getNodeStyle(id int64, node common.Node) nodeStyle {
	if style, hasStyle := g.nodeStyles[id]; hasStyle {
		return style
	}
	return g.getBaseNodeStyle(id, node)
}

func (g *Generator) getBaseNodeStyle(id int64, node common.Node) nodeStyle {
	// create a default style
	style := nodeStyle{}

	if node.IsOfType(common.IsCard) {

		if node.IsOfType(common.ContainsBluePage) && node.IsOfType(common.ContainsRedPage) {
			style.penWidth = 3
			style.borderColor = "#6a1cea"
		} else if node.IsOfType(common.ContainsBluePage) {
			style.penWidth = 3
			style.borderColor = "#0000ff"
		} else if node.IsOfType(common.ContainsRedPage) {
			style.penWidth = 3
			style.borderColor = "#ff0000"
		} else if node.IsOfType(common.ContainsWhitePage) {
			style.penWidth = 3
			style.borderColor = "#dedbdb"
		}

		// apply stack-based colors if enabled
		if g.config.ColorByStack {
			if color, exists := g.stackColors[node.StackName]; exists {
				style.fillColor = color.fillColor

				if style.borderColor == "" {
					style.borderColor = color.borderColor
				}
			}
		}

		// add secondary name (if available)
		name := g.getDisplayName(node)
		if !g.config.UseNodeNames {
			name = fmt.Sprint(id)
		}
		if node.SecondaryName != nil {
			style.label = fmt.Sprintf("%s\n%s", name, *node.SecondaryName)
		}
	} else if node.IsOfType(common.IsStack) {
		style.fillColor = "#fff9c4"
		style.borderColor = "#fbc02d"
	}

	return style
}

// applyCustomPathStylingToNode keeps style values structured until DOT encoding.
// Parsing a style string on commas would corrupt commas inside quoted labels.
func (g *Generator) applyCustomPathStylingToNode(style nodeStyle, nodeObj common.Node) nodeStyle {
	style.fillColor = "#E74C3C"
	style.fontColor = "#FFFFFF"
	if !nodeObj.IsOfType(common.ContainsBluePage) &&
		!nodeObj.IsOfType(common.ContainsRedPage) &&
		!nodeObj.IsOfType(common.ContainsWhitePage) {
		style.penWidth = 2.0
		style.borderColor = "#B71C1C"
	}
	return style
}

// buildNodeStyleString converts a nodeStyle to a DOT style attribute
func (g *Generator) buildNodeStyleString(style nodeStyle) string {
	var parts []string

	// add a property if its value is not empty
	appendIfNotEmpty := func(key, value string) {
		if value != "" {
			parts = append(parts, fmt.Sprintf("%s=\"%s\"", key, escapeForDOT(value)))
		}
	}

	appendIfNotEmpty("shape", style.shape)
	appendIfNotEmpty("style", style.style)
	appendIfNotEmpty("fillcolor", style.fillColor)
	appendIfNotEmpty("color", style.borderColor)
	appendIfNotEmpty("fontcolor", style.fontColor)

	if style.penWidth > 0 {
		parts = append(parts, fmt.Sprintf("penwidth=%.1f", style.penWidth))
	}

	if style.tooltip != "" {
		parts = append(parts, fmt.Sprintf("tooltip=\"%s\"", escapeForDOT(style.tooltip)))
	}

	appendIfNotEmpty("label", style.label)

	sort.Strings(parts)

	return strings.Join(parts, ", ")
}

// escapeForDOT escapes special characters for DOT string literals
func escapeForDOT(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	return strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n").Replace(input)
}
