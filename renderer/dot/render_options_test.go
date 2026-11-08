package dot

import (
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
)

func TestAnalysisRespectsHighlightOptions(t *testing.T) {
	for _, options := range []struct{ sources, sinks bool }{
		{false, false}, {true, false}, {false, true}, {true, true},
	} {
		g, _ := newTestGenerator(t, []common.EdgeAttribute{common.IntraAge})
		g.graph.Process()
		g.config.HighlightSources = options.sources
		g.config.HighlightSinks = options.sinks
		output, err := g.Generate(nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(output, "Source node") != options.sources || strings.Contains(output, "Sink node") != options.sinks {
			t.Errorf("source/sink options %+v were ignored", options)
		}
	}
}

func TestNumericLabelsKeepSecondaryNames(t *testing.T) {
	for _, analysis := range []bool{false, true} {
		g, metadata := newTestGenerator(t, []common.EdgeAttribute{common.IntraAge})
		secondary := "Background, detail"
		metadata.Nodes[0].SecondaryName = &secondary
		copy := g.graph.NodeMap["A"]
		copy.SecondaryName = &secondary
		g.graph.NodeMap["A"] = copy
		g.graph.Process()
		g.config.UseNodeNames = false
		g.config.IncludeAnalysis = analysis
		output, err := g.Generate(nil)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output, `label="0\nBackground, detail"`) {
			t.Errorf("analysis=%v: numeric label was replaced by the node name", analysis)
		}
	}
}

func TestAnalysisPreservesPageBorders(t *testing.T) {
	for _, page := range []struct {
		attrs []common.NodeAttribute
		color string
	}{
		{[]common.NodeAttribute{common.ContainsBluePage}, "#0000ff"},
		{[]common.NodeAttribute{common.ContainsRedPage}, "#ff0000"},
		{[]common.NodeAttribute{common.ContainsWhitePage}, "#dedbdb"},
		{[]common.NodeAttribute{common.ContainsBluePage, common.ContainsRedPage}, "#6a1cea"},
	} {
		for _, index := range []int{0, 1} {
			g, metadata := newTestGenerator(t, []common.EdgeAttribute{common.IntraAge})
			node := metadata.Nodes[index]
			node.Attributes = append(node.Attributes, page.attrs...)
			g.graph.Process()
			if _, err := g.Generate(nil); err != nil {
				t.Fatal(err)
			}
			style := g.getNodeStyle(node.GraphID, g.graph.NodeMap[node.Name])
			fill := "#98FB98"
			if index == 1 {
				fill = "#FFA07A"
			}
			if style.borderColor != page.color || style.penWidth != 3 || style.fillColor != fill {
				t.Errorf("page %v, node %d: lost page border or analysis fill: %+v", page.attrs, index, style)
			}
		}
	}
}
