package layout

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strings"
)

// Point is a node center in top-down point units. Positioned DOT writes X,-Y
// so Neato's pinned renderer preserves the arrangement's orientation.
type Point struct{ X, Y float64 }

// GroupSummary describes the selected seed and final node envelope for a group.
// Crossings are a straight-chord placement proxy, not a final routing count.
type GroupSummary struct {
	Name              string
	Seed              string
	Nodes             int
	OriginalCrossings int
	FlatCrossings     int
	Bounds            Bounds
}

type Arrangement struct {
	Positions map[int64]Point
	Groups    []GroupSummary
}

func positiveFinite(value float64) bool { return finite(value) && value > 0 }

// PositionOnly obtains the original point-sized boxes and layout centers without
// expensive obstacle routing. DOT graph overlap settings are retained, and the
// same font/DPI options as the final render are used. The returned straight
// routes are seed geometry only, not a drawing to publish.
func PositionOnly(ctx context.Context, dot string) (Drawing, error) {
	if ctx == nil {
		return Drawing{}, fmt.Errorf("layout context is nil")
	}
	if len(dot) > maxDOTBytes {
		return Drawing{}, fmt.Errorf("DOT exceeds layout input size limit")
	}
	ctx, cancel := context.WithTimeout(ctx, seedTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "neato", "-Tjson", "-Gdpi=600", "-Gsplines=line")
	cmd.Stdin = strings.NewReader(dot)
	output := cappedOutput{maximum: maxJSONBytes}
	diagnostic := cappedOutput{maximum: maxErrorBytes, discard: true}
	cmd.Stdout, cmd.Stderr = &output, &diagnostic
	err := cmd.Run()
	if ctx.Err() != nil {
		return Drawing{}, fmt.Errorf("Neato seed layout canceled or timed out: %w", ctx.Err())
	}
	if err != nil {
		return Drawing{}, fmt.Errorf("Neato seed layout failed: %w\n%s", err, diagnostic.buffer.String())
	}
	drawing, err := FromJSON(output.buffer.Bytes())
	if err != nil {
		return Drawing{}, fmt.Errorf("Neato produced invalid seed layout: %w", err)
	}
	return drawing, nil
}

type groupPlacement struct {
	name          string
	ids           []int64
	local         map[int64]Point
	width, height float64
	center        Point
	pose          placementPose
	placed        bool
	summary       GroupSummary
}
type placementPose struct{ x, y, sx, sy, angle float64 }
type placementEdge struct {
	source, target int64
	group          string
}
type placementSegment struct {
	source, target int64
	a, b           Point
}
type placementScore struct {
	crossings, hits int
	length          float64
}

func (s placementScore) objective() float64 { return float64(s.crossings+2*s.hits) + s.length/200000 }

type placementState struct {
	groups []*groupPlacement
	nodes  map[int64]Node
	edges  []placementEdge
	hub    *groupPlacement
}

