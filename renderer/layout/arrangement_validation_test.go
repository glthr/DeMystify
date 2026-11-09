package layout

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"
)

// Synthetic only: opposite seed improvements make accidental all-original or
// all-flat selection observable without relying on the optimizer's exact poses.
func validationArrangementSeeds() (Drawing, Drawing, map[int64]string) {
	original := Drawing{Bounds: Bounds{MaxX: 1000, MaxY: 240}}
	coordinates := []Point{{60, 60}, {180, 180}, {60, 180}, {180, 60}, {460, 60}, {580, 60}, {460, 180}, {580, 180}, {960, 100}}
	groups := make(map[int64]string)
	for i, point := range coordinates {
		name := "First Age"
		if i >= 4 {
			name = "Second Age"
		}
		if i == 8 {
			name = "Empty Age"
		}
		id := int64(i)
		groups[id] = name
		original.Nodes = append(original.Nodes, Node{ID: id, X: point.X, Y: point.Y, Width: 32, Height: 20, Fill: "#d9fcc5", Stroke: "#000000", FontColor: "#000000", PenWidth: 1, Shape: "box", Style: "filled",
			Labels: []Operation{{Op: "F", Size: 10, Face: "Arial"}, {Op: "T", Text: name, Align: "c", Pt: []float64{point.X, point.Y}, Width: 24}}})
	}
	// The two within-Age connectors make this one cross-Age component.
	// Disconnected small components may legitimately be packed separately.
	for _, pair := range [][2]int64{{0, 1}, {2, 3}, {4, 5}, {6, 7}, {1, 4}, {1, 2}, {5, 6}} {
		original.Edges = append(original.Edges, Edge{Source: pair[0], Target: pair[1], Color: "#000000", PenWidth: .6})
	}
	flat := validationCloneDrawing(original)
	flat.Nodes[1].Y, flat.Nodes[3].Y = 60, 180 // First Age uncrosses.
	flat.Nodes[5].Y, flat.Nodes[7].Y = 180, 60 // Second Age crosses.
	return original, flat, groups
}

func validationCloneDrawing(input Drawing) Drawing {
	encoded, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}
	var clone Drawing
	if err = json.Unmarshal(encoded, &clone); err != nil {
		panic(err)
	}
	return clone
}

func validationArrangementSnapshot(original, flat Drawing, groups map[int64]string) []byte {
	encoded, err := json.Marshal([]any{original, flat, groups})
	if err != nil {
		panic(err)
	}
	return encoded
}

func validationPointDistance(a, b Point) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }

func validationAssertNoNodeOverlaps(t *testing.T, positions map[int64]Point, nodes []Node) {
	t.Helper()
	if len(positions) != len(nodes) {
		t.Fatalf("position count %d != node count %d", len(positions), len(nodes))
	}
	for i, a := range nodes {
		p, ok := positions[a.ID]
		if !ok || math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
			t.Fatalf("missing/nonfinite position for node %d: %+v", a.ID, p)
		}
		if p.X-a.Width/2 < -1e-7 || p.Y-a.Height/2 < -1e-7 {
			t.Fatalf("normalized node %d extends outside the top-left origin", a.ID)
		}
		for _, b := range nodes[i+1:] {
			q := positions[b.ID]
			if math.Abs(p.X-q.X) < (a.Width+b.Width)/2-1e-6 && math.Abs(p.Y-q.Y) < (a.Height+b.Height)/2-1e-6 {
				t.Fatalf("node boxes %d and %d overlap", a.ID, b.ID)
			}
		}
	}
}

