package layout

import (
	"context"
	"encoding/json"
	"math"
	"os/exec"
	"reflect"
	"testing"
)

func TestArrangeAgesSelectsEachSeedAndPreservesInternalGeometry(t *testing.T) {
	makeNode := func(id int64, x, y float64) Node {
		return Node{ID: id, X: x, Y: y, Width: 20, Height: 10, Fill: "#ffffff", Stroke: "#000000", FontColor: "#000000", PenWidth: 1, Shape: "box", Style: "filled"}
	}
	original := Drawing{Nodes: []Node{
		makeNode(0, -100, -100), makeNode(1, 100, 100), makeNode(2, -100, 100), makeNode(3, 100, -100),
		makeNode(4, 400, -100), makeNode(5, 400, 100), makeNode(6, 600, -100), makeNode(7, 600, 100), makeNode(8, 9000, 9000),
	}, Edges: []Edge{{Source: 0, Target: 1}, {Source: 2, Target: 3}, {Source: 1, Target: 2}, {Source: 4, Target: 5}, {Source: 6, Target: 7}, {Source: 5, Target: 6}, {Source: 1, Target: 4}}}
	flat := original
	flat.Nodes = append([]Node(nil), original.Nodes...)
	flat.Nodes[1].X, flat.Nodes[1].Y = -100, 100
	flat.Nodes[2].X, flat.Nodes[2].Y = 100, -100
	flat.Nodes[3].X, flat.Nodes[3].Y = 100, 100
	flat.Nodes[4].X, flat.Nodes[4].Y = 400, -100
	flat.Nodes[5].X, flat.Nodes[5].Y = 600, 100
	flat.Nodes[6].X, flat.Nodes[6].Y = 400, 100
	flat.Nodes[7].X, flat.Nodes[7].Y = 600, -100
	groups := map[int64]string{0: "A", 1: "A", 2: "A", 3: "A", 4: "B", 5: "B", 6: "B", 7: "B", 8: "A"}
	beforeOriginal, _ := json.Marshal(original)
	beforeFlat, _ := json.Marshal(flat)
	result, err := ArrangeAges(context.Background(), original, flat, groups)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Positions) != 9 || len(result.Groups) != 2 {
		t.Fatalf("incomplete inventory: %+v", result)
	}
	if result.Groups[0].Name != "A" || result.Groups[0].Seed != "flat" || result.Groups[0].OriginalCrossings != 1 || result.Groups[0].FlatCrossings != 0 {
		t.Fatalf("group A chose wrong seed: %+v", result.Groups[0])
	}
	if result.Groups[1].Name != "B" || result.Groups[1].Seed != "original" || result.Groups[1].OriginalCrossings != 0 || result.Groups[1].FlatCrossings != 1 {
		t.Fatalf("group B chose wrong seed: %+v", result.Groups[1])
	}
	for _, ids := range [][]int64{{0, 1, 2, 3}, {4, 5, 6, 7}} {
		selected := original.Nodes
		if ids[0] == 0 {
			selected = flat.Nodes
		}
		for i, a := range ids {
			for _, b := range ids[i+1:] {
				pa, pb := result.Positions[a], result.Positions[b]
				na, nb := selected[a], selected[b]
				if math.Abs(math.Abs(pa.X-pb.X)-math.Abs(na.X-nb.X)) > 1e-8 || math.Abs(math.Abs(pa.Y-pb.Y)-math.Abs(na.Y-nb.Y)) > 1e-8 {
					t.Fatalf("internal geometry changed for %d/%d", a, b)
				}
			}
		}
	}
	a, b := result.Groups[0].Bounds, result.Groups[1].Bounds
	if a.MinX < b.MaxX && a.MaxX > b.MinX && a.MinY < b.MaxY && a.MaxY > b.MinY {
		t.Fatalf("group envelopes overlap: %+v %+v", a, b)
	}
	// The disconnected label is repacked near its own group instead of retaining
	// a position thousands of points away from every related record.
	if result.Groups[0].Bounds.MaxX-result.Groups[0].Bounds.MinX > 500 || result.Groups[0].Bounds.MaxY-result.Groups[0].Bounds.MinY > 500 {
		t.Fatalf("isolate inflated group: %+v", result.Groups[0])
	}
	again, err := ArrangeAges(context.Background(), original, flat, groups)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, again) {
		t.Fatal("arrangement is not deterministic")
	}
	afterOriginal, _ := json.Marshal(original)
	afterFlat, _ := json.Marshal(flat)
	if string(beforeOriginal) != string(afterOriginal) || string(beforeFlat) != string(afterFlat) {
		t.Fatal("arrangement mutated a seed")
	}
}

