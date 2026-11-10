package main

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
	"github.com/glthr/DeMystify/parser"
)

// TestOriginalMystImport is opt-in: original game files must stay outside Git.
func TestOriginalMystImport(t *testing.T) {
	dir := os.Getenv("DEMYSTIFY_TEST_DATA")
	if dir == "" {
		t.Skip("set DEMYSTIFY_TEST_DATA to the six original stacks or to their XML conversion")
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
	wantPairs, wantRecords, fresh := mystInputProfile(t, dir)
	if metadata.TotalStacks != 6 || metadata.TotalCards != 1355 || metadata.TotalNodes != 1364 || metadata.TotalEdges != wantPairs || len(metadata.Edges) != wantRecords {
		t.Fatalf("unexpected graph size: stacks=%d cards=%d nodes=%d active pairs=%d records=%d", metadata.TotalStacks, metadata.TotalCards, metadata.TotalNodes, metadata.TotalEdges, len(metadata.Edges))
	}
	if sources, sinks, isolated := len(g.FindNodesWithNoIncomingEdges()), len(g.FindNodesWithNoOutgoingEdges()), len(g.FindIsolatedNodes()); sources != 37 || sinks != 51 || isolated != 19 {
		t.Fatalf("unexpected source/sink/isolated counts: %d/%d/%d", sources, sinks, isolated)
	}
	for _, pair := range [][2]string{
		{"Channelwood Age:100894", "Channelwood Age:26005"},
		{"Dunny Age:11088", "Dunny Age:2203"},
		{"Mechanical Age:46649", "Mechanical Age:47045"},
		{"Myst:28552", "Myst:23444"},
		{"Myst:55938", "Myst:23444"},
		{"Myst:77008", "Myst:23444"},
		{"Myst:72560", "Myst:23444"},
		{"Myst:77588", "Myst:23444"},
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
	// An intermediate elevator view continues to the level chosen by its caller,
	// whether the script spells the transition as vs left or vs "left". The
	// caller keeps its own reference to that level.
	for _, pair := range [][2]string{
		{"Channelwood Age:45661", "Channelwood Age:44320"},
		{"Channelwood Age:96885", "Channelwood Age:36337"},
		{"Channelwood Age:96885", "Channelwood Age:84124"},
		{"Channelwood Age:23467", "Channelwood Age:36337"},
		{"Channelwood Age:46136", "Channelwood Age:84124"},
		// Sound, display and wait commands between two requests do not end a
		// sequence: the electrified cage and the fireplace are transition views.
		{"Mechanical Age:32201", "Mechanical Age:31976"},
		{"Myst:87741", "Myst:56926"},
		{"Myst:49781", "Myst:56926"},
		// The fireplace button redisplays its own card before it shows the
		// closed door and the lit panel; that redraw hides neither request.
		{"Myst:61587", "Myst:63906"},
	} {
		if !g.HasEdge(pair[0], pair[1]) {
			t.Errorf("missing sequence reference %s -> %s", pair[0], pair[1])
		}
	}
	// The complete list, so that a record cannot gain or lose the attribute
	// unnoticed, and a looser rule cannot join two requests across a helper
	// that navigates or across two alternatives of one condition.
	var steps []string
	classes := make(map[common.EdgeAttribute]int)
	for _, edge := range metadata.Edges {
		if edge.IsOfType(common.SequenceContinuation) {
			steps = append(steps, edge.Source.Name+" -> "+edge.Target.Name)
		}
		for _, attribute := range edge.Attributes {
			classes[attribute]++
		}
	}
	slices.Sort(steps)
	if !slices.Equal(steps, automaticNextSteps) {
		for _, step := range steps {
			if !slices.Contains(automaticNextSteps, step) {
				t.Errorf("unexpected automatic next step %s", step)
			}
		}
		for _, step := range automaticNextSteps {
			if !slices.Contains(steps, step) {
				t.Errorf("missing automatic next step %s", step)
			}
		}
	}
	if classes[common.Backtracking] != 47 || classes[common.Disabled] != 26 || classes[common.NotImplemented] != 3 || classes[common.CrossAge] != 21 ||
		classes[common.RestrictiveTransitivityTail] != 5 || classes[common.RestrictiveTransitivityHead] != 5 {
		t.Errorf("unexpected record classes: saved returns=%d disabled=%d absent targets=%d between Ages=%d paired tails=%d paired heads=%d",
			classes[common.Backtracking], classes[common.Disabled], classes[common.NotImplemented], classes[common.CrossAge],
			classes[common.RestrictiveTransitivityTail], classes[common.RestrictiveTransitivityHead])
	}
	// Eleven marker buttons and six lever buttons hold a body whose handler
	// declaration was commented out: 17 of the 26 disabled records.
	if self := len(g.FindNodesWithSelfLoops()); self != 9 {
		t.Errorf("got %d nodes with an active reference to themselves, want 9", self)
	}
	verifySharedStackHandlerEntries(t, g)
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
		{"Selenitic Age", "Channelwood Age", 26154, 88814, 130, true},
		{"Myst", "Myst", 28679, 44018, 14, true},
		// A view shared by two callers does not join their routes: reaching the
		// other elevator level still takes the ride, and path search does not
		// continue from the intermediate view itself.
		{"Channelwood Age", "Channelwood Age", 23467, 84124, 4, true},
		{"Channelwood Age", "Channelwood Age", 96885, 84124, 0, false},
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

// The 31 automatic next steps of the six stacks, sorted.
var automaticNextSteps = []string{
	"Channelwood Age:45661 -> Channelwood Age:44320",
	"Channelwood Age:45661 -> Channelwood Age:85222",
	"Channelwood Age:46695 -> Channelwood Age:89646",
	"Channelwood Age:85801 -> Channelwood Age:101798",
	"Channelwood Age:85801 -> Channelwood Age:85642",
	"Channelwood Age:96885 -> Channelwood Age:36337",
	"Channelwood Age:96885 -> Channelwood Age:84124",
	"Channelwood Age:97270 -> Channelwood Age:84244",
	"Channelwood Age:98476 -> Channelwood Age:101798",
	"Dunny Age:6997 -> Dunny Age:7365",
	"Dunny Age:8424 -> Dunny Age:8487",
	"Mechanical Age:31976 -> Mechanical Age:14628",
	"Mechanical Age:32201 -> Mechanical Age:31976",
	"Mechanical Age:43632 -> Mechanical Age:31555",
	"Mechanical Age:49328 -> Mechanical Age:39710",
	"Mechanical Age:49328 -> Mechanical Age:43114",
	"Mechanical Age:50114 -> Mechanical Age:50726",
	"Mechanical Age:50726 -> Mechanical Age:48777",
	"Mechanical Age:50726 -> Mechanical Age:49567",
	"Myst:37618 -> Myst:37341",
	"Myst:37752 -> Myst:37618",
	"Myst:57929 -> Myst:45039",
	"Myst:60085 -> Myst:60372",
	"Myst:61587 -> Myst:63906",
	"Myst:85634 -> Myst:52926",
	"Myst:85634 -> Myst:64906",
	"Myst:87741 -> Myst:56926",
	"Myst:90595 -> Myst:88671",
	"Myst:90595 -> Myst:89742",
	"Myst:90595 -> Myst:90877",
	"Selenitic Age:7689 -> Selenitic Age:7990",
}

// Identify the two verified input snapshots from the loaded stacks themselves,
// independently of graph output, so that the original stacks are accepted as
// well as an XML conversion of them.
func mystInputProfile(t *testing.T, dir string) (uint, int, bool) {
	t.Helper()
	corpus, err := parser.LoadInput(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, stack := range corpus.Stacks {
		if stack.Name != "Dunny Age" {
			continue
		}
		for _, card := range stack.Cards {
			if card.ID != 5200 {
				continue
			}
			for _, part := range card.Parts {
				if part.ID != 2 {
					continue
				}
				switch strings.ToLower(strings.Join(strings.Fields(part.Script.Text), " ")) {
				case "on mouseup 5119 end mouseup":
					return 3238, 3281, false
				case "on mouseup go to card id 5119 end mouseup":
					return 3239, 3282, true
				}
			}
		}
	}
	t.Fatal("unrecognized Dunny:5200 part-2 script; verify the original-data profile")
	return 0, 0, false
}

func verifySharedStackHandlerEntries(t *testing.T, g *graph.MystGraph) {
	t.Helper()
	targets := make(map[string]bool)
	for target, callers := range map[int][]int{
		86839: {13703, 19143, 20137, 21498, 26492, 33578},
		87249: {17667, 32368, 2465, 24265, 29197, 34505},
		87418: {14360, 19870, 20934, 22077, 27330, 35578},
	} {
		toName := fmt.Sprintf("Channelwood Age:%d", target)
		targets[toName] = true
		if g.HasEdge("Channelwood Age", toName) {
			t.Errorf("handler definition became a world move to %s", toName)
		}
		for _, caller := range callers {
			fromName := fmt.Sprintf("Channelwood Age:%d", caller)
			from, fromExists := g.GetNodeID(fromName)
			to, toExists := g.GetNodeID(toName)
			weight, exists := g.Weight(from, to)
			if !fromExists || !toExists || !exists || !math.IsInf(weight, 1) {
				t.Errorf("missing contextual entry %s -> %s", fromName, toName)
			}
			if g.HasEdge(toName, fromName) {
				t.Errorf("invented unconditional return %s -> %s", toName, fromName)
			}
		}
	}
	entries := 0
	for _, edge := range g.Metadata.Edges {
		if targets[edge.Target.Name] {
			entries++
			if !edge.Source.IsOfType(common.IsCard) || !edge.IsOfType(common.Backtracking) {
				t.Errorf("incorrect close-up entry: %+v", edge)
			}
		}
	}
	if entries != 18 {
		t.Errorf("got %d close-up entries, want exactly 18 callers", entries)
	}
}
