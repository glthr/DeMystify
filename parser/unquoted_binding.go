package parser

import (
	"path/filepath"
	"regexp"
)

var unquotedCardNamePattern = regexp.MustCompile(`(?i)\bgo(?:\s+to)?\s+card\s+([a-z][a-z0-9_]*)\b`)
var unquotedNameStrings = regexp.MustCompile(`"[^"]*"`)

// Refuse a name used anywhere else as an unquoted identifier. This intentionally
// overapproximates bindings across the imported corpus rather than guessing
// variable values, and includes parameter/declaration/assignment/repeat forms.
func (p *Parser) unquotedNameIsStatic(source any, name string) bool {
	word := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(name) + `\b`)
	used := func(raw string, disabled bool) bool {
		command, commented := normalizeCommand(raw)
		if disabled || commented {
			return false
		}
		command = unquotedNameStrings.ReplaceAllString(command, " ")
		command = unquotedCardNamePattern.ReplaceAllString(command, " ")
		return word.MatchString(command)
	}
	var scripts []HyperTalk
	directories := map[string]*HyperCardStack{}
	addStack := func(s *HyperCardStack) {
		if s != nil {
			scripts = append(scripts, s.Script...)
			if s.Directory != "" {
				directories[s.Directory] = s
			}
		}
	}
	addCard := func(c *HyperCardCard) {
		if c != nil {
			scripts = append(scripts, c.Scripts...)
			if c.Filepath != "" {
				directories[filepath.Dir(c.Filepath)] = c.Stack
			}
		}
	}
	for _, s := range p.stacks {
		addStack(s)
	}
	for _, c := range p.cards {
		addCard(c)
	}
	switch s := source.(type) {
	case *HyperCardStack:
		addStack(s)
	case *HyperCardCard:
		addCard(s)
		if s != nil {
			addStack(s.Stack)
		}
	}
	for _, script := range scripts {
		for _, line := range script.lines {
			if used(line.Line, line.IsDisabled) {
				return false
			}
		}
	}
	for directory, stack := range directories {
		background, verified := p.backgroundScripts(stack, directory)
		if !verified {
			return false
		}
		for _, script := range background {
			for _, line := range script.lines {
				if used(line.Line, line.IsDisabled) {
					return false
				}
			}
		}
	}
	return true
}
