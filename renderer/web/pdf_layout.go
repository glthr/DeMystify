package web

import (
	"encoding/json"
	"fmt"
	"math"

	pdflayout "github.com/glthr/DeMystify/renderer/layout"
)

// attachPDFLayout validates the independently rendered drawing, without
// substituting its merged visual edges for the full metadata edge records.
func attachPDFLayout(data *graphData, drawing *pdflayout.Drawing) error {
	if drawing == nil {
		return nil
	}
	// This also checks every nested drawing operation, label, and Bezier
	// control point for NaN/Inf, before any output directory is created.
	if _, err := json.Marshal(drawing); err != nil {
		return fmt.Errorf("invalid PDF graph drawing: %w", err)
	}
	box := drawing.Bounds
	if !finiteValues(box.MinX, box.MinY, box.MaxX, box.MaxY, box.MaxX-box.MinX, box.MaxY-box.MinY) || box.MaxX <= box.MinX || box.MaxY <= box.MinY {
		return fmt.Errorf("PDF graph drawing has invalid bounds")
	}
	if len(drawing.Nodes) != len(data.Nodes) {
		return fmt.Errorf("PDF graph drawing has %d nodes; graph metadata has %d", len(drawing.Nodes), len(data.Nodes))
	}
	expected := make(map[int64]bool, len(data.Nodes))
	for _, node := range data.Nodes {
		expected[node.ID] = true
	}
	seen := make(map[int64]bool, len(drawing.Nodes))
	const tolerance = 0.1 // Graphviz JSON geometry is rounded in points.
	for _, node := range drawing.Nodes {
		if !expected[node.ID] || seen[node.ID] {
			return fmt.Errorf("PDF graph drawing has an unknown or duplicate node ID %d", node.ID)
		}
		seen[node.ID] = true
		if !finiteValues(node.X, node.Y, node.Width, node.Height) || node.Width <= 0 || node.Height <= 0 {
			return fmt.Errorf("PDF graph drawing node %d has invalid geometry", node.ID)
		}
		if node.X-node.Width/2 < box.MinX-tolerance || node.X+node.Width/2 > box.MaxX+tolerance || node.Y-node.Height/2 < box.MinY-tolerance || node.Y+node.Height/2 > box.MaxY+tolerance {
			return fmt.Errorf("PDF graph drawing node %d lies outside its bounds", node.ID)
		}
	}
	for _, edge := range drawing.Edges {
		if !seen[edge.Source] || !seen[edge.Target] {
			return fmt.Errorf("PDF graph drawing edge %d → %d refers to a missing node", edge.Source, edge.Target)
		}
	}
	// Bezier control points may legitimately lie outside the visible bounds;
	// their finiteness, not the bounds of their control polygon, matters here.
	data.PDFLayout = drawing
	data.Meta.DefaultLayout = "pdf"
	return nil
}

func finiteValues(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}
