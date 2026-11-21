package parser

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/glthr/DeMystify/hypercard"
)

// HyperCard sends a click to the topmost part under the pointer, and a part
// without a handler passes the message to the card, never to the part beneath.
// A part whose rectangle lies entirely inside a later button of the same card
// therefore receives nothing, as long as that button stays where it is.
//
// The proof is static and deliberately narrow. The cover is one later button
// of the card layer, stored visible and enabled. No script that can run on the
// card (its own, its parts', the backgrounds' and the stack's) may change a
// part or send it a message, unless it names, by a quoted name, an id or a
// number, a part that is neither the covered one nor its cover. Anything less
// explicit — a variable, the target, me inside either part, a do command whose
// text is not a known command — leaves the part reachable. Of the stack and
// background scripts, only the handlers that can run on this card are read
// (liveSharedLines). The player stack that hosts the Ages is not part of the
// input and is not read.
var (
	partChange = regexp.MustCompile(`(?i)\b(?:hide|show|enable|disable|select|delete|set|send|do)\b`)
	// A designator: [card|bg] button|part <id N | N | "name" | word>.
	partDesignator = regexp.MustCompile(`(?i)\b(?:(card|cd|background|bkgnd|bg)\s+)?(button|btn|part|field|fld)\s+(id\s+[0-9]+|[0-9]+|"[^"]*"|\(|[a-z_][a-z0-9_]*)`)
	partSelf       = regexp.MustCompile(`(?i)\b(?:me|target)\b`)
	partTarget     = regexp.MustCompile(`(?i)\btarget\b`)
)

// shadowedParts returns, for each part of the card in layer order, the index of
// the later button that shadows it, or -1.
func shadowedParts(stack hypercard.Stack, card hypercard.Card) []int {
	covers, found := coveredParts(stack, card)
	if !found {
		return covers
	}
	return stableCovers(stack, card, covers)
}

// coveredParts is the geometry alone: for each scripted, visible part, the
// first later button, visible and enabled as stored, that contains it.
func coveredParts(stack hypercard.Stack, card hypercard.Card) ([]int, bool) {
	covers := make([]int, len(card.Parts))
	found := false
	for i, part := range card.Parts {
		covers[i] = -1
		if part.Hidden || strings.TrimSpace(part.Script.Text) == "" {
			continue
		}
		rect, ok := clipToCard(part.Rect, stack)
		if !ok {
			continue
		}
		for j := i + 1; j < len(card.Parts); j++ {
			cover := card.Parts[j]
			if cover.Type == "button" && !cover.Hidden && cover.Enabled && contains(cover.Rect, rect) {
				covers[i], found = j, true
				break
			}
		}
	}
	return covers, found
}

func stableCovers(stack hypercard.Stack, card hypercard.Card, covers []int) []int {
	shared := liveSharedLines(stack, card)
	for i, j := range covers {
		if j < 0 {
			continue
		}
		stable := !mayReach(shared, card, i, j, false) && !mayReach(splitScript(card.Script.Text), card, i, j, false)
		for k, part := range card.Parts {
			stable = stable && !mayReach(splitScript(part.Script.Text), card, i, j, k == i || k == j)
		}
		if !stable {
			covers[i] = -1
		}
	}
	return covers
}

// Messages that HyperCard, or the movie windows of the player, send by
// themselves. A handler of the stack or of a background with any other name
// runs only when something calls it.
var systemMessages = map[string]bool{
	"mouseup": true, "mousedown": true, "mousestilldown": true, "mouseenter": true, "mouseleave": true,
	"mousewithin": true, "mousedoubleclick": true, "opencard": true, "closecard": true, "openbackground": true,
	"closebackground": true, "openstack": true, "closestack": true, "idle": true, "newcard": true,
	"deletecard": true, "newbackground": true, "deletebackground": true, "newstack": true, "deletestack": true,
	"newbutton": true, "deletebutton": true, "newfield": true, "deletefield": true, "openfield": true,
	"closefield": true, "exitfield": true, "enterinfield": true, "returninfield": true, "keydown": true,
	"commandkeydown": true, "returnkey": true, "enterkey": true, "tabkey": true, "arrowkey": true,
	"controlkey": true, "functionkey": true, "domenu": true, "choose": true, "help": true, "close": true,
	"quit": true, "startup": true, "resume": true, "suspend": true, "resumestack": true, "suspendstack": true,
	"hide": true, "show": true, "movewindow": true, "sizewindow": true, "appleevent": true,
	"mousedowninmovie": true, "mouseupinmovie": true, "movieidle": true, "closemovie": true, "moviedone": true,
}

var scriptWord = regexp.MustCompile(`(?i)[a-z_][a-z0-9_]*`)

func scriptWords(words map[string]bool, text string) {
	for _, word := range scriptWord.FindAllString(text, -1) {
		words[strings.ToLower(word)] = true
	}
}

