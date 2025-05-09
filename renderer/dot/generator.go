package dot

import (
	"fmt"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
)

type Config struct {
	UseNodeNames     bool
	ShowLegend       bool
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
	generator := &Generator{
		graph:             graph,
		metadata:          metadata,
		config:            DefaultConfig(),
		nodeStyles:        make(map[int64]nodeStyle),
		edgeStyles:        make(map[string]edgeStyle),
		stackColors:       make(map[string]nodeColors),
		customPathEdgeSet: make(map[string]bool),
	}

	generator.initialize()
	return generator
}

// initialize sets up the generator for DOT generation
func (g *Generator) initialize() {
	g.initializeEdgesMap()
	g.initializeCustomPath()
}

// initializeEdgesMap builds the internal edge map from metadata.Edges
func (g *Generator) initializeEdgesMap() {
	edgesMap := make(map[int64]map[int64][]common.Edge)

	for i := range g.metadata.Edges {
		edge := g.metadata.Edges[i]
		fromID := edge.Source.GraphID
		toID := edge.Target.GraphID

		if edgesMap[fromID] == nil {
			edgesMap[fromID] = make(map[int64][]common.Edge)
		}

		edgesMap[fromID][toID] = append(edgesMap[fromID][toID], *edge)
	}

	g.edgesMap = edgesMap
}

// initializeCustomPath sets up the custom path edge set for fast lookups
func (g *Generator) initializeCustomPath() {
	g.customPathEdgeSet = make(map[string]bool)

	if len(g.customPath) > 1 {
		for i := 0; i < len(g.customPath)-1; i++ {
			from := g.customPath[i]
			to := g.customPath[i+1]
			g.customPathEdgeSet[fmt.Sprintf("%d->%d", from, to)] = true
		}
	}
}

// Generate creates a DOT representation of the Myst Graph
// NOTE: it would be preferable to use a library instead of performing string manipulations
func (g *Generator) Generate(path []int64) (string, error) {
	if path != nil {
		g.customPath = path
		g.initializeCustomPath()
	}

	if g.config.ColorByStack && len(g.stackColors) == 0 {
		g.assignStackColors()
	}

	if g.config.IncludeAnalysis {
		if err := g.applyAnalysisStyles(); err != nil {
			return "", err
		}
	}

	dotStr, err := g.buildDOT()
	if err != nil {
		return "", err
	}

	return dotStr, nil
}

func (g *Generator) isOnCustomPath(fromID, toID int64) bool {
	return g.customPathEdgeSet[fmt.Sprintf("%d->%d", fromID, toID)]
}
