package parser

import (
	"regexp"
	"strings"
)

var handlerLiteralArg = regexp.MustCompile(`^[0-9]+$`)

func (p *Parser) projectHandlerCalls(card *HyperCardCard, script HyperTalk) []*HyperCardLink {
	local := declaredHandlers(card.Scripts)
	if shadowsHandlerPrimitive(local) {
		return nil
	}
	var links []*HyperCardLink
	for _, handler := range scriptHandlers(script) {
		if handler.kind != "on" {
			continue
		}
		var commands []string
		for _, line := range handler.body {
			if command := activeCommand(line); command != "" {
				commands = append(commands, command)
			}
		}
		if len(commands) != 1 {
			continue
		}
		fields := strings.Fields(commands[0])
		name := strings.ToLower(fields[0])
		summary, ok := p.stackHandlers[card.Stack][name]
		if !ok || local[name] != 0 || len(fields)-1 != summary.parameters || summary.parameters == 1 && !handlerLiteralArg.MatchString(fields[1]) {
			continue
		}
		link := &HyperCardLink{Source: card, Target: summary.target}
		if p.pushedLinks == nil {
			p.pushedLinks = make(map[*HyperCardLink]bool)
		}
		p.pushedLinks[link] = true
		card.IsPushCard = true
		links = append(links, link)
	}
	return links
}
