package dot

import (
	"bytes"
	"fmt"
	"sort"
)

// writeComponentClusters organizes graph into visually separated components
// NOTE: the general idea is to try to separate the isolated components from the
// main Myst Graph components
func (g *Generator) writeComponentClusters(buf *bytes.Buffer) {
	components := g.metadata.Stats.ConnectedComponents

	if len(components) <= 1 {
		return
	}

	buf.WriteString(fmt.Sprintf("  // Main component\n"))

	// categorize the components
	mainComp, smallComps, singletons := g.categorizeComponents(components)

	g.createLayoutNodes(buf)

	// place the main component
	g.placeMainComponent(buf, mainComp)

	// place the singletons
	g.placeSingletons(buf, singletons)

	// place the small components
	g.placeSmallComponents(buf, smallComps)
}

// categorizeComponents separates components into main, small, and singletons
func (g *Generator) categorizeComponents(components [][]int64) ([]int64, [][]int64, []int64) {
	// always treat first component as main
	mainComp := components[0]

	// collect small components and singletons
	var smallComps [][]int64
	var singletons []int64

	for i := 1; i < len(components); i++ {
		comp := components[i]
		if len(comp) == 1 {
			singletons = append(singletons, comp[0])
		} else if len(comp) <= 10 {
			smallComps = append(smallComps, comp)
		}
	}

	// sort singletons for deterministic output
	sort.Slice(singletons, func(i, j int) bool {
		return singletons[i] < singletons[j]
	})

	// sort small components
	for i := range smallComps {
		sort.Slice(smallComps[i], func(a, b int) bool {
			return smallComps[i][a] < smallComps[i][b]
		})
	}
	sort.Slice(smallComps, func(i, j int) bool {
		return smallComps[i][0] < smallComps[j][0]
	})

	return mainComp, smallComps, singletons
}

// createLayoutNodes adds invisible control nodes for layout positioning
func (g *Generator) createLayoutNodes(buf *bytes.Buffer) {
	buf.WriteString("  // Layout control nodes\n")

	// cardinal direction anchors
	anchors := []string{"TOP", "BOTTOM", "LEFT", "RIGHT"}
	for _, anchor := range anchors {
		buf.WriteString(fmt.Sprintf("  \"__%s__\" [shape=none, width=0.01, height=0.01, style=invis];\n", anchor))
	}

	// basic constraints
	buf.WriteString("  { rank=source; \"__TOP__\"; }\n")
	buf.WriteString("  { rank=sink; \"__BOTTOM__\"; }\n")
	buf.WriteString("  \"__TOP__\" -> \"__BOTTOM__\" [style=invis, weight=0.01];\n")
	buf.WriteString("  \"__LEFT__\" -> \"__RIGHT__\" [style=invis, weight=0.01];\n")
}

// placeMainComponent positions the main component in the graph
func (g *Generator) placeMainComponent(buf *bytes.Buffer, nodes []int64) {
	buf.WriteString("  // Main component\n")
	buf.WriteString("  \"__MAIN_ANCHOR__\" [shape=none, width=0.01, height=0.01, style=invis];\n")

	// connect to top of graph
	buf.WriteString("  \"__TOP__\" -> \"__MAIN_ANCHOR__\" [style=invis, weight=100];\n")

	// connect anchor to first node
	if len(nodes) > 0 {
		buf.WriteString(fmt.Sprintf("  \"__MAIN_ANCHOR__\" -> \"%d\" [style=invis, weight=100];\n", nodes[0]))
	}

	// ensure all nodes have constraint set
	for _, nodeID := range nodes {
		buf.WriteString(fmt.Sprintf("  \"%d\" [constraint=true];\n", nodeID))
	}
}

// placeSingletons places singleton nodes in the bottom-right area
func (g *Generator) placeSingletons(buf *bytes.Buffer, singletons []int64) {
	if len(singletons) == 0 {
		return
	}

	buf.WriteString("\n  // Singleton components\n")
	buf.WriteString("  \"__SINGLETON_ANCHOR__\" [shape=none, width=0.01, height=0.01, style=invis];\n")

	buf.WriteString("  \"__BOTTOM__\" -> \"__SINGLETON_ANCHOR__\" [style=invis, weight=100];\n")
	buf.WriteString("  \"__RIGHT__\" -> \"__SINGLETON_ANCHOR__\" [style=invis, weight=100];\n")

	// push away from the main component
	buf.WriteString("  \"__MAIN_ANCHOR__\" -> \"__SINGLETON_ANCHOR__\" [style=invis, weight=0.0001, len=100, constraint=true];\n")

	// connect to the first singleton
	buf.WriteString(fmt.Sprintf("  \"__SINGLETON_ANCHOR__\" -> \"%d\" [style=invis, weight=5];\n", singletons[0]))

	// create an invisible subgraph to group singletons
	buf.WriteString("  subgraph cluster_singletons {\n")
	buf.WriteString("    graph [style=invis];\n")

	for _, nodeID := range singletons {
		buf.WriteString(fmt.Sprintf("    \"%d\";\n", nodeID))
	}

	for i := 0; i < len(singletons)-1; i++ {
		buf.WriteString(fmt.Sprintf("    \"%d\" -> \"%d\" [style=invis, weight=10];\n",
			singletons[i], singletons[i+1]))
	}

	buf.WriteString("  }\n")
}

// placeSmallComponents positions small components at bottom-left
func (g *Generator) placeSmallComponents(buf *bytes.Buffer, components [][]int64) {
	if len(components) == 0 {
		return
	}

	buf.WriteString("\n  // Small non-singleton components\n")
	buf.WriteString("  \"__SMALL_ANCHOR__\" [shape=none, width=0.01, height=0.01, style=invis];\n")

	// position at the bottom left
	buf.WriteString("  \"__BOTTOM__\" -> \"__SMALL_ANCHOR__\" [style=invis, weight=100];\n")
	buf.WriteString("  \"__LEFT__\" -> \"__SMALL_ANCHOR__\" [style=invis, weight=100];\n")

	// keep away from the main component
	buf.WriteString("  \"__MAIN_ANCHOR__\" -> \"__SMALL_ANCHOR__\" [style=invis, weight=0.0001, len=100, constraint=true];\n")

	buf.WriteString("  \"__SMALL_ANCHOR__\" -> \"__SINGLETON_ANCHOR__\" [style=invis, weight=0.0001, len=100, constraint=true];\n")

	for i, comp := range components {
		if len(comp) == 0 {
			continue
		}

		// connect the first component to anchor
		if i == 0 {
			buf.WriteString(fmt.Sprintf("  \"__SMALL_ANCHOR__\" -> \"%d\" [style=invis, weight=5];\n", comp[0]))
		}

		// create a subgraph for this component
		buf.WriteString(fmt.Sprintf("  subgraph cluster_small_%d {\n", i+1))
		buf.WriteString("    graph [style=invis];\n")

		for _, nodeID := range comp {
			buf.WriteString(fmt.Sprintf("    \"%d\";\n", nodeID))
		}

		for j := 0; j < len(comp)-1; j++ {
			buf.WriteString(fmt.Sprintf("    \"%d\" -> \"%d\" [style=invis, weight=10];\n",
				comp[j], comp[j+1]))
		}

		buf.WriteString("  }\n")

		if i > 0 && len(components[i-1]) > 0 {
			buf.WriteString(fmt.Sprintf("  \"%d\" -> \"%d\" [style=invis, weight=1];\n",
				components[i-1][0], comp[0]))
		}
	}
}
