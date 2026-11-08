package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testCard(stack *HyperCardStack, id int, scripts ...string) *HyperCardCard {
	card := &HyperCardCard{ID: id, Name: fmt.Sprintf("%s:%d", stack.Name, id), Stack: stack}
	for _, text := range scripts {
		var script HyperTalk
		for _, line := range splitScript(text) {
			script.lines = append(script.lines, ScriptLine{Line: line, IsDisabled: strings.HasPrefix(strings.TrimSpace(line), "--")})
		}
		card.Scripts = append(card.Scripts, script)
	}
	return card
}

func TestCardCommandsRejectOverflowingIDs(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	card := testCard(stack, 1)
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{card}}
	for _, command := range []string{
		"go card id 999999999999999999999999999999",
		`go card id 999999999999999999999999999999 of stack "Example"`,
		`push card id 999999999999999999999999999999 of stack "Example"`,
	} {
		if _, err := p.parseCardCommand(card, command); err == nil {
			t.Errorf("want an error for overflowing ID in %q", command)
		}
	}
}

func TestCreateVirtualCardUsesCanonicalStackName(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	p := &Parser{stacks: []*HyperCardStack{stack}}
	card, err := p.createVirtualCard("example", 42)
	if err != nil {
		t.Fatal(err)
	}
	if card.Name != "Example:42" || card.ID != 42 {
		t.Fatalf("got virtual card %+v, want canonical name and ID", card)
	}
}

func TestSplitScriptSupportsClassicMacLineEndings(t *testing.T) {
	if got := splitScript("one\rtwo\r\nthree\nfour"); len(got) != 4 {
		t.Fatalf("got %q, want four script lines", got)
	}
}

func TestParseSimpleCardRejectsWrongXMLRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "card_1.xml")
	if err := os.WriteFile(path, []byte("<stack><id>1</id></stack>"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSimpleCard(&HyperCardStack{Name: "Example"}, path, nil); err == nil {
		t.Fatal("want an error for a stack XML document passed as a card")
	}
}
