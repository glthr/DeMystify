package riven

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

// HotspotChange is evidence about activation, not proof that its script runs.
// BLST lookups and direct enables target the first hotspot with the given ID.
type HotspotChange struct {
	Resource string           `json:"resource"`
	Event    string           `json:"event"`
	Offset   int              `json:"byte_offset"`
	Opcode   uint16           `json:"opcode"`
	Enabled  bool             `json:"enabled"`
	BLST     uint16           `json:"blst_index,omitempty"`
	Guards   []map[string]any `json:"guards,omitempty"`
	Caller   string           `json:"caller,omitempty"`
}

// HotspotState separates activation at entry from a conservative lifetime
// classification. An unknown/native effect is never converted to Disabled.
type HotspotState struct {
	Status         string          `json:"status"`
	EntryState     string          `json:"entry_state"`
	Reason         string          `json:"reason"`
	CanEnableLater bool            `json:"can_enable_later"`
	Changes        []HotspotChange `json:"changes,omitempty"`
	// Shadow is "always" when the pointer can never reach the hotspot,
	// "conditional" when it can only while a covering hotspot that scripts
	// switch is off, and "unknown" when the only covers have an unknown
	// status. It says nothing about activation: Status keeps describing the
	// hotspot itself.
	Shadow       string `json:"shadow,omitempty"`
	ShadowedBy   []int  `json:"shadowed_by,omitempty"`
	ShadowReason string `json:"shadow_reason,omitempty"`
}

const (
	ShadowAlways      = "always"
	ShadowConditional = "conditional"
	ShadowUnknown     = "unknown"
)

func hotspotRectValid(r [4]int) bool { return r[0] < r[2] && r[1] < r[3] }
func hotspotRectContains(outer, inner [4]int) bool {
	return outer[0] <= inner[0] && outer[1] <= inner[1] && outer[2] >= inner[2] && outer[3] >= inner[3]
}

// applyHotspotShadows records which hotspots the pointer cannot reach. The
// interpreter gives a mouse event to one hotspot only: the last enabled one
// whose rectangle contains the pointer (the pinned engine walks the list in
// file order and keeps the last hit; the original 1.0 program sorts by the
// stored index, descending, and takes the first). A hotspot whose whole
// rectangle lies inside a later one therefore receives nothing while that
// later hotspot is on. The claim is made only when file order and stored index
// agree, and only from whole containment by a single later rectangle: a
// hotspot hidden by several partial covers together is not reported. An empty
// or inverted rectangle contains no point, and the pinned engine also disables
// such a hotspot when it loads the card.
//
// "always" needs a cover whose status is "enabled": no script, caller or
// native effect can switch it off. Otherwise a cover of conditional status
// (scripts or Zip switch it) gives "conditional", and covers of unknown status
// alone give "unknown", which claims nothing, as for activation. States must
// be final: a later status change would invalidate the result.
func applyHotspotShadows(c *Card, states map[int]HotspotState) {
	for i := range c.Hotspots {
		state := states[i]
		state.Shadow, state.ShadowedBy, state.ShadowReason = "", nil, ""
		h := c.Hotspots[i]
		if !hotspotRectValid(h.Rect) {
			state.Shadow = ShadowAlways
			state.ShadowReason = "empty or inverted rectangle: no point lies inside it, and the interpreter disables such a hotspot when it loads the card"
			states[i] = state
			continue
		}
		var always, sometimes, unknown []int
		for j := i + 1; j < len(c.Hotspots); j++ {
			cover := c.Hotspots[j]
			if cover.Index <= h.Index || !hotspotRectValid(cover.Rect) || !hotspotRectContains(cover.Rect, h.Rect) {
				continue
			}
			switch states[j].Status {
			case "enabled":
				always = append(always, j)
			case "conditional":
				sometimes = append(sometimes, j)
			case "unknown":
				unknown = append(unknown, j)
			}
		}
		switch {
		case len(always) != 0:
			state.Shadow, state.ShadowedBy = ShadowAlways, always
			state.ShadowReason = "a later hotspot that is never switched off contains this rectangle and receives every mouse event in it"
		case len(sometimes) != 0:
			state.Shadow, state.ShadowedBy = ShadowConditional, sometimes
			state.ShadowReason = "a later hotspot contains this rectangle; the pointer reaches this one only while that hotspot is off"
		case len(unknown) != 0:
			state.Shadow, state.ShadowedBy = ShadowUnknown, unknown
			state.ShadowReason = "a later hotspot of unknown activation contains this rectangle"
		}
		states[i] = state
	}
}

