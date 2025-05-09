package graph

import (
	"slices"

	"github.com/glthr/DeMystify/common"

	"gonum.org/v1/gonum/graph/multi"
)

func (g *MystGraph) AddEdge(edge *common.Edge, weight float64) error {
	// NOTE: using multi.WeightedLine as it notably supports self-loops
	g.Graph.SetWeightedLine(multi.WeightedLine{
		F: multi.Node(edge.Source.GraphID),
		T: multi.Node(edge.Target.GraphID),
		W: weight,
	})

	if _, ok := g.EdgeMap[edge.Source.GraphID]; !ok {
		g.EdgeMap[edge.Source.GraphID] = make(map[int64][]common.Edge)
	}

	// check if an identical edge already exists
	areAttributesEqual := func(attrs1, attrs2 []common.EdgeAttribute) bool {
		if len(attrs1) != len(attrs2) {
			return false
		}

		attrMap1 := make(map[common.EdgeAttribute]bool)
		for _, attr := range attrs1 {
			attrMap1[attr] = true
		}

		for _, attr := range attrs2 {
			if !attrMap1[attr] {
				return false
			}
		}

		return true
	}

	isDuplicate := false
	if edges, ok := g.EdgeMap[edge.Source.GraphID][edge.Target.GraphID]; ok {
		for _, existingEdge := range edges {
			if existingEdge.TransitivityID == edge.TransitivityID &&
				areAttributesEqual(existingEdge.Attributes, edge.Attributes) {
				isDuplicate = true
				break
			}
		}
	}

	// only add to the edge map if it is not a duplicate
	if !isDuplicate {
		newEdge := common.Edge{
			Attributes:     edge.Attributes,
			Source:         edge.Source,
			Target:         edge.Target,
			TransitivityID: edge.TransitivityID,
		}
		g.EdgeMap[edge.Source.GraphID][edge.Target.GraphID] = append(
			g.EdgeMap[edge.Source.GraphID][edge.Target.GraphID], newEdge)
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
	for i, edge := range g.Metadata.Edges {
		if edge.Source.GraphID == sourceID && edge.Target.GraphID == targetID {
			for _, attr := range edge.Attributes {
				if attr == attribute {
					return false // attribute already exists
				}
			}

			g.Metadata.Edges[i].Attributes = append(g.Metadata.Edges[i].Attributes, attribute)

			// update the edge map as well
			if targetMap, ok := g.EdgeMap[sourceID]; ok {
				if edges, ok := targetMap[targetID]; ok {
					for j := range edges {
						edges[j].Attributes = append(edges[j].Attributes, attribute)
					}
					targetMap[targetID] = edges
				}
			}

			return true
		}
	}

	return false
}

// GetAllEdges returns all edges from sourceID to targetID
func (g *MystGraph) GetAllEdges(sourceID, targetID int64) ([]common.Edge, bool) {
	if targetMap, ok := g.EdgeMap[sourceID]; ok {
		if edges, ok := targetMap[targetID]; ok && len(edges) > 0 {

			// sort edges by ID before returning
			slices.SortFunc(edges, func(a, b common.Edge) int {
				if a.Source.GraphID < b.Source.GraphID {
					return -1
				} else if a.Source.GraphID > b.Source.GraphID {
					return 1
				}
				return 0
			})
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
