package parser

import (
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/hypercard"
)

func TestCommandsDoNotInventLiteralTargetsFromExpressions(t *testing.T) {
	stack, other := &HyperCardStack{Name: "Example"}, &HyperCardStack{Name: "Other"}
	name := "Gate"
	for _, command := range []string{
		"go card id 2 + offset",
		"go card id 2.5",
		"go card id 2suffix",
		"go card id 2 of stack destination",
		`go card id 2 of stack "Other" & suffix`,
		`go card "Gate" & suffix`,
		`go card "Gate" of stack "Other" & suffix`,
		"go this card of stack destination",
		"push card id 2 of stack destination",
		"pop card into savedAddress",
	} {
		source, target, remote := testCard(stack, 1), testCard(stack, 2), testCard(other, 2)
		target.OriginalName, remote.OriginalName = &name, &name
		p := &Parser{stacks: []*HyperCardStack{stack, other}, cards: []*HyperCardCard{source, target, remote}}
		link, err := p.parseCardCommand(source, command)
		if err != nil || link != nil || source.IsPushCard || source.IsPopCard {
			t.Errorf("unsupported expression %q invented navigation: link=%+v, push=%v, pop=%v, err=%v", command, link, source.IsPushCard, source.IsPopCard, err)
		}
	}
}

func TestCommandsRejectZeroCardIDs(t *testing.T) {
	stack := &HyperCardStack{Name: "Example"}
	source := testCard(stack, 1)
	p := &Parser{stacks: []*HyperCardStack{stack}, cards: []*HyperCardCard{source}}
	for _, command := range []string{"go card id 0", `go card id 0 of stack "Example"`, `push card id 0 of stack "Example"`} {
		if _, err := p.parseCardCommand(source, command); err == nil {
			t.Errorf("accepted nonpositive card ID in %q", command)
		}
	}
}

// Commented text cannot stop the analysis, whatever it looks like. An active
// command of the same shape is still an error.
func TestCommentsThatResembleCommandsDoNotAbortTheAnalysis(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want []string
	}{
		"commented push, active go":  {"--push card id 4 of stack \"Example\"\ngo card id 2", []string{"Example:1 -> Example:2"}},
		"note quoting a push":        {"-- was: push card id 4 of stack \"Example\"\ngo card id 2", []string{"Example:1 -> Example:2"}},
		"note quoting an invalid go": {"-- used to be: go to card id 0\ngo card id 3", []string{"Example:1 -> Example:3"}},
		"commented invalid go":       {"--go to card id 0", nil},
		"commented push and its go":  {"--push card id 4 of stack \"Example\"\n--go card id 2", []string{"Example:1 -> Example:2 disabled", "Example:2 -> Example:4 disabled"}},
	} {
		t.Run(name, func(t *testing.T) {
			p, err := NewParserFromCorpus(sequenceCorpus("on mouseUp\n" + tc.body + "\nend mouseUp"))
			if err != nil {
				t.Fatalf("a comment stopped the analysis: %v", err)
			}
			metadata, err := p.Process()
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, edge := range metadata.Edges {
				record := edge.Source.Name + " -> " + edge.Target.Name
				if edge.IsOfType(common.Disabled) {
					record += " disabled"
				}
				got = append(got, record)
			}
			if strings.Join(got, "; ") != strings.Join(tc.want, "; ") {
				t.Fatalf("records %v, want %v", got, tc.want)
			}
		})
	}
	for name, body := range map[string]string{"active push without a go": "push card id 4 of stack \"Example\"", "active invalid go": "go to card id 0"} {
		if _, err := NewParserFromCorpus(sequenceCorpus("on mouseUp\n" + body + "\nend mouseUp")); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// A handler switched off by commenting out its declaration leaves its body in
// no handler, so HyperCard never runs it. Its references are disabled, like
// those of commented lines, and it sets no card state.
func TestBodyUnderACommentedOutDeclarationIsReadAsCommented(t *testing.T) {
	for name, tc := range map[string]struct {
		script string
		want   []string
	}{
		"removed handler":          {"--on mouseUp\ngo card id 2\ngo this card\nend mouseUp", []string{"Example:1 -> Example:2 disabled", "Example:1 -> Example:1 disabled"}},
		"removed function":         {"--function pick\ngo card id 2\nend pick", []string{"Example:1 -> Example:2 disabled"}},
		"annotated declaration":    {"-- on skipIt  -- removed\nvd\ngo to card id 2\nend skipIt", []string{"Example:1 -> Example:2 disabled"}},
		"beside its live twin":     {"on mouseUp\ngo card id 2\nend mouseUp\n--on skipIt\ngo card id 2\nend skipIt", []string{"Example:1 -> Example:2", "Example:1 -> Example:2 disabled"}},
		"live handler afterwards":  {"--on mouseUp\ngo card id 2\nend mouseUp\non mouseDown\ngo card id 3\nend mouseDown", []string{"Example:1 -> Example:2 disabled", "Example:1 -> Example:3"}},
		"already commented line":   {"--on mouseUp\n--go card id 2\ngo card id 3\nend mouseUp", []string{"Example:1 -> Example:2 disabled", "Example:1 -> Example:3 disabled"}},
		"invalid line in the body": {"--on mouseUp\ngo to card id 0\ngo card id 2\nend mouseUp", []string{"Example:1 -> Example:2 disabled"}},
		"two removed handlers":     {"--on a\npush card id 4 of stack \"Example\"\nend a\n--on b\ngo card id 2\nend b", []string{"Example:1 -> Example:2 disabled"}},
		// Not removed: the comment sits inside a live handler, has no end line
		// of its own, or only begins with the word "on".
		"comment in a live handler": {"on mouseUp\n--on mouseDown\ngo card id 2\nend mouseUp", []string{"Example:1 -> Example:2"}},
		"no end line":               {"--on mouseUp\ngo card id 2", []string{"Example:1 -> Example:2"}},
		"a sentence":                {"-- on the way out\ngo card id 2\nend mouseUp", []string{"Example:1 -> Example:2"}},
		"end of another handler":    {"--on mouseUp\ngo card id 2\non mouseDown\ngo card id 3\nend mouseUp\nend mouseDown", []string{"Example:1 -> Example:2", "Example:1 -> Example:3"}},
	} {
		t.Run(name, func(t *testing.T) {
			metadata := sequenceMetadata(t, sequenceCorpus(tc.script))
			var got []string
			for _, edge := range metadata.Edges {
				record := edge.Source.Name + " -> " + edge.Target.Name
				if edge.IsOfType(common.Disabled) {
					record += " disabled"
				}
				got = append(got, record)
			}
			if strings.Join(got, "; ") != strings.Join(tc.want, "; ") {
				t.Fatalf("records %v, want %v", got, tc.want)
			}
		})
	}
	// A removed body cannot make its card a saved-return target.
	corpus := sequenceCorpus("on mouseUp\npush card\ngo card id 2\nend mouseUp")
	corpus.Stacks[0].Cards[1].Parts = []hypercard.Part{{ID: 1, Script: hypercard.Script{Text: "--on mouseUp\nvd\npop card\nend mouseUp"}}}
	metadata := sequenceMetadata(t, corpus)
	if len(metadata.Edges) != 1 || metadata.Edges[0].IsOfType(common.Backtracking) {
		t.Fatalf("a removed pop card made a saved return: %+v", metadata.Edges)
	}
}
