package graph

import (
	"github.com/glthr/DeMystify/common"
	"testing"
)

func TestRound2AddedNodeUpdatesMetadata(t *testing.T) {
	metadata := &common.Metadata{}
	g, err := NewGraph(metadata)
	if err != nil {
		t.Fatal(err)
	}
	node := &common.Node{Name: "new card"}
	if err := g.AddNode(node); err != nil {
		t.Fatal(err)
	}
	if len(metadata.Nodes) != 1 || metadata.Nodes[0] != node || metadata.TotalNodes != 1 {
		t.Fatalf("new node missing from metadata: nodes=%v total=%d", metadata.Nodes, metadata.TotalNodes)
	}
}

func TestRound2AddedEdgeUpdatesMetadata(t *testing.T) {
	g, nodes := newTestGraph(t, 2)
	edge := &common.Edge{Source: nodes[0], Target: nodes[1]}
	if err := g.AddEdge(edge, 1); err != nil {
		t.Fatal(err)
	}
	if len(g.Metadata.Edges) != 1 || g.Metadata.TotalEdges != 1 {
		t.Fatalf("new edge missing from metadata: edges=%v total=%d", g.Metadata.Edges, g.Metadata.TotalEdges)
	}
	if err := g.AddEdge(edge, 2); err != nil {
		t.Fatal(err)
	}
	if len(g.Metadata.Edges) != 1 || g.Metadata.TotalEdges != 1 {
		t.Fatal("duplicate edge changed metadata counts")
	}
}
