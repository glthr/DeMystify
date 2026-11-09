package parser

import (
	"regexp"
	"strings"

	"github.com/glthr/DeMystify/common"
)

var (
	sequenceLiteralGo = regexp.MustCompile(`(?i)^go(?:\s+to)?\s+card\s+(?:id\s+[0-9]+|"[^"]+"|[a-z][a-z0-9_]*)$`)
	// The direction is one literal word; scripts spell it with or without quotes.
	sequenceVS     = regexp.MustCompile(`(?i)^vs\s+(?:left|right|up|down|"(?:left|right|up|down)")$`)
	sequenceVisual = regexp.MustCompile(`(?i)^visual\s+effect\s+(?:cut|plain|dissolve|(?:scroll|wipe)\s+(?:left|right|up|down))(?:\s+(?:very\s+)?(?:fast|slow))?$`)
)

// Between two requests, these commands of the shared Myst player display, play
// a sound or wait, and none changes the current card. vd, dplay, playQT and
// soundIdle are handlers of the player stack; HTVisual, HTLock and HTAddPict
// are its externals; play, wait and set are HyperTalk built-ins. Like VS, this
// is a stated convention of this abstraction rather than a summary of
// arbitrary calls: every name occurs between two requests in the six stacks,
// an imported handler of the same name takes precedence, and a call written
// with parentheses could run an imported function, so it still ends the run.
var sequenceTransparent = map[string]bool{
	"vd": true, "dplay": true, "playqt": true, "soundidle": true,
	"htvisual": true, "htlock": true, "htaddpict": true,
	"play": true, "wait": true, "set": true,
}

// literalSequence tracks static order only. It does not execute a handler or
// model the openCard/closeCard callbacks triggered by a go command.
type literalSequence struct {
	owner      *HyperCardCard
	eligible   map[int]int
	handler    int
	current    *HyperCardCard
	suppressed bool
	shared     map[string]int
	local      map[string]int
	declared   map[*HyperCardCard]map[string]int
}

func (p *Parser) newLiteralSequence(source any, script HyperTalk) *literalSequence {
	card, ok := source.(*HyperCardCard)
	if !ok || card.Stack == nil {
		return nil
	}
	eligible := make(map[int]int)
	for _, handler := range scriptHandlers(script) {
		if handler.kind != "on" || !linearSequenceHandler(handler) {
			continue
		}
		for i := handler.start + 1; i < handler.end; i++ {
			eligible[i] = handler.start + 1
		}
	}
	if len(eligible) == 0 {
		return nil
	}
	backgrounds, verified := p.backgroundScripts(card.Stack)
	if !verified {
		return nil
	}
	shared := declaredHandlers(card.Stack.Script)
	for name, count := range declaredHandlers(backgrounds) {
		shared[name] += count
	}
	local := declaredHandlers(card.Scripts)
	if shared["go"] != 0 || local["go"] != 0 {
		return nil
	}
	return &literalSequence{owner: card, eligible: eligible, shared: shared, local: local, declared: make(map[*HyperCardCard]map[string]int)}
}

// Reject the whole handler rather than approximating joins, loop iterations or
// saved-return state. Unrecognized ordinary calls still break individual runs.
func linearSequenceHandler(handler handlerDefinition) bool {
	goCount := 0
	for _, line := range handler.body {
		command := activeCommand(line)
		fields := strings.Fields(command)
		if len(fields) == 0 {
			continue
		}
		if strings.Contains(command, "¬") {
			return false
		}
		switch strings.ToLower(fields[0]) {
		case "if", "else", "repeat", "end", "exit", "next", "return", "pass", "push", "pop", "on", "function":
			return false
		case "go":
			if !sequenceLiteralGo.MatchString(command) {
				return false
			}
			goCount++
		}
	}
	return goCount >= 2
}

func (s *literalSequence) cardDeclarations(card *HyperCardCard) map[string]int {
	if names, found := s.declared[card]; found {
		return names
	}
	names := declaredHandlers(card.Scripts)
	s.declared[card] = names
	return names
}

func (s *literalSequence) transparent(command string) bool {
	name := strings.ToLower(strings.Fields(command)[0])
	return sequenceTransparent[name] && !s.overridden(name) &&
		!strings.ContainsAny(unquotedNameStrings.ReplaceAllString(command, ""), "()")
}

func (s *literalSequence) overridden(name string) bool {
	return s.shared[name] != 0 || s.local[name] != 0 || s.current != nil && s.cardDeclarations(s.current)[name] != 0
}

// observe returns the ordered continuation recorded for this line, if any. The
// handler's own card keeps its reference to every card it requests; the step
// from the view requested just before is recorded beside that reference.
func (s *literalSequence) observe(index int, line ScriptLine, link *HyperCardLink) *HyperCardLink {
	if s == nil {
		return nil
	}
	handler, eligible := s.eligible[index]
	if !eligible || handler != s.handler {
		s.current = nil
		s.suppressed = false
		s.handler = handler
	}
	if !eligible || s.suppressed {
		return nil
	}
	command, commented := normalizeCommand(line.Line)
	command = strings.TrimSpace(command)
	if commented || line.IsDisabled {
		// Ordinary comments are harmless; commented navigation cannot change
		// the current card or provide a bridge to a continuation.
		fields := strings.Fields(command)
		if len(fields) > 0 && (strings.EqualFold(fields[0], "go") || strings.EqualFold(fields[0], "push") || strings.EqualFold(fields[0], "pop")) {
			s.current = nil
		}
		return nil
	}
	if command == "" {
		return nil
	}
	if sequenceLiteralGo.MatchString(command) && link != nil && !link.IsDisabled && !link.IsNotImplemented && link.TransitivityRank == common.DefaultTransitivity {
		target, ok := link.Target.(*HyperCardCard)
		if s.overridden("go") {
			s.current = nil
			s.suppressed = true
			return nil
		}
		if !ok || target.Stack != s.owner.Stack {
			s.current = nil
			return nil
		}
		// Requesting the owner first, or the same view twice, is not a step
		// between two views: the owner's own reference already records it.
		var continuation *HyperCardLink
		if s.current != nil && s.current != s.owner && s.current != target {
			continuation = &HyperCardLink{Source: s.current, Target: target, IsSequenceContinuation: true}
		}
		s.current = target
		return continuation
	}
	// VS is an explicit visual-only convention of this abstraction, not an
	// inferred summary of arbitrary external handlers. Imported overrides win.
	if sequenceVS.MatchString(command) && !s.overridden("vs") || sequenceVisual.MatchString(command) && !s.overridden("visual") || s.transparent(command) {
		return nil
	}
	s.current = nil
	return nil
}
