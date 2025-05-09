package graph

import (
	"fmt"
	"github.com/glthr/DeMystify/common"
	"math"

	gograph "gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/multi"
)

type MystGraph struct {
	Graph      *multi.WeightedDirectedGraph
	NodeMap    map[string]common.Node
	Metadata   *common.Metadata
	EdgeMap    map[int64]map[int64][]common.Edge
	IdNameMap  map[int64]string
	IdStackMap map[int64]string

	traverser    *traverser
	nodeAnalyzer *nodesAnalyzer
	pathAnalyzer *pathAnalyzer
}

func NewGraph(metadata *common.Metadata) (*MystGraph, error) {
	if metadata == nil {
		return nil, fmt.Errorf("metadata cannot be nil")
	}

	g := &MystGraph{
		Graph:      multi.NewWeightedDirectedGraph(),
		Metadata:   metadata,
		NodeMap:    make(map[string]common.Node, len(metadata.Nodes)),
		EdgeMap:    make(map[int64]map[int64][]common.Edge),
		IdNameMap:  make(map[int64]string, len(metadata.Nodes)),
		IdStackMap: make(map[int64]string, len(metadata.Nodes)),
	}

	// add nodes
	for _, node := range metadata.Nodes {
		if err := g.AddNode(node); err != nil {
			return nil, fmt.Errorf("failed to add node: %w", err)
		}
	}

	// add edges
	for _, edge := range metadata.Edges {
		if err := g.addEdgeWithAttributes(edge); err != nil {
			return nil, fmt.Errorf("failed to add edge: %w", err)
		}
	}

	// initialize analyzers
	g.traverser = newTraverser(g)
	g.nodeAnalyzer = newNodeAnalyzer(g)
	g.pathAnalyzer = newPathAnalyzer(g)

	// update metadata
	metadata.TotalNodes = uint(g.Graph.Nodes().Len())
	metadata.TotalEdges = uint(g.Graph.Edges().Len())

	return g, nil
}

// addEdgeWithAttributes adds an edge to the graph based on its attributes
func (g *MystGraph) addEdgeWithAttributes(edge *common.Edge) error {
	switch {
	case edge.IsOfType(common.Disabled):
		return g.addDisabledEdge(edge)
	case edge.IsOfType(common.Backtracking):
		return g.AddEdge(edge, math.Inf(1))
	default:
		return g.AddEdge(edge, 1.0)
	}
}

// addDisabledEdge adds a disabled edge to the EdgeMap without adding it to the graph
func (g *MystGraph) addDisabledEdge(edge *common.Edge) error {
	if _, ok := g.EdgeMap[edge.Source.GraphID]; !ok {
		g.EdgeMap[edge.Source.GraphID] = make(map[int64][]common.Edge)
	}
	g.EdgeMap[edge.Source.GraphID][edge.Target.GraphID] = append(
		g.EdgeMap[edge.Source.GraphID][edge.Target.GraphID],
		*edge,
	)
	return nil
}

// Node returns the node with the given ID
func (g *MystGraph) Node(id int64) gograph.Node {
	return g.Graph.Node(id)
}

// Nodes returns all nodes in the graph
func (g *MystGraph) Nodes() gograph.Nodes {
	return g.Graph.Nodes()
}

// From returns all nodes that can be reached directly from the node with the given ID
func (g *MystGraph) From(id int64) gograph.Nodes {
	return g.Graph.From(id)
}

// To returns all nodes that directly reach to the node with the given ID
func (g *MystGraph) To(id int64) gograph.Nodes {
	return g.Graph.To(id)
}

// HasEdgeBetween returns whether an edge exists between nodes with the given IDs
// regardless of the direction
func (g *MystGraph) HasEdgeBetween(xid, yid int64) bool {
	return g.Graph.HasEdgeBetween(xid, yid)
}

// HasEdgeFromTo returns whether an edge exists from u to v, with IDs uid and vid
func (g *MystGraph) HasEdgeFromTo(uid, vid int64) bool {
	return g.Graph.HasEdgeFromTo(uid, vid)
}

// Edge returns the edge from u to v, with IDs uid and vid, if such an edge exists
func (g *MystGraph) Edge(uid, vid int64) gograph.Edge {
	return g.Graph.Edge(uid, vid)
}
