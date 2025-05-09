package parser

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/glthr/DeMystify/common"
)

type HyperTalk struct {
	lines []ScriptLine
}

type ScriptLine struct {
	Line       string
	IsDisabled bool
}

func (p *Parser) parseCardCommand(source any, command string) (*HyperCardLink, error) {
	if link, err := p.handleGoToNamedCardOfStack(source, command); link != nil || err != nil {
		return link, err
	}
	if link, err := p.handleGoToCardOfStack(source, command); link != nil || err != nil {
		return link, err
	}
	if link, err := p.handleGoToCard(source, command); link != nil || err != nil {
		return link, err
	}
	if handled := p.handlePopCard(source, command); handled {
		return nil, nil
	}
	if handled := p.handlePagePatterns(source, command); handled {
		return nil, nil
	}
	if link, err := p.handlePushCardOfStack(source, command); link != nil || err != nil {
		return link, err
	}
	if handled := p.handlePushCard(source, command); handled {
		return nil, nil
	}
	return nil, nil
}

func (p *Parser) handleGoToNamedCardOfStack(source any, command string) (*HyperCardLink, error) {
	// `go [to] card "{card name}" of stack "{stack name}"`
	pattern := regexp.MustCompile(`(?i)go( to)? card "([^"]+)" of stack "([^"]+)"`)
	matches := pattern.FindStringSubmatch(command)
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
	pattern := regexp.MustCompile(`(?i)go( to)? card id (\d+) of stack "([^"]+)"`)
	matches := pattern.FindStringSubmatch(command)
	if matches == nil {
		return nil, nil
	}

	cardID, _ := strconv.Atoi(matches[2])
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
	pattern := regexp.MustCompile(`(?i)go( to)? card id (\d+)`)
	matches := pattern.FindStringSubmatch(command)
	if matches == nil {
		return nil, nil
	}

	cardID, _ := strconv.Atoi(matches[2])
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
	// `pop card"`
	pattern := regexp.MustCompile(`(?i)pop card`)
	if matches := pattern.FindStringSubmatch(command); matches != nil {
		source.(*HyperCardCard).IsPopCard = true
		return true
	}
	return false
}

func (p *Parser) handlePagePatterns(source any, command string) bool {
	bluePagePattern := regexp.MustCompile(`(?i)put "(\d+),A,0" into ALL_Page`)
	redPagePattern := regexp.MustCompile(`(?i)put "(\d+),S,0" into ALL_Page`)
	whitePagePattern := regexp.MustCompile(`(?i)put "Atrus" into ALL_Page`)

	if matches := bluePagePattern.FindStringSubmatch(command); matches != nil {
		source.(*HyperCardCard).HasBluePage = true
		return true
	}

	if matches := redPagePattern.FindStringSubmatch(command); matches != nil {
		source.(*HyperCardCard).HasRedPage = true
		return true
	}

	if matches := whitePagePattern.FindStringSubmatch(command); matches != nil {
		source.(*HyperCardCard).HasWhitePage = true
		return true
	}

	return false
}

func (p *Parser) handlePushCardOfStack(source any, command string) (*HyperCardLink, error) {
	// `push card id {card id} of stack "{stack name}"`
	pattern := regexp.MustCompile(`(?i)push card id (\d+) of stack "([^"]+)"`)
	matches := pattern.FindStringSubmatch(command)
	if matches == nil {
		return nil, nil
	}

	cardID, _ := strconv.Atoi(matches[1])
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
	pattern := regexp.MustCompile(`(?i)push card`)
	if matches := pattern.FindStringSubmatch(command); matches != nil {
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

	return &HyperCardCard{
		Name: fmt.Sprintf("%s:%s",
			stackName,
			cardName,
		),
		Stack: stack,
	}, nil
}
