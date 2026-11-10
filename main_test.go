package main

import (
	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
	"github.com/glthr/DeMystify/parser"
	"github.com/glthr/DeMystify/renderer/dot"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckNeatoInstalledUsesPath(t *testing.T) {
	dir := t.TempDir()
	name := "neato"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("placeholder"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if err := CheckNeatoInstalled(); err != nil {
		t.Fatalf("neato is available without an external which/where command: %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if err := CheckNeatoInstalled(); err == nil {
		t.Fatal("expected error when neato is absent")
	}
}

func parsedTestGraph(t *testing.T) (*parser.Parser, *graph.MystGraph, *common.Metadata) {
	t.Helper()
	root := t.TempDir()
	stackDir := filepath.Join(root, "Test Age")
	if err := os.Mkdir(stackDir, 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"stack_-1.xml": `<stack><name>Test Age</name><card id="1" name="Start"/><card id="2" name="End"/></stack>`,
		"card_1.xml":   `<card><id>1</id><script>go card id 2</script></card>`,
		"card_2.xml":   `<card><id>2</id></card>`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(stackDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := parser.NewParser(root)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := p.Process()
	if err != nil {
		t.Fatal(err)
	}
	g, err := graph.NewGraph(metadata)
	if err != nil {
		t.Fatal(err)
	}
	return p, g, metadata
}

func TestComputeShortestPathLookupErrors(t *testing.T) {
	p, g, _ := parsedTestGraph(t)
	for _, tc := range []struct {
		name     string
		from, to int
	}{
		{name: "missing source", from: 99, to: 2},
		{name: "missing target", from: 1, to: 99},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ComputeShortestPath(p, g, "Test Age", tc.from, "Test Age", tc.to)
			if err == nil || result != nil {
				t.Fatalf("got (%v, %v), want nil result and lookup error", result, err)
			}
		})
	}
}

func TestRunReportsUsageBeforeCheckingDependencies(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, args := range [][]string{nil, {"one", "two"}} {
		if err := run(args); err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Fatalf("run(%v) = %v, want usage error", args, err)
		}
	}
}

func TestSyntheticPipeline(t *testing.T) {
	p, g, metadata := parsedTestGraph(t)
	g.Process()
	shortest, err := ComputeShortestPath(p, g, "Test Age", 1, "Test Age", 2)
	if err != nil {
		t.Fatal(err)
	}
	if shortest.Distance != 1 || len(shortest.Path) != 2 {
		t.Fatalf("unexpected shortest path: %+v", shortest)
	}
	content, err := dot.NewGenerator(g, metadata).Generate(shortest.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Test Age:1", "Test Age:2"} {
		if !strings.Contains(content, label) {
			t.Fatalf("DOT output is missing %q", label)
		}
	}
	neato, err := exec.LookPath("neato")
	if err != nil {
		t.Skip("Graphviz is unavailable; parser, graph, and DOT checks passed")
	}
	cmd := exec.Command(neato, "-Tdot")
	cmd.Stdin = strings.NewReader(content)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Graphviz rejected generated DOT: %v\n%s", err, output)
	}
}

// A run prints the figures the project publishes, not only two totals.
func TestAnalysisSummaryReportsRecordsFlagsAndTheLongestDistance(t *testing.T) {
	a := &common.Node{Name: "Test Age:1", Attributes: []common.NodeAttribute{common.IsCard}}
	b := &common.Node{Name: "Test Age:2", Attributes: []common.NodeAttribute{common.IsCard}}
	c := &common.Node{Name: "Test Age:3", Attributes: []common.NodeAttribute{common.IsCard}}
	d := &common.Node{Name: "Test Age:4", Attributes: []common.NodeAttribute{common.IsCard}}
	record := func(source, target *common.Node, attributes ...common.EdgeAttribute) *common.Edge {
		return &common.Edge{Source: source, Target: target, Attributes: append([]common.EdgeAttribute{common.IntraAge}, attributes...)}
	}
	metadata := &common.Metadata{Nodes: []*common.Node{a, b, c, d}, Edges: []*common.Edge{
		record(a, b), record(b, c), record(a, c),
		record(b, c, common.SequenceContinuation), record(c, c), record(c, a, common.Backtracking), record(a, a, common.Disabled),
	}}
	g, err := graph.NewGraph(metadata)
	if err != nil {
		t.Fatal(err)
	}
	g.Process()
	want := "Edge records: 7 (saved returns: 1, automatic next steps: 1, disabled: 1)\n" +
		"Source / sink / isolated flags: 1 / 1 / 1\n" +
		"Nodes with an active reference to themselves: 1\n" +
		"Largest finite shortest distance: 1 (Test Age:1 to Test Age:2)\n"
	if got := analysisSummary(metadata); got != want {
		t.Fatalf("summary:\n%s\nwant:\n%s", got, want)
	}
	if got := analysisSummary(&common.Metadata{}); strings.Contains(got, "Largest") {
		t.Fatalf("an empty graph reported a distance:\n%s", got)
	}
}
