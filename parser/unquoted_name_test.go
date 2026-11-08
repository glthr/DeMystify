package parser

import "testing"

func unquotedNameFixture() (*Parser, *HyperCardCard, *HyperCardCard) {
	stack := &HyperCardStack{Name: "Example"}
	source, target := testCard(stack, 1), testCard(stack, 2)
	name := "Black"
	target.OriginalName = &name
	return &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source, target}}, source, target
}

func TestUnquotedOriginalCardNames(t *testing.T) {
	for _, command := range []string{"go card Black", "go to card bLaCk", "if ready then go card Black"} {
		p, source, target := unquotedNameFixture()
		link, err := p.parseCardCommand(source, command)
		if err != nil || link == nil || link.Target != target {
			t.Fatalf("%q: got %+v, %v", command, link, err)
		}
	}
	for _, command := range []string{"go card", "go card 2", "go card missing", "go card Black & suffix", "go card Black of card 1", "put \"go card Black\" into message"} {
		p, source, _ := unquotedNameFixture()
		if link, err := p.parseCardCommand(source, command); err != nil || link != nil {
			t.Fatalf("unsupported %q: got %+v, %v", command, link, err)
		}
	}
}
