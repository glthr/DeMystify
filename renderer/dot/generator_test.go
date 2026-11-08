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

func TestBidirectionalMergeRetainsParallelEdges(t *testing.T) {
	g, metadata := newTestGenerator(t, []common.EdgeAttribute{common.IntraAge}, []common.EdgeAttribute{common.IntraAge})
	metadata.Edges = append(metadata.Edges, &common.Edge{Source: metadata.Nodes[0], Target: metadata.Nodes[1], Attributes: []common.EdgeAttribute{common.Disabled}})
	g.initializeEdgesMap()
	output, err := g.Generate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(edgeLines(output)) != 2 || !strings.Contains(output, "Disabled edge") {
		t.Fatalf("bidirectional merge lost disabled parallel edge:\n%s", output)
	}
}

func TestCustomPathPreservesReverseAndParallelEdges(t *testing.T) {
	g, metadata := newTestGenerator(t, []common.EdgeAttribute{common.IntraAge}, []common.EdgeAttribute{common.IntraAge})
	metadata.Edges = append(metadata.Edges, &common.Edge{Source: metadata.Nodes[0], Target: metadata.Nodes[1], Attributes: []common.EdgeAttribute{common.Disabled}})
	g.initializeEdgesMap()
	output, err := g.Generate([]int64{metadata.Nodes[0].GraphID, metadata.Nodes[1].GraphID})
	if err != nil {
		t.Fatal(err)
	}
	if len(edgeLines(output)) != 3 || !strings.Contains(output, "Disabled edge") {
		t.Fatalf("path highlight lost reverse or parallel edges:\n%s", output)
	}
}

func TestBidirectionalMergeRetainsTransitiveEdges(t *testing.T) {
	g, metadata := newTestGenerator(t, []common.EdgeAttribute{common.IntraAge}, []common.EdgeAttribute{common.IntraAge})
	metadata.Edges = append(metadata.Edges,
		&common.Edge{Source: metadata.Nodes[0], Target: metadata.Nodes[1], Attributes: []common.EdgeAttribute{common.RestrictiveTransitivityTail}, TransitivityID: 1},
		&common.Edge{Source: metadata.Nodes[0], Target: metadata.Nodes[1], Attributes: []common.EdgeAttribute{common.RestrictiveTransitivityTail}, TransitivityID: 2},
	)
	output, err := g.Generate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(edgeLines(output)) != 3 || !strings.Contains(output, "Tail ID 1") || !strings.Contains(output, "Tail ID 2") {
		t.Fatalf("bidirectional merge discarded transitive edges:\n%s", output)
	}
}

func TestBidirectionalMergeMatchesEachEdgesAttributes(t *testing.T) {
	g, metadata := newTestGenerator(t, []common.EdgeAttribute{common.IntraAge}, []common.EdgeAttribute{common.CrossAge})
	metadata.Edges = append(metadata.Edges, &common.Edge{Source: metadata.Nodes[0], Target: metadata.Nodes[1], Attributes: []common.EdgeAttribute{common.CrossAge}})
	output, err := g.Generate(nil)
	if err != nil {
		t.Fatal(err)
	}
	lines := edgeLines(output)
	if len(lines) != 2 {
		t.Fatalf("unmatched forward edge lost: %v", lines)
	}
	for _, line := range lines {
		if strings.Contains(line, "dir=both") && !strings.Contains(line, "CrossAge connection") {
			t.Fatalf("merged edge used attributes from an unmatched edge: %s", line)
		}
	}
}
