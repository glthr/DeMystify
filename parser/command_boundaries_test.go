package parser

import "testing"

func TestCommandsDoNotInventLiteralTargetsFromExpressions(t *testing.T) {
	stack, other := &HyperCardStack{Name: "Example"}, &HyperCardStack{Name: "Other"}
	name := "Gate"
	for _, command := range []string{
		"go card id 2 + offset",
		"go card id 2.5",
		"go card id 2suffix",
		"go card id 2 of stack destination",
		`go card id 2 of stack "Other" & suffix`,
		`go card "Gate" & suffix`,
		`go card "Gate" of stack "Other" & suffix`,
		"go this card of stack destination",
		"push card id 2 of stack destination",
		"pop card into savedAddress",
	} {
		source, target, remote := testCard(stack, 1), testCard(stack, 2), testCard(other, 2)
		target.OriginalName, remote.OriginalName = &name, &name
		p := &Parser{stacks: []*HyperCardStack{stack, other}, cards: []*HyperCardCard{source, target, remote}}
		link, err := p.parseCardCommand(source, command)
		if err != nil || link != nil || source.IsPushCard || source.IsPopCard {
			t.Errorf("unsupported expression %q invented navigation: link=%+v, push=%v, pop=%v, err=%v", command, link, source.IsPushCard, source.IsPopCard, err)
		}
	}
}
