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
