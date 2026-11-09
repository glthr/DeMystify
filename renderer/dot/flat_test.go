package dot

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
	pdflayout "github.com/glthr/DeMystify/renderer/layout"
)

func newFlatTestGenerator(t *testing.T) (*Generator, *common.Metadata) {
	t.Helper()
	original, secondary := "quoted \"view\"", "picture, label=kept"
	a := &common.Node{Name: "Myst:1", StackName: "Myst", OriginalName: &original, SecondaryName: &secondary, Attributes: []common.NodeAttribute{common.IsCard, common.IsSource, common.ContainsBluePage}}
	b := &common.Node{Name: "Myst:2", StackName: "Myst", Attributes: []common.NodeAttribute{common.IsCard, common.IsSink, common.ContainsRedPage}}
	c := &common.Node{Name: "Other:3", StackName: "Other", Attributes: []common.NodeAttribute{common.IsCard, common.IsIsolated, common.ContainsWhitePage}}
	d := &common.Node{Name: "Other", StackName: "Other", Attributes: []common.NodeAttribute{common.IsStack}}
	metadata := &common.Metadata{Nodes: []*common.Node{a, b, c, d}, Edges: []*common.Edge{
		{Source: a, Target: b, Attributes: []common.EdgeAttribute{common.IntraAge}},
		{Source: b, Target: a, Attributes: []common.EdgeAttribute{common.IntraAge}},
		{Source: a, Target: b, Attributes: []common.EdgeAttribute{common.Disabled}},
		{Source: a, Target: a, Attributes: []common.EdgeAttribute{common.SelfReference}},
		{Source: a, Target: b, Attributes: []common.EdgeAttribute{common.RestrictiveTransitivityTail}, TransitivityID: 7},
		{Source: d, Target: a, Attributes: []common.EdgeAttribute{common.Backtracking}},
	}}
	model, err := graph.NewGraph(metadata)
	if err != nil {
		t.Fatal(err)
	}
	metadata.Stats.ConnectedComponents = [][]int64{{a.GraphID, b.GraphID, d.GraphID}, {c.GraphID}}
	return NewGenerator(model, metadata), metadata
}

func TestOriginalGenerateByteCompatibility(t *testing.T) {
	g, metadata := newFlatTestGenerator(t)
	// Recorded before extracting shared style preparation or adding positions.
	for _, test := range []struct {
		path []int64
		hash string
	}{
		{nil, "faf1578d838342308d1a3e4da5bc1c6fa053fb5fec12a676061501c82412c470"},
		{[]int64{metadata.Nodes[0].GraphID, metadata.Nodes[1].GraphID, metadata.Nodes[0].GraphID}, "72bc0b947c7edc877ffbe14ef1bdd2db1e19cdb2712669fd65c0497db8e43bc1"},
	} {
		output, err := g.Generate(test.path)
		if err != nil {
			t.Fatal(err)
		}
		if hash := fmt.Sprintf("%x", sha256.Sum256([]byte(output))); hash != test.hash {
			t.Fatalf("original Generate changed for path %v: got %s, want %s", test.path, hash, test.hash)
		}
	}
}

var numericStatement = regexp.MustCompile(`^\s*"-?\d+" (\[|-> )`)
var positionAttribute = regexp.MustCompile(`, pos="[^"]+"`)

func visibleStatements(output string) []string {
	var result []string
	for _, line := range strings.Split(output, "\n") {
		if numericStatement.MatchString(line) && !strings.Contains(line, "style=invis") && !strings.Contains(line, "[constraint=true]") {
			result = append(result, strings.TrimSpace(line))
		}
	}
	return result
}

func assertNoComponentControls(t *testing.T, output string) {
	t.Helper()
	for _, forbidden := range []string{"subgraph", "style=invis", `"__`} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("flat output retained %q:\n%s", forbidden, output)
		}
	}
}

