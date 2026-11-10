package parser

import (
	"regexp"
	"strings"

	"github.com/glthr/DeMystify/common"
)

var (
	sequenceLiteralGo = regexp.MustCompile(`(?i)^go(?:\s+to)?\s+card\s+(?:id\s+[0-9]+|"[^"]+"|[a-z][a-z0-9_]*)$`)
	// A go that names no destination redisplays the card already showing.
	sequenceRedrawGo = regexp.MustCompile(`(?i)^go(?:\s+to)?(?:\s+this)?\s+card$`)
	// The direction is one literal word; scripts spell it with or without quotes.
	sequenceVS     = regexp.MustCompile(`(?i)^vs\s+(?:left|right|up|down|"(?:left|right|up|down)")$`)
	sequenceVisual = regexp.MustCompile(`(?i)^visual\s+effect\s+(?:cut|plain|dissolve|(?:scroll|wipe)\s+(?:left|right|up|down))(?:\s+(?:very\s+)?(?:fast|slow))?$`)
	// Leaving a branch by its end continues with the line that follows it.
	sequenceEndIf = regexp.MustCompile(`(?i)^end\s+if$`)
	// A saved return anywhere in the handler: push or pop, even after "then".
	sequenceSavedReturn = regexp.MustCompile(`(?i)\b(?:push|pop)\b`)
	// A go command starts a line or follows then or else.
	sequenceGoStart = regexp.MustCompile(`(?i)(?:^|\b(?:then|else)\s+)go\b`)
	sequenceElse    = regexp.MustCompile(`(?i)\s+else\b`)
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
		if handler.kind != "on" || !sequenceHandler(handler) {
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

// sequenceHandler reports whether a handler's lines can be read as runs. A
// run is a stretch of lines inside one branch or one loop body: a condition,
// an else, a loop boundary, an exit or any other command ends it at its own
// line, in observe, so joins and loop iterations are never approximated.
//
// Only what a run cannot localize hides the whole handler: a continued line, a
// saved return, and a go that is neither a literal card nor a redraw. Such a
// go may leave the stack, after which a later card id means another card.
func sequenceHandler(handler handlerDefinition) bool {
	requests := 0
	for _, line := range handler.body {
		// A continued line takes the next one with it, comment or not.
		if strings.Contains(line.Line, "¬") {
			return false
		}
		command := activeCommand(line)
		fields := strings.Fields(command)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "on", "function":
			return false
		}
		if sequenceSavedReturn.MatchString(unquotedNameStrings.ReplaceAllString(command, "")) {
			return false
		}
		for _, request := range goCommands(command) {
			switch {
			case sequenceRedrawGo.MatchString(request):
			case sequenceLiteralGo.MatchString(request):
				// Only a request that is the whole line can begin or end a run.
				if request == command {
					requests++
				}
			default:
				return false
			}
		}
	}
	return requests >= 2
}

// goCommands returns every go command of a line: the one that starts it and
// those that follow then or else, each up to a following else. Quoted text is
// not searched.
func goCommands(command string) []string {
	masked := unquotedNameStrings.ReplaceAllStringFunc(command, func(quoted string) string { return strings.Repeat("\"", len(quoted)) })
	var requests []string
	for _, match := range sequenceGoStart.FindAllStringIndex(masked, -1) {
		begin := match[1] - len("go")
		end := len(masked)
		if next := sequenceElse.FindStringIndex(masked[begin:]); next != nil {
			end = begin + next[0]
		}
		requests = append(requests, strings.TrimSpace(command[begin:end]))
	}
	return requests
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
	// Only display, sound and wait commands separate the request from this
	// line, so no block was opened since: the branch that ends here holds the
	// request, and the handler goes on with the line below.
	if sequenceEndIf.MatchString(command) {
		return nil
	}
	s.current = nil
	return nil
}
