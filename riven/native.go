package riven

import (
	"fmt"
	"strings"
)

// Native transitions are reviewed facts from this pinned reconstruction of the
// interpreter. They remain distinct from commands decoded from the game discs.
const ScummVMCommit = "aed0208e772b128d3f953207a297fc521c32833a"
const SourceBase = "https://github.com/scummvm/scummvm/blob/" + ScummVMCommit + "/engines/mohawk/"

var StackIDs = map[uint16]string{1: "ospit", 2: "pspit", 3: "rspit", 4: "tspit", 5: "bspit", 6: "gspit", 7: "jspit", 8: "aspit"}

type SpecialRedirect struct {
	SourceStack string
	SourceRMap  uint32
	TargetStack string
	TargetRMap  uint32
	Reason      string
}

// SpecialRedirects is evaluated in order against the requested destination,
// before its CARD resource is entered. It is not a list of outgoing edges.
var SpecialRedirects = []SpecialRedirect{
	{"aspit", 0x1f04, "ospit", 0x44ad, "Trap book"},
	{"bspit", 0x1c0e7, "ospit", 0x2e76, "Dome linking book"},
	{"gspit", 0x111b1, "ospit", 0x2e76, "Dome linking book"},
	{"jspit", 0x28a18, "rspit", 0x0f94, "Tay linking book"},
	{"jspit", 0x26228, "ospit", 0x2e76, "Dome linking book"},
	{"ospit", 0x5f0d, "pspit", 0x3bf0, "Return from 233rd Age"},
	{"ospit", 0x470a, "jspit", 0x1508e, "Return from 233rd Age"},
	{"ospit", 0x5c52, "gspit", 0x10bea, "Return from 233rd Age"},
	{"ospit", 0x5d68, "bspit", 0x1adfd, "Return from 233rd Age"},
	{"ospit", 0x5e49, "tspit", 0x0e87, "Return from 233rd Age"},
	{"pspit", 0x4108, "ospit", 0x2e76, "Dome linking book"},
	{"rspit", 0x32d8, "jspit", 0x1c474, "Return from Tay"},
	{"tspit", 0x21b69, "ospit", 0x2e76, "Dome linking book"},
}

// LocalCardID resolves an RMAP identity using the interpreter's last-match
// policy. Index zero is valid; zero is not used as a missing-value sentinel.
func LocalCardID(stack string, globalID uint32, stacks map[string]*Stack) (uint16, error) {
	s, ok := stacks[stack]
	if !ok || s == nil {
		return 0, fmt.Errorf("%s: stack unavailable while resolving RMAP 0x%x", stack, globalID)
	}
	match := -1
	for i, value := range s.RMap {
		if value == globalID {
			match = i
		}
	}
	if match < 0 {
		return 0, fmt.Errorf("%s: RMAP 0x%x has no CARD index", stack, globalID)
	}
	if match > int(^uint16(0)) {
		return 0, fmt.Errorf("%s: RMAP 0x%x index %d exceeds CARD id range", stack, globalID, match)
	}
	return uint16(match), nil
}

// ResolveDestination applies the native table in engine order, preserving its
// complete evidence chain. Missing source identities are harmless for other
// editions; a matched redirect whose destination is absent is an error.
func ResolveDestination(stack string, card uint16, stacks map[string]*Stack) (string, uint16, []map[string]any, error) {
	stack = strings.ToLower(stack)
	chain := make([]map[string]any, 0)
	for _, redirect := range SpecialRedirects {
		if stack != redirect.SourceStack {
			continue
		}
		local, err := LocalCardID(stack, redirect.SourceRMap, stacks)
		if err != nil { // The source RMAP need not exist in an unrelated edition.
			continue
		}
		if card != local {
			continue
		}
		destination, err := LocalCardID(redirect.TargetStack, redirect.TargetRMap, stacks)
		if err != nil {
			return stack, card, chain, err
		}
		chain = append(chain, map[string]any{
			"source_stack": stack, "source_card_id": card, "source_rmap": redirect.SourceRMap,
			"target_stack": redirect.TargetStack, "target_card_id": destination, "target_rmap": redirect.TargetRMap,
			"reason": redirect.Reason, "source": SourceBase + "riven.cpp#L588-L630",
		})
		stack, card = redirect.TargetStack, destination
	}
	return stack, card, chain, nil
}

// NativeTransition deliberately has no destination for a dynamic saved return.
// CardID is -1 when GlobalID should be resolved through the target stack's RMAP.
// Sequence is nil outside a documented ordered sequence; zero remains valid.
type NativeTransition struct {
	Stack     string
	GlobalID  uint32
	CardID    int
	Kind      string
	Condition string
	Source    string
	Sequence  *int
}

func nativeGlobal(stack string, globalID uint32, kind, condition, source string) NativeTransition {
	return NativeTransition{Stack: stack, GlobalID: globalID, CardID: -1, Kind: kind, Condition: condition, Source: SourceBase + source}
}
func nativeCard(stack string, card int, kind, condition, source string) NativeTransition {
	return NativeTransition{Stack: stack, CardID: card, Kind: kind, Condition: condition, Source: SourceBase + source}
}
func nativeSequence(transition NativeTransition, sequence int) NativeTransition {
	transition.Sequence = &sequence
	return transition
}