func TestArrangementValidationSelectsEachBetterSeedAndPreservesInputs(t *testing.T) {
	original, flat, groups := validationArrangementSeeds()
	before := validationArrangementSnapshot(original, flat, groups)
	result, err := ArrangeAges(context.Background(), original, flat, groups)
	if err != nil {
		t.Fatal(err)
	}
	if after := validationArrangementSnapshot(original, flat, groups); string(before) != string(after) {
		t.Fatal("arrangement mutated its seeds or group map")
	}
	validationAssertNoNodeOverlaps(t, result.Positions, original.Nodes)
	byName := make(map[string]GroupSummary)
	for _, group := range result.Groups {
		byName[group.Name] = group
		actual := Bounds{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
		count := 0
		for _, node := range original.Nodes {
			if groups[node.ID] != group.Name {
				continue
			}
			count++
			point := result.Positions[node.ID]
			actual.MinX = math.Min(actual.MinX, point.X-node.Width/2)
			actual.MinY = math.Min(actual.MinY, point.Y-node.Height/2)
			actual.MaxX = math.Max(actual.MaxX, point.X+node.Width/2)
			actual.MaxY = math.Max(actual.MaxY, point.Y+node.Height/2)
		}
		if group.Nodes != count || actual != group.Bounds {
			t.Fatalf("group envelope does not describe its actual node boxes: %+v, calculated %+v", group, actual)
		}
	}
	for _, test := range []struct {
		name, seed     string
		original, flat int
	}{{"First Age", "flat", 1, 0}, {"Second Age", "original", 0, 1}, {"Empty Age", "original", 0, 0}} {
		got := byName[test.name]
		if got.Seed != test.seed || got.OriginalCrossings != test.original || got.FlatCrossings != test.flat {
			t.Fatalf("seed choice for %s = %+v", test.name, got)
		}
	}
	// Translation/reflection may change signs, but cannot rescale the selected
	// embeddings. The isolated ninth node is deliberately exempt from rigidity.
	for _, test := range []struct {
		ids  []int64
		seed Drawing
	}{{[]int64{0, 1, 2, 3}, flat}, {[]int64{4, 5, 6, 7}, original}} {
		for i, a := range test.ids {
			for _, b := range test.ids[i+1:] {
				want := math.Hypot(test.seed.Nodes[a].X-test.seed.Nodes[b].X, test.seed.Nodes[a].Y-test.seed.Nodes[b].Y)
				if got := validationPointDistance(result.Positions[a], result.Positions[b]); math.Abs(got-want) > 1e-7 {
					t.Fatalf("selected seed distorted pair %d/%d: got %g, want %g", a, b, got, want)
				}
			}
		}
	}
	for i, a := range result.Groups {
		for _, b := range result.Groups[i+1:] {
			gapX := math.Max(b.Bounds.MinX-a.Bounds.MaxX, a.Bounds.MinX-b.Bounds.MaxX)
			gapY := math.Max(b.Bounds.MinY-a.Bounds.MaxY, a.Bounds.MinY-b.Bounds.MaxY)
			if math.Max(gapX, gapY) < 160-1e-7 {
				t.Fatalf("group envelopes %s/%s lack documented gap", a.Name, b.Name)
			}
		}
	}
}

func TestArrangementValidationRejectsForeignOrMalformedSeeds(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Drawing, *Drawing, map[int64]string)
	}{
		{"duplicate original node", func(a, b *Drawing, g map[int64]string) { a.Nodes[1].ID = 0 }},
		{"duplicate flat node", func(a, b *Drawing, g map[int64]string) { b.Nodes[1].ID = 0 }},
		{"missing flat node", func(a, b *Drawing, g map[int64]string) { b.Nodes = b.Nodes[:8] }},
		{"different flat ID", func(a, b *Drawing, g map[int64]string) { b.Nodes[8].ID = 99 }},
		{"missing group", func(a, b *Drawing, g map[int64]string) { delete(g, 8) }},
		{"extra group", func(a, b *Drawing, g map[int64]string) { g[99] = "Other" }},
		{"blank group", func(a, b *Drawing, g map[int64]string) { g[8] = " \t" }},
		{"unknown original source", func(a, b *Drawing, g map[int64]string) { a.Edges[0].Source = 99 }},
		{"unknown flat target", func(a, b *Drawing, g map[int64]string) { b.Edges[0].Target = 99 }},
		{"lost parallel record", func(a, b *Drawing, g map[int64]string) { a.Edges = append(a.Edges, a.Edges[0]) }},
		{"edge direction changed", func(a, b *Drawing, g map[int64]string) {
			b.Edges[0].Source, b.Edges[0].Target = b.Edges[0].Target, b.Edges[0].Source
		}},
		{"edge color changed", func(a, b *Drawing, g map[int64]string) { b.Edges[0].Color = "#ff0000" }},
		{"edge width changed", func(a, b *Drawing, g map[int64]string) { b.Edges[0].PenWidth = 2 }},
		{"edge negative width", func(a, b *Drawing, g map[int64]string) { b.Edges[0].PenWidth = -1 }},
		{"new return arrow", func(a, b *Drawing, g map[int64]string) {
			b.Edges[0].Tail = []Operation{{Op: "P", Points: [][]float64{{0, 0}, {1, 0}, {1, 1}}}}
		}},
		{"node box changed", func(a, b *Drawing, g map[int64]string) { b.Nodes[0].Width++ }},
		{"node label changed", func(a, b *Drawing, g map[int64]string) { b.Nodes[0].Labels[1].Text = "Other card" }},
		{"node label font changed", func(a, b *Drawing, g map[int64]string) { b.Nodes[0].Labels[0].Face = "Courier" }},
		{"node zero width", func(a, b *Drawing, g map[int64]string) { a.Nodes[0].Width = 0 }},
		{"node negative height", func(a, b *Drawing, g map[int64]string) { a.Nodes[0].Height = -20 }},
		{"node nonfinite X", func(a, b *Drawing, g map[int64]string) { a.Nodes[0].X = math.NaN() }},
		{"node infinite Y", func(a, b *Drawing, g map[int64]string) { b.Nodes[0].Y = math.Inf(-1) }},
		{"node infinite size", func(a, b *Drawing, g map[int64]string) { b.Nodes[0].Height = math.Inf(1) }},
		{"edge nonfinite width", func(a, b *Drawing, g map[int64]string) { b.Edges[0].PenWidth = math.NaN() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, b, g := validationArrangementSeeds()
			test.edit(&a, &b, g)
			if _, err := ArrangeAges(context.Background(), a, b, g); err == nil {
				t.Fatal("invalid arrangement seed accepted")
			}
		})
	}
}