// ArrangeAges keeps an organic embedding within each group and separates the
// groups around the most connected one. It selects each group's lower-crossing
// seed, then searches translations and horizontal/vertical reflections so its
// linking points face the other groups. Labels and point-sized boxes stay upright.
//
// Both seeds must contain exactly the same visible graph and node styles. The
// group map must cover that inventory exactly. No Age names or game IDs are
// assumed. This function does not invoke Graphviz or mutate either seed. The
// selected seed must have nonoverlapping boxes after small disconnected components are
// repacked; rigid placement cannot repair overlaps inside that embedding.
// returned positions need a final shared Neato -n2 render for PDF and browser;
// its chord/box proxy does not guarantee fewer routed crossings for every graph.
func ArrangeAges(ctx context.Context, original, flat Drawing, groupByID map[int64]string) (Arrangement, error) {
	if ctx == nil {
		return Arrangement{}, fmt.Errorf("layout context is nil")
	}
	ctx, cancel := context.WithTimeout(ctx, seedTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Arrangement{}, err
	}
	on, fn, err := validateArrangementSeeds(original, flat, groupByID)
	if err != nil {
		return Arrangement{}, err
	}
	names := make([]string, 0)
	idsByGroup := make(map[string][]int64)
	ids := make([]int64, 0, len(on))
	for id := range on {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		name := groupByID[id]
		if _, ok := idsByGroup[name]; !ok {
			names = append(names, name)
		}
		idsByGroup[name] = append(idsByGroup[name], id)
	}
	sort.Strings(names)
	packing, err := smallPackingComponents(ctx, ids, original.Edges, groupByID)
	if err != nil {
		return Arrangement{}, err
	}
	packed := make(map[int64]bool)
	for _, blocks := range packing {
		for _, block := range blocks {
			for _, id := range block {
				packed[id] = true
			}
		}
	}
	originalCross, err := groupChordCrossings(ctx, original, on, groupByID)
	if err != nil {
		return Arrangement{}, err
	}
	flatCross, err := groupChordCrossings(ctx, flat, fn, groupByID)
	if err != nil {
		return Arrangement{}, err
	}
	degree := make(map[int64]int)
	weights := make(map[string]int)
	state := placementState{nodes: make(map[int64]Node, len(on))}
	for _, edge := range original.Edges {
		degree[edge.Source]++
		degree[edge.Target]++
		name := ""
		if groupByID[edge.Source] == groupByID[edge.Target] {
			name = groupByID[edge.Source]
		} else {
			weights[groupByID[edge.Source]]++
			weights[groupByID[edge.Target]]++
		}
		state.edges = append(state.edges, placementEdge{edge.Source, edge.Target, name})
	}
	originalCentroids := make(map[string]Point)
	for _, name := range names {
		groupIDs := idsByGroup[name]
		seed := on
		which := "original"
		if flatCross[name] < originalCross[name] {
			seed = fn
			which = "flat"
		}
		g := &groupPlacement{name: name, ids: groupIDs, local: make(map[int64]Point, len(groupIDs)), summary: GroupSummary{Name: name, Seed: which, Nodes: len(groupIDs), OriginalCrossings: originalCross[name], FlatCrossings: flatCross[name]}}
		core := Bounds{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
		haveCore := false
		var centroid Point
		for _, id := range groupIDs {
			n := seed[id]
			state.nodes[id] = n
			g.local[id] = Point{n.X, n.Y}
			centroid.X += on[id].X
			centroid.Y += on[id].Y
			if !packed[id] {
				includeNode(&core, n)
				haveCore = true
			}
		}
		centroid.X /= float64(len(groupIDs))
		centroid.Y /= float64(len(groupIDs))
		originalCentroids[name] = centroid
		// Entire small components have no connection to the core. Keep each block's
		// internal geometry, but place it beside its own group instead of retaining
		// a remote seed position that needlessly enlarges the envelope.
		if !haveCore {
			core = Bounds{}
		}
		blocks := packing[name]
		sort.Slice(blocks, func(i, j int) bool {
			a, b := blocks[i], blocks[j]
			isolatedA, isolatedB := len(a) == 1 && degree[a[0]] == 0, len(b) == 1 && degree[b[0]] == 0
			if isolatedA != isolatedB {
				return isolatedA
			}
			return a[0] < b[0]
		})
		packGroupBlocks(g, blocks, core, state.nodes)
		envelope := Bounds{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
		for _, id := range groupIDs {
			n := state.nodes[id]
			n.X, n.Y = g.local[id].X, g.local[id].Y
			includeNode(&envelope, n)
		}
		for i, id := range groupIDs {
			if i%64 == 0 {
				if err := ctx.Err(); err != nil {
					return Arrangement{}, err
				}
			}
			a, pa := state.nodes[id], g.local[id]
			for _, otherID := range groupIDs[i+1:] {
				b, pb := state.nodes[otherID], g.local[otherID]
				if math.Abs(pa.X-pb.X) < (a.Width+b.Width)/2-1e-6 &&
					math.Abs(pa.Y-pb.Y) < (a.Height+b.Height)/2-1e-6 {
					return Arrangement{}, fmt.Errorf("group %q selected seed has overlapping nodes %d and %d", name, id, otherID)
				}
			}
		}
		g.width, g.height = envelope.MaxX-envelope.MinX, envelope.MaxY-envelope.MinY
		g.center = Point{(envelope.MinX + envelope.MaxX) / 2, (envelope.MinY + envelope.MaxY) / 2}
		if !positiveFinite(g.width) || !positiveFinite(g.height) || !finite(g.center.X) || !finite(g.center.Y) {
			return Arrangement{}, fmt.Errorf("group %q has invalid extent", name)
		}
		state.groups = append(state.groups, g)
		if state.hub == nil || weights[name] > weights[state.hub.name] {
			state.hub = g
		}
	}
	state.apply(state.hub, placementPose{sx: 1, sy: 1})
	others := make([]*groupPlacement, 0, len(state.groups)-1)
	for _, g := range state.groups {
		if g != state.hub {
			others = append(others, g)
		}
	}
	sort.Slice(others, func(i, j int) bool {
		a, b := others[i], others[j]
		aa, ba := a.width*a.height, b.width*b.height
		if aa == ba {
			return a.name < b.name
		}
		return aa > ba
	})
	hubCentroid := originalCentroids[state.hub.name]
	for _, g := range others {
		c := originalCentroids[g.name]
		angle := math.Atan2(c.Y-hubCentroid.Y, c.X-hubCentroid.X)
		x, y, ok := state.locate(g, angle)
		if !ok {
			return Arrangement{}, fmt.Errorf("cannot separate group %q", g.name)
		}
		state.apply(g, placementPose{x: x, y: y, sx: 1, sy: 1, angle: angle})
	}
	best, err := state.score(ctx)
	if err != nil {
		return Arrangement{}, err
	}
	phases := []struct{ step, radius int }{{15, -1}, {5, 20}, {2, 6}}
	for _, phase := range phases {
		for _, g := range others {
			if err := ctx.Err(); err != nil {
				return Arrangement{}, err
			}
			old := g.pose
			bestPose := old
			groupBest := best
			angles := make([]float64, 0)
			if phase.radius < 0 {
				for d := 0; d < 360; d += phase.step {
					angles = append(angles, float64(d)*math.Pi/180)
				}
			} else {
				for d := -phase.radius; d <= phase.radius; d += phase.step {
					angles = append(angles, old.angle+float64(d)*math.Pi/180)
				}
			}
			for _, angle := range angles {
				x, y, ok := state.locate(g, angle)
				if !ok {
					continue
				}
				for _, reflection := range [][2]float64{{1, 1}, {-1, 1}, {1, -1}, {-1, -1}} {
					pose := placementPose{x: x, y: y, sx: reflection[0], sy: reflection[1], angle: angle}
					state.apply(g, pose)
					score, err := state.score(ctx)
					if err != nil {
						return Arrangement{}, err
					}
					if score.objective() < groupBest.objective()-1e-8 {
						groupBest = score
						bestPose = pose
					}
				}
			}
			state.apply(g, bestPose)
			best = groupBest
		}
	}
	return state.result(), nil
}

func validateArrangementSeeds(original, flat Drawing, groups map[int64]string) (map[int64]Node, map[int64]Node, error) {
	if len(original.Nodes) == 0 || len(original.Nodes) != len(flat.Nodes) || len(groups) != len(original.Nodes) {
		return nil, nil, fmt.Errorf("arrangement seeds and groups must have the same nonempty node inventory")
	}
	if len(original.Nodes) > 100000 || len(original.Edges) > 400000 || len(flat.Edges) > 400000 {
		return nil, nil, fmt.Errorf("arrangement exceeds graph size limit")
	}
	read := func(d Drawing) (map[int64]Node, error) {
		nodes := make(map[int64]Node, len(d.Nodes))
		for _, n := range d.Nodes {
			if _, ok := nodes[n.ID]; ok {
				return nil, fmt.Errorf("duplicate node %d in arrangement seed", n.ID)
			}
			if !finite(n.X) || !finite(n.Y) || !positiveFinite(n.Width) || !positiveFinite(n.Height) || !finite(n.PenWidth) || n.PenWidth < 0 {
				return nil, fmt.Errorf("invalid geometry for arrangement node %d", n.ID)
			}
			if strings.TrimSpace(groups[n.ID]) == "" {
				return nil, fmt.Errorf("arrangement node %d has no group", n.ID)
			}
			nodes[n.ID] = n
		}
		for _, e := range d.Edges {
			if _, ok := nodes[e.Source]; !ok {
				return nil, fmt.Errorf("unknown arrangement edge source %d", e.Source)
			}
			if _, ok := nodes[e.Target]; !ok {
				return nil, fmt.Errorf("unknown arrangement edge target %d", e.Target)
			}
			if !finite(e.PenWidth) || e.PenWidth < 0 {
				return nil, fmt.Errorf("invalid edge width")
			}
		}
		return nodes, nil
	}
	on, err := read(original)
	if err != nil {
		return nil, nil, err
	}
	fn, err := read(flat)
	if err != nil {
		return nil, nil, err
	}
	for id, n := range on {
		f, ok := fn[id]
		if !ok {
			return nil, nil, fmt.Errorf("arrangement seeds have different node inventories")
		}
		if n.Width != f.Width || n.Height != f.Height || n.Shape != f.Shape || n.Style != f.Style || !strings.EqualFold(n.Fill, f.Fill) || !strings.EqualFold(n.Stroke, f.Stroke) || !strings.EqualFold(n.FontColor, f.FontColor) || n.PenWidth != f.PenWidth || !sameNodeText(n.Labels, f.Labels) {
			return nil, nil, fmt.Errorf("arrangement seeds change node %d style or box", id)
		}
	}
	type edgeKey struct {
		source, target int64
		color, style   string
		width          float64
		head, tail     bool
	}
	inventory := func(d Drawing) map[edgeKey]int {
		m := make(map[edgeKey]int)
		for _, e := range d.Edges {
			m[edgeKey{e.Source, e.Target, strings.ToLower(e.Color), e.Style, e.PenWidth, hasArrow(e.Head), hasArrow(e.Tail)}]++
		}
		return m
	}
	a, b := inventory(original), inventory(flat)
	if len(a) != len(b) {
		return nil, nil, fmt.Errorf("arrangement seeds have different visible edges")
	}
	for k, v := range a {
		if b[k] != v {
			return nil, nil, fmt.Errorf("arrangement seeds have different visible edges")
		}
	}
	return on, fn, nil
}
func sameNodeText(a, b []Operation) bool {
	filter := func(ops []Operation) []Operation {
		r := make([]Operation, 0)
		for _, op := range ops {
			if op.Op == "T" || op.Op == "F" || op.Op == "t" {
				op.Pt = nil
				op.Points = nil
				op.Rect = nil
				op.Width = 0
				r = append(r, op)
			}
		}
		return r
	}
	x, y := filter(a), filter(b)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i].Op != y[i].Op || x[i].Text != y[i].Text || x[i].Face != y[i].Face || x[i].Size != y[i].Size || x[i].Fontchar != y[i].Fontchar || x[i].Align != y[i].Align {
			return false
		}
	}
	return true
}
func hasArrow(ops []Operation) bool {
	for _, op := range ops {
		if op.Op == "P" || op.Op == "p" || op.Op == "E" || op.Op == "e" || op.Op == "b" || op.Op == "B" || op.Op == "L" {
			return true
		}
	}
	return false
}

