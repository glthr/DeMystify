package dot

import (
	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
)

type Config struct {
	UseNodeNames     bool
	ShowLegend       bool // Reserved; currently has no effect on DOT output.
	IncludeAnalysis  bool
	ColorByStack     bool
	HighlightSinks   bool
	HighlightSources bool
}

func DefaultConfig() Config {
	return Config{
		UseNodeNames:     true,
		IncludeAnalysis:  true,
		ColorByStack:     true,
		HighlightSinks:   true,
		HighlightSources: true,
	}
}

// NewGenerator creates a new DOT generator with the provided graph and metadata
func NewGenerator(graph *graph.MystGraph, metadata *common.Metadata) *Generator {
	return &Generator{
		graph:    graph,
		metadata: metadata,
		config:   DefaultConfig(),
	}
}

// Generate creates a DOT representation of the Myst Graph
// NOTE: it would be preferable to use a library instead of performing string manipulations
func (g *Generator) Generate(path []int64) (string, error) {
	g.customPath = append([]int64(nil), path...)
	g.nodeStyles = make(map[int64]nodeStyle)

	g.stackColors = make(map[string]nodeColors)
	if g.config.ColorByStack {
		g.assignStackColors()
	}

	if g.config.IncludeAnalysis {
		if err := g.applyAnalysisStyles(); err != nil {
			return "", err
		}
	}

	return g.buildDOT(), nil
}
