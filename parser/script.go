package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/glthr/DeMystify/common"
)

type HyperTalk struct {
	lines []ScriptLine
}

type ScriptLine struct {
	Line       string
	IsDisabled bool
}

var (
	goToNamedCardOfStackPattern = regexp.MustCompile(`(?i)\bgo(\s+to)?\s+card\s+"([^"]+)"\s+of\s+stack\s+"([^"]+)"`)
	goToCardOfStackPattern      = regexp.MustCompile(`(?i)\bgo(\s+to)?\s+card\s+id\s+(\d+)\s+of\s+stack\s+"([^"]+)"`)
	goToNamedCardPattern        = regexp.MustCompile(`(?i)\bgo(\s+to)?\s+card\s+"([^"]+)"`)
	goToCurrentCardPattern      = regexp.MustCompile(`(?i)\bgo(\s+to)?\s+this\s+card\b`)
	goToCardPattern             = regexp.MustCompile(`(?i)\bgo(\s+to)?\s+card\s+id\s+(\d+)`)
	popCardPattern              = regexp.MustCompile(`(?i)\bpop\s+card\b`)
	bluePagePattern             = regexp.MustCompile(`(?i)\bput\s+"(\d+),A,0"\s+into\s+ALL_Page\b`)
	redPagePattern              = regexp.MustCompile(`(?i)\bput\s+"(\d+),S,0"\s+into\s+ALL_Page\b`)
	whitePagePattern            = regexp.MustCompile(`(?i)\bput\s+"Atrus"\s+into\s+ALL_Page\b`)
	pushCardOfStackPattern      = regexp.MustCompile(`(?i)\bpush\s+card\s+id\s+(\d+)\s+of\s+stack\s+"([^"]+)"`)
	pushCardPattern             = regexp.MustCompile(`(?i)\bpush\s+card\b`)
)

func (p *Parser) parseCardCommand(source any, command string) (*HyperCardLink, error) {
	command = strings.TrimSpace(command)
	disabled := strings.HasPrefix(command, "--")
	if disabled {
		command = strings.TrimSpace(strings.TrimPrefix(command, "--"))
	}
	// Keep whole-line commented navigation as disabled links, but do not
	// mistake text in trailing comments for executable commands.
	quoted := false
	for i := 0; i < len(command); i++ {
		if command[i] == '"' {
			quoted = !quoted
		} else if !quoted && strings.HasPrefix(command[i:], "--") {
			command = command[:i]
			break
		}
	}
	if link, err := p.handleGoToNamedCardOfStack(source, command); link != nil || err != nil {
		return link, err
	}
	if link, err := p.handleGoToCardOfStack(source, command); link != nil || err != nil {
		return link, err
	}
	if link, err := p.handleGoToNamedCard(source, command); link != nil || err != nil {
		return link, err
	}
	if link := p.handleGoToCurrentCard(source, command); link != nil {
		return link, nil
	}
	if link, err := p.handleGoToCard(source, command); link != nil || err != nil {
		return link, err
	}
	if !disabled {
		if handled := p.handlePopCard(source, command); handled {
			return nil, nil
		}
		if handled := p.handlePagePatterns(source, command); handled {
			return nil, nil
		}
	}
	if link, err := p.handlePushCardOfStack(source, command); link != nil || err != nil {
		return link, err
	}
	if !disabled {
		if handled := p.handlePushCard(source, command); handled {
			return nil, nil
		}
	}
	return nil, nil
}

// findCommandMatch accepts complete literal commands outside string literals.
func findCommandMatch(pattern *regexp.Regexp, command string) []string {
	quoted := false
	previous := 0
	for _, indexes := range pattern.FindAllStringSubmatchIndex(command, -1) {
		for i := previous; i < indexes[0]; i++ {
			if command[i] == '"' {
				quoted = !quoted
			}
		}
		previous = indexes[0]
		// An unsupported expression or scope must not be truncated into a
		// literal target by a shorter fallback pattern.
		if quoted || strings.TrimSpace(command[indexes[1]:]) != "" {
			continue
		}
		matches := make([]string, len(indexes)/2)
		for i := range matches {
			if indexes[2*i] >= 0 {
				matches[i] = command[indexes[2*i]:indexes[2*i+1]]
			}
		}
		return matches
	}
	return nil
}

func (p *Parser) handleGoToNamedCardOfStack(source any, command string) (*HyperCardLink, error) {
	// `go [to] card "{card name}" of stack "{stack name}"`
	matches := findCommandMatch(goToNamedCardOfStackPattern, command)
	if matches == nil {
		return nil, nil
	}

	cardName := matches[2]
	stackName := matches[3]
	target, err := p.GetCardByStackAndName(stackName, cardName)
	if err != nil {
		target, err = p.createVirtualCard(stackName, cardName)
		if err != nil {
			return nil, err
		}
		return &HyperCardLink{
			Source:           source,
			Target:           target,
			IsCrossAges:      true,
			IsNotImplemented: true,
		}, nil
	}

	return &HyperCardLink{
		Source:           source,
		Target:           target,
		IsCrossAges:      true,
		IsNotImplemented: false,
	}, nil
}

