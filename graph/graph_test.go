package graph

import (
	"github.com/glthr/DeMystify/common"
	"testing"
)

func newTestGraph(t *testing.T, count int) (*MystGraph, []*common.Node) {
	t.Helper()
	nodes := make([]*common.Node, count)
	for i := range nodes {
		nodes[i] = &common.Node{Name: string(rune('A' + i)), StackName: "Test Age"}
	}
	g, err := NewGraph(&common.Metadata{Nodes: nodes})
	if err != nil {
		t.Fatal(err)
	}
	return g, nodes
}
