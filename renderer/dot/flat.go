package dot

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"

	pdflayout "github.com/glthr/DeMystify/renderer/layout"
)

// GenerateFlat retains Generate's visible nodes, styles and connections without
// the invisible component controls and clusters used by the original layout.
func (g *Generator) GenerateFlat(path []int64) (string, error) {
	if err := g.prepare(path); err != nil {
		return "", err
	}
	return g.buildFlatDOT(nil), nil
}

// GeneratePositioned pins every visible node to a center expressed in top-down
// points. The caller can route the result with Neato's -n2 mode. Positions must
// cover exactly the rendered inventory, including any implied path endpoints.
func (g *Generator) GeneratePositioned(path []int64, positions map[int64]pdflayout.Point) (string, error) {
	if err := g.prepare(path); err != nil {
		return "", err
	}
	visible := g.visibleNodeIDs()
	encoded := make(map[int64]string, len(visible))
	for _, id := range visible {
		point, exists := positions[id]
		if !exists {
			return "", fmt.Errorf("missing position for visible node %d", id)
		}
		if math.IsNaN(point.X) || math.IsNaN(point.Y) || math.IsInf(point.X, 0) || math.IsInf(point.Y, 0) {
			return "", fmt.Errorf("position for node %d must be finite", id)
		}
		encoded[id] = positionNumber(point.X) + "," + positionNumber(-point.Y)
	}
	var extra []int64
	for id := range positions {
		if _, exists := encoded[id]; !exists {
			extra = append(extra, id)
		}
	}
	if len(extra) > 0 {
		sort.Slice(extra, func(i, j int) bool { return extra[i] < extra[j] })
		return "", fmt.Errorf("position supplied for non-visible node %d", extra[0])
	}
	return g.buildFlatDOT(encoded), nil
}

func positionNumber(value float64) string {
	if value == 0 {
		return "0" // Avoid a negative zero after reversing the vertical axis.
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func (g *Generator) visibleNodeIDs() []int64 {
	ids := make(map[int64]bool)
	nodes := g.graph.Nodes()
	for nodes.Next() {
		id := nodes.Node().ID()
		if _, exists := g.nodeByID(id); exists {
			ids[id] = true
		}
	}
	for _, edge := range g.metadata.Edges {
		ids[edge.Source.GraphID], ids[edge.Target.GraphID] = true, true
	}
	if len(g.customPath) > 1 {
		for _, id := range g.customPath {
			ids[id] = true
		}
	}
	result := make([]int64, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func (g *Generator) buildFlatDOT(positions map[int64]string) string {
	var buf bytes.Buffer
	buf.WriteString("/* Generated with DeMystify (github.com/glthr/DeMystify) */\n\n")
	buf.WriteString("digraph G {\n")
	g.writeGraphAttributes(&buf)
	g.writeNodesWithPositions(&buf, positions)
	g.renderMetadataEdges(&buf)
	buf.WriteString("}\n")
	return buf.String()
}
