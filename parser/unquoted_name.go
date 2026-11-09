package parser

import "strings"

// Unbound single identifiers can be literals in HyperTalk. Keep object syntax
// and constants out of this deliberately limited named-card resolver.
func (p *Parser) handleGoToUnquotedCardName(source any, command string) *HyperCardLink {
	match := findCommandMatch(unquotedCardNamePattern, command)
	if match == nil {
		return nil
	}
	name := match[1]
	switch strings.ToLower(name) {
	case "id", "this", "next", "prev", "previous", "first", "second", "third", "fourth", "fifth", "sixth", "seventh",
		"eighth", "ninth", "tenth", "last", "mid", "middle", "any", "recent", "back", "forth", "marked", "card",
		"cd", "background", "bkgnd", "stack", "me", "it", "target", "the", "empty", "true", "false", "pi",
		"space", "tab", "quote", "return", "linefeed", "formfeed", "none", "zero", "one", "two", "three", "four",
		"five", "six", "seven", "eight", "nine", "ten":
		return nil
	}
	var stack *HyperCardStack
	switch s := source.(type) {
	case *HyperCardCard:
		if s != nil {
			stack = s.Stack
		}
	case *HyperCardStack:
		stack = s
	}
	if stack == nil {
		return nil
	}
	target, err := p.getScriptCardByName(stack.Name, name)
	if err != nil || !p.unquotedNameIsStatic(source, name) {
		return nil
	}
	return &HyperCardLink{Source: source, Target: target}
}
