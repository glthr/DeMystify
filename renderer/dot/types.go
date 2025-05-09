package dot

import (
	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
)

// Generator handles the conversion of a Myst Graph to DOT format
type Generator struct {
	graph       *graph.MystGraph
	config      Config
	metadata    *common.Metadata
	nodeStyles  map[int64]nodeStyle
	edgeStyles  map[string]edgeStyle
	stackColors map[string]nodeColors
	customPath  []int64
	// from -> to -> edges
	edgesMap map[int64]map[int64][]common.Edge
	// pre-calculated custom path edges for fast lookup
	customPathEdgeSet map[string]bool
}

// nodeStyle contains styling attributes for a node
type nodeStyle struct {
	shape       string
	style       string
	fillColor   string
	borderColor string
	fontColor   string
	penWidth    float64
	tooltip     string
	label       string
}

// edgeStyle contains styling attributes for an edge
type edgeStyle struct {
	color         string
	style         string
	penWidth      float64
	bidirectional bool
	weight        float64
	minLen        int
	tooltip       string
	arrowtail     string // used for restrictive transitivity edges
	arrowhead     string // used for restrictive transitivity edges
	isSelfLoop    bool   // flag for self-loop edges
}

type nodeColors struct {
	fillColor   string
	borderColor string
}

// NOTE: I arbitrarily set these colors after several attempts. It could be cool to use gradients representing when cards
// have been created based on their original IDs (automatically incremented by the HyperCard software).
var nodesColors = []string{
	"#ffffbA",
	"#d9fcc5",
	"#e6baff",
	"#c3fff3",
	"#bae1ff",
	"#f7e8ee",
}
