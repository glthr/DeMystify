package dot

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/glthr/DeMystify/common"
)

// applyAnalysisStyles applies styles based on graph analysis
func (g *Generator) applyAnalysisStyles() error {
	applyStyle := func(id int64, fillColor, borderColor, tooltip string, penWidth float64) error {
		nodeObj, err := g.graph.GetNodeFromID(id)
		if err != nil {
			return err
		}

		style := nodeStyle{
			fillColor:   fillColor,
			borderColor: borderColor,
			penWidth:    penWidth,
			tooltip:     tooltip,
		}

		name := g.getDisplayName(*nodeObj)
		secondaryName := nodeObj.SecondaryName
		if secondaryName != nil {
			background := escapeForDOT(*secondaryName)
			style.label = fmt.Sprintf("%s\\n%s", name, background)
		}

		g.nodeStyles[id] = style
		return nil
	}

	for _, node := range g.metadata.Nodes {
		if node.IsOfType(common.IsIsolated) {
			if err := applyStyle(node.GraphID, "#FFFF99", "", "Isolated node", 0); err != nil {
				return err
			}
		} else if node.IsOfType(common.IsSink) {
			if err := applyStyle(node.GraphID, "#FFA07A", "#FF4500", "Sink node (no outgoing connections)", 1.5); err != nil {
				return err
			}
		} else if node.IsOfType(common.IsSource) {
			if err := applyStyle(node.GraphID, "#98FB98", "#228B22", "Source node (no incoming connections)", 1.5); err != nil {
				return err
			}
		}
	}

	return nil
}

// assignStackColors assigns colors to stacks for consistent visualization
func (g *Generator) assignStackColors() {
	// NOTE: use a slice to ensure the deterministic collection of unique stacks
	var uniqueStacks []string
	stackExists := make(map[string]bool)

	nodes := g.graph.Nodes()
	for nodes.Next() {
		node := nodes.Node()
		id := node.ID()
		name, exists := g.graph.GetNodeName(id)

		if !exists {
			continue
		}

		nodeObj, nodeExists := g.graph.NodeMap[name]
		if !nodeExists {
			continue
		}

		if !stackExists[nodeObj.StackName] {
			uniqueStacks = append(uniqueStacks, nodeObj.StackName)
			stackExists[nodeObj.StackName] = true
		}
	}

	// sort the stack names to ensure a deterministic assignment
	sort.Strings(uniqueStacks)

	// assign colors to each stack in order
	for i, stack := range uniqueStacks {
		if _, exists := g.stackColors[stack]; !exists {
			colorIndex := i % len(nodesColors)
			fillColor := nodesColors[colorIndex]
			nodeColor := nodeColors{
				fillColor:   fillColor,
				borderColor: darkenColor(fillColor),
			}

			g.stackColors[stack] = nodeColor
		}
	}
}

// darkenColor darkens a node color to use it as a border color
func darkenColor(originalColor string) string {
	parseComponent := func(hex string) (int64, error) {
		return strconv.ParseInt(hex, 16, 64)
	}

	r, err1 := parseComponent(originalColor[1:3])
	g, err2 := parseComponent(originalColor[3:5])
	b, err3 := parseComponent(originalColor[5:7])

	if err1 != nil || err2 != nil || err3 != nil {
		return originalColor
	}

	darken := func(component int64) int64 {
		newValue := int64(float64(component) * 0.8)
		if newValue < 0 {
			return 0
		}
		return newValue
	}

	r = darken(r)
	g = darken(g)
	b = darken(b)

	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}
