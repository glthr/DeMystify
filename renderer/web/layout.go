package web

import (
	"math"
	"sort"
)

// layout never changes graph records. Forces only suggest a stable local order;
// final rectangular packing guarantees that thumbnail cards cannot overlap.
func layout(data *graphData) {
	type group struct {
		name          string
		ids           []int
		width, height float64
	}
	groupsByName := make(map[string][]int)
	for i, node := range data.Nodes {
		groupsByName[node.Stack] = append(groupsByName[node.Stack], i)
	}
	names := make([]string, 0, len(groupsByName))
	for name := range groupsByName {
		names = append(names, name)
	}
	sort.Strings(names)
	groups := make([]group, 0, len(names))
	var totalArea float64
	for _, name := range names {
		ids := groupsByName[name]
		order := forceOrder(data, ids)
		maxHeight := 56.0
		for _, i := range ids {
			maxHeight = math.Max(maxHeight, data.Nodes[i].Height)
		}
		stepX, stepY := 236.0, maxHeight+56
		columns := max(1, int(math.Ceil(math.Sqrt(float64(len(ids))*stepY/stepX))))
		rows := (len(ids) + columns - 1) / columns
		for rank, local := range order {
			data.Nodes[ids[local]].X = float64(rank%columns)*stepX + stepX/2
			data.Nodes[ids[local]].Y = float64(rank/columns)*stepY + stepY/2 + 36
		}
		g := group{name: name, ids: ids, width: float64(columns) * stepX, height: float64(rows)*stepY + 36}
		groups = append(groups, g)
		totalArea += (g.width + 160) * (g.height + 160)
	}
	targetWidth := math.Max(600, math.Sqrt(totalArea)*1.4)
	x, y, rowHeight := 80.0, 80.0, 0.0
	data.Meta.Groups = []layoutGroup{}
	data.Meta.Layout = bounds{MinX: 0, MinY: 0, MaxX: 320, MaxY: 240}
	for _, g := range groups {
		if x > 80 && x+g.width > targetWidth {
			x, y, rowHeight = 80, y+rowHeight+160, 0
		}
		for _, id := range g.ids {
			data.Nodes[id].X += x
			data.Nodes[id].Y += y
		}
		data.Meta.Groups = append(data.Meta.Groups, layoutGroup{Name: g.name, bounds: bounds{MinX: x, MinY: y, MaxX: x + g.width, MaxY: y + g.height}})
		data.Meta.Layout.MaxX = math.Max(data.Meta.Layout.MaxX, x+g.width+80)
		data.Meta.Layout.MaxY = math.Max(data.Meta.Layout.MaxY, y+g.height+80)
		x += g.width + 160
		rowHeight = math.Max(rowHeight, g.height)
	}
}

func forceOrder(data *graphData, ids []int) []int {
	n := len(ids)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	if n < 3 {
		return order
	}
	type point struct{ x, y float64 }
	positions := make([]point, n)
	local := make(map[int64]int, n)
	for i, id := range ids {
		local[data.Nodes[id].ID] = i
		angle := float64(i) * 2.399963229728653
		radius := 220 * math.Sqrt(float64(i)+0.5)
		positions[i] = point{math.Cos(angle) * radius, math.Sin(angle) * radius}
	}
	type pair struct{ a, b int }
	var pairs []pair
	seen := make(map[pair]bool)
	for _, edge := range data.Edges {
		a, aok := local[edge.Source]
		b, bok := local[edge.Target]
		if !aok || !bok || a == b {
			continue
		}
		disabled := false
		for _, attribute := range edge.Attributes {
			if attribute == "disabled" {
				disabled = true
			}
		}
		if disabled {
			continue
		}
		if a > b {
			a, b = b, a
		}
		p := pair{a, b}
		if !seen[p] {
			seen[p] = true
			pairs = append(pairs, p)
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].a != pairs[j].a {
			return pairs[i].a < pairs[j].a
		}
		return pairs[i].b < pairs[j].b
	})
	for iteration := 0; iteration < 60; iteration++ {
		forces := make([]point, n)
		for a := 0; a < n; a++ {
			for b := a + 1; b < n; b++ {
				dx, dy := positions[a].x-positions[b].x, positions[a].y-positions[b].y
				distance := math.Max(1, math.Hypot(dx, dy))
				magnitude := 220 * 220 / distance
				fx, fy := dx/distance*magnitude, dy/distance*magnitude
				forces[a].x += fx
				forces[a].y += fy
				forces[b].x -= fx
				forces[b].y -= fy
			}
		}
		for _, p := range pairs {
			dx, dy := positions[p.a].x-positions[p.b].x, positions[p.a].y-positions[p.b].y
			distance := math.Max(1, math.Hypot(dx, dy))
			magnitude := distance * distance / 220 * 0.035
			fx, fy := dx/distance*magnitude, dy/distance*magnitude
			forces[p.a].x -= fx
			forces[p.a].y -= fy
			forces[p.b].x += fx
			forces[p.b].y += fy
		}
		for i := range positions {
			forces[i].x -= positions[i].x * 0.01
			forces[i].y -= positions[i].y * 0.01
			magnitude := math.Max(1, math.Hypot(forces[i].x, forces[i].y))
			step := math.Min(20*(1-float64(iteration)/70), magnitude)
			positions[i].x += forces[i].x / magnitude * step
			positions[i].y += forces[i].y / magnitude * step
		}
	}
	// Read the relaxed shape in horizontal bands, then snap to nonoverlapping
	// cells. Node IDs provide a total tie-breaker for all ordering decisions.
	sort.Slice(order, func(a, b int) bool {
		i, j := order[a], order[b]
		if positions[i].y != positions[j].y {
			return positions[i].y < positions[j].y
		}
		return data.Nodes[ids[i]].ID < data.Nodes[ids[j]].ID
	})
	columns := max(1, int(math.Ceil(math.Sqrt(float64(n)*188/236))))
	for start := 0; start < n; start += columns {
		end := min(n, start+columns)
		sort.Slice(order[start:end], func(a, b int) bool {
			i, j := order[start+a], order[start+b]
			if positions[i].x != positions[j].x {
				return positions[i].x < positions[j].x
			}
			return data.Nodes[ids[i]].ID < data.Nodes[ids[j]].ID
		})
	}
	return order
}