func (p *Parser) handleGoToCardOfStack(source any, command string) (*HyperCardLink, error) {
	// `go [to] card id {card id} of stack "{stack name}"`
	matches := findCommandMatch(goToCardOfStackPattern, command)
	if matches == nil {
		return nil, nil
	}

	cardID, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil, fmt.Errorf("invalid card ID %q: %w", matches[2], err)
	}
	stackName := matches[3]

	target, err := p.GetCardByStackAndID(stackName, cardID)
	if err != nil {
		target, err = p.createVirtualCard(stackName, cardID)
		if err != nil {
			return nil, err
		}
		return &HyperCardLink{
			Source:           source,
			Target:           target,
			IsCrossAges:      true,
			IsNotImplemented: true,
		}, nil
	}

	return &HyperCardLink{
		Source:           source,
		Target:           target,
		IsCrossAges:      true,
		IsNotImplemented: false,
	}, nil
}

func (p *Parser) handleGoToNamedCard(source any, command string) (*HyperCardLink, error) {
	matches := findCommandMatch(goToNamedCardPattern, command)
	if matches == nil {
		return nil, nil
	}
	var stackName string
	switch src := source.(type) {
	case *HyperCardCard:
		stackName = src.Stack.Name
	case *HyperCardStack:
		stackName = src.Name
	}
	target, err := p.GetCardByStackAndName(stackName, matches[2])
	notImplemented := err != nil
	if notImplemented {
		target, err = p.createVirtualCard(stackName, matches[2])
		if err != nil {
			return nil, err
		}
	}
	return &HyperCardLink{Source: source, Target: target, IsNotImplemented: notImplemented}, nil
}

func (p *Parser) handleGoToCurrentCard(source any, command string) *HyperCardLink {
	card, ok := source.(*HyperCardCard)
	if !ok || findCommandMatch(goToCurrentCardPattern, command) == nil {
		// A stack handler's current card is determined at runtime.
		return nil
	}
	return &HyperCardLink{Source: card, Target: card}
}

func (p *Parser) handleGoToCard(source any, command string) (*HyperCardLink, error) {
	// `go [to] card id {id}`
	matches := findCommandMatch(goToCardPattern, command)
	if matches == nil {
		return nil, nil
	}

	cardID, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil, fmt.Errorf("invalid card ID %q: %w", matches[2], err)
	}
	var stackName string
	switch src := source.(type) {
	case *HyperCardCard:
		stackName = src.Stack.Name
	case *HyperCardStack:
		stackName = src.Name
	}

	target, err := p.GetCardByStackAndID(stackName, cardID)
	if err != nil {
		target, err = p.createVirtualCard(stackName, cardID)
		if err != nil {
			return nil, err
		}
		return &HyperCardLink{
			Source:           source,
			Target:           target,
			IsCrossAges:      false,
			IsNotImplemented: true,
		}, nil
	}

	return &HyperCardLink{
		Source:           source,
		Target:           target,
		IsCrossAges:      false,
		IsNotImplemented: false,
	}, nil
}

func (p *Parser) handlePopCard(source any, command string) bool {
	card, ok := source.(*HyperCardCard)
	if !ok {
		return false
	}
	// `pop card"`
	if matches := findCommandMatch(popCardPattern, command); matches != nil {
		card.IsPopCard = true
		return true
	}
	return false
}

func (p *Parser) handlePagePatterns(source any, command string) bool {
	card, ok := source.(*HyperCardCard)
	if !ok {
		return false
	}
	if matches := findCommandMatch(bluePagePattern, command); matches != nil {
		card.HasBluePage = true
		return true
	}

	if matches := findCommandMatch(redPagePattern, command); matches != nil {
		card.HasRedPage = true
		return true
	}

	if matches := findCommandMatch(whitePagePattern, command); matches != nil {
		card.HasWhitePage = true
		return true
	}

	return false
}

func (p *Parser) handlePushCardOfStack(source any, command string) (*HyperCardLink, error) {
	// `push card id {card id} of stack "{stack name}"`
	matches := findCommandMatch(pushCardOfStackPattern, command)
	if matches == nil {
		return nil, nil
	}

	cardID, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, fmt.Errorf("invalid card ID %q: %w", matches[1], err)
	}
	stackName := matches[2]

	target, err := p.GetCardByStackAndID(stackName, cardID)
	if err != nil {
		target, err = p.createVirtualCard(stackName, cardID)
		if err != nil {
			return nil, err
		}
		return &HyperCardLink{
			Source:           source,
			Target:           target,
			IsCrossAges:      true,
			IsNotImplemented: true,
			IsBacktracking:   false,
			TransitivityRank: common.RestrictiveTransitivity,
		}, nil
	}

	return &HyperCardLink{
		Source:           source,
		Target:           target,
		IsCrossAges:      true,
		IsNotImplemented: false,
		IsBacktracking:   false,
		TransitivityRank: common.RestrictiveTransitivity,
	}, nil
}

func (p *Parser) handlePushCard(source any, command string) bool {
	// `push card`
	if matches := findCommandMatch(pushCardPattern, command); matches != nil {
		switch s := source.(type) {
		case *HyperCardStack:
			s.IsPushStack = true
		case *HyperCardCard:
			s.IsPushCard = true
		}
		return true
	}
	return false
}

func (p *Parser) createVirtualCard(stackName string, cardID any) (*HyperCardCard, error) {
	cardName := fmt.Sprintf("%v", cardID)
	stack, err := p.GetStackByName(stackName)
	if err != nil {
		return nil, err
	}

	card := &HyperCardCard{
		Name: fmt.Sprintf("%s:%s",
			stack.Name,
			cardName,
		),
		Stack: stack,
	}
	if id, ok := cardID.(int); ok {
		card.ID = id
	}
	return card, nil
}
