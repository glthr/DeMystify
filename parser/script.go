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
	goToNamedCardOfStackPattern = regexp.MustCompile(`(?i)go( to)? card "([^"]+)" of stack "([^"]+)"`)
	goToCardOfStackPattern      = regexp.MustCompile(`(?i)go( to)? card id (\d+) of stack "([^"]+)"`)
	goToCardPattern             = regexp.MustCompile(`(?i)go( to)? card id (\d+)`)
	popCardPattern              = regexp.MustCompile(`(?i)pop card`)
	bluePagePattern             = regexp.MustCompile(`(?i)put "(\d+),A,0" into ALL_Page`)
	redPagePattern              = regexp.MustCompile(`(?i)put "(\d+),S,0" into ALL_Page`)
	whitePagePattern            = regexp.MustCompile(`(?i)put "Atrus" into ALL_Page`)
	pushCardOfStackPattern      = regexp.MustCompile(`(?i)push card id (\d+) of stack "([^"]+)"`)
	pushCardPattern             = regexp.MustCompile(`(?i)push card`)
)

func (p *Parser) parseCardCommand(source any, command string) (*HyperCardLink, error) {
	disabled := strings.HasPrefix(strings.TrimSpace(command), "--")
	if link, err := p.handleGoToNamedCardOfStack(source, command); link != nil || err != nil {
		return link, err
	}
	if link, err := p.handleGoToCardOfStack(source, command); link != nil || err != nil {
		return link, err
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

// findCommandMatch ignores command-like text inside HyperTalk string literals.
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
		if quoted {
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
