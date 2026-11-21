package parser

import (
	"testing"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/hypercard"
)

func shadowButton(id int, name string, left, top, right, bottom int, script string) hypercard.Part {
	return hypercard.Part{ID: id, Type: "button", Name: name, Enabled: true,
		Rect: hypercard.Rectangle{Left: left, Top: top, Right: right, Bottom: bottom}, Script: hypercard.Script{Text: script}}
}

// The layout of Myst:89742: two "right" buttons lie entirely under a later,
// wider "right" button. Parts are listed in layer order, back to front.
func shadowFixture(change func(stack *hypercard.Stack)) hypercard.Corpus {
	stack := hypercard.Stack{Name: "Example", Width: 544, Height: 332, BackgroundsComplete: true,
		Backgrounds: []hypercard.Background{{ID: 7}},
		Cards: []hypercard.Card{
			{ID: 1, BackgroundID: 7, Parts: []hypercard.Part{
				shadowButton(1, "right", 400, 0, 544, 332, "on mouseUp\ngo to card id 2\nend mouseUp"),
				shadowButton(2, "right", 400, -4, 560, 340, "on mouseUp\ngo to card id 2\nend mouseUp"),
				shadowButton(3, "left", 0, 0, 164, 332, "on mouseUp\ngo to card id 4\nend mouseUp"),
				shadowButton(4, "right", 380, 0, 544, 332, "on mouseUp\ngo to card id 3\nend mouseUp"),
			}},
			{ID: 2, BackgroundID: 7}, {ID: 3, BackgroundID: 7}, {ID: 4, BackgroundID: 7},
		}}
	if change != nil {
		change(&stack)
	}
	return hypercard.Corpus{Stacks: []hypercard.Stack{stack}}
}

func shadowedRecords(t *testing.T, corpus hypercard.Corpus) (shadowed, active []string) {
	t.Helper()
	p, err := NewParserFromCorpus(corpus)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := p.Process()
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range metadata.Edges {
		record := edge.Source.Name + " -> " + edge.Target.Name
		if edge.IsOfType(common.Shadowed) {
			shadowed = append(shadowed, record)
		} else {
			active = append(active, record)
		}
	}
	return shadowed, active
}

func TestReferenceUnderALaterButtonIsShadowed(t *testing.T) {
	shadowed, active := shadowedRecords(t, shadowFixture(nil))
	if len(shadowed) != 1 || shadowed[0] != "Example:1 -> Example:2" {
		t.Errorf("shadowed records: got %v, want the one reference of the two covered buttons", shadowed)
	}
	if len(active) != 2 || active[0] != "Example:1 -> Example:4" || active[1] != "Example:1 -> Example:3" {
		t.Errorf("active records: got %v, want the references of the cover and of the left button", active)
	}
}

// A covered copy of a reference that a reachable part repeats adds nothing.
func TestShadowedCopyOfAReachableReferenceIsNotARecord(t *testing.T) {
	shadowed, active := shadowedRecords(t, shadowFixture(func(stack *hypercard.Stack) {
		stack.Cards[0].Parts[3].Script.Text = "on mouseUp\ngo to card id 2\nend mouseUp"
	}))
	if len(shadowed) != 0 || len(active) != 2 {
		t.Errorf("got shadowed %v and active %v, want two active records", shadowed, active)
	}
}

// A covered part sets no state of its card.
func TestShadowedPartSetsNoCardState(t *testing.T) {
	corpus := shadowFixture(func(stack *hypercard.Stack) {
		stack.Cards[0].Parts[0].Script.Text = "on mouseUp\npop card\nput \"1,A,0\" into ALL_Page\nend mouseUp"
	})
	p, err := NewParserFromCorpus(corpus)
	if err != nil {
		t.Fatal(err)
	}
	if card := p.cards[0]; card.IsPopCard || card.HasBluePage {
		t.Errorf("a part that no click reaches set the state of its card: %+v", card)
	}
}

