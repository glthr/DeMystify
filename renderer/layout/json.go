package layout

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type graphJSON struct {
	BB            string       `json:"bb"`
	SubgraphCount int          `json:"_subgraph_cnt"`
	Objects       []objectJSON `json:"objects"`
	Edges         []edgeJSON   `json:"edges"`
	Draw          []Operation  `json:"_draw_"`
	Labels        []Operation  `json:"_ldraw_"`
}

type objectJSON struct {
	GVID      *int            `json:"_gvid"`
	Name      string          `json:"name"`
	Nodes     json.RawMessage `json:"nodes"`
	Pos       string          `json:"pos"`
	Width     string          `json:"width"`
	Height    string          `json:"height"`
	Fill      string          `json:"fillcolor"`
	Stroke    string          `json:"color"`
	FontColor string          `json:"fontcolor"`
	PenWidth  string          `json:"penwidth"`
	Shape     string          `json:"shape"`
	Style     string          `json:"style"`
	Draw      []Operation     `json:"_draw_"`
	Labels    []Operation     `json:"_ldraw_"`
}

type edgeJSON struct {
	Source     *int        `json:"tail"`
	Target     *int        `json:"head"`
	Color      string      `json:"color"`
	Style      string      `json:"style"`
	PenWidth   string      `json:"penwidth"`
	Draw       []Operation `json:"_draw_"`
	Head       []Operation `json:"_hdraw_"`
	Tail       []Operation `json:"_tdraw_"`
	Labels     []Operation `json:"_ldraw_"`
	HeadLabels []Operation `json:"_hldraw_"`
	TailLabels []Operation `json:"_tldraw_"`
}

