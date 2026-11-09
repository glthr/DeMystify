package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

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

func TestUnquotedNamesRejectPossibleBindings(t *testing.T) {
	for _, binding := range []string{"on visit black", "function visit black", "global black", "local black", "put \"other\" into black", "add 1 to black", "repeat with black = 1 to 2"} {
		t.Run(binding, func(t *testing.T) {
			p, source, _ := unquotedNameFixture()
			source.Scripts = testCard(source.Stack, 1, binding).Scripts
			if link, err := p.parseCardCommand(source, "go card Black"); err != nil || link != nil {
				t.Fatalf("bound target produced %+v, %v", link, err)
			}
		})
	}
	p, source, target := unquotedNameFixture()
	source.Scripts = testCard(source.Stack, 1, "--global black\nput \"black\" into other").Scripts
	if link, err := p.parseCardCommand(source, "go card Black"); err != nil || link == nil || link.Target != target {
		t.Fatalf("quoted/commented word blocked literal: %+v %v", link, err)
	}
}

func TestUnquotedNamesCheckOtherScriptsAndBackgrounds(t *testing.T) {
	p, source, _ := unquotedNameFixture()
	source.Stack.Script = testCard(source.Stack, 9, "global black").Scripts
	if link, _ := p.parseCardCommand(source, "go card Black"); link != nil {
		t.Fatal("stack binding ignored")
	}
	p, source, _ = unquotedNameFixture()
	p.cards = append(p.cards, testCard(source.Stack, 3, "put 1 into black"))
	if link, _ := p.parseCardCommand(source, "go card Black"); link != nil {
		t.Fatal("other modeled script binding ignored")
	}
	for _, tc := range []struct {
		text string
		want bool
	}{
		{`<background><id>7</id><script>global black</script></background>`, false},
		{`<background><id>7</id><part><script>global black</script></part></background>`, false},
		{`<background><id>7</id><script>put "black" into other</script></background>`, true},
		{`<background><id>8</id></background>`, false},
		{`<broken>`, false},
	} {
		p, source, target := unquotedNameFixture()
		dir := t.TempDir()
		write := func(name, text string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
		}
		write("stack_-1.xml", `<stack><name>Example</name><background id="7" file="background_7.xml"/><card id="1"/><card id="2" name="Black"/></stack>`)
		write("background_7.xml", tc.text)
		for _, card := range p.cards {
			card.Filepath = filepath.Join(dir, fmt.Sprintf("card_%d.xml", card.ID))
			write(filepath.Base(card.Filepath), fmt.Sprintf(`<card><id>%d</id><owner>7</owner></card>`, card.ID))
		}
		var err error
		p, err = NewParser(dir)
		if err != nil {
			t.Fatal(err)
		}
		source, target = p.cards[0], p.cards[1]
		link, err := p.parseCardCommand(source, "go card Black")
		if err != nil || (link != nil) != tc.want || tc.want && link.Target != target {
			t.Fatalf("background scope: link=%+v err=%v want=%v", link, err, tc.want)
		}
	}
}

func TestUnquotedNamesAreNotLimitedToBlack(t *testing.T) {
	p, source, target := unquotedNameFixture()
	name := "Landing"
	target.OriginalName = &name
	if link, err := p.parseCardCommand(source, "go card landing"); err != nil || link == nil || link.Target != target {
		t.Fatalf("known unbound name: %+v %v", link, err)
	}
	for _, name := range []string{"last", "empty", "id", "one"} {
		target.OriginalName = &name
		if link, _ := p.parseCardCommand(source, "go card "+name); link != nil {
			t.Fatalf("reserved %q treated as a literal card", name)
		}
	}
}
