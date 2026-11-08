package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
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

func TestProcessResolvesStackLinkToStackNode(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	card := testCard(stack, 1)
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{card}, links: []*HyperCardLink{{Source: stack, Target: card}}}
	metadata, err := p.Process()
	if err != nil {
		t.Fatal(err)
	}
	if got := metadata.Edges[0].Source; !got.IsOfType(common.IsStack) || got.Name != stack.Name {
		t.Fatalf("stack link starts at %+v, want the stack node", got)
	}
}

func TestProcessClassifiesEdgesByActualStacks(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	from, to := testCard(stack, 1), testCard(stack, 2)
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{from, to}, links: []*HyperCardLink{{Source: from, Target: to, IsCrossAges: true}}}
	metadata, err := p.Process()
	if err != nil {
		t.Fatal(err)
	}
	if edge := metadata.Edges[0]; !edge.IsOfType(common.IntraAge) || edge.IsOfType(common.CrossAge) {
		t.Fatalf("an explicit reference to the same stack is not cross-age: %+v", edge)
	}
}

func TestIdentifyLinksPairsPushWithFollowingGo(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	source := testCard(stack, 1, "go card id 2\npush card id 4 of stack \"Example\"\ngo card id 3")
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source, testCard(stack, 2), testCard(stack, 3), testCard(stack, 4)}}
	if err := p.identifyLinks(); err != nil {
		t.Fatal(err)
	}
	var tail, head *HyperCardLink
	for _, link := range p.links {
		if link.TransitivityRank == common.Tail {
			tail = link
		}
		if link.TransitivityRank == common.Head {
			head = link
		}
	}
	if tail == nil || head == nil || tail.Target != p.cards[2] || head.Source != p.cards[2] || head.Target != p.cards[3] {
		t.Fatalf("push was not paired with the following go: tail=%+v, head=%+v", tail, head)
	}
}

func TestIdentifyLinksRejectsPushWithoutGo(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	source := testCard(stack, 1, "push card id 2 of stack \"Example\"")
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source, testCard(stack, 2)}}
	if err := p.identifyLinks(); err == nil {
		t.Fatal("want an error for a push without a following go")
	}
}

func TestIdentifyLinksHandlesSeveralPushesInOneScript(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	source := testCard(stack, 1, "push card id 3 of stack \"Example\"\ngo card id 2\npush card id 5 of stack \"Example\"\ngo card id 4")
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source, testCard(stack, 2), testCard(stack, 3), testCard(stack, 4), testCard(stack, 5)}}
	if err := p.identifyLinks(); err != nil {
		t.Fatal(err)
	}
	heads := 0
	for _, link := range p.links {
		if link.TransitivityRank == common.Head {
			heads++
			source := link.Source.(*HyperCardCard)
			target := link.Target.(*HyperCardCard)
			if target.ID != source.ID+1 {
				t.Fatalf("incorrect return pair: %d -> %d", source.ID, target.ID)
			}
		}
	}
	if heads != 2 {
		t.Fatalf("got %d return links, want 2", heads)
	}
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

func TestCardCommandsIgnoreStringLiteralsAndInlineComments(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	for _, command := range []string{
		`put "go card id 2" into text`,
		`put "pop card" into text`,
		`put "push card" into text`,
		`doSomething -- go card id 2`,
		`logo card id 2`,
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

func TestCardCommandsMatchOutsideStringLiterals(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	card := testCard(stack, 1)
	target := testCard(stack, 2)
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{card, target}}
	for _, command := range []string{
		`if value is "go card id 1" then go card id 2`,
		"go\tto\tcard\tid\t2",
		`go card id 2 -- a comment containing "go card id 1"`,
	} {
		link, err := p.parseCardCommand(card, command)
		if err != nil || link == nil || link.Target != target {
			t.Errorf("command %q: got %+v, %v; want target %s", command, link, err, target.Name)
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