// FromJSON reads Graphviz -Tjson output (not json0). Coordinates become
// top-down points with an origin at the drawing's top left. Node sizes convert
// from inches to points; Bezier controls, arrows and paint order are retained.
// Subgraphs and invisible layout anchors/edges do not become graph records.
// See https://graphviz.org/docs/outputs/json/ for the source format.
func FromJSON(data []byte) (Drawing, error) {
	if len(data) > maxJSONBytes {
		return Drawing{}, fmt.Errorf("Graphviz JSON exceeds size limit")
	}
	var input graphJSON
	if err := json.Unmarshal(data, &input); err != nil {
		return Drawing{}, fmt.Errorf("reading Graphviz JSON: %w", err)
	}
	bb, err := vector(input.BB, 4)
	if err != nil || bb[2] <= bb[0] || bb[3] <= bb[1] || !finite(bb[2]-bb[0]) || !finite(bb[3]-bb[1]) {
		return Drawing{}, fmt.Errorf("Graphviz drawing requires a finite positive bounding box")
	}
	if input.SubgraphCount < 0 || input.SubgraphCount > len(input.Objects) {
		return Drawing{}, fmt.Errorf("invalid Graphviz subgraph count")
	}
	transform := coordinateTransform{minX: bb[0], maxY: bb[3]}
	result := Drawing{Bounds: Bounds{MaxX: bb[2] - bb[0], MaxY: bb[3] - bb[1]}, Nodes: []Node{}, Edges: []Edge{}}
	result.Draw, err = transform.operations(input.Draw)
	if err != nil {
		return Drawing{}, fmt.Errorf("graph drawing: %w", err)
	}
	result.Labels, err = transform.operations(input.Labels)
	if err != nil {
		return Drawing{}, fmt.Errorf("graph labels: %w", err)
	}
	ids := make(map[int]int64)
	ignored := make(map[int]bool)
	allGVID := make(map[int]bool)
	nodeIDs := make(map[int64]bool)
	for i, object := range input.Objects {
		if object.GVID == nil || *object.GVID < 0 || allGVID[*object.GVID] {
			return Drawing{}, fmt.Errorf("missing, negative or repeated Graphviz object ID")
		}
		gvid := *object.GVID
		allGVID[gvid] = true
		if i < input.SubgraphCount || object.Nodes != nil || invisible(object.Style) {
			ignored[gvid] = true
			continue
		}
		id, err := strconv.ParseInt(object.Name, 10, 64)
		if err != nil || id < 0 || strconv.FormatInt(id, 10) != object.Name {
			return Drawing{}, fmt.Errorf("visible Graphviz node %q does not have a canonical numeric graph ID", object.Name)
		}
		if nodeIDs[id] {
			return Drawing{}, fmt.Errorf("repeated graph node ID %d", id)
		}
		nodeIDs[id] = true
		position, err := vector(object.Pos, 2)
		if err != nil {
			return Drawing{}, fmt.Errorf("node %d position: %w", id, err)
		}
		width, widthErr := number(object.Width)
		height, heightErr := number(object.Height)
		width, height = width*72, height*72
		if widthErr != nil || heightErr != nil || !finite(width) || !finite(height) || width <= 0 || height <= 0 {
			return Drawing{}, fmt.Errorf("node %d requires positive finite width and height", id)
		}
		penWidth, err := lineWidth(object.PenWidth)
		if err != nil {
			return Drawing{}, fmt.Errorf("node %d: %w", id, err)
		}
		draw, err := transform.operations(object.Draw)
		if err != nil {
			return Drawing{}, fmt.Errorf("node %d drawing: %w", id, err)
		}
		labels, err := transform.operations(object.Labels)
		if err != nil {
			return Drawing{}, fmt.Errorf("node %d labels: %w", id, err)
		}
		x, y := transform.point(position[0], position[1])
		if !finite(x) || !finite(y) {
			return Drawing{}, fmt.Errorf("node %d normalized position is not finite", id)
		}
		fill, stroke, font := object.Fill, object.Stroke, object.FontColor
		if fill == "" {
			fill = paintColor(draw, "C", "transparent")
		}
		if stroke == "" {
			stroke = paintColor(draw, "c", "#000000")
		}
		if font == "" {
			font = paintColor(labels, "c", "#000000")
		}
		shape := object.Shape
		if shape == "" {
			shape = "ellipse"
		}
		result.Nodes = append(result.Nodes, Node{ID: id, X: x, Y: y, Width: width, Height: height,
			Fill: fill, Stroke: stroke, FontColor: font, PenWidth: penWidth,
			Shape: shape, Style: object.Style, Draw: draw, Labels: labels})
		ids[gvid] = id
	}
	for i, edge := range input.Edges {
		if invisible(edge.Style) {
			continue
		}
		if edge.Source == nil || edge.Target == nil {
			return Drawing{}, fmt.Errorf("edge %d has no Graphviz endpoint ID", i)
		}
		if !allGVID[*edge.Source] || !allGVID[*edge.Target] {
			return Drawing{}, fmt.Errorf("edge %d refers to an unknown Graphviz object ID", i)
		}
		if ignored[*edge.Source] || ignored[*edge.Target] {
			continue
		}
		penWidth, err := lineWidth(edge.PenWidth)
		if err != nil {
			return Drawing{}, fmt.Errorf("edge %d: %w", i, err)
		}
		value := Edge{Source: ids[*edge.Source], Target: ids[*edge.Target],
			Color: edge.Color, Style: edge.Style, PenWidth: penWidth}
		for _, group := range []struct {
			source []Operation
			target *[]Operation
		}{{edge.Draw, &value.Draw}, {edge.Head, &value.Head}, {edge.Tail, &value.Tail}} {
			*group.target, err = transform.operations(group.source)
			if err != nil {
				return Drawing{}, fmt.Errorf("edge %d drawing: %w", i, err)
			}
		}
		combined := append(append(append([]Operation(nil), edge.Labels...), edge.HeadLabels...), edge.TailLabels...)
		value.Labels, err = transform.operations(combined)
		if err != nil {
			return Drawing{}, fmt.Errorf("edge %d labels: %w", i, err)
		}
		if value.Color == "" {
			value.Color = paintColor(value.Draw, "c", "#000000")
		}
		result.Edges = append(result.Edges, value)
	}
	return result, nil
}

type coordinateTransform struct{ minX, maxY float64 }

func (t coordinateTransform) point(x, y float64) (float64, float64) {
	return x - t.minX, t.maxY - y
}

