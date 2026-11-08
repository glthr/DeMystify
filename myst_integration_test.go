package main

import (
	"os"
	"testing"

	"github.com/glthr/DeMystify/graph"
	"github.com/glthr/DeMystify/parser"
)

// TestOriginalMystImport is opt-in: original game files must stay outside Git.
func TestOriginalMystImport(t *testing.T) {
	dir := os.Getenv("DEMYSTIFY_TEST_DATA")
	if dir == "" {
		t.Skip("set DEMYSTIFY_TEST_DATA to the converted six-stack directory")
	}
	p, err := parser.NewParser(dir)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := p.Process()
	if err != nil {
		t.Fatal(err)
	}
	g, err := graph.NewGraph(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.TotalStacks != 6 || metadata.TotalCards != 1355 || metadata.TotalNodes != 1364 || metadata.TotalEdges != 3214 || len(metadata.Edges) != 3227 {
		t.Fatalf("unexpected graph size: stacks=%d cards=%d nodes=%d active pairs=%d records=%d", metadata.TotalStacks, metadata.TotalCards, metadata.TotalNodes, metadata.TotalEdges, len(metadata.Edges))
	}
	if sources, sinks, isolated := len(g.FindNodesWithNoIncomingEdges()), len(g.FindNodesWithNoOutgoingEdges()), len(g.FindIsolatedNodes()); sources != 36 || sinks != 62 || isolated != 18 {
		t.Fatalf("unexpected source/sink/isolated counts: %d/%d/%d", sources, sinks, isolated)
	}
	for _, pair := range [][2]string{
		{"Channelwood Age:100894", "Channelwood Age:26005"},
		{"Dunny Age:11088", "Dunny Age:2203"},
		{"Mechanical Age:46649", "Mechanical Age:47045"},
		{"Myst:28552", "Myst:23444"},
		{"Myst:55938", "Myst:23444"},
		{"Myst:77008", "Myst:23444"},
		{"Selenitic Age:59926", "Selenitic Age:62357"},
		{"Stoneship Age:53570", "Stoneship Age:25833"},
	} {
		from, fromExists := g.GetNodeID(pair[0])
		to, toExists := g.GetNodeID(pair[1])
		weight, exists := g.Weight(from, to)
		if !fromExists || !toExists || !exists || weight != 1 {
			t.Errorf("missing named-card transition %s -> %s", pair[0], pair[1])
		}
	}
	for _, tc := range []struct {
		fromStack, toStack string
		from, to           int
		distance           float64
		reachable          bool
	}{
		{"Myst", "Dunny Age", 8336, 11088, 24, true},
		{"Myst", "Myst", 28679, 44018, 14, true},
		{"Dunny Age", "Dunny Age", 7365, 11283, 0, false},
	} {
		path, err := ComputeShortestPath(p, g, tc.fromStack, tc.from, tc.toStack, tc.to)
		if (err == nil) != tc.reachable {
			t.Errorf("%s:%d -> %s:%d: unexpected reachability: %v", tc.fromStack, tc.from, tc.toStack, tc.to, err)
		} else if tc.reachable && (path.Distance != tc.distance || len(path.Path) != int(tc.distance)+1) {
			t.Errorf("%s:%d -> %s:%d: got %+v, want distance %v", tc.fromStack, tc.from, tc.toStack, tc.to, path, tc.distance)
		}
	}
}