func TestCoverThatAScriptCanChangeDoesNotShadow(t *testing.T) {
	cover := func(change func(part *hypercard.Part)) func(*hypercard.Stack) {
		return func(stack *hypercard.Stack) { change(&stack.Cards[0].Parts[3]) }
	}
	cardScript := func(text string) func(*hypercard.Stack) {
		return func(stack *hypercard.Stack) { stack.Cards[0].Script.Text = text }
	}
	stackScript := func(text string) func(*hypercard.Stack) {
		return func(stack *hypercard.Stack) { stack.Script.Text = text }
	}
	for name, change := range map[string]func(*hypercard.Stack){
		"cover stored hidden":   cover(func(part *hypercard.Part) { part.Hidden = true }),
		"cover stored disabled": cover(func(part *hypercard.Part) { part.Enabled = false }),
		"cover is a field":      cover(func(part *hypercard.Part) { part.Type = "field" }),
		"cover is narrower":     cover(func(part *hypercard.Part) { part.Rect.Left = 401 }),
		"cover hides itself": cover(func(part *hypercard.Part) {
			part.Script.Text = "on mouseUp\nhide me\nclick at the clickLoc\nend mouseUp"
		}),
		"card hides the name":          cardScript("on openCard\nif done then hide card button \"right\"\nend openCard"),
		"card hides the id":            cardScript("on openCard\nhide card button id 4\nend openCard"),
		"card disables the number":     cardScript("on openCard\nset the enabled of button 4 to false\nend openCard"),
		"card moves an unknown button": cardScript("on openCard\nset the rect of card button which to 0,0,10,10\nend openCard"),
		"card sends to the covered":    cardScript("on openCard\nsend mouseUp to card button 1\nend openCard"),
		"card runs unknown text":       cardScript("on openCard\ndo field \"commands\"\nend openCard"),
		"stack hides the target":       stackScript("on mouseDown\nhide the target\nend mouseDown"),
		"stack helper the card calls": func(stack *hypercard.Stack) {
			stack.Script.Text = "on reveal\nhide the target\nend reveal"
			stack.Cards[0].Parts[3].Script.Text = "on mouseUp\nreveal\nend mouseUp"
		},
		"background hides a button": func(stack *hypercard.Stack) {
			stack.Backgrounds[0].Script.Text = "on openCard\nhide card button x\nend openCard"
		},
	} {
		if shadowed, _ := shadowedRecords(t, shadowFixture(change)); len(shadowed) != 0 {
			t.Errorf("%s: got shadowed records %v, want none", name, shadowed)
		}
	}
	// The reverse order is no cover either: the wider button lies beneath.
	reversed := shadowFixture(func(stack *hypercard.Stack) {
		parts := stack.Cards[0].Parts
		parts[0], parts[1], parts[3] = parts[3], parts[0], parts[1]
	})
	if shadowed, _ := shadowedRecords(t, reversed); len(shadowed) != 0 {
		t.Errorf("a button above its supposed cover was shadowed: %v", shadowed)
	}
}

func TestCommandsThatCannotReachTheCoverKeepTheShadow(t *testing.T) {
	for name, change := range map[string]func(*hypercard.Stack){
		"another button by name": func(stack *hypercard.Stack) {
			stack.Cards[0].Script.Text = "on openCard\nhide card button \"left\"\nset the rect of card button id 3 to 0,0,10,10\nshow button 3\nend openCard"
		},
		"a background button": func(stack *hypercard.Stack) {
			stack.Cards[0].Script.Text = "on openCard\nhide bg button \"right\"\nend openCard"
		},
		"windows and the menu bar": func(stack *hypercard.Stack) {
			stack.Script.Text = "on openCard\nhide menubar\nset cursor to none\nsend idle to window \"movie\"\nset the windowRect of window \"movie\" to the rect of card button \"marker\"\nend openCard"
		},
		"a helper that only other cards call": func(stack *hypercard.Stack) {
			stack.Script.Text = "on hidePillar\nhide the target\nshow the target\nend hidePillar"
		},
		"a do command that plays a sound": func(stack *hypercard.Stack) {
			stack.Script.Text = "on openCard\ndo (\"Play \" & quote & line x of sounds & quote)\nend openCard"
		},
		"me in another part": func(stack *hypercard.Stack) {
			stack.Cards[0].Parts[2].Script.Text = "on mouseUp\nhide me\ngo to card id 4\nend mouseUp"
		},
	} {
		if shadowed, _ := shadowedRecords(t, shadowFixture(change)); len(shadowed) != 1 {
			t.Errorf("%s: got shadowed records %v, want one", name, shadowed)
		}
	}
}
