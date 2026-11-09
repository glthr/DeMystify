package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOutputOptionsKeepPDFAndAddBrowserExport(t *testing.T) {
	options, err := parseOutputOptions([]string{"stacks"})
	if err != nil {
		t.Fatal(err)
	}
	if options.format != "all" || options.webDir != "generated/web" || options.webLayout != "auto" || options.arrangement != "original" || options.input != "stacks" {
		t.Fatalf("default output: %+v", options)
	}
	options, err = parseOutputOptions([]string{"--format", "html", "--assets", "views", "--web-output", "viewer", "stacks"})
	if err != nil {
		t.Fatal(err)
	}
	if options.format != "html" || options.assetsDir != "views" || options.webDir != "viewer" {
		t.Fatalf("HTML options: %+v", options)
	}
	options, err = parseOutputOptions([]string{"--layout", "original", "stacks"})
	if err != nil || options.arrangement != "original" {
		t.Fatalf("original arrangement: %+v, %v", options, err)
	}
	options, err = parseOutputOptions([]string{"--layout", "ages", "stacks"})
	if err != nil || options.arrangement != "ages" {
		t.Fatalf("optional Age arrangement: %+v, %v", options, err)
	}
	for _, args := range [][]string{{"--format", "unknown", "stacks"}, {"--web-output", "", "stacks"}, {"--web-layout", "unknown", "stacks"}, {"--layout", "unknown", "stacks"}, {"--missing", "stacks"}} {
		if _, err := parseOutputOptions(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestHTMLModeDoesNotRequireNeato(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stack_-1.xml"), []byte("<stack><name>Example</name></stack>"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "card_1.xml"), []byte("<card><id>1</id></card>"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	err := run([]string{"--format", "html", dir})
	if err == nil || !strings.Contains(err.Error(), "expected 6 stacks") {
		t.Fatalf("HTML mode should reach input validation without Neato: %v", err)
	}
	if err := run([]string{"--format", "pdf", dir}); err == nil || !strings.Contains(err.Error(), "neato") {
		t.Fatalf("PDF mode should require Neato: %v", err)
	}
	if err := run([]string{"--format", "html", "--web-layout", "pdf", dir}); err == nil || !strings.Contains(err.Error(), "neato") {
		t.Fatalf("explicit PDF layout should require Neato: %v", err)
	}
	if err := run([]string{"--format", "html", "--web-layout", "atlas", dir}); err == nil || !strings.Contains(err.Error(), "expected 6 stacks") {
		t.Fatalf("explicit atlas should not require Neato: %v", err)
	}
}
