// Package layout adapts Neato's JSON drawing output to top-down point units.
// Graphviz draw-operation order and routed geometry are preserved; metadata
// edge records remain the caller's separate graph model.
package layout

type Bounds struct {
	MinX float64 `json:"minX"`
	MinY float64 `json:"minY"`
	MaxX float64 `json:"maxX"`
	MaxY float64 `json:"maxY"`
}

type Drawing struct {
	Bounds Bounds      `json:"bounds"`
	Nodes  []Node      `json:"nodes"`
	Edges  []Edge      `json:"edges"`
	Draw   []Operation `json:"draw,omitempty"`
	Labels []Operation `json:"labels,omitempty"`
}

// Node X/Y are centers. Width/Height retain Graphviz's original boxes in points.
type Node struct {
	ID        int64       `json:"id"`
	X         float64     `json:"x"`
	Y         float64     `json:"y"`
	Width     float64     `json:"width"`
	Height    float64     `json:"height"`
	Fill      string      `json:"fill"`
	Stroke    string      `json:"stroke"`
	FontColor string      `json:"fontColor"`
	PenWidth  float64     `json:"penWidth"`
	Shape     string      `json:"shape"`
	Style     string      `json:"style"`
	Draw      []Operation `json:"draw,omitempty"`
	Labels    []Operation `json:"labels,omitempty"`
}

type Edge struct {
	Source   int64       `json:"source"`
	Target   int64       `json:"target"`
	Color    string      `json:"color"`
	Style    string      `json:"style"`
	PenWidth float64     `json:"penWidth"`
	Draw     []Operation `json:"draw,omitempty"`
	Head     []Operation `json:"head,omitempty"`
	Tail     []Operation `json:"tail,omitempty"`
	Labels   []Operation `json:"labels,omitempty"`
}

// Operation uses Graphviz JSON keys and opcode case. Point positions are
// normalized to top-down coordinates; radii, font sizes and widths are unchanged.
// For e/E rect is [centerX,centerY,radiusX,radiusY]; for I it is [left,top,w,h].
type Operation struct {
	Op       string      `json:"op"`
	Points   [][]float64 `json:"points,omitempty"`
	Rect     []float64   `json:"rect,omitempty"`
	Color    string      `json:"color,omitempty"`
	Style    string      `json:"style,omitempty"`
	Pt       []float64   `json:"pt,omitempty"`
	Align    string      `json:"align,omitempty"`
	Width    float64     `json:"width,omitempty"`
	Text     string      `json:"text,omitempty"`
	Size     float64     `json:"size,omitempty"`
	Face     string      `json:"face,omitempty"`
	Fontchar int         `json:"fontchar,omitempty"`
	Grad     string      `json:"grad,omitempty"`
	P0       []float64   `json:"p0,omitempty"`
	P1       []float64   `json:"p1,omitempty"`
	Stops    []Stop      `json:"stops,omitempty"`
	Name     string      `json:"name,omitempty"`
}

type Stop struct {
	Frac  float64 `json:"frac"`
	Color string  `json:"color"`
}