type hotspotEnableRecord struct{ index, enabled, hotspot uint16 }
type hotspotBits uint8

const (
	hotspotEnabled  hotspotBits = 1
	hotspotDisabled hotspotBits = 2
	hotspotEither               = hotspotEnabled | hotspotDisabled
)

type hotspotAnalysis struct {
	stack       *Stack
	card        *Card
	list        []hotspotEnableRecord
	byID        map[uint16]int
	changes     map[int][]HotspotChange
	later       map[int]bool
	laterChange map[int]bool
	opaque      bool
	entryOpaque bool
}

func hotspotRecords(s *Stack, c *Card) ([]hotspotEnableRecord, error) {
	ref, ok := s.Resources["BLST"][c.ID]
	if !ok {
		return nil, nil
	}
	if ref.Archive == nil {
		return nil, fmt.Errorf("%s BLST:%d lacks an archive", s.Name, c.ID)
	}
	b, err := ref.Archive.Read("BLST", c.ID)
	if err != nil {
		return nil, err
	}
	if len(b) < 2 {
		return nil, fmt.Errorf("%s BLST:%d truncated", s.Name, c.ID)
	}
	n := int(binary.BigEndian.Uint16(b))
	if len(b) != 2+n*6 {
		return nil, fmt.Errorf("%s BLST:%d invalid record count", s.Name, c.ID)
	}
	list := make([]hotspotEnableRecord, n)
	for i := range list {
		p := b[2+i*6:]
		list[i] = hotspotEnableRecord{be16(p), be16(p[2:]), be16(p[4:])}
	}
	return list, nil
}

func newHotspotAnalysis(s *Stack, c *Card) (*hotspotAnalysis, error) {
	list, err := hotspotRecords(s, c)
	if err != nil {
		return nil, err
	}
	a := &hotspotAnalysis{stack: s, card: c, list: list, byID: map[uint16]int{}, changes: map[int][]HotspotChange{}, later: map[int]bool{}, laterChange: map[int]bool{}}
	for i, h := range c.Hotspots {
		if _, ok := a.byID[h.ID]; !ok {
			a.byID[h.ID] = i
		}
	}
	return a, nil
}

// target applies the pinned engine's first-match rules. A missing BLST index
// or missing direct hotspot has no effect; it is not an implicit disable.
func (a *hotspotAnalysis) target(cmd Command) (ordinal int, enabled bool, ok bool, err error) {
	if cmd.Opcode != 9 && cmd.Opcode != 10 && cmd.Opcode != 43 {
		return 0, false, false, nil
	}
	if len(cmd.Args) != 1 {
		return 0, false, false, fmt.Errorf("%s:%d malformed hotspot command%d at +0x%x", a.stack.Name, a.card.ID, cmd.Opcode, cmd.Offset)
	}
	id := cmd.Args[0]
	enabled = cmd.Opcode == 9
	if cmd.Opcode == 43 {
		found := false
		for _, r := range a.list {
			if r.index == id {
				id = r.hotspot
				enabled = r.enabled == 1
				found = true
				break
			}
		}
		if !found {
			return 0, false, false, nil
		}
	}
	ordinal, ok = a.byID[id]
	return ordinal, enabled, ok, nil
}

