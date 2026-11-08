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

func TestStackCardOnlyCommandsDoNotPanic(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	p := &Parser{stacks: []*HyperCardStack{stack}}
	for _, command := range []string{"pop card", `put "1,A,0" into ALL_Page`, `put "1,S,0" into ALL_Page`, `put "Atrus" into ALL_Page`} {
		if _, err := p.parseCardCommand(stack, command); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDisabledCommandsDoNotSetCardProperties(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	card := testCard(stack, 1)
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{card}}
	for _, command := range []string{"-- push card", "-- pop card", `-- put "1,A,0" into ALL_Page`, `-- put "1,S,0" into ALL_Page`, `-- put "Atrus" into ALL_Page`} {
		if _, err := p.parseCardCommand(card, command); err != nil {
			t.Fatal(err)
		}
	}
	if card.IsPushCard || card.IsPopCard || card.HasBluePage || card.HasRedPage || card.HasWhitePage {
		t.Fatalf("commented commands changed card properties: %+v", card)
	}
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

func TestCardCommandsIgnoreStringLiterals(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	for _, command := range []string{
		`put "go card id 2" into text`,
		`put "pop card" into text`,
		`put "push card" into text`,
	} {
		card := testCard(stack, 1)
		p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{card, testCard(stack, 2)}}
		link, err := p.parseCardCommand(card, command)
		if err != nil {
			t.Fatal(err)
		}
		if link != nil || card.IsPushCard || card.IsPopCard {
			t.Errorf("non-command %q changed card navigation: link=%+v card=%+v", command, link, card)
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
