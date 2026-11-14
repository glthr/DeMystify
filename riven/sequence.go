package riven

import "fmt"

// sequenceReference is a caller-owned observation of static instruction order,
// not an independently selectable route from sourceID to targetID. The shared
// graph displays it as SequenceContinuation and excludes it from path search.
// All literal requests from the handler's owner remain separate references.
type sequenceReference struct {
	sourceID, targetID uint16
	evidence           map[string]any
}

// sequenceDisplayCommand lists non-navigating primitives that can separate two
// requests in a display/sound/wait run. This deliberately excludes arbitrary
// native calls, variable changes, refresh/callbacks, stored movie opcodes, Zip,
// stack changes and opcode 36 whose purpose remains unknown.
func sequenceDisplayCommand(op uint16) bool {
	switch op {
	case 1, 3, 4, 12, 13, 14, 18, 20, 21, 28, 29, 31, 32, 33, 34, 37, 39, 40, 41, 44, 46:
		return true
	}
	return false
}

// sequenceCallbacksStable rejects a card if a load, enter, update or leave callback
// could navigate, refresh, run opaque native code or install deferred work.
// Reading a switch or changing a primitive variable/hotspot is non-navigating;
// no attempt is made to solve those conditions or reconstruct their state.
func sequenceCallbacksStable(card *Card) bool {
	if card == nil {
		return false
	}
	var stable func([]Command) bool
	stable = func(commands []Command) bool {
		for _, command := range commands {
			if command.Opcode == 8 {
				for _, branch := range command.Branches {
					if !stable(branch.Commands) {
						return false
					}
				}
				continue
			}
			if !sequenceDisplayCommand(command.Opcode) && command.Opcode != 7 && command.Opcode != 9 && command.Opcode != 10 && command.Opcode != 24 && command.Opcode != 43 {
				return false
			}
		}
		return true
	}
	for _, script := range card.Scripts {
		if (script.Type == 6 || script.Type == 7 || script.Type == 9 || script.Type == 10) && !stable(script.Commands) {
			return false
		}
	}
	return true
}

func sequenceReferences(stack *Stack, card *Card, stacks map[string]*Stack) ([]sequenceReference, error) {
	if stack == nil || card == nil || stack.Name == "" || stacks[stack.Name] == nil {
		return nil, fmt.Errorf("sequence references require an owning card and stack")
	}
	owner := SourceID(stack.Name, card.ID)
	result := []sequenceReference{}
	process := func(script Script, hotspot *Hotspot, ordinal int) error {
		// Each event and hotspot is independent. State never leaks between them.
		var walk func([]Command, []map[string]any, bool) (bool, error)
		walk = func(commands []Command, guards []map[string]any, stackStable bool) (bool, error) {
			var previous *Command
			between := []uint16{}
			for index := range commands {
				command := &commands[index]
				if command.Opcode == 8 {
					// Branches get independent runs. No predecessor is inherited from
					// before the switch, and no branch result becomes a join edge.
					previous = nil
					values := []uint16{}
					for _, branch := range command.Branches {
						if branch.Value != 65535 {
							values = append(values, branch.Value)
						}
					}
					allStable := stackStable
					for _, branch := range command.Branches {
						excluded := []uint16{}
						if branch.Value == 65535 {
							excluded = append(excluded, values...)
						}
						guard := map[string]any{"variable": command.Variable, "variable_name": NameAt(stack.Names, 4, int(command.Variable)), "value": branch.Value, "switch_offset": command.Offset, "excluded_values": excluded}
						nested := append(append([]map[string]any{}, guards...), guard)
						stable, err := walk(branch.Commands, nested, stackStable)
						if err != nil {
							return false, err
						}
						allStable = allStable && stable
					}
					stackStable = allStable
					continue
				}
				if command.Opcode == 2 {
					if len(command.Args) != 1 {
						return false, fmt.Errorf("%s sequence contains malformed card change at +0x%x", owner, command.Offset)
					}
					if !stackStable {
						continue
					}
					resolvedStack, resolvedID, redirects, err := ResolveDestination(stack.Name, command.Args[0], stacks)
					if err != nil {
						return false, err
					}
					if resolvedStack != stack.Name || len(redirects) != 0 || resolvedID != command.Args[0] {
						previous = nil
						stackStable = false
						continue
					}
					if previous != nil && previous.Args[0] != card.ID && previous.Args[0] != command.Args[0] {
						ev := map[string]any{
							"sequence_owner": owner, "resource": fmt.Sprintf("CARD:%d", card.ID),
							"event": EventName(script.Type), "byte_offset": command.Offset, "previous_byte_offset": previous.Offset,
							"opcode": command.Opcode, "previous_literal_target": SourceID(stack.Name, previous.Args[0]),
							"literal_target": SourceID(stack.Name, command.Args[0]), "guards": append([]map[string]any{}, guards...),
							"between_opcodes": append([]uint16{}, between...),
							"semantics":       "same caller script and branch; static instruction order, not an independent player action",
						}
						if hotspot != nil {
							ev["resource"] = fmt.Sprintf("HSPT:%d", card.ID)
							ev["hotspot_ordinal"], ev["hotspot_blst_id"] = ordinal, hotspot.ID
							ev["hotspot_name"], ev["hotspot_rect"] = hotspot.Name, hotspot.Rect
						}
						result = append(result, sequenceReference{previous.Args[0], command.Args[0], ev})
					}
					// A missing card or opaque callback cannot be used as the source
					// of the next instruction-order observation. A destination's
					// callback may change the current stack before control returns.
					if sequenceCallbacksStable(stack.Cards[command.Args[0]]) {
						previous = command
					} else {
						previous = nil
						stackStable = false
					}
					between = nil
					continue
				}
				if sequenceDisplayCommand(command.Opcode) {
					if previous != nil {
						between = append(between, command.Opcode)
					}
					continue
				}
				previous = nil
				// These primitive state changes end the display run but leave the
				// current stack known. All other opaque operations invalidate it.
				if command.Opcode != 7 && command.Opcode != 9 && command.Opcode != 10 && command.Opcode != 24 && command.Opcode != 43 {
					stackStable = false
				}
			}
			return stackStable, nil
		}
		_, err := walk(script.Commands, []map[string]any{}, sequenceCallbacksStable(card))
		return err
	}
	for _, script := range card.Scripts {
		if err := process(script, nil, -1); err != nil {
			return nil, err
		}
	}
	for ordinal := range card.Hotspots {
		hotspot := &card.Hotspots[ordinal]
		for _, script := range hotspot.Scripts {
			if err := process(script, hotspot, ordinal); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}
