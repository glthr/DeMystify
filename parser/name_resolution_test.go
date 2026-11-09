package parser

import "testing"

func TestScriptNamesDoNotResolveGeneratedGraphAliases(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	source, byID, byName := testCard(stack, 1), testCard(stack, 2), testCard(stack, 3)
	name := "Example:2"
	byName.OriginalName = &name
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source, byID, byName}}
	for _, command := range []string{`go card "Example:2"`, `go card "Example:2" of stack "Example"`} {
		link, err := p.parseCardCommand(source, command)
		if err != nil || link == nil || link.Target != byName {
			t.Errorf("%q: got %+v, %v; want the original card name", command, link, err)
		}
	}
	if got, err := p.GetCardByStackAndName("Example", "Example:2"); err != nil || got != byID {
		t.Fatal("public lookup must keep its generated-name compatibility")
	}
}

func TestQuotedCardPositionsAreNotMistakenForNames(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	source, second, named := testCard(stack, 1), testCard(stack, 7), testCard(stack, 42)
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source, second, named}}
	for _, name := range []string{"2", "0002", "999999999999999999999999999999", "-2", "2.0", "2.", ".5", "-.5"} {
		named.OriginalName = &name
		for _, scope := range []string{"", ` of stack "Example"`} {
			command := `go card "` + name + `"` + scope
			link, err := p.parseCardCommand(source, command)
			if err != nil || link != nil {
				t.Errorf("unsupported ordinal %q was treated as a card name: link=%+v, err=%v", command, link, err)
			}
		}
	}
}

func TestQuotedNamesAreNotParsedAsGeneralFloatingPointValues(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	source, target := testCard(stack, 1), testCard(stack, 2)
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source, target}}
	for _, name := range []string{"2A", "2.0A", "NaN", "Inf", "+2"} {
		target.OriginalName = &name
		for _, scope := range []string{"", ` of stack "Example"`} {
			if link, err := p.parseCardCommand(source, `go card "`+name+`"`+scope); err != nil || link == nil || link.Target != target {
				t.Errorf("literal card name %q did not resolve: link=%+v err=%v", name, link, err)
			}
		}
	}
}
