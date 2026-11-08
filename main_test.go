package main

import (
	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
	"github.com/glthr/DeMystify/parser"
	"os"
	"path/filepath"
	"runtime"
	"testing"
"strings"
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