func TestArrangementValidationContextAndIsolatedGroups(t *testing.T) {
	a, b, g := validationArrangementSeeds()
	if _, err := ArrangeAges(nil, a, b, g); err == nil {
		t.Fatal("nil context accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ArrangeAges(canceled, a, b, g); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context: %v", err)
	}
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	if _, err := ArrangeAges(expired, a, b, g); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired context: %v", err)
	}
	if _, err := ArrangeAges(context.Background(), Drawing{}, Drawing{}, nil); err == nil {
		t.Fatal("empty inventory accepted")
	}
	for _, count := range []int{1, 12} {
		seed := Drawing{}
		groups := make(map[int64]string)
		for i := 0; i < count; i++ {
			id := int64(i)
			seed.Nodes = append(seed.Nodes, Node{ID: id, X: float64(i) * 80, Y: 20, Width: 32, Height: 20})
			groups[id] = "Only Age"
		}
		result, err := ArrangeAges(context.Background(), seed, validationCloneDrawing(seed), groups)
		if err != nil {
			t.Fatal(err)
		}
		validationAssertNoNodeOverlaps(t, result.Positions, seed.Nodes)
		if len(result.Groups) != 1 || result.Groups[0].Nodes != count {
			t.Fatalf("isolated group inventory: %+v", result)
		}
	}
}

func TestArrangementValidationDeterministicUnderEnumerationChanges(t *testing.T) {
	a, b, g := validationArrangementSeeds()
	first, err := ArrangeAges(context.Background(), a, b, g)
	if err != nil {
		t.Fatal(err)
	}
	reverse := func(d *Drawing) {
		for i, j := 0, len(d.Nodes)-1; i < j; i, j = i+1, j-1 {
			d.Nodes[i], d.Nodes[j] = d.Nodes[j], d.Nodes[i]
		}
		for i, j := 0, len(d.Edges)-1; i < j; i, j = i+1, j-1 {
			d.Edges[i], d.Edges[j] = d.Edges[j], d.Edges[i]
		}
	}
	reverse(&a)
	reverse(&b)
	second, err := ArrangeAges(context.Background(), a, b, g)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("arrangement depends on node/edge enumeration order")
	}
}