func groupChordCrossings(ctx context.Context, d Drawing, nodes map[int64]Node, groups map[int64]string) (map[string]int, error) {
	segments := make([]placementSegment, 0, len(d.Edges))
	names := make([]string, 0, len(d.Edges))
	for _, e := range d.Edges {
		if e.Source == e.Target || groups[e.Source] != groups[e.Target] {
			continue
		}
		a, b := nodeChord(nodes[e.Source], nodes[e.Target])
		segments = append(segments, placementSegment{e.Source, e.Target, a, b})
		names = append(names, groups[e.Source])
	}
	counts := make(map[string]int)
	for i, a := range segments {
		if i%32 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		for j := i + 1; j < len(segments); j++ {
			b := segments[j]
			if names[i] != names[j] || incident(a, b) {
				continue
			}
			if segmentsCross(a.a, a.b, b.a, b.b) {
				counts[names[i]]++
			}
		}
	}
	return counts, nil
}

// Components use all visible edges in either direction. A cross-group link or
// any connection to a larger component makes a block ineligible for packing.
func smallPackingComponents(ctx context.Context, ids []int64, edges []Edge, groups map[int64]string) (map[string][][]int64, error) {
	adjacent := make(map[int64][]int64, len(ids))
	for _, edge := range edges {
		adjacent[edge.Source] = append(adjacent[edge.Source], edge.Target)
		adjacent[edge.Target] = append(adjacent[edge.Target], edge.Source)
	}
	seen := make(map[int64]bool, len(ids))
	result := make(map[string][][]int64)
	for _, start := range ids {
		if seen[start] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		block := []int64{start}
		seen[start] = true
		name, sameGroup := groups[start], true
		for next := 0; next < len(block); next++ {
			if next%256 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			id := block[next]
			if groups[id] != name {
				sameGroup = false
			}
			for _, other := range adjacent[id] {
				if !seen[other] {
					seen[other] = true
					block = append(block, other)
				}
			}
		}
		if len(block) <= 10 && sameGroup {
			sort.Slice(block, func(i, j int) bool { return block[i] < block[j] })
			result[name] = append(result[name], block)
		}
	}
	return result, nil
}

