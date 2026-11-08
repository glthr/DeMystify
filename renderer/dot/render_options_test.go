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