func activationName(b hotspotBits) string {
	switch b {
	case hotspotEnabled:
		return "enabled"
	case hotspotDisabled:
		return "disabled"
	default:
		return "conditional"
	}
}
func copyHotspots(in []hotspotBits) []hotspotBits { return append([]hotspotBits(nil), in...) }
func mergeHotspots(a, b []hotspotBits) {
	for i := range a {
		a[i] |= b[i]
	}
}
func unknownHotspots(a []hotspotBits) {
	for i := range a {
		a[i] = hotspotEither
	}
}

func hotspotGuards(s *Stack, c Command, branch Branch, prior []map[string]any) []map[string]any {
	excluded := []uint16{}
	if branch.Value == 65535 {
		for _, b := range c.Branches {
			if b.Value != 65535 {
				excluded = append(excluded, b.Value)
			}
		}
	}
	g := map[string]any{"variable": c.Variable, "variable_name": NameAt(s.Names, 4, int(c.Variable)), "value": branch.Value, "switch_offset": c.Offset, "excluded_values": excluded}
	return append(append([]map[string]any(nil), prior...), g)
}

// collect retains all possible activation writes. It does not label a control
// disabled merely because no enable was found. Initial ordinary state is on.
func (a *hotspotAnalysis) collect(cs []Command, resource string, event uint16, guards []map[string]any, current bool) error {
	for _, cmd := range cs {
		if cmd.Opcode == 8 {
			for _, b := range cmd.Branches {
				if err := a.collect(b.Commands, resource, event, hotspotGuards(a.stack, cmd, b, guards), current); err != nil {
					return err
				}
			}
			// If a branch can change card, subsequent commands do not certainly
			// refer to the script owner. Treat that continuation as opaque.
			if commandsChangeCard(cmd.Branches) {
				current = false
			}
			continue
		}
		if cmd.Opcode == 17 || cmd.Opcode == 36 {
			a.opaque = true
		}
		if cmd.Opcode == 38 && (len(cmd.Args) < 5 || cmd.Args[3] != 40) {
			// The verified corpus stores only ambient-sound activation. A
			// different delayed operation needs a caller-sensitive callback
			// model before activation can be proved permanent.
			a.opaque = true
		}
		if _, known := opNames[cmd.Opcode]; !known {
			a.opaque = true
		}
		if cmd.Opcode == 2 || cmd.Opcode == 27 {
			current = false
			continue
		}
		if !current {
			if cmd.Opcode == 9 || cmd.Opcode == 43 {
				// A sequence may return to a fresh instance of the owner card.
				// Do not apply this write to the old owner, or prove it cannot
				// later enable the owner without current-card path analysis.
				a.opaque = true
			}
			continue
		}
		ordinal, enabled, ok, err := a.target(cmd)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		change := HotspotChange{Resource: resource, Event: EventName(event), Offset: cmd.Offset, Opcode: cmd.Opcode, Enabled: enabled, Guards: guards}
		if cmd.Opcode == 43 {
			change.BLST = cmd.Args[0]
		}
		a.changes[ordinal] = append(a.changes[ordinal], change)
		// Screen updates can repeat later; mouse/frame/leave events can run
		// after entry. Merely enabling before a final load disable is not a
		// later route back to that hotspot.
		if enabled && event != 6 && event != 9 {
			a.later[ordinal] = true
		}
		if event != 6 && event != 9 {
			a.laterChange[ordinal] = true
		}
	}
	return nil
}

func commandsChangeCard(bs []Branch) bool {
	for _, b := range bs {
		for _, c := range b.Commands {
			if c.Opcode == 2 || c.Opcode == 27 || (c.Opcode == 8 && commandsChangeCard(c.Branches)) {
				return true
			}
		}
	}
	return false
}

