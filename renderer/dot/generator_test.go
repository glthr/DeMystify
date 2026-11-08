package dot

import (
	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
	"os/exec"
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

func TestSmallComponentsDoNotCreatePhantomSingleton(t *testing.T) {
	g, metadata := newTestGenerator(t)
	metadata.Stats.ConnectedComponents = [][]int64{{metadata.Nodes[0].GraphID, metadata.Nodes[1].GraphID}, {2, 3}}
	output, err := g.Generate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, `"__SINGLETON_ANCHOR__"`) {
		t.Fatalf("small component created nonexistent singleton anchor:\n%s", output)
	}
}

func TestNodeLabelEscapingAndPathStyling(t *testing.T) {
	g, metadata := newTestGenerator(t)
	node := metadata.Nodes[0]
	original := "quoted \"name\" \\ path\nnext"
	secondary := "one, label=two"
	copy := g.graph.NodeMap[node.Name]
	copy.OriginalName = &original
	copy.SecondaryName = &secondary
	g.graph.NodeMap[node.Name] = copy
	output, err := g.Generate([]int64{node.GraphID})
	if err != nil {
		t.Fatal(err)
	}
	expected := `label="A (quoted \"name\" \\ path\nnext)\none, label=two"`
	if !strings.Contains(output, expected) {
		t.Fatalf("label was not safely escaped or path style corrupted its comma:\n%s", output)
	}
	if neato, err := exec.LookPath("neato"); err == nil {
		cmd := exec.Command(neato, "-Tdot")
		cmd.Stdin = strings.NewReader(output)
		if diagnostic, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Graphviz rejected generated DOT: %v\n%s", err, diagnostic)
		}
	}
}
