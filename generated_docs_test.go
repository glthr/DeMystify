package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The published DOT hash is quoted in prose. Keep it tied to the tracked file,
// so that regenerating the graph cannot leave an outdated receipt behind.
func TestGeneratedReadmeQuotesTrackedDOTHash(t *testing.T) {
	dot, err := os.ReadFile("generated/graph.dot")
	if err != nil {
		t.Fatal(err)
	}
	readme, err := os.ReadFile("generated/README.md")
	if err != nil {
		t.Fatal(err)
	}
	// A checkout may convert line endings; the receipt describes the LF file.
	want := fmt.Sprintf("%x", sha256.Sum256(bytes.ReplaceAll(dot, []byte("\r\n"), []byte("\n"))))
	if !strings.Contains(string(readme), "`"+want+"`") {
		t.Fatalf("generated/README.md does not quote the SHA256 of generated/graph.dot (%s)", want)
	}
}

// The node and connection counts are quoted in prose too, in two files that
// had drifted apart before. Both follow from the tracked DOT alone.
func TestReadmesQuoteTheCountsOfTheTrackedDOT(t *testing.T) {
	dot, err := os.ReadFile("generated/graph.dot")
	if err != nil {
		t.Fatal(err)
	}
	nodes, pairs := dotCounts(strings.ReplaceAll(string(dot), "\r\n", "\n"))
	count := fmt.Sprintf("%s nodes and %s non-disabled directed", thousands(nodes), thousands(pairs))
	for _, name := range []string{"README.md", "generated/README.md"} {
		text, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(text), count) {
			t.Errorf("%s does not quote the tracked graph: %q", name, count)
		}
	}
}

var (
	dotNode = regexp.MustCompile(`^\s*"(\d+)" \[.*\blabel=`)
	dotEdge = regexp.MustCompile(`^\s*"(\d+)" -> "(\d+)" \[(.*)\];$`)
)

// dotCounts returns the labelled nodes and the non-disabled directed pairs of
// a generated DOT. A stroke written with an unquoted dir=both stands for one
// reference in each direction; a tee head marks a disabled reference.
func dotCounts(dot string) (int, int) {
	nodes := 0
	pairs := make(map[[2]string]bool)
	for _, line := range strings.Split(dot, "\n") {
		if dotNode.MatchString(line) {
			nodes++
			continue
		}
		edge := dotEdge.FindStringSubmatch(line)
		if edge == nil || strings.Contains(edge[3], "style=invis") || strings.Contains(edge[3], `arrowhead="tee"`) {
			continue
		}
		pairs[[2]string{edge[1], edge[2]}] = true
		if strings.Contains(edge[3], "dir=both") {
			pairs[[2]string{edge[2], edge[1]}] = true
		}
	}
	return nodes, len(pairs)
}

func thousands(n int) string {
	text := fmt.Sprint(n)
	for i := len(text) - 3; i > 0; i -= 3 {
		text = text[:i] + "," + text[i:]
	}
	return text
}
