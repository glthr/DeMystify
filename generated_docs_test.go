package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
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
