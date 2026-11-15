package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGameOptionsSelectTheSharedPipeline(t *testing.T) {
	options, err := parseOutputOptions([]string{"--game", "riven", "--output", "local-export", "--extract-images", "local-views", "discs"})
	if err != nil {
		t.Fatal(err)
	}
	if options.game != "riven" || options.outputRoot != "local-export" || options.extractImages != "local-views" || options.input != "discs" {
		t.Fatalf("lost game options: %+v", options)
	}
	if options.audit {
		t.Fatal("the audit must be requested explicitly")
	}
	if options, err = parseOutputOptions([]string{"--game", "riven", "--audit", "discs"}); err != nil || !options.audit {
		t.Fatalf("lost the audit option: %+v, %v", options, err)
	}
	for _, args := range [][]string{{"--game", "unknown", "input"}, {"--extract-images", "images", "--assets", "other", "input"}} {
		if _, err := parseOutputOptions(args); err == nil {
			t.Fatalf("accepted inconsistent options: %v", args)
		}
	}
}

func TestInputDetectionRecognizesContainersAndKeepsHyperCardDefault(t *testing.T) {
	input := t.TempDir()
	if game, err := inputGame(input, "auto"); err != nil || game != "myst" {
		t.Fatalf("native default: %s,%v", game, err)
	}
	nested := filepath.Join(input, "disc-data")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "RIVEN1.ISO"), []byte("synthetic detector fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if game, err := inputGame(input, "auto"); err != nil || game != "riven" {
		t.Fatalf("container detection: %s,%v", game, err)
	}
	if game, err := inputGame(input, "myst"); err != nil || game != "myst" {
		t.Fatalf("explicit engine selection: %s,%v", game, err)
	}
}

func TestRivenOutputMustNotOverlapTheOriginalInputDirectory(t *testing.T) {
	input := t.TempDir()
	if err := protectInputDirectory(input, filepath.Join(input, "generated")); err == nil {
		t.Fatal("accepted writes within original inputs")
	}
	if err := protectInputDirectory(input, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