// liveSharedLines returns the lines of the stack and background scripts that
// can run while the card is showing: those of the handlers for a system
// message, of the handlers that a script of the card names (a call, or a
// message name in a string), and of the handlers those name in turn. A helper
// that only other cards call cannot run here.
func liveSharedLines(stack hypercard.Stack, card hypercard.Card) []string {
	scripts := []hypercard.Script{stack.Script}
	for _, background := range stack.Backgrounds {
		scripts = append(scripts, background.Script)
		for _, part := range background.Parts {
			scripts = append(scripts, part.Script)
		}
	}
	var handlers []handlerDefinition
	for _, script := range scripts {
		handlers = append(handlers, scriptHandlers(compileScript(script, true))...)
	}
	named := make(map[string]bool)
	scriptWords(named, card.Script.Text)
	for _, part := range card.Parts {
		scriptWords(named, part.Script.Text)
	}
	live := make([]bool, len(handlers))
	for changed := true; changed; {
		changed = false
		for i, handler := range handlers {
			if live[i] || !systemMessages[handler.name] && !named[handler.name] {
				continue
			}
			live[i], changed = true, true
			for _, line := range handler.body {
				scriptWords(named, line.Line)
			}
		}
	}
	var lines []string
	for i, handler := range handlers {
		if live[i] {
			for _, line := range handler.body {
				lines = append(lines, line.Line)
			}
		}
	}
	return lines
}

// A do command runs text built at run time. It is harmless only when that text
// begins with a literal whose first word is another command and nothing in the
// expression can add a line to it.
var (
	doCommand   = regexp.MustCompile(`(?i)^(?:.*\bthen\s+|else\s+)?do\b\s*\(?\s*(.*)$`)
	doLiteral   = regexp.MustCompile(`^"\s*([A-Za-z]+) [^"]*"`)
	doMultiline = regexp.MustCompile(`(?i)\b(?:return|cr|linefeed|numtochar|lines?)\b`)
)

func dynamicDo(command string) (isDo, harmless bool) {
	match := doCommand.FindStringSubmatch(strings.TrimSpace(command))
	if match == nil {
		return false, false
	}
	literal := doLiteral.FindStringSubmatch(match[1])
	if literal == nil || partChange.MatchString(literal[1]) {
		return true, false
	}
	rest := unquotedNameStrings.ReplaceAllString(match[1], `""`)
	// "line x of" a container is one line by definition; any other way to
	// bring a line break into the text keeps the command opaque.
	rest = regexp.MustCompile(`(?i)\bline\s+[a-z0-9_]+\s+of\b`).ReplaceAllString(rest, "")
	return true, !doMultiline.MatchString(rest)
}

func clipToCard(rect hypercard.Rectangle, stack hypercard.Stack) (hypercard.Rectangle, bool) {
	if stack.Width > 0 && stack.Height > 0 {
		rect.Left, rect.Top = max(rect.Left, 0), max(rect.Top, 0)
		rect.Right, rect.Bottom = min(rect.Right, stack.Width), min(rect.Bottom, stack.Height)
	}
	return rect, rect.Right > rect.Left && rect.Bottom > rect.Top
}

func contains(outer, inner hypercard.Rectangle) bool {
	return outer.Left <= inner.Left && outer.Top <= inner.Top && outer.Right >= inner.Right && outer.Bottom >= inner.Bottom
}

// mayReach reports whether a script holds a command that could change the
// covered part or its cover, or send either a message. own marks the script of
// one of the two parts, where me and the target are that part.
func mayReach(lines []string, card hypercard.Card, covered, cover int, own bool) bool {
	for _, raw := range lines {
		command, disabled := normalizeCommand(raw)
		if disabled || !partChange.MatchString(unquotedNameStrings.ReplaceAllString(command, `""`)) {
			continue
		}
		if isDo, harmless := dynamicDo(command); isDo {
			if !harmless {
				return true
			}
			continue
		}
		designators := partDesignator.FindAllStringSubmatch(command, -1)
		if partSelf.MatchString(unquotedNameStrings.ReplaceAllString(command, `""`)) {
			// me in another part's script is that part; in a card, background or
			// stack script the target is whichever part received the message.
			if own || partTarget.MatchString(command) {
				return true
			}
		}
		for _, designator := range designators {
			layer, kind, name := strings.ToLower(designator[1]), strings.ToLower(designator[2]), designator[3]
			if strings.HasPrefix(layer, "b") {
				continue // a background part is never a card part
			}
			for _, index := range []int{covered, cover} {
				if designates(card, index, kind, name) {
					return true
				}
			}
		}
	}
	return false
}

// designates reports whether a designator may name the part at index. Only a
// quoted name, an id or a number can be told apart from it.
func designates(card hypercard.Card, index int, kind, name string) bool {
	part := card.Parts[index]
	field := kind == "field" || kind == "fld"
	if kind != "part" && field != (part.Type == "field") {
		return false
	}
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(name, `"`):
		return strings.EqualFold(strings.Trim(name, `"`), part.Name)
	case strings.HasPrefix(lower, "id"):
		id, err := strconv.Atoi(strings.TrimSpace(name[2:]))
		return err != nil || id == part.ID
	case name[0] >= '0' && name[0] <= '9':
		number, err := strconv.Atoi(name)
		if err != nil {
			return true
		}
		// The number counts the parts of that kind, or all parts, in layer order.
		position := 0
		for i, other := range card.Parts {
			if kind == "part" || other.Type == part.Type {
				position++
			}
			if i == index {
				break
			}
		}
		return number == position
	}
	return true // an expression or a bare word: a variable, or a name
}
