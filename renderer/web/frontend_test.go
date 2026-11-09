package web

import (
	"context"
	"os/exec"
	"regexp"
	"testing"
	"time"
)

// Include the offline viewer's DOM/canvas contract regressions in ordinary Go
// checks when Node is installed. The harness does not claim visual browser QA.
func TestFrontendBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is unavailable; skipping viewer DOM/canvas contract tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, node, "--test", "viewer_test.cjs").CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("viewer contract tests exceeded 30 seconds: %v\n%s", ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("viewer contract tests failed: %v\n%s", err, output)
	}
}

// The narrow layout hides explicit line breaks (viewer.css), so the words on
// both sides of one must remain separated by a space.
func TestTemplateLineBreaksKeepWordsSeparated(t *testing.T) {
	template, err := viewerFiles.ReadFile("viewer.html")
	if err != nil {
		t.Fatal(err)
	}
	if glued := regexp.MustCompile(`\S<br\s*/?>\S`).Find(template); glued != nil {
		t.Fatalf("hiding this line break joins two words: %q", glued)
	}
}
