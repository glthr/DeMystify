package parser

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/glthr/DeMystify/hypercard"
)

func TestNativeCompatibilityOrderKeepsOriginalCardOrder(t *testing.T) {
	stack := hypercard.Stack{
		Cards:       []hypercard.Card{{ID: 2}, {ID: 11}, {ID: 1}},
		Backgrounds: []hypercard.Background{{ID: 2}, {ID: 11}, {ID: 1}},
		CardOrder:   []int{2, 11, 1},
	}
	compatibilityOrder(&stack)
	var cards, backgrounds []int
	for _, card := range stack.Cards {
		cards = append(cards, card.ID)
	}
	for _, background := range stack.Backgrounds {
		backgrounds = append(backgrounds, background.ID)
	}
	if !reflect.DeepEqual(cards, []int{1, 11, 2}) || !reflect.DeepEqual(backgrounds, cards) {
		t.Fatalf("incorrect compatibility order: cards=%v backgrounds=%v", cards, backgrounds)
	}
	if !reflect.DeepEqual(stack.CardOrder, []int{2, 11, 1}) {
		t.Fatalf("native order was changed: %v", stack.CardOrder)
	}
}

func TestLoadInputDetectsXMLAndRejectsMixedDirectory(t *testing.T) {
	root := t.TempDir()
	xmlDir := filepath.Join(root, "Converted.xstk")
	if err := os.Mkdir(xmlDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xmlDir, stackFileName), []byte("<stack><name>Example</name></stack>"), 0600); err != nil {
		t.Fatal(err)
	}
	corpus, err := LoadInput(root)
	if err != nil || len(corpus.Stacks) != 1 || corpus.Stacks[0].Name != "Example" {
		t.Fatalf("XML autodetection: corpus=%+v err=%v", corpus, err)
	}
	// A native signature is sufficient for detecting ambiguity; decoding must
	// not silently choose XML and hide the other input.
	if err := os.WriteFile(filepath.Join(root, "Native"), []byte{0, 0, 6, 0, 'S', 'T', 'A', 'K'}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadInput(root); err == nil || !strings.Contains(err.Error(), "native and XML") {
		t.Fatalf("mixed directory: got %v", err)
	}
}

func TestLoadInputRejectsInvalidInputs(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "not-a-stack")
	if err := os.WriteFile(file, []byte("unrelated file"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, file, filepath.Join(root, "missing")} {
		if _, err := LoadInput(path); err == nil {
			t.Errorf("accepted %s", path)
		}
	}
}
