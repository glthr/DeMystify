package graph

import (
	"fmt"
	"math"
	"slices"

	"github.com/glthr/DeMystify/common"

	"gonum.org/v1/gonum/graph/multi"
)

// notTraversed reports whether a displayed edge is excluded from path search:
// a saved return, or the automatic step that continues a caller's handler.
func notTraversed(edge *common.Edge) bool {
	return edge.IsOfType(common.Backtracking) || edge.IsOfType(common.SequenceContinuation)
}

func (g *MystGraph) AddEdge(edge *common.Edge, weight float64) error {
	if err := g.validateEdge(edge); err != nil {
		return err
	}
	if math.IsNaN(weight) || weight < 0 {
		return fmt.Errorf("edge weight must be non-negative, got %v", weight)
	}

	if _, ok := g.EdgeMap[edge.Source.GraphID]; !ok {
		g.EdgeMap[edge.Source.GraphID] = make(map[int64][]common.Edge)
	}

	// check if an identical edge already exists
	areAttributesEqual := func(attrs1, attrs2 []common.EdgeAttribute) bool {
		if len(attrs1) != len(attrs2) {
			return false
		}

		attrMap1 := make(map[common.EdgeAttribute]int)
		for _, attr := range attrs1 {
			attrMap1[attr]++
		}

		for _, attr := range attrs2 {
			if attrMap1[attr] == 0 {
				return false
			}
			attrMap1[attr]--
		}

		return true
	}

	lineID := len(g.EdgeMap[edge.Source.GraphID][edge.Target.GraphID])
	for i, existingEdge := range g.EdgeMap[edge.Source.GraphID][edge.Target.GraphID] {
		if existingEdge.TransitivityID == edge.TransitivityID &&
			areAttributesEqual(existingEdge.Attributes, edge.Attributes) {
			lineID = i
			break
		}
	}

	// Each metadata edge has a stable line ID within this source/target pair.
	// Re-adding an identical edge updates its weight; distinct parallel edges
	// retain separate lines rather than overwriting line zero.
	if edge.IsOfType(common.Disabled) {
		// Gonum cannot remove a line whose source/target ID set was never allocated.
		lines := g.Graph.WeightedLines(edge.Source.GraphID, edge.Target.GraphID)
		for lines.Next() {
			if lines.WeightedLine().ID() == int64(lineID) {
				g.Graph.RemoveLine(edge.Source.GraphID, edge.Target.GraphID, int64(lineID))
				break
			}
		}
	} else {
		if notTraversed(edge) {
			weight = math.Inf(1)
		}
		g.Graph.SetWeightedLine(multi.WeightedLine{
			F: g.Node(edge.Source.GraphID), T: g.Node(edge.Target.GraphID),
			W: weight, UID: int64(lineID),
		})
	}
	if lineID == len(g.EdgeMap[edge.Source.GraphID][edge.Target.GraphID]) {
		newEdge := common.Edge{
			Attributes:     slices.Clone(edge.Attributes),
			Source:         edge.Source,
			Target:         edge.Target,
			TransitivityID: edge.TransitivityID,
		}
		g.EdgeMap[edge.Source.GraphID][edge.Target.GraphID] = append(
			g.EdgeMap[edge.Source.GraphID][edge.Target.GraphID], newEdge)
		if !g.building {
			g.Metadata.Edges = append(g.Metadata.Edges, &newEdge)
		}
	}

	if !g.building {
		g.Metadata.TotalEdges = uint(g.Graph.Edges().Len())
	}
	return nil
}

// GetEdgeAttributes returns the attributes for the edge from sourceID to targetID
func (g *MystGraph) GetEdgeAttributes(sourceID, targetID int64) ([]common.EdgeAttribute, bool) {
	edges, exists := g.GetAllEdges(sourceID, targetID)
	if !exists {
		return nil, false
	}

	var allAttributes []common.EdgeAttribute
	for _, edge := range edges {
		allAttributes = append(allAttributes, edge.Attributes...)
	}

	return allAttributes, true
}

