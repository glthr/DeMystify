package parser

import "testing"

func TestEdgeDeduplicationDoesNotJoinAmbiguousNames(t *testing.T) {
	ab, a := &HyperCardStack{Name: "A-B"}, &HyperCardStack{Name: "A"}
	c, bc := &HyperCardStack{Name: "C"}, &HyperCardStack{Name: "B-C"}
	first, second := testCard(c, 1), testCard(bc, 1)
	left := &HyperCardLink{Source: ab, Target: first}
	right := &HyperCardLink{Source: a, Target: second}
	for _, links := range [][]*HyperCardLink{{left, right, left}, {right, left, right}} {
		p := &Parser{stacks: []*HyperCardStack{ab, a, c, bc}, cards: []*HyperCardCard{first, second}, links: links}
		metadata, err := p.Process()
		if err != nil {
			t.Fatal(err)
		}
		if len(metadata.Edges) != 2 {
			t.Fatalf("distinct endpoint pairs were merged: got %d edges, want 2", len(metadata.Edges))
		}
	}
}
