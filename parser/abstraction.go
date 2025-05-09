package parser

import (
	"errors"
	"fmt"

	"github.com/glthr/DeMystify/common"
)

// Process transforms cards and stacks objects into a proto graph
// (first step of abstraction)
func (p *Parser) Process() (*common.Metadata, error) {
	metadata := &common.Metadata{}

	// create nodes
	for _, stack := range p.stacks {
		node := &common.Node{
			Attributes: []common.NodeAttribute{common.IsStack},
			Name:       stack.Name,
		}

		metadata.Nodes = append(metadata.Nodes, node)
	}

	for _, card := range p.cards {
		node := &common.Node{
			Attributes:    []common.NodeAttribute{common.IsCard},
			StackName:     card.Stack.Name,
			Name:          card.Name,
			OriginalName:  card.OriginalName,
			SecondaryName: card.Background,
		}

		if card.HasBluePage {
			node.Attributes = append(node.Attributes, common.ContainsBluePage)
		}

		if card.HasRedPage {
			node.Attributes = append(node.Attributes, common.ContainsRedPage)
		}

		if card.HasWhitePage {
			node.Attributes = append(node.Attributes, common.ContainsWhitePage)
		}

		metadata.Nodes = append(metadata.Nodes, node)
	}

	// create edges
	edgeExists := make(map[string]bool)
	for _, link := range p.links {
		sourceNode, sourceStack := findNodeStack(metadata.Nodes, link.Source)
		targetNode, targetStack := findNodeStack(metadata.Nodes, link.Target)

		if targetNode == nil {
			// create virtual card if target does not exist
			if cardTarget, ok := link.Target.(*HyperCardCard); ok {
				targetNode = &common.Node{
					Attributes: []common.NodeAttribute{common.IsCard, common.IsVirtual},
					StackName:  cardTarget.Stack.Name,
					Name:       cardTarget.Name,
				}

				metadata.Nodes = append(metadata.Nodes, targetNode)
			} else {
				return nil, errors.New("target node not found")
			}
		}

		if sourceNode == nil {
			return nil, errors.New("source node not found")
		}

		edge := &common.Edge{
			Attributes:     getEdgeAttributes(link, sourceStack, targetStack),
			Source:         sourceNode,
			Target:         targetNode,
			TransitivityID: link.TransitivityID,
		}

		key := fmt.Sprintf("%s-%s:%v:%d", edge.Source.Name, edge.Target.Name, edge.Attributes, edge.TransitivityID)
		if !edgeExists[key] {
			// only record an edge once, as an identical link can appear multiple time in a script
			metadata.Edges = append(metadata.Edges, edge)
			edgeExists[key] = true
		}
	}

	metadata.TotalCards = uint(len(p.cards))
	metadata.TotalStacks = uint(len(p.stacks))

	return metadata, nil
}

func findNodeStack(nodes []*common.Node, nodeObj any) (*common.Node, *HyperCardStack) {
	for _, node := range nodes {
		switch v := nodeObj.(type) {
		case *HyperCardStack:
			if v.Name == node.StackName {
				return node, v
			}
		case *HyperCardCard:
			if v.Name == node.Name {
				return node, v.Stack
			}
		}
	}
	return nil, nil
}

func getEdgeAttributes(
	link *HyperCardLink,
	sourceStack, targetStack *HyperCardStack,
) []common.EdgeAttribute {
	var edgeTypes []common.EdgeAttribute

	if link.IsDisabled {
		edgeTypes = append(edgeTypes, common.Disabled)
	}

	if link.IsCrossAges {
		edgeTypes = append(edgeTypes, common.CrossAge)
	} else {
		edgeTypes = append(edgeTypes, common.IntraAge)
	}

	if link.IsNotImplemented {
		edgeTypes = append(edgeTypes, common.NotImplemented)
	}

	if sourceStack == targetStack && link.IsBacktracking && link.TransitivityRank == common.DefaultTransitivity {
		edgeTypes = append(edgeTypes, common.Backtracking)
	}

	if link.TransitivityRank == common.Tail {
		edgeTypes = append(edgeTypes, common.RestrictiveTransitivityTail)
	}

	if link.TransitivityRank == common.Head {
		edgeTypes = append(edgeTypes, common.RestrictiveTransitivityHead)
	}

	return edgeTypes
}
