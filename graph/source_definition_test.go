package graph

import (
	"github.com/glthr/DeMystify/common"
	"slices"
	"testing"
)

func TestSourcesIgnoreSelfLoopsButRespectExternalIncomingEdges(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		loop, incoming, wantSource bool
		attributes                 []common.EdgeAttribute
	}{
		{name: "isolated remains source", wantSource: true},
		{name: "self loop only", loop: true, wantSource: true},
		{name: "external incoming", loop: true, incoming: true},
		{name: "disabled external incoming", loop: true, incoming: true, wantSource: true, attributes: []common.EdgeAttribute{common.Disabled}},
		{name: "backtracking external incoming", loop: true, incoming: true, attributes: []common.EdgeAttribute{common.Backtracking}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes := []*common.Node{{Name: "A"}, {Name: "B"}}
			metadata := &common.Metadata{Nodes: nodes}
			if tc.loop {
				metadata.Edges = append(metadata.Edges, &common.Edge{Source: nodes[0], Target: nodes[0]})
			}
			if tc.incoming {
				metadata.Edges = append(metadata.Edges, &common.Edge{Source: nodes[1], Target: nodes[0], Attributes: tc.attributes})
			}
			g, err := NewGraph(metadata)
			if err != nil {
				t.Fatal(err)
			}
			isA := func(n common.NodeInfo) bool { return n.ID == 0 }
			if got := slices.ContainsFunc(g.FindNodesWithNoIncomingEdges(), isA); got != tc.wantSource {
				t.Fatalf("source=%v want %v", got, tc.wantSource)
			}
			if nodes[0].IsOfType(common.IsSource) != tc.wantSource || g.NodeMap[nodes[0].Name].IsOfType(common.IsSource) != tc.wantSource {
				t.Fatal("source flag disagrees with analysis")
			}
			if tc.loop && (slices.ContainsFunc(g.FindNodesWithNoOutgoingEdges(), isA) || slices.ContainsFunc(g.FindIsolatedNodes(), isA)) {
				t.Fatal("self loop must retain existing sink and isolate behavior")
			}
			if !tc.loop && !slices.ContainsFunc(g.FindIsolatedNodes(), isA) {
				t.Fatal("isolated fixture no longer isolated")
			}
		})
	}
}

func TestSourceFlagTracksExternalIncomingChanges(t *testing.T) {
	g, nodes := newTestGraph(t, 2)
	if err := g.AddEdge(&common.Edge{Source: nodes[0], Target: nodes[0]}, 1); err != nil {
		t.Fatal(err)
	}
	g.FindNodesWithNoIncomingEdges()
	if !nodes[0].IsOfType(common.IsSource) {
		t.Fatal("self-referencing card should be a source")
	}
	if err := g.AddEdge(&common.Edge{Source: nodes[1], Target: nodes[0]}, 1); err != nil {
		t.Fatal(err)
	}
	g.FindNodesWithNoIncomingEdges()
	if nodes[0].IsOfType(common.IsSource) {
		t.Fatal("external incoming edge did not remove source status")
	}
	g.AddEdgeAttribute(1, 0, common.Disabled)
	g.FindNodesWithNoIncomingEdges()
	if !nodes[0].IsOfType(common.IsSource) {
		t.Fatal("disabling external incoming edge did not restore source status")
	}
}
