package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoveryIncludesTheProvidedStackRoot(t *testing.T) {
	root := t.TempDir()
	stackDir := filepath.Join(root, "Converted.xstk")
	if err := os.Mkdir(stackDir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		stackFileName: "<stack><name>Example</name></stack>",
		"card_1.xml":  "<card><id>1</id></card>",
	} {
		if err := os.WriteFile(filepath.Join(stackDir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	paths := []string{root, stackDir}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(stackDir, alias); err == nil {
		paths = append(paths, alias)
	}
	for _, path := range paths {
		p, err := NewParser(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.stacks) != 1 || len(p.cards) != 1 {
			t.Errorf("%s: got %d stacks and %d cards, want one of each", path, len(p.stacks), len(p.cards))
		}
	}
}

func TestDiscoveryRejectsFilesAndEmptyDirectories(t *testing.T) {
	for _, path := range []string{t.TempDir(), writeXMLFixture(t, "stack.xml", "<stack/>")} {
		if _, err := NewParser(path); err == nil {
			t.Errorf("accepted an input with no stack directories: %s", path)
		}
	}
}
