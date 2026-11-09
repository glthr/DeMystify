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

func TestAddingNodeAfterClearingNameMapping(t *testing.T) {
	g, _ := newTestGraph(t, 1)
	g.SetNameMapping(nil)
	if got := g.GetNameForID(0); got != "Node 0" {
		t.Fatalf("cleared mapping fallback = %q", got)
	}
	node := &common.Node{Name: "new card"}
	if err := g.AddNode(node); err != nil {
		t.Fatal(err)
	}
	if got, ok := g.GetNodeName(node.GraphID); !ok || got != node.Name {
		t.Fatalf("new node name = %q, %v", got, ok)
	}
	if got := g.Metadata.TotalNodes; got != 2 {
		t.Fatalf("node count = %d, want 2", got)
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

func TestRound2DisabledMutationUpdatesTotal(t *testing.T) {
	nodes := []*common.Node{{Name: "A"}, {Name: "B"}}
	edge := &common.Edge{Source: nodes[0], Target: nodes[1]}
	g, err := NewGraph(&common.Metadata{Nodes: nodes, Edges: []*common.Edge{edge}})
	if err != nil {
		t.Fatal(err)
	}
	g.AddEdgeAttribute(0, 1, common.Disabled)
	if g.Metadata.TotalEdges != 0 {
		t.Fatalf("disabled edge still counted: %d", g.Metadata.TotalEdges)
	}
}

func TestRound2ReaddingDisabledEdgeDoesNotEnableIt(t *testing.T) {
	nodes := []*common.Node{{Name: "A"}, {Name: "B"}}
	edge := &common.Edge{Source: nodes[0], Target: nodes[1]}
	g, err := NewGraph(&common.Metadata{Nodes: nodes, Edges: []*common.Edge{edge}})
	if err != nil {
		t.Fatal(err)
	}
	g.AddEdgeAttribute(0, 1, common.Disabled)
	if err := g.AddEdge(edge, 1); err != nil {
		t.Fatal(err)
	}
	if g.HasEdgeFromTo(0, 1) {
		t.Fatal("readding disabled metadata resurrected the physical line")
	}
}

func TestRound2AddedBacktrackingEdgeRespectsPolicy(t *testing.T) {
	g, nodes := newTestGraph(t, 2)
	if err := g.AddEdge(&common.Edge{Source: nodes[0], Target: nodes[1], Attributes: []common.EdgeAttribute{common.Backtracking}}, 1); err != nil {
		t.Fatal(err)
	}
	if p, err := g.ComputeShortestPath(0, 1, nil); err == nil {
		t.Fatalf("Backtracking attribute ignored: %+v", p)
	}
}

func TestRound2AnalysisClearsObsoleteNodeFlags(t *testing.T) {
	g, nodes := newTestGraph(t, 2)
	g.Process()
	if err := g.AddEdge(&common.Edge{Source: nodes[0], Target: nodes[1]}, 1); err != nil {
		t.Fatal(err)
	}
	g.Process()
	for i, n := range nodes {
		if n.IsOfType(common.IsIsolated) || g.NodeMap[n.Name].IsOfType(common.IsIsolated) {
			t.Errorf("connected node%d remains isolated", i)
		}
	}
	if nodes[0].IsOfType(common.IsSink) || nodes[1].IsOfType(common.IsSource) {
		t.Fatal("outdated sink/source flags survived reanalysis")
	}
}
