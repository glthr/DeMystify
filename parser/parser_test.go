package parser

import (
	"fmt"
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