func packGroupBlocks(group *groupPlacement, blocks [][]int64, core Bounds, nodes map[int64]Node) {
	rowWidth := core.MaxX - core.MinX
	if rowWidth == 0 {
		rowWidth = 600
	}
	bounds := make([]Bounds, len(blocks))
	for i, block := range blocks {
		box := Bounds{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
		for _, id := range block {
			node := nodes[id]
			node.X, node.Y = group.local[id].X, group.local[id].Y
			includeNode(&box, node)
		}
		bounds[i] = box
		rowWidth = math.Max(rowWidth, box.MaxX-box.MinX)
	}
	x, bottom, rowHeight := core.MinX, core.MinY-80, 0.0
	for i, block := range blocks {
		box := bounds[i]
		width, height := box.MaxX-box.MinX, box.MaxY-box.MinY
		if x > core.MinX && x+width > core.MinX+rowWidth {
			x, bottom, rowHeight = core.MinX, bottom-rowHeight-40, 0
		}
		dx, dy := x-box.MinX, bottom-box.MaxY
		for _, id := range block {
			point := group.local[id]
			group.local[id] = Point{point.X + dx, point.Y + dy}
		}
		x += width + 40
		rowHeight = math.Max(rowHeight, height)
	}
}

func (s *placementState) apply(g *groupPlacement, p placementPose) {
	g.pose = p
	g.placed = true
	for _, id := range g.ids {
		n := s.nodes[id]
		local := g.local[id]
		n.X = p.x + p.sx*(local.X-g.center.X)
		n.Y = p.y + p.sy*(local.Y-g.center.Y)
		s.nodes[id] = n
	}
}
func (s *placementState) locate(g *groupPlacement, angle float64) (float64, float64, bool) {
	ux, uy := math.Cos(angle), math.Sin(angle)
	rx, ry := math.Inf(1), math.Inf(1)
	if math.Abs(ux) > 1e-8 {
		rx = ((g.width+s.hub.width)/2 + 240) / math.Abs(ux)
	}
	if math.Abs(uy) > 1e-8 {
		ry = ((g.height+s.hub.height)/2 + 240) / math.Abs(uy)
	}
	r := math.Min(rx, ry)
	for i := 0; i < 100; i++ {
		x, y := r*ux, r*uy
		clear := true
		for _, other := range s.groups {
			if other == g || !other.placed {
				continue
			}
			if math.Abs(x-other.pose.x) < (g.width+other.width)/2+160 && math.Abs(y-other.pose.y) < (g.height+other.height)/2+160 {
				clear = false
				break
			}
		}
		if clear {
			return x, y, true
		}
		r += 220
	}
	return 0, 0, false
}
func (s *placementState) score(ctx context.Context) (placementScore, error) {
	if err := ctx.Err(); err != nil {
		return placementScore{}, err
	}
	internal, bridges := make([]placementSegment, 0, len(s.edges)), make([]placementSegment, 0)
	var score placementScore
	for _, e := range s.edges {
		if e.source == e.target {
			continue
		}
		a, b := s.nodes[e.source], s.nodes[e.target]
		start, end := nodeChord(a, b)
		seg := placementSegment{e.source, e.target, start, end}
		if e.group != "" {
			internal = append(internal, seg)
		} else {
			bridges = append(bridges, seg)
			score.length += math.Hypot(a.X-b.X, a.Y-b.Y)
		}
	}
	for i, bridge := range bridges {
		if i%8 == 0 {
			if err := ctx.Err(); err != nil {
				return placementScore{}, err
			}
		}
		for _, edge := range internal {
			if !incident(bridge, edge) && segmentsCross(bridge.a, bridge.b, edge.a, edge.b) {
				score.crossings++
			}
		}
		for _, other := range bridges[i+1:] {
			if !incident(bridge, other) && segmentsCross(bridge.a, bridge.b, other.a, other.b) {
				score.crossings++
			}
		}
		for id, n := range s.nodes {
			if id != bridge.source && id != bridge.target && segmentHitsNode(bridge.a, bridge.b, n) {
				score.hits++
			}
		}
	}
	return score, nil
}
func (s *placementState) result() Arrangement {
	bounds := Bounds{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
	for _, n := range s.nodes {
		includeNode(&bounds, n)
	}
	result := Arrangement{Positions: make(map[int64]Point, len(s.nodes)), Groups: make([]GroupSummary, 0, len(s.groups))}
	for id, n := range s.nodes {
		result.Positions[id] = Point{n.X - bounds.MinX, n.Y - bounds.MinY}
	}
	for _, g := range s.groups {
		summary := g.summary
		summary.Bounds = Bounds{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
		for _, id := range g.ids {
			n := s.nodes[id]
			p := result.Positions[id]
			n.X, n.Y = p.X, p.Y
			includeNode(&summary.Bounds, n)
		}
		result.Groups = append(result.Groups, summary)
	}
	return result
}
func includeNode(b *Bounds, n Node) {
	b.MinX = math.Min(b.MinX, n.X-n.Width/2)
	b.MinY = math.Min(b.MinY, n.Y-n.Height/2)
	b.MaxX = math.Max(b.MaxX, n.X+n.Width/2)
	b.MaxY = math.Max(b.MaxY, n.Y+n.Height/2)
}
func nodeChord(a, b Node) (Point, Point) {
	dx, dy := b.X-a.X, b.Y-a.Y
	if dx == 0 && dy == 0 {
		return Point{a.X, a.Y}, Point{b.X, b.Y}
	}
	tip := func(n Node, x, y float64) Point {
		tx, ty := math.Inf(1), math.Inf(1)
		if x != 0 {
			tx = n.Width / 2 / math.Abs(x)
		}
		if y != 0 {
			ty = n.Height / 2 / math.Abs(y)
		}
		t := math.Min(tx, ty)
		return Point{n.X + x*t, n.Y + y*t}
	}
	return tip(a, dx, dy), tip(b, -dx, -dy)
}
func incident(a, b placementSegment) bool {
	return a.source == b.source || a.source == b.target || a.target == b.source || a.target == b.target
}
func segmentsCross(a, b, c, d Point) bool {
	rx, ry, sx, sy := b.X-a.X, b.Y-a.Y, d.X-c.X, d.Y-c.Y
	det := rx*sy - ry*sx
	if math.Abs(det) < 1e-9 {
		return false
	}
	qx, qy := c.X-a.X, c.Y-a.Y
	t, u := (qx*sy-qy*sx)/det, (qx*ry-qy*rx)/det
	return t >= -1e-8 && t <= 1+1e-8 && u >= -1e-8 && u <= 1+1e-8
}
func segmentHitsNode(a, b Point, n Node) bool {
	const eps = 1e-5
	x0, y0, x1, y1 := n.X-n.Width/2+eps, n.Y-n.Height/2+eps, n.X+n.Width/2-eps, n.Y+n.Height/2-eps
	if x0 >= x1 || y0 >= y1 {
		return false
	}
	dx, dy := b.X-a.X, b.Y-a.Y
	low, high := 0.0, 1.0
	for _, pq := range [][2]float64{{-dx, a.X - x0}, {dx, x1 - a.X}, {-dy, a.Y - y0}, {dy, y1 - a.Y}} {
		p, q := pq[0], pq[1]
		if math.Abs(p) < 1e-12 {
			if q < 0 {
				return false
			}
		} else if p < 0 {
			low = math.Max(low, q/p)
		} else {
			high = math.Min(high, q/p)
		}
		if low > high {
			return false
		}
	}
	return high-low > 1e-8
}