func TestPositionOnlyRealNeatoKeepsVisibleInventoryAndLabels(t *testing.T) {
	if _, err := exec.LookPath("neato"); err != nil {
		t.Skip("neato not installed")
	}
	d, err := PositionOnly(context.Background(), `digraph { graph[overlap=false]; node[shape=box]; "0"[label="First"]; "1"[label="Second"]; "0"->"1"[dir=both]; }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Nodes) != 2 || len(d.Edges) != 1 {
		t.Fatalf("wrong visible inventory: %d nodes/%d edges", len(d.Nodes), len(d.Edges))
	}
	for _, n := range d.Nodes {
		if n.Width <= 0 || n.Height <= 0 || len(n.Labels) == 0 {
			t.Fatalf("missing node geometry/label: %+v", n)
		}
	}
	if !hasArrow(d.Edges[0].Head) || !hasArrow(d.Edges[0].Tail) {
		t.Fatal("bidirectional seed lost arrow identity")
	}
	if _, err := ArrangeAges(context.Background(), d, d, map[int64]string{0: "First group", 1: "Second group"}); err != nil {
		t.Fatalf("real seed rejected: %v", err)
	}
}

func TestArrangeAgesRejectsOverlapThatRigidPlacementCannotRepair(t *testing.T) {
	seed := Drawing{Nodes: []Node{{ID: 0, X: 0, Y: 0, Width: 20, Height: 10}, {ID: 1, X: 5, Y: 0, Width: 20, Height: 10}}}
	groups := map[int64]string{0: "One group", 1: "One group"}
	// Disconnected records can safely be repacked into the group margin.
	if _, err := ArrangeAges(context.Background(), seed, seed, groups); err != nil {
		t.Fatalf("repackable isolates rejected: %v", err)
	}
	seed.Edges = []Edge{{Source: 0, Target: 1}}
	if _, err := ArrangeAges(context.Background(), seed, seed, groups); err == nil {
		t.Fatal("overlapping connected boxes accepted")
	}
}

func TestArrangeAgesPacksRemoteSmallComponentAsRigidBlock(t *testing.T) {
	seed := Drawing{}
	groups := make(map[int64]string)
	// Eleven connected nodes remain the Age's core; the remote three-node
	// component has no link to it and should not dictate the Age's envelope.
	for i := int64(0); i < 11; i++ {
		seed.Nodes = append(seed.Nodes, Node{ID: i, X: float64(i) * 80, Y: 200, Width: 32, Height: 20})
		groups[i] = "One group"
		if i > 0 {
			seed.Edges = append(seed.Edges, Edge{Source: i - 1, Target: i})
		}
	}
	for i, p := range []Point{{9000, 9000}, {9100, 9060}, {9200, 9020}} {
		id := int64(11 + i)
		seed.Nodes = append(seed.Nodes, Node{ID: id, X: p.X, Y: p.Y, Width: 32, Height: 20})
		groups[id] = "One group"
	}
	seed.Edges = append(seed.Edges, Edge{Source: 11, Target: 12}, Edge{Source: 12, Target: 13})
	result, err := ArrangeAges(context.Background(), seed, seed, groups)
	if err != nil {
		t.Fatal(err)
	}
	box := result.Groups[0].Bounds
	if box.MaxX-box.MinX > 1000 || box.MaxY-box.MinY > 500 {
		t.Fatalf("remote component still inflates envelope: %+v", box)
	}
	for _, ids := range [][]int64{{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, {11, 12, 13}} {
		for i, a := range ids {
			for _, b := range ids[i+1:] {
				got := math.Hypot(result.Positions[a].X-result.Positions[b].X, result.Positions[a].Y-result.Positions[b].Y)
				want := math.Hypot(seed.Nodes[a].X-seed.Nodes[b].X, seed.Nodes[a].Y-seed.Nodes[b].Y)
				if math.Abs(got-want) > 1e-8 {
					t.Fatalf("block geometry changed: %d/%d", a, b)
				}
			}
		}
	}
	for _, small := range []int64{11, 12, 13} {
		for core := int64(0); core < 11; core++ {
			a, b := result.Positions[small], result.Positions[core]
			if math.Abs(a.X-b.X) < 32 && math.Abs(a.Y-b.Y) < 20 {
				t.Fatalf("packed nodes overlap: %d/%d", small, core)
			}
		}
	}
}

func TestSmallPackingComponentsRequiresWholeSingleGroupComponent(t *testing.T) {
	ids := []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	groups := make(map[int64]string)
	for _, id := range ids {
		groups[id] = "A"
	}
	groups[13] = "B"
	var edges []Edge
	for id := int64(1); id <= 10; id++ {
		edges = append(edges, Edge{Source: id - 1, Target: id})
	}
	edges = append(edges, Edge{Source: 11, Target: 12}, Edge{Source: 12, Target: 13})
	packed, err := smallPackingComponents(context.Background(), ids, edges, groups)
	if err != nil {
		t.Fatal(err)
	}
	if len(packed) != 0 {
		t.Fatalf("packed large or cross-group component: %+v", packed)
	}
}
