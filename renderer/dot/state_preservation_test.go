package dot

import (
	"reflect"
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
)

func TestCustomPathRetainsDisabledState(t *testing.T) {
	g, metadata := newTestGenerator(t, []common.EdgeAttribute{common.Disabled})
	output, err := g.Generate([]int64{metadata.Nodes[0].GraphID, metadata.Nodes[1].GraphID})
	if err != nil {
		t.Fatal(err)
	}
	lines := edgeLines(output)
	if len(lines) != 1 || !strings.Contains(lines[0], "Path edge (disabled)") || !strings.Contains(lines[0], `arrowhead="tee"`) {
		t.Fatalf("highlight concealed disabled edge state: %v", lines)
	}
}

func TestComponentLayoutDoesNotMutateAnalysis(t *testing.T) {
	g, metadata := newTestGenerator(t)
	metadata.Stats.ConnectedComponents = [][]int64{{0, 1, 2}, {4, 3}}
	want := [][]int64{{0, 1, 2}, {4, 3}}
	if _, err := g.Generate(nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(metadata.Stats.ConnectedComponents, want) {
		t.Fatalf("rendering reordered analysis data: %v", metadata.Stats.ConnectedComponents)
	}
}