func TestFlatRetainsVisibleStylesAndRecords(t *testing.T) {
	for _, config := range []Config{DefaultConfig(), {}, {UseNodeNames: true, ColorByStack: true}, {IncludeAnalysis: true, HighlightSources: true, HighlightSinks: false}} {
		g, metadata := newFlatTestGenerator(t)
		g.config = config
		for _, path := range [][]int64{nil, {metadata.Nodes[0].GraphID, metadata.Nodes[1].GraphID, 99, metadata.Nodes[0].GraphID}} {
			original, err := g.Generate(path)
			if err != nil {
				t.Fatal(err)
			}
			flat, err := g.GenerateFlat(path)
			if err != nil {
				t.Fatal(err)
			}
			assertNoComponentControls(t, flat)
			if !reflect.DeepEqual(visibleStatements(original), visibleStatements(flat)) {
				t.Fatalf("flat output changed visible nodes or records for config %+v, path %v", config, path)
			}
			if strings.Contains(flat, "pos=") {
				t.Fatal("flat output unexpectedly pins nodes")
			}
		}
	}
}

func fixturePositions(metadata *common.Metadata) map[int64]pdflayout.Point {
	positions := make(map[int64]pdflayout.Point)
	for index, node := range metadata.Nodes {
		positions[node.GraphID] = pdflayout.Point{X: float64(index) * 200, Y: float64(index%2) * 100}
	}
	return positions
}

func TestPositionedRetainsVisibleStylesAndTopDownCoordinates(t *testing.T) {
	g, metadata := newFlatTestGenerator(t)
	positions := fixturePositions(metadata)
	positions[metadata.Nodes[0].GraphID] = pdflayout.Point{X: 12.5, Y: 300.125}
	positions[metadata.Nodes[1].GraphID] = pdflayout.Point{X: -30.25, Y: -90.75}
	path := []int64{metadata.Nodes[0].GraphID, metadata.Nodes[1].GraphID}
	flat, err := g.GenerateFlat(path)
	if err != nil {
		t.Fatal(err)
	}
	positioned, err := g.GeneratePositioned(path, positions)
	if err != nil {
		t.Fatal(err)
	}
	assertNoComponentControls(t, positioned)
	if !reflect.DeepEqual(visibleStatements(flat), visibleStatements(positionAttribute.ReplaceAllString(positioned, ""))) {
		t.Fatalf("positions changed visible styles or records:\n%s", positioned)
	}
	for _, expected := range []string{`pos="12.5,-300.125!"`, `pos="-30.25,90.75!"`, `pos="400,0!"`} {
		if !strings.Contains(positioned, expected) {
			t.Errorf("missing coordinate %s", expected)
		}
	}
	if strings.Count(positioned, "pos=") != len(metadata.Nodes) {
		t.Fatal("every canonical node must be pinned exactly once")
	}
}

func TestPositionedRejectsIncompleteExtraOrNonfinitePositions(t *testing.T) {
	g, metadata := newFlatTestGenerator(t)
	for _, test := range []struct {
		name string
		edit func(map[int64]pdflayout.Point)
		want string
	}{
		{"missing", func(p map[int64]pdflayout.Point) { delete(p, metadata.Nodes[0].GraphID) }, "missing position"},
		{"extra", func(p map[int64]pdflayout.Point) { p[99] = pdflayout.Point{} }, "non-visible node 99"},
		{"nan-x", func(p map[int64]pdflayout.Point) { p[0] = pdflayout.Point{X: math.NaN()} }, "must be finite"},
		{"nan-y", func(p map[int64]pdflayout.Point) { p[0] = pdflayout.Point{Y: math.NaN()} }, "must be finite"},
		{"positive-infinity", func(p map[int64]pdflayout.Point) { p[0] = pdflayout.Point{X: math.Inf(1)} }, "must be finite"},
		{"negative-infinity", func(p map[int64]pdflayout.Point) { p[0] = pdflayout.Point{Y: math.Inf(-1)} }, "must be finite"},
	} {
		t.Run(test.name, func(t *testing.T) {
			positions := fixturePositions(metadata)
			test.edit(positions)
			output, err := g.GeneratePositioned(nil, positions)
			if err == nil || !strings.Contains(err.Error(), test.want) || output != "" {
				t.Fatalf("got output %q, error %v; want %q", output, err, test.want)
			}
		})
	}
	if _, err := g.GeneratePositioned(nil, nil); err == nil {
		t.Fatal("nil positions accepted for a nonempty graph")
	}
}

