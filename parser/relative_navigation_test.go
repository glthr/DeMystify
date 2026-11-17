package parser

import "testing"

// HyperCard resolves `go next`, `go prev`, `go first` and `go last` against
// the stored order of the cards, which is not the order of their IDs, and
// wraps around at both ends.
func TestCardCommandsResolveRelativePositions(t *testing.T) {
	stack := &HyperCardStack{Name: "Example", CardOrder: []int{30, 10, 20}}
	first, middle, last := testCard(stack, 30), testCard(stack, 10), testCard(stack, 20)
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{middle, last, first}}
	for _, test := range []struct {
		source  *HyperCardCard
		command string
		target  *HyperCardCard
	}{
		{middle, "go next", last},
		{middle, "go to next card", last},
		{last, "go next", first},
		{middle, "go prev", first},
		{first, "go to previous card", last},
		{middle, "go first", first},
		{middle, "go to the last card", last},
		{middle, "if done then go first -- back to the start", first},
	} {
		link, err := p.parseCardCommand(test.source, test.command)
		if err != nil || link == nil || link.Target != test.target || link.Source != test.source || link.IsCrossAges || link.IsNotImplemented {
			t.Errorf("%q from card %d: got %+v, %v; want card %d", test.command, test.source.ID, link, err, test.target.ID)
		}
	}
	// A position that depends on a value, on another stack or on marks is not guessed.
	for _, command := range []string{"go next marked card", "go to card x", `go first of stack "Other"`, `put "go next" into hint`, "go nextCard"} {
		if link, err := p.parseCardCommand(middle, command); err != nil || link != nil {
			t.Errorf("%q: got %+v, %v; want no link", command, link, err)
		}
	}
	// A stack script has no current card: only the two ends are known.
	if link, _ := p.parseCardCommand(stack, "go next"); link != nil {
		t.Errorf("a stack's next card was guessed: %+v", link)
	}
	if link, _ := p.parseCardCommand(stack, "go last"); link == nil || link.Target != last {
		t.Errorf("a stack's last card was not resolved: %+v", link)
	}
	// Without a stored order, nothing is resolved.
	unordered := &HyperCardStack{Name: "Unordered"}
	alone := testCard(unordered, 1)
	q := &Parser{stacks: []*HyperCardStack{unordered}, cards: []*HyperCardCard{alone}}
	if link, _ := q.parseCardCommand(alone, "go next"); link != nil {
		t.Errorf("a position was resolved without a card order: %+v", link)
	}
}

// An openCard handler that only moves on makes its card a pass-through view,
// not an isolated one: the link is recorded like any other go command.
func TestIdentifyLinksRecordsRelativeNavigation(t *testing.T) {
	stack := &HyperCardStack{Name: "Example", CardOrder: []int{5, 7, 6}}
	start := testCard(stack, 7, "on openCard\nhide menubar\ngo next\nend openCard", "on mouseUp\n--go prev\ngo first\nend mouseUp")
	next, first := testCard(stack, 6), testCard(stack, 5)
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{first, next, start}}
	if err := p.identifyLinks(); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		target   *HyperCardCard
		disabled bool
	}{{next, false}, {first, true}, {first, false}}
	if len(p.links) != len(want) {
		t.Fatalf("got %d links, want %d", len(p.links), len(want))
	}
	for i, link := range p.links {
		if link.Source != start || link.Target != want[i].target || link.IsDisabled != want[i].disabled {
			t.Errorf("link %d: got %+v", i, link)
		}
	}
}
