package parser

import (
	"testing"

	"github.com/glthr/DeMystify/common"
)

func TestRestrictiveReturnDoesNotRewriteAnotherHandler(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	source := testCard(stack, 1, "on ordinary\ngo card id 2\nend ordinary\non returning\npush card id 3 of stack \"Example\"\ngo card id 2\nend returning")
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source, testCard(stack, 2), testCard(stack, 3)}}
	if err := p.identifyLinks(); err != nil {
		t.Fatal(err)
	}
	metadata, err := p.Process()
	if err != nil {
		t.Fatal(err)
	}
	plain := 0
	for _, edge := range metadata.Edges {
		if edge.Source.Name == "Example:1" && edge.Target.Name == "Example:2" && !edge.IsOfType(common.RestrictiveTransitivityTail) {
			plain++
		}
	}
	if plain != 1 {
		t.Fatalf("independent ordinary handler lost its unrestricted route: got %d, want 1", plain)
	}
}
