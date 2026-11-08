package main

import (
	"encoding/xml"
	"os"
	"strings"
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
	wantPairs, wantRecords, fresh := mystInputProfile(t, p)
	if metadata.TotalStacks != 6 || metadata.TotalCards != 1355 || metadata.TotalNodes != 1364 || metadata.TotalEdges != wantPairs || len(metadata.Edges) != wantRecords {
		t.Fatalf("unexpected graph size: stacks=%d cards=%d nodes=%d active pairs=%d records=%d", metadata.TotalStacks, metadata.TotalCards, metadata.TotalNodes, metadata.TotalEdges, len(metadata.Edges))
	}
	if sources, sinks, isolated := len(g.FindNodesWithNoIncomingEdges()), len(g.FindNodesWithNoOutgoingEdges()), len(g.FindIsolatedNodes()); sources != 37 || sinks != 62 || isolated != 18 {
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
	localDistance := float64(3)
	if fresh {
		localDistance = 1
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
		{"Dunny Age", "Dunny Age", 5200, 5119, localDistance, true},
	} {
		path, err := ComputeShortestPath(p, g, tc.fromStack, tc.from, tc.toStack, tc.to)
		if (err == nil) != tc.reachable {
			t.Errorf("%s:%d -> %s:%d: unexpected reachability: %v", tc.fromStack, tc.from, tc.toStack, tc.to, err)
		} else if tc.reachable && (path.Distance != tc.distance || len(path.Path) != int(tc.distance)+1) {
			t.Errorf("%s:%d -> %s:%d: got %+v, want distance %v", tc.fromStack, tc.from, tc.toStack, tc.to, path, tc.distance)
		}
	}
}

// Identify the two verified input snapshots from XML, independently of graph output.
func mystInputProfile(t *testing.T, p *parser.Parser) (uint, int, bool) {
	t.Helper()
	card, err := p.GetCardByStackAndID("Dunny Age", 5200)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(card.Filepath)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Parts []struct {
			ID     int    `xml:"id"`
			Script string `xml:"script"`
		} `xml:"part"`
	}
	if err := xml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, part := range raw.Parts {
		if part.ID != 2 {
			continue
		}
		switch strings.ToLower(strings.Join(strings.Fields(part.Script), " ")) {
		case "on mouseup 5119 end mouseup":
			return 3214, 3227, false
		case "on mouseup go to card id 5119 end mouseup":
			return 3215, 3228, true
		}
	}
	t.Fatal("unrecognized Dunny:5200 part-2 script; verify the original-data profile")
	return 0, 0, false
}
