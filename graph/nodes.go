package graph

import (
	"fmt"
	"github.com/glthr/DeMystify/common"
	"gonum.org/v1/gonum/graph/multi"
)

// GetNodeFromID returns the node with the given ID
func (g *MystGraph) GetNodeFromID(ID int64) (*common.Node, error) {
	for _, node := range g.NodeMap {
		if node.GraphID == ID {
			return &node, nil
		}
	}

	return nil, common.NodeNotFoundErr
}

// AddNode adds a new node to the graph
func (g *MystGraph) AddNode(node *common.Node) error {
	// check if a node already exists
	if _, exists := g.NodeMap[node.Name]; exists {
		return common.NodeAlreadyExistsErr
	}

	// create a new node with the next available ID
	node.GraphID = g.Graph.NewNode().ID()
	g.Graph.AddNode(multi.Node(node.GraphID))

	g.NodeMap[node.Name] = *node
	g.IdNameMap[node.GraphID] = node.Name

	return nil
}

// GetNodeID returns the ID for a given node name
func (g *MystGraph) GetNodeID(name string) (int64, bool) {
	node, exists := g.NodeMap[name]
	if !exists {
		return -1, false
	}
	return node.GraphID, true
}

// GetNodeName returns the name for a given node ID
func (g *MystGraph) GetNodeName(id int64) (string, bool) {
	name, exists := g.IdNameMap[id]
	return name, exists
}

// GetNodeStack returns the stack for a given node ID
func (g *MystGraph) GetNodeStack(id int64) (string, bool) {
	stack, exists := g.IdStackMap[id]
	return stack, exists
}

// SetNameMapping sets or updates the ID to name mapping
func (g *MystGraph) SetNameMapping(idToName map[int64]string) {
	g.IdNameMap = idToName
}

// GetNameForID returns the name for a given node ID
func (g *MystGraph) GetNameForID(id int64) string {
	if g.IdNameMap == nil {
		return fmt.Sprintf("Node %d", id)
	}

	if name, ok := g.IdNameMap[id]; ok {
		return name
	}
	return fmt.Sprintf("Node %d", id)
}

// addNodeAttribute adds an attribute to a node in the metadata
func (g *MystGraph) addNodeAttribute(id int64, attribute common.NodeAttribute) {
	// iterate through the nodes in the metadata to find the matching one
	for i := range g.Metadata.Nodes {
		if g.Metadata.Nodes[i].GraphID == id {
			for _, attr := range g.Metadata.Nodes[i].Attributes {
				if attr == attribute {
					return // Attribute already exists, no need to add
				}
			}

			g.Metadata.Nodes[i].Attributes = append(g.Metadata.Nodes[i].Attributes, attribute)
			break
		}
	}
}

// AllNodes returns all node names in the graph
func (g *MystGraph) AllNodes() []string {
	var names []string
	nodes := g.Graph.Nodes()
	for nodes.Next() {
		node := nodes.Node()
		if name, ok := g.GetNodeName(node.ID()); ok {
			names = append(names, name)
		}
	}
	return names
}

// GetSuccessors returns all nodes that are targets of edges from the given node
func (g *MystGraph) GetSuccessors(name string) ([]string, error) {
	id, exists := g.GetNodeID(name)
	if !exists {
		return nil, fmt.Errorf("node '%s' not found in graph", name)
	}

	var successors []string
	nodes := g.Graph.From(id)
	for nodes.Next() {
		node := nodes.Node()
		if name, ok := g.GetNodeName(node.ID()); ok {
			successors = append(successors, name)
		}
	}

	return successors, nil
}

// GetPredecessors returns all nodes that have edges pointing to the given node
func (g *MystGraph) GetPredecessors(name string) ([]string, error) {
	id, exists := g.GetNodeID(name)
	if !exists {
		return nil, fmt.Errorf("node '%s' not found in graph", name)
	}

	var predecessors []string
	nodes := g.Graph.To(id)
	for nodes.Next() {
		node := nodes.Node()
		if name, ok := g.GetNodeName(node.ID()); ok {
			predecessors = append(predecessors, name)
		}
	}

	return predecessors, nil
}
