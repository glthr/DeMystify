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

func TestIdentifyLinksDoesNotPairAcrossHandlers(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	for _, declaration := range []string{"on", "function"} {
		source := testCard(stack, 1, declaration+" first\npush card id 2 of stack \"Example\"\nend first\n"+declaration+" second\ngo card id 3\nend second")
		p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source, testCard(stack, 2), testCard(stack, 3)}}
		if err := p.identifyLinks(); err == nil {
			t.Errorf("%s handlers: want an error instead of a return path through a different handler", declaration)
		}
	}
}

func TestIdentifyLinksPromotesAllDuplicateRestrictedLinks(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	source := testCard(stack, 1, "go card id 2", "go card id 2", "push card id 3 of stack \"Example\"\ngo card id 2")
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source, testCard(stack, 2), testCard(stack, 3)}}
	if err := p.identifyLinks(); err != nil {
		t.Fatal(err)
	}
	for _, link := range p.links {
		if link.Source == source && link.Target == p.cards[1] && link.TransitivityRank != common.Tail {
			t.Fatalf("duplicate go bypasses restricted link: %+v", link)
		}
	}
}

func TestIdentifyLinksAssignsDistinctTransitivityIDs(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	source := testCard(stack, 1, "push card id 3 of stack \"Example\"\ngo card id 2", "push card id 5 of stack \"Example\"\ngo card id 4")
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source, testCard(stack, 2), testCard(stack, 3), testCard(stack, 4), testCard(stack, 5)}}
	if err := p.identifyLinks(); err != nil {
		t.Fatal(err)
	}
	ids := make(map[int64]bool)
	for _, link := range p.links {
		if link.TransitivityRank == common.Tail {
			ids[link.TransitivityID] = true
		}
	}
	if len(ids) != 2 {
		t.Fatalf("got %d distinct restricted pair IDs, want 2", len(ids))
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

func TestIdentifyLinksPreservesDisabledStackCommands(t *testing.T) {
	stack := &HyperCardStack{Name: "Example", Script: []HyperTalk{{lines: []ScriptLine{{Line: "-- go card id 1", IsDisabled: true}}}}}
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{testCard(stack, 1)}}
	if err := p.identifyLinks(); err != nil {
		t.Fatal(err)
	}
	if len(p.links) != 1 || !p.links[0].IsDisabled {
		t.Fatalf("commented stack command must produce a disabled link: %+v", p.links)
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

func TestCardCommandsResolveQuotedLocalNames(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	source, target := testCard(stack, 1), testCard(stack, 2)
	name := "Transition"
	target.OriginalName = &name
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source, target}}
	for _, origin := range []any{source, stack} {
		for _, command := range []string{`go card "Transition"`, `go to card "transition"`} {
			link, err := p.parseCardCommand(origin, command)
			if err != nil || link == nil || link.Target != target || link.IsCrossAges || link.IsNotImplemented {
				t.Errorf("%q from %T: got %+v, %v; want local card %s", command, origin, link, err, target.Name)
			}
		}
	}
	link, err := p.parseCardCommand(source, `go to card "Unavailable"`)
	if err != nil || link == nil || !link.IsNotImplemented || link.Target.(*HyperCardCard).Name != "Example:Unavailable" {
		t.Fatalf("unknown local name: got %+v, %v; want an unimplemented virtual target", link, err)
	}
}

func TestCardCommandsResolveExplicitCurrentCard(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	source := testCard(stack, 1, "go this card\n-- go to this card")
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source}}
	if err := p.identifyLinks(); err != nil {
		t.Fatal(err)
	}
	if len(p.links) != 2 {
		t.Fatalf("got %d self-reference links, want active and disabled links", len(p.links))
	}
	for i, link := range p.links {
		if link.Source != source || link.Target != source || link.IsDisabled != (i == 1) || link.IsCrossAges || link.IsNotImplemented {
			t.Errorf("incorrect self-reference: %+v", link)
		}
	}
	if link, err := p.parseCardCommand(stack, "go this card"); err != nil || link != nil {
		t.Fatalf("a stack's runtime current card must not be guessed: got %+v, %v", link, err)
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

func FuzzIdentifyScriptLinks(f *testing.F) {
	for _, script := range []string{
		"go card id 2",
		"push card id 3 of stack \"Example\"\ngo card id 2",
		"pop card\r-- push card\rput \"1,A,0\" into ALL_Page",
		"push card id 2 of stack \"Example\"",
		"go card id 999999999999999999999999999999999",
	} {
		f.Add(script)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 4096 {
			t.Skip()
		}
		stack := &HyperCardStack{Name: "Example"}
		card := testCard(stack, 1, text)
		p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{card, testCard(stack, 2), testCard(stack, 3)}}
		for _, source := range []any{stack, card} {
			var id int64
			_, _ = p.identifyScriptLinks(source, card.Scripts[0], &id)
		}
	})
}