func (a *hotspotAnalysis) entry(cs []Command, state []hotspotBits) error {
	for _, cmd := range cs {
		if cmd.Opcode == 8 {
			merged := make([]hotspotBits, len(state))
			hasDefault := false
			seen := map[uint16]bool{}
			for _, b := range cmd.Branches {
				if seen[b.Value] {
					continue
				}
				seen[b.Value] = true
				if b.Value == 65535 {
					hasDefault = true
				}
				sub := copyHotspots(state)
				if err := a.entry(b.Commands, sub); err != nil {
					return err
				}
				mergeHotspots(merged, sub)
			}
			if !hasDefault {
				mergeHotspots(merged, state)
			}
			copy(state, merged)
			continue
		}
		if cmd.Opcode == 2 || cmd.Opcode == 27 {
			a.entryOpaque = true
			unknownHotspots(state)
			return nil
		}
		// Refresh recursively enters the card; explicit screen-update nesting
		// can invoke update scripts before this simple phase pass expects it.
		if cmd.Opcode == 17 || cmd.Opcode == 19 || cmd.Opcode == 20 || cmd.Opcode == 21 || cmd.Opcode == 36 {
			a.entryOpaque = true
			unknownHotspots(state)
		}
		if _, known := opNames[cmd.Opcode]; !known {
			a.entryOpaque = true
			unknownHotspots(state)
		}
		ordinal, enabled, ok, err := a.target(cmd)
		if err != nil {
			return err
		}
		if ok {
			if enabled {
				state[ordinal] = hotspotEnabled
			} else {
				state[ordinal] = hotspotDisabled
			}
		}
	}
	return nil
}

// HotspotAvailability models the card's own entry and events. Import callers
// should use HotspotAvailabilityForStacks, which also considers foreign script
// continuations. The map is keyed by HSPT ordinal, not potentially repeated ID.
func HotspotAvailability(s *Stack, c *Card) (map[int]HotspotState, error) {
	a, err := newHotspotAnalysis(s, c)
	if err != nil {
		return nil, err
	}
	for _, sc := range c.Scripts {
		if err = a.collect(sc.Commands, fmt.Sprintf("CARD:%d", c.ID), sc.Type, nil, true); err != nil {
			return nil, err
		}
	}
	for _, h := range c.Hotspots {
		for _, sc := range h.Scripts {
			if err = a.collect(sc.Commands, fmt.Sprintf("HSPT:%d", c.ID), sc.Type, nil, true); err != nil {
				return nil, err
			}
		}
	}
	state := make([]hotspotBits, len(c.Hotspots))
	for i := range c.Hotspots {
		state[i] = hotspotEnabled
	}
	// Pinned entry order: load, forced screen update (CARD update), enter.
	for _, event := range []uint16{6, 10, 9} {
		for _, sc := range c.Scripts {
			if sc.Type == event {
				if err = a.entry(sc.Commands, state); err != nil {
					return nil, err
				}
			}
		}
	}
	out := map[int]HotspotState{}
	for i, h := range c.Hotspots {
		entry := activationName(state[i])
		status := entry
		reason := "ordinary hotspots start enabled; stored HSPT flags do not encode an off default"
		if entry == "conditional" {
			reason = "conditional entry scripts can select different hotspot activation states"
		}
		if a.later[i] || (state[i] != hotspotDisabled && a.laterChange[i]) {
			status = "conditional"
			reason = "entry activation can change in later scripts"
		}
		if a.opaque || a.entryOpaque {
			status = "unknown"
			reason = "native, refresh, screen-update or unknown-opcode effects prevent a permanent activation proof"
		}
		if h.Flags&1 != 0 {
			status = "conditional"
			entry = "conditional"
			reason = "Zip activation depends on azip and the visited-card registry"
		}
		if status == "disabled" {
			reason = "disabled on every conservatively modeled entry path; no later enabling script or opaque effect found"
		}
		if a.entryOpaque {
			entry = "unknown"
		}
		out[i] = HotspotState{Status: status, EntryState: entry, Reason: reason, CanEnableLater: a.later[i], Changes: a.changes[i]}
	}
	applyHotspotShadows(c, out)
	return out, nil
}

type availabilityAddress struct {
	stack string
	card  uint16
}