// ExternalTransitions reports potential navigation caused by an opcode-17
// function. Conditions are annotations, not a game-state satisfiability proof.
// ScummVM-added menu commands only matter when an imported script calls them.
func ExternalTransitions(stack, external string) []NativeTransition {
	stack, name := strings.ToLower(stack), strings.ToLower(external)
	if stack == "aspit" {
		switch name {
		case "xasetupcomplete":
			return []NativeTransition{nativeGlobal("aspit", 0xe2e, "native", "", "riven_stacks/aspit.cpp#L142-L148")}
		case "xaatrusbookback", "xacathbookback", "xtrapbookback":
			return []NativeTransition{nativeGlobal("", 0, "dynamic-return", "returnstackid / returncardid saved when the inventory item was opened", "riven_inventory.cpp#L156-L167")}
		case "xaresumegame":
			return []NativeTransition{nativeGlobal("", 0, "dynamic-return", "menu saved stack and card; resumes the same caller", "riven.cpp#L338-L347")}
		case "xanewgame":
			return []NativeTransition{
				nativeSequence(nativeCard("aspit", 2, "new-game", "new game confirmed", "riven_stacks/aspit.cpp#L369-L389"), 0),
				nativeSequence(nativeGlobal("tspit", 0x6e9a, "new-game", "after aspit:2; new game confirmed", "riven_stacks/aspit.cpp#L369-L389"), 1),
			}
		}
	}
	if stack == "jspit" {
		switch name {
		case "xvga1300_carriage":
			out := make([]NativeTransition, 0, 3)
			for i, globalID := range []uint32{0x18d4d, 0x18ab5, 0x17167} {
				out = append(out, nativeSequence(nativeGlobal("jspit", globalID, "native", "jgallows != 1 and player clicks during the carriage window", "riven_stacks/jspit.cpp#L238-L333"), i))
			}
			return out
		case "xhandlecontrolup", "xhandlecontroldown":
			condition := "handle dragged up"
			if name == "xhandlecontrolup" {
				condition = "handle dragged down"
			}
			return []NativeTransition{nativeGlobal("jspit", 0x1e374, "native", condition, "riven_stacks/jspit.cpp#L375-L420")}
		case "xjlagoon700_alert":
			// The alert video of the middle and lower stairs is the same
			// interaction as the timer of its card, with the same destination:
			// same kind, so that the record gains a callsite, not a twin.
			return []NativeTransition{nativeGlobal("jspit", 0x7beb, "native-timer", "jsunners == 0; click or move forward while the alert video plays", "riven_stacks/jspit.cpp#L487-L500")}
		case "xjlagoon800_alert":
			return []NativeTransition{nativeGlobal("jspit", 0xb6ca, "native-timer", "jsunners == 0; click or move forward while the alert video plays", "riven_stacks/jspit.cpp#L502-L522")}
		case "xhandlecontrolmid":
			return []NativeTransition{
				nativeGlobal("jspit", 0x1e597, "native", "handle dragged up", "riven_stacks/jspit.cpp#L422-L460"),
				nativeGlobal("jspit", 0x1e29c, "native", "handle dragged down", "riven_stacks/jspit.cpp#L422-L460"),
			}
		}
	}
	if stack == "ospit" {
		switch name {
		case "xbookclick":
			return []NativeTransition{
				nativeGlobal("ospit", 0x2885, "native", "click trap book in the native video window", "riven_stacks/ospit.cpp#L69-L129"),
				nativeCard("aspit", 1, "ending", "no click on third request; ending credits then game reset", "riven_stacks/ospit.cpp#L138-L146"),
			}
		case "xorollcredittime":
			return []NativeTransition{
				nativeGlobal("rspit", 0x3338, "native", "returnstackid is rspit (ScummVM workaround)", "riven_stacks/ospit.cpp#L47-L55"),
				nativeCard("aspit", 1, "ending", "returnstackid is not rspit; ending credits then game reset", "riven_stack.cpp#L208-L269"),
			}
		}
	}
	if stack == "rspit" && name == "xrcredittime" {
		return []NativeTransition{nativeCard("aspit", 1, "ending", "ending credits then game reset", "riven_stack.cpp#L208-L269")}
	}
	if stack == "tspit" && name == "xtexterior300_telescopedown" {
		return []NativeTransition{nativeCard("aspit", 1, "ending", "telescope at bottom with cover open and pin released; ending credits then game reset", "riven_stacks/tspit.cpp#L59-L161")}
	}
	return nil
}

// TimerTransitions describes the three native Sunner video interactions.
func TimerTransitions(stack string, globalID uint32) []NativeTransition {
	if strings.ToLower(stack) != "jspit" {
		return nil
	}
	var target uint32
	switch globalID {
	case 0x77d6:
		target = 0x79bd
	case 0x79bd:
		target = 0x7beb
	case 0x7beb:
		target = 0xb6ca
	default:
		return nil
	}
	return []NativeTransition{nativeGlobal("jspit", target, "native-timer", "jsunners == 0; click or move forward while timer video plays", "riven_stacks/jspit.cpp#L542-L668")}
}

// InventoryModel records caller-sensitive entry/return policy without creating
// false all-to-all routes between every game card through the journals.
func InventoryModel() map[string]any {
	return map[string]any{
		"source":         SourceBase + "riven_inventory.cpp#L91-L167",
		"available_from": "non-aspit cards when inventory is visible",
		"items": []map[string]any{
			{"name": "Atrus journal", "target_stack": "aspit", "target_card_id": 5, "condition": "always owned"},
			{"name": "Catherine journal", "target_stack": "aspit", "target_card_id": 6, "condition": "rrebel in {5,6}"},
			{"name": "Trap book", "target_stack": "aspit", "target_card_id": 7, "condition": "rrebel in {5,6} and atrapbook == 1"},
		},
		"return":       "external back command returns to saved returnstackid / returncardid (RMAP)",
		"graph_policy": "metadata only; inventory entry/return is caller-sensitive and excluded from ordinary navigation distances",
	}
}
