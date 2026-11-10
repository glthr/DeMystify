package parser

import (
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
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