func TestPositionedCoversImplicitPathEndpoints(t *testing.T) {
	g, metadata := newFlatTestGenerator(t)
	positions := fixturePositions(metadata)
	path := []int64{metadata.Nodes[0].GraphID, 99}
	if _, err := g.GeneratePositioned(path, positions); err == nil || !strings.Contains(err.Error(), "visible node 99") {
		t.Fatalf("implicit visible node was not validated: %v", err)
	}
	positions[99] = pdflayout.Point{X: 1500, Y: 250}
	output, err := g.GeneratePositioned(path, positions)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `"99" [pos="1500,-250!"];`) || !strings.Contains(output, `"0" -> "99"`) {
		t.Fatalf("implicit endpoint lost its default style or connection:\n%s", output)
	}
	if _, err := g.GeneratePositioned([]int64{99}, positions); err == nil || !strings.Contains(err.Error(), "non-visible node 99") {
		t.Fatalf("a singleton path ID should not create a new visible node: %v", err)
	}
}

func TestFlatAndPositionedPreserveInputsAndResetRenderState(t *testing.T) {
	g, metadata := newFlatTestGenerator(t)
	positions := fixturePositions(metadata)
	path := []int64{metadata.Nodes[0].GraphID, metadata.Nodes[1].GraphID}
	before, err := json.Marshal([]any{metadata, g.graph.NodeMap, g.graph.EdgeMap, g.graph.IdNameMap, g.graph.IdStackMap, positions, path})
	if err != nil {
		t.Fatal(err)
	}
	original, err := g.Generate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.GeneratePositioned(path, positions); err != nil {
		t.Fatal(err)
	}
	flat, err := g.GenerateFlat(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(flat, "pos=") || strings.Contains(flat, "Custom path:") || strings.Contains(flat, "#E74C3C") {
		t.Fatal("flat generation retained positions or previous highlighting")
	}
	again, err := g.Generate(nil)
	if err != nil || again != original {
		t.Fatalf("original output changed after flat/positioned generation: %v", err)
	}
	after, err := json.Marshal([]any{metadata, g.graph.NodeMap, g.graph.EdgeMap, g.graph.IdNameMap, g.graph.IdStackMap, positions, path})
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("generation mutated graph, metadata, positions or caller path")
	}
}

func TestPositionedRoutesWithoutMovingNodeCenters(t *testing.T) {
	g, metadata := newFlatTestGenerator(t)
	positions := fixturePositions(metadata)
	output, err := g.GeneratePositioned(nil, positions)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("neato"); err != nil {
		t.Skip("Neato is unavailable")
	}
	drawing, err := pdflayout.RunPinned(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(drawing.Nodes) != len(positions) {
		t.Fatalf("visible inventory changed: got %d nodes, want %d", len(drawing.Nodes), len(positions))
	}
	base := drawing.Nodes[0]
	for _, node := range drawing.Nodes {
		want, origin := positions[node.ID], positions[base.ID]
		if math.Abs((node.X-base.X)-(want.X-origin.X)) > .02 || math.Abs((node.Y-base.Y)-(want.Y-origin.Y)) > .02 {
			t.Fatalf("Neato moved node %d relative to %d: got (%g,%g), expected (%g,%g)", node.ID, base.ID, node.X-base.X, node.Y-base.Y, want.X-origin.X, want.Y-origin.Y)
		}
	}
}
