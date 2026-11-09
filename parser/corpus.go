package parser

import (
	"fmt"
	"strings"

	"github.com/glthr/DeMystify/hypercard"
)

// NewParserFromCorpus analyzes already loaded data without opening source files.
// Stacks and cards retain the adapter's enumeration; CardOrder is independent.
func NewParserFromCorpus(corpus hypercard.Corpus) (*Parser, error) {
	if len(corpus.Stacks) == 0 {
		return nil, fmt.Errorf("corpus contains no stacks")
	}
	p := &Parser{backgrounds: make(map[*HyperCardStack]backgroundScope)}
	for _, input := range corpus.Stacks {
		if strings.TrimSpace(input.Name) == "" {
			return nil, fmt.Errorf("stack requires a name")
		}
		if _, err := p.GetStackByName(input.Name); err == nil {
			return nil, fmt.Errorf("duplicate stack name %q", input.Name)
		}
		stack := analysisStack(input)
		p.stacks = append(p.stacks, stack)
		seen := make(map[int]bool, len(input.Cards))
		for _, card := range input.Cards {
			if card.ID <= 0 {
				return nil, fmt.Errorf("invalid card ID %d in stack %q: expected a positive integer", card.ID, input.Name)
			}
			if seen[card.ID] {
				return nil, fmt.Errorf("duplicate card ID %d in stack %q", card.ID, input.Name)
			}
			seen[card.ID] = true
			p.cards = append(p.cards, analysisCard(stack, card))
		}
		order := make(map[int]bool, len(input.CardOrder))
		for _, id := range input.CardOrder {
			if id <= 0 || order[id] {
				return nil, fmt.Errorf("invalid or repeated card ID %d in stack %q order", id, input.Name)
			}
			if !seen[id] {
				return nil, fmt.Errorf("missing card ID %d declared in stack %q order", id, input.Name)
			}
			order[id] = true
		}
		if len(order) != 0 && len(order) != len(seen) {
			return nil, fmt.Errorf("stack %q order does not cover every card", input.Name)
		}
		p.backgrounds[stack] = compileBackgroundScope(input)
	}
	for _, stack := range p.stacks {
		p.prepareStackHandlers(stack)
	}
	if err := p.identifyLinks(); err != nil {
		return nil, err
	}
	return p, nil
}

func compileScript(script hypercard.Script, trim bool) HyperTalk {
	result := HyperTalk{RawText: script.Text, Source: script.Source}
	for _, line := range splitScript(script.Text) {
		if trim {
			line = strings.TrimSpace(line)
		}
		result.lines = append(result.lines, ScriptLine{Line: line, IsDisabled: strings.HasPrefix(line, "--")})
	}
	return result
}

func analysisStack(input hypercard.Stack) *HyperCardStack {
	ids := make(map[int]bool, len(input.CardOrder))
	for _, id := range input.CardOrder {
		ids[id] = true
	}
	return &HyperCardStack{Name: input.Name, Source: input.Source,
		Script:    []HyperTalk{compileScript(input.Script, true)},
		CardOrder: append([]int(nil), input.CardOrder...), cardIDs: ids}
}

func analysisCard(stack *HyperCardStack, input hypercard.Card) *HyperCardCard {
	card := &HyperCardCard{ID: input.ID, Name: fmt.Sprintf("%s:%d", stack.Name, input.ID),
		Stack: stack, Filepath: input.Source.Path, Source: input.Source, BackgroundID: input.BackgroundID}
	if input.Name != "" {
		name := input.Name
		card.OriginalName = &name
	}
	for _, part := range input.Parts {
		card.Scripts = append(card.Scripts, compileScript(part.Script, true))
	}
	card.Scripts = append(card.Scripts, compileScript(input.Script, true))
	for _, content := range input.Contents {
		if content.Layer == "background" && content.ID == 1 {
			text := content.Text
			card.Background = &text
			break
		}
	}
	return card
}

func compileBackgroundScope(input hypercard.Stack) backgroundScope {
	scope := backgroundScope{verified: input.BackgroundsComplete}
	ids := make(map[int]bool, len(input.Backgrounds))
	for _, background := range input.Backgrounds {
		if background.ID <= 0 || ids[background.ID] {
			scope.verified = false
		}
		ids[background.ID] = true
		// Background root scripts precede part scripts, matching XML analysis.
		scope.scripts = append(scope.scripts, compileScript(background.Script, false))
		for _, part := range background.Parts {
			scope.scripts = append(scope.scripts, compileScript(part.Script, false))
		}
	}
	for _, card := range input.Cards {
		// A verified empty inventory is meaningful; an unavailable one is not.
		if len(ids) == 0 && card.BackgroundID == 0 {
			continue
		}
		if !ids[card.BackgroundID] {
			scope.verified = false
		}
	}
	return scope
}