// AddEdgeAttribute adds an attribute to an edge in the metadata
func (g *MystGraph) AddEdgeAttribute(sourceID, targetID int64, attribute common.EdgeAttribute) bool {
	changed := false
	for _, edge := range g.Metadata.Edges {
		if edge.Source.GraphID == sourceID && edge.Target.GraphID == targetID {
			if !slices.Contains(edge.Attributes, attribute) {
				edge.Attributes = append(slices.Clone(edge.Attributes), attribute)
				changed = true
			}
		}
	}
	for i := range g.EdgeMap[sourceID][targetID] {
		edge := &g.EdgeMap[sourceID][targetID][i]
		if !slices.Contains(edge.Attributes, attribute) {
			edge.Attributes = append(slices.Clone(edge.Attributes), attribute)
			changed = true
		}
	}
	if changed && (attribute == common.Disabled || attribute == common.Backtracking || attribute == common.SequenceContinuation) {
		lines := g.Graph.WeightedLines(sourceID, targetID)
		for lines.Next() {
			line := lines.WeightedLine()
			if attribute == common.Disabled {
				g.Graph.RemoveLine(sourceID, targetID, line.ID())
			} else {
				g.Graph.SetWeightedLine(multi.WeightedLine{
					F: line.From(), T: line.To(), W: math.Inf(1), UID: line.ID(),
				})
			}
		}
	}
	if changed {
		g.Metadata.TotalEdges = uint(g.Graph.Edges().Len())
	}
	return changed
}

// GetAllEdges returns all edges from sourceID to targetID
func (g *MystGraph) GetAllEdges(sourceID, targetID int64) ([]common.Edge, bool) {
	if targetMap, ok := g.EdgeMap[sourceID]; ok {
		if edges, ok := targetMap[targetID]; ok && len(edges) > 0 {
			return edges, true
		}
	}
	return nil, false
}

// IsBacktrackingEdge checks if any edge between source and target is a backtracking edge
func (g *MystGraph) IsBacktrackingEdge(sourceID, targetID int64) bool {
	edges, exists := g.GetAllEdges(sourceID, targetID)
	if !exists {
		return false
	}

	for _, edge := range edges {
		if edge.IsOfType(common.Backtracking) {
			return true
		}
	}
	return false
}

// IsDisabledEdge checks if any edge between source and target is a disabled edge
func (g *MystGraph) IsDisabledEdge(sourceID, targetID int64) bool {
	edges, exists := g.GetAllEdges(sourceID, targetID)
	if !exists {
		return false
	}

	for _, edge := range edges {
		if edge.IsOfType(common.Disabled) {
			return true
		}
	}
	return false
}

// HasEdge checks if a directed edge exists from source to target node
func (g *MystGraph) HasEdge(fromName, toName string) bool {
	fromID, fromExists := g.GetNodeID(fromName)
	toID, toExists := g.GetNodeID(toName)

	if !fromExists || !toExists {
		return false
	}

	if g.Graph.HasEdgeFromTo(fromID, toID) {
		return true
	}

	// also check in EdgeMap (for disabled edges that might not be in the graph)
	if targetMap, ok := g.EdgeMap[fromID]; ok {
		edges, exists := targetMap[toID]
		return exists && len(edges) > 0
	}

	return false
}

func (g *MystGraph) validateEdge(edge *common.Edge) error {
	if edge == nil || edge.Source == nil || edge.Target == nil {
		return fmt.Errorf("edge and its source and target cannot be nil")
	}
	for _, endpoint := range []*common.Node{edge.Source, edge.Target} {
		node, ok := g.NodeMap[endpoint.Name]
		if !ok || node.GraphID != endpoint.GraphID {
			return fmt.Errorf("edge endpoint %q is not in the graph", endpoint.Name)
		}
	}
	return nil
}
