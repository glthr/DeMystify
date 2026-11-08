package dot

import (
	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
	"strings"
	"testing"
)

func newTestGenerator(t *testing.T, attrs ...[]common.EdgeAttribute) (*Generator, *common.Metadata) {
	t.Helper()
	a := &common.Node{Name: "A", StackName: "Myst", Attributes: []common.NodeAttribute{common.IsCard}}
	b := &common.Node{Name: "B", StackName: "Myst", Attributes: []common.NodeAttribute{common.IsCard}}
	metadata := &common.Metadata{Nodes: []*common.Node{a, b}}
	for i, attributes := range attrs {
		source, target := a, b
		if i%2 == 1 {
			source, target = b, a
		}
		metadata.Edges = append(metadata.Edges, &common.Edge{Source: source, Target: target, Attributes: attributes})
	}
	g, err := graph.NewGraph(metadata)
	if err != nil {
		t.Fatal(err)
	}
	return NewGenerator(g, metadata), metadata
}

func edgeLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, " -> ") {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestGenerateNilClearsPreviousPath(t *testing.T) {
	g, metadata := newTestGenerator(t, []common.EdgeAttribute{common.IntraAge})
	if _, err := g.Generate([]int64{metadata.Nodes[0].GraphID, metadata.Nodes[1].GraphID}); err != nil {
		t.Fatal(err)
	}
	output, err := g.Generate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "#E74C3C") || strings.Contains(output, "Custom path:") {
		t.Fatalf("nil path retained previous highlighting:\n%s", output)
	}
}
