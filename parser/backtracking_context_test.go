package parser

import "testing"

func TestBacktrackingPushDoesNotLeakAcrossHandlersOrParts(t *testing.T) {
	for _, scripts := range [][]string{
		{"on zoom\npush card\ngo card id 2\nend zoom\non ordinary\ngo card id 3\nend ordinary"},
		{"push card\ngo card id 2", "go card id 3"},
	} {
		stack := &HyperCardStack{Name: "Example"}
		source := testCard(stack, 1, scripts...)
		p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source, testCard(stack, 2, "pop card"), testCard(stack, 3, "pop card")}}
		if err := p.identifyLinks(); err != nil {
			t.Fatal(err)
		}
		for _, link := range p.links {
			if link.Source == source {
				want := link.Target.(*HyperCardCard).ID == 2
				if link.IsBacktracking != want {
					t.Errorf("target %s: backtracking=%v, want %v", link.Target.(*HyperCardCard).Name, link.IsBacktracking, want)
				}
			}
		}
	}
}
