package main

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/glthr/DeMystify/graph"
	"github.com/glthr/DeMystify/hypercard"
	"github.com/glthr/DeMystify/parser"
	"github.com/glthr/DeMystify/renderer/dot"
)

// Original game files remain optional local inputs and must never enter Git.
func TestNativeMystParity(t *testing.T) {
	nativePath := os.Getenv("DEMYSTIFY_NATIVE_TEST_DATA")
	xmlPath := os.Getenv("DEMYSTIFY_TEST_DATA")
	if nativePath == "" || xmlPath == "" {
		t.Skip("set DEMYSTIFY_NATIVE_TEST_DATA and DEMYSTIFY_TEST_DATA to matching native and XML stacks")
	}
	native, err := parser.LoadInput(nativePath)
	if err != nil {
		t.Fatal(err)
	}
	xml, err := parser.LoadXML(xmlPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(native.Stacks) != 6 || len(native.Stacks) != len(xml.Stacks) {
		t.Fatalf("unexpected stack inventory: native=%d XML=%d", len(native.Stacks), len(xml.Stacks))
	}
	cards := 0
	for i := range native.Stacks {
		a, b := canonicalStack(native.Stacks[i]), canonicalStack(xml.Stacks[i])
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("decoded stack %q differs from matching XML (identities, order, scripts, parts, contents or ownership)", a.Name)
		}
		cards += len(a.Cards)
	}
	if cards != 1355 {
		t.Fatalf("got %d native cards, want 1355", cards)
	}
	var generated [2]string
	var metadata [2]any
	for i, corpus := range []hypercard.Corpus{native, xml} {
		p, err := parser.NewParserFromCorpus(corpus)
		if err != nil {
			t.Fatal(err)
		}
		m, err := p.Process()
		if err != nil {
			t.Fatal(err)
		}
		g, err := graph.NewGraph(m)
		if err != nil {
			t.Fatal(err)
		}
		if m.TotalNodes != 1364 || m.TotalEdges != 3249 || len(m.Edges) != 3275 {
			t.Fatalf("input %d: got %d nodes, %d active pairs, %d edge records", i, m.TotalNodes, m.TotalEdges, len(m.Edges))
		}
		g.Process()
		metadata[i] = m
		generated[i], err = dot.NewGenerator(g, m).Generate(nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(metadata[0], metadata[1]) {
		t.Fatal("native and XML imports produce different complete graph metadata")
	}
	if generated[0] != generated[1] {
		t.Fatal("native and XML imports produce different DOT output")
	}
}

func canonicalStack(stack hypercard.Stack) hypercard.Stack {
	stack.Source = hypercard.Source{}
	stack.Diagnostics = nil
	stack.Script = canonicalScript(stack.Script)
	stack.Cards = append([]hypercard.Card(nil), stack.Cards...)
	stack.Backgrounds = append([]hypercard.Background(nil), stack.Backgrounds...)
	for i := range stack.Cards {
		card := &stack.Cards[i]
		card.Source = hypercard.Source{}
		card.Script = canonicalScript(card.Script)
		card.Parts = canonicalParts(card.Parts)
		card.Contents = canonicalContents(card.Contents)
	}
	for i := range stack.Backgrounds {
		background := &stack.Backgrounds[i]
		background.Source = hypercard.Source{}
		background.Script = canonicalScript(background.Script)
		background.Parts = canonicalParts(background.Parts)
		background.Contents = canonicalContents(background.Contents)
	}
	return stack
}

func canonicalScript(script hypercard.Script) hypercard.Script {
	return hypercard.Script{Text: canonicalText(script.Text)}
}

func canonicalText(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

func canonicalParts(parts []hypercard.Part) []hypercard.Part {
	result := append([]hypercard.Part(nil), parts...)
	for i := range result {
		result[i].Source = hypercard.Source{}
		result[i].Script = canonicalScript(result[i].Script)
	}
	return result
}

func canonicalContents(contents []hypercard.Content) []hypercard.Content {
	result := append([]hypercard.Content(nil), contents...)
	for i := range result {
		result[i].Text = canonicalText(result[i].Text)
	}
	return result
}