func (t coordinateTransform) operations(input []Operation) ([]Operation, error) {
	if input == nil {
		return nil, nil
	}
	result := make([]Operation, 0, len(input))
	for _, operation := range input {
		if !finite(operation.Width) || operation.Width < 0 || !finite(operation.Size) || operation.Size < 0 {
			return nil, fmt.Errorf("invalid draw-operation width or font size")
		}
		for _, point := range operation.Points {
			if err := t.transformPoint(point, 2); err != nil {
				return nil, err
			}
		}
		if operation.Pt != nil {
			if err := t.transformPoint(operation.Pt, 2); err != nil {
				return nil, err
			}
		}
		for _, point := range [][]float64{operation.P0, operation.P1} {
			if point != nil {
				if len(point) != 2 && len(point) != 3 {
					return nil, fmt.Errorf("invalid gradient point")
				}
				if err := t.transformPoint(point, len(point)); err != nil {
					return nil, err
				}
				if len(point) == 3 && point[2] < 0 {
					return nil, fmt.Errorf("negative gradient radius")
				}
			}
		}
		for _, stop := range operation.Stops {
			if !finite(stop.Frac) || stop.Frac < 0 || stop.Frac > 1 {
				return nil, fmt.Errorf("invalid gradient stop")
			}
		}
		switch operation.Op {
		case "e", "E":
			if len(operation.Rect) != 4 || !finiteSlice(operation.Rect) || operation.Rect[2] < 0 || operation.Rect[3] < 0 {
				return nil, fmt.Errorf("invalid ellipse rectangle")
			}
			operation.Rect[0], operation.Rect[1] = t.point(operation.Rect[0], operation.Rect[1])
			if !finiteSlice(operation.Rect) {
				return nil, fmt.Errorf("non-finite normalized ellipse")
			}
		case "b", "B":
			if len(operation.Points) < 4 || (len(operation.Points)-1)%3 != 0 {
				return nil, fmt.Errorf("invalid cubic Bezier control-point count")
			}
		case "p", "P":
			if len(operation.Points) < 3 {
				return nil, fmt.Errorf("invalid polygon point count")
			}
		case "L":
			if len(operation.Points) < 2 {
				return nil, fmt.Errorf("invalid polyline point count")
			}
		case "T":
			if len(operation.Pt) != 2 || operation.Align != "l" && operation.Align != "c" && operation.Align != "r" {
				return nil, fmt.Errorf("invalid text position or alignment")
			}
		case "F":
			if operation.Face == "" {
				return nil, fmt.Errorf("missing draw-operation font face")
			}
		case "t":
			if operation.Fontchar < 0 || operation.Fontchar > 127 {
				return nil, fmt.Errorf("invalid font style flags")
			}
		case "c", "C":
			if operation.Grad != "" && operation.Grad != "none" && operation.Grad != "linear" && operation.Grad != "radial" {
				return nil, fmt.Errorf("unsupported draw-operation gradient")
			}
			if operation.Grad == "linear" || operation.Grad == "radial" {
				points := 2
				if operation.Grad == "radial" {
					points = 3
				}
				if len(operation.P0) != points || len(operation.P1) != points || len(operation.Stops) < 2 {
					return nil, fmt.Errorf("incomplete draw-operation gradient")
				}
			}
		case "S":
		case "I":
			return nil, fmt.Errorf("external image drawing operations are unsupported")
		default:
			return nil, fmt.Errorf("unsupported Graphviz draw operation %q", operation.Op)
		}
		result = append(result, operation)
	}
	return result, nil
}

func (t coordinateTransform) transformPoint(point []float64, size int) error {
	if len(point) != size || !finiteSlice(point) {
		return fmt.Errorf("invalid drawing point")
	}
	point[0], point[1] = t.point(point[0], point[1])
	if !finiteSlice(point) {
		return fmt.Errorf("non-finite normalized point")
	}
	return nil
}

func vector(value string, count int) ([]float64, error) {
	parts := strings.Split(value, ",")
	if len(parts) != count {
		return nil, fmt.Errorf("expected %d coordinates", count)
	}
	values := make([]float64, count)
	for i, part := range parts {
		value, err := number(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	return values, nil
}

func number(value string) (float64, error) {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || !finite(parsed) {
		return 0, fmt.Errorf("invalid finite number %q", value)
	}
	return parsed, nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func finiteSlice(values []float64) bool {
	for _, value := range values {
		if !finite(value) {
			return false
		}
	}
	return true
}

func invisible(style string) bool {
	for _, token := range strings.Split(style, ",") {
		if strings.EqualFold(strings.TrimSpace(token), "invis") {
			return true
		}
	}
	return false
}

func lineWidth(value string) (float64, error) {
	if value == "" {
		return 1, nil
	}
	width, err := number(value)
	if err != nil || width < 0 {
		return 0, fmt.Errorf("invalid pen width %q", value)
	}
	return width, nil
}

func paintColor(operations []Operation, op, fallback string) string {
	for _, operation := range operations {
		if operation.Op == op && operation.Color != "" {
			return operation.Color
		}
	}
	return fallback
}