// HotspotAvailabilityForStacks adds a conservative cross-card pass. Commands
// after a literal or reviewed native card change operate on the current card,
// not the script owner. Ambiguous destinations never yield a disabled proof.
func HotspotAvailabilityForStacks(stacks map[string]*Stack) (map[string]map[int]HotspotState, error) {
	out := map[string]map[int]HotspotState{}
	analyses := map[availabilityAddress]*hotspotAnalysis{}
	keys := make([]string, 0, len(stacks))
	for k := range stacks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, sk := range keys {
		s := stacks[sk]
		for _, id := range sortedIDs(s.Cards) {
			c := s.Cards[id]
			states, err := HotspotAvailability(s, c)
			if err != nil {
				return nil, err
			}
			out[SourceID(sk, id)] = states
			a, err := newHotspotAnalysis(s, c)
			if err != nil {
				return nil, err
			}
			analyses[availabilityAddress{sk, id}] = a
		}
	}
	globalAmbiguity := false
	for _, sk := range keys {
		s := stacks[sk]
		for _, id := range sortedIDs(s.Cards) {
			c := s.Cards[id]
			owner := availabilityAddress{sk, id}
			foreignUnknown := func(current []availabilityAddress, cmd Command, reason string) {
				for _, to := range current {
					if to == owner {
						continue // The owner's local pass already accounts for opacity.
					}
					for ordinal, state := range out[SourceID(to.stack, to.card)] {
						state.Status = "unknown"
						state.Reason = fmt.Sprintf("%s in caller %s at +0x%x", reason, SourceID(owner.stack, owner.card), cmd.Offset)
						out[SourceID(to.stack, to.card)][ordinal] = state
					}
				}
			}
			var scan func([]Command, []availabilityAddress, bool, string, uint16, []map[string]any) ([]availabilityAddress, bool, error)
			scan = func(cs []Command, current []availabilityAddress, ambiguous bool, resource string, event uint16, guards []map[string]any) ([]availabilityAddress, bool, error) {
				for _, cmd := range cs {
					if cmd.Opcode == 8 {
						merged := append([]availabilityAddress(nil), current...)
						for _, b := range cmd.Branches {
							next, unknown, err := scan(b.Commands, current, ambiguous, resource, event, hotspotGuards(s, cmd, b, guards))
							if err != nil {
								return nil, false, err
							}
							ambiguous = ambiguous || unknown
							merged = append(merged, next...)
						}
						current = uniqueAvailabilityAddresses(merged)
						continue
					}
					if cmd.Opcode == 2 || cmd.Opcode == 27 {
						next := []availabilityAddress{}
						for _, from := range current {
							destStack, destID := from.stack, uint16(0)
							var err error
							if cmd.Opcode == 2 {
								if len(cmd.Args) != 1 {
									return nil, false, fmt.Errorf("malformed card change in availability pass")
								}
								destID = cmd.Args[0]
							} else {
								target := stacks[from.stack]
								if target == nil {
									ambiguous = true
									continue
								}
								destStack = strings.ToLower(NameAt(target.Names, 5, int(cmd.StackNameID)))
								destID, err = LocalCardID(destStack, cmd.GlobalCardID, stacks)
							}
							if err == nil {
								var resolvedStack string
								var resolvedID uint16
								resolvedStack, resolvedID, _, err = ResolveDestination(destStack, destID, stacks)
								if err == nil {
									destStack, destID = resolvedStack, resolvedID
								}
							}
							if err != nil {
								ambiguous = true
								continue
							}
							next = append(next, availabilityAddress{destStack, destID})
						}
						current = uniqueAvailabilityAddresses(next)
						for _, destination := range current {
							stack := stacks[destination.stack]
							if stack == nil || !sequenceCallbacksStable(stack.Cards[destination.card]) {
								foreignUnknown([]availabilityAddress{destination}, cmd, "entry callbacks can change the current card or activation")
								ambiguous = true
							}
						}
						continue
					}
					if cmd.Opcode == 19 || cmd.Opcode == 20 || cmd.Opcode == 21 || cmd.Opcode == 36 {
						foreignUnknown(current, cmd, "unresolved current-card callback effects")
						ambiguous = true
					}
					if _, known := opNames[cmd.Opcode]; !known {
						foreignUnknown(current, cmd, "unknown opcode effects")
						ambiguous = true
					}
					if cmd.Opcode == 17 {
						// The existing native-navigation table is an exhaustive pinned
						// audit of native card-changing sites; keep conditional no-hop
						// paths as well as every reviewed possible destination.
						next := append([]availabilityAddress(nil), current...)
						for _, from := range current {
							target := stacks[from.stack]
							if target == nil || len(cmd.Args) < 1 {
								ambiguous = true
								continue
							}
							external := NameAt(target.Names, 3, int(cmd.Args[0]))
							for _, tr := range ExternalTransitions(from.stack, external) {
								if tr.Stack == "" {
									ambiguous = true
									continue
								}
								destID := uint16(tr.CardID)
								var err error
								if tr.CardID < 0 {
									destID, err = LocalCardID(tr.Stack, tr.GlobalID, stacks)
								}
								if err != nil {
									ambiguous = true
									continue
								}
								next = append(next, availabilityAddress{tr.Stack, destID})
							}
						}
						current = uniqueAvailabilityAddresses(next)
						foreignUnknown(current, cmd, "opaque native activation effects")
						continue
					}
					control := cmd
					if cmd.Opcode == 38 && (len(cmd.Args) < 5 || cmd.Args[3] != 40) {
						foreignUnknown(current, cmd, "deferred current-card activation effects")
						ambiguous = true
					}
					if cmd.Opcode == 38 && len(cmd.Args) >= 5 {
						control = Command{Opcode: cmd.Args[3], Args: cmd.Args[4:5], Offset: cmd.Offset}
						if control.Opcode == 9 || control.Opcode == 43 {
							ambiguous = true
						}
					}
					if control.Opcode != 9 && control.Opcode != 10 && control.Opcode != 43 {
						continue
					}
					if ambiguous && (control.Opcode == 9 || control.Opcode == 43) {
						globalAmbiguity = true
					}
					for _, to := range current {
						if to == owner {
							continue
						}
						a := analyses[to]
						if a == nil {
							continue
						}
						ordinal, enabled, ok, err := a.target(control)
						if err != nil {
							return nil, false, err
						}
						if !ok {
							continue
						}
						states := out[SourceID(to.stack, to.card)]
						state := states[ordinal]
						state.CanEnableLater = state.CanEnableLater || enabled
						state.Status = "unknown"
						state.Reason = "a caller-owned continuation can change this current-card hotspot"
						state.Changes = append(state.Changes, HotspotChange{Resource: resource, Event: EventName(event), Offset: cmd.Offset, Opcode: control.Opcode, Enabled: enabled, Guards: guards, Caller: SourceID(owner.stack, owner.card)})
						states[ordinal] = state
					}
				}
				return current, ambiguous, nil
			}
			for _, sc := range c.Scripts {
				if _, _, err := scan(sc.Commands, []availabilityAddress{owner}, false, fmt.Sprintf("CARD:%d", id), sc.Type, nil); err != nil {
					return nil, err
				}
			}
			for _, h := range c.Hotspots {
				for _, sc := range h.Scripts {
					if _, _, err := scan(sc.Commands, []availabilityAddress{owner}, false, fmt.Sprintf("HSPT:%d", id), sc.Type, nil); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	if globalAmbiguity {
		for _, states := range out {
			for i, state := range states {
				if state.Status == "disabled" {
					state.Status = "unknown"
					state.Reason = "an enabling continuation has an unresolved current-card destination"
					states[i] = state
				}
			}
		}
	}
	// The caller pass can only have made covers less certain: recompute.
	for _, sk := range keys {
		s := stacks[sk]
		for _, id := range sortedIDs(s.Cards) {
			applyHotspotShadows(s.Cards[id], out[SourceID(sk, id)])
		}
	}
	return out, nil
}

func uniqueAvailabilityAddresses(in []availabilityAddress) []availabilityAddress {
	seen := map[availabilityAddress]bool{}
	out := []availabilityAddress{}
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
