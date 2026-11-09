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
	for _, node := range drawing.Nodes {
		if !expected[node.ID] || seen[node.ID] {
			return fmt.Errorf("PDF graph drawing has an unknown or duplicate node ID %d", node.ID)
		}
		seen[node.ID] = true
		if !finiteValues(node.X, node.Y, node.Width, node.Height) || node.Width <= 0 || node.Height <= 0 {
			return fmt.Errorf("PDF graph drawing node %d has invalid geometry", node.ID)
		}
		left, right := node.X-node.Width/2, node.X+node.Width/2
		top, bottom := node.Y-node.Height/2, node.Y+node.Height/2
		if !finiteValues(left, right, top, bottom) {
			return fmt.Errorf("PDF graph drawing node %d has overflowing geometry", node.ID)
		}
		toleranceX := pdfCoordinateTolerance(box.MinX, box.MaxX, node.X, node.Width)
		toleranceY := pdfCoordinateTolerance(box.MinY, box.MaxY, node.Y, node.Height)
		if box.MinX-left > toleranceX || right-box.MaxX > toleranceX || box.MinY-top > toleranceY || bottom-box.MaxY > toleranceY {
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

// Graphviz's pos/bb strings use five significant digits. At 10,000 points the
// final digit is a whole point, so independently rounded centers and bounds can
// differ by up to one point. Width/height strings use inches; include the half-
// size conversion's rounding too. Keep the ordinary small-drawing floor while
// scaling only to the coordinate precision, not to a percentage of a node box.
func pdfCoordinateTolerance(lower, upper, center, size float64) float64 {
	quantum := func(value float64) float64 {
		if value == 0 {
			return 0
		}
		return math.Pow10(int(math.Floor(math.Log10(math.Abs(value)))) - 4)
	}
	magnitude := math.Max(math.Abs(lower), math.Max(math.Abs(upper), math.Abs(center)))
	return math.Max(0.1, quantum(magnitude)+18*quantum(size/72)+1e-8)
}

func finiteValues(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}
