package parser

// backgroundScope is prepared while loading the corpus, never on demand.
type backgroundScope struct {
	scripts  []HyperTalk
	verified bool
}

func (p *Parser) backgroundScripts(stack *HyperCardStack) ([]HyperTalk, bool) {
	if scope, ok := p.backgrounds[stack]; ok {
		if !scope.verified {
			return nil, false
		}
		return scope.scripts, true
	}
	// Hand-built in-memory parser fixtures historically supplied their entire
	// scope without files. A path-bearing object without a loaded scope remains
	// unknown; it must never acquire a proof by reopening a mutable XML file.
	if stack == nil || stack.Directory != "" || stack.Source.Path != "" {
		return nil, false
	}
	for _, card := range p.cards {
		if card.Stack == stack && (card.Filepath != "" || card.Source.Path != "") {
			return nil, false
		}
	}
	return nil, p.backgrounds == nil
}
