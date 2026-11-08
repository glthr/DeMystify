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
