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

	// process nodes in sorted order
	for _, id := range nodeIDs {
		name, exists := g.graph.GetNodeName(id)
		if !exists {
			continue
		}

		nodeObj, nodeExists := g.graph.NodeMap[name]
		if !nodeExists {
			continue
		}

		// check if node is on custom path
		isOnCustomPath := customPathNodes[id]

		// get node style (which may include custom style from analysis)
		baseStyleStr := g.getNodeStyleString(id, nodeObj)
		styleStr := baseStyleStr

		// apply custom path styling if applicable
		if isOnCustomPath {
			styleStr = g.applyCustomPathStylingToNode(baseStyleStr, nodeObj, true)
		}

		// add label if not already present
		styleStr = g.ensureNodeLabel(styleStr, nodeObj, id)

		buf.WriteString(fmt.Sprintf("  \"%d\" [%s];\n", id, styleStr))
	}
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

// getNodeStyleString gets the style string for a node
func (g *Generator) getNodeStyleString(id int64, node common.Node) string {
	if style, hasStyle := g.nodeStyles[id]; hasStyle {
		return g.buildNodeStyleString(style)
	}

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
		if node.SecondaryName != nil {
			style.label = fmt.Sprintf("%s\\n%s", name,
				escapeForDOT(*node.SecondaryName))
		}
	} else if node.IsOfType(common.IsStack) {
		style.fillColor = "#fff9c4"
		style.borderColor = "#fbc02d"
	}

	return g.buildNodeStyleString(style)
}

// applyCustomPathStylingToNode enhances style for nodes on the custom path
func (g *Generator) applyCustomPathStylingToNode(baseStyleStr string, nodeObj common.Node, isOnCustomPath bool) string {
	if !isOnCustomPath {
		return baseStyleStr
	}

	styleMap := make(map[string]string)

	parts := strings.Split(baseStyleStr, ", ")
	for _, part := range parts {
		keyValue := strings.SplitN(part, "=", 2)
		if len(keyValue) == 2 {
			key := strings.TrimSpace(keyValue[0])
			value := strings.TrimSpace(keyValue[1])
			styleMap[key] = value
		}
	}

	// force the required styling for nodes on custom path
	styleMap["fillcolor"] = "\"#E74C3C\"" // red background
	styleMap["fontcolor"] = "\"#FFFFFF\"" // white text

	// special page nodes keep their border styling
	if !nodeObj.IsOfType(common.ContainsBluePage) &&
		!nodeObj.IsOfType(common.ContainsRedPage) &&
		!nodeObj.IsOfType(common.ContainsWhitePage) {
		// for regular nodes, add a darker red border for contrast
		styleMap["penwidth"] = "2.0"
		styleMap["color"] = "\"#B71C1C\"" // darker red border
	}

	// convert the map back to a style string
	var newStyleParts []string
	for key, value := range styleMap {
		newStyleParts = append(newStyleParts, fmt.Sprintf("%s=%s", key, value))
	}

	sort.Strings(newStyleParts)

	return strings.Join(newStyleParts, ", ")
}

// ensureNodeLabel makes sure the node has a label attribute
func (g *Generator) ensureNodeLabel(styleStr string, nodeObj common.Node, id int64) string {
	if strings.Contains(styleStr, "label=") {
		return styleStr
	}

	// get the display name for the node
	var label string
	if g.config.UseNodeNames {
		label = g.getDisplayName(nodeObj)
	} else {
		label = fmt.Sprintf("%d", id)
	}

	return styleStr + fmt.Sprintf(", label=\"%s\"", label)
}

// buildNodeStyleString converts a nodeStyle to a DOT style attribute
func (g *Generator) buildNodeStyleString(style nodeStyle) string {
	var parts []string

	// add a property if its value is not empty
	appendIfNotEmpty := func(key, value string) {
		if value != "" {
			parts = append(parts, fmt.Sprintf("%s=\"%s\"", key, value))
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
		parts = append(parts, fmt.Sprintf("tooltip=\"%s\"", strings.ReplaceAll(style.tooltip, "\"", "\\\"")))
	}

	appendIfNotEmpty("label", style.label)

	sort.Strings(parts)

	return strings.Join(parts, ", ")
}

// escapeForDOT escapes special characters for DOT string literals
func escapeForDOT(input string) string {
	// escape backslashes first to avoid double escaping
	result := strings.ReplaceAll(input, "\\", "\\\\")
	// escape quotes
	result = strings.ReplaceAll(result, "\"", "\\\"")
	return result
}
