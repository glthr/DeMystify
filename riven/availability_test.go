package riven

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func availabilityFixture() (*Stack, *Card) {
	c := &Card{ID: 1, GlobalID: 100, Hotspots: []Hotspot{{ID: 1, Name: "forward", Rect: [4]int{0, 0, 608, 392}}}}
	s := &Stack{Name: "jspit", Cards: map[uint16]*Card{1: c}, Names: map[uint16][]string{}, Resources: map[string]map[uint16]ResourceRef{}}
	return s, c
}
func availabilityCommand(op uint16, args ...uint16) Command {
	return Command{Opcode: op, Args: args, Offset: 20}
}
func availabilityLoad(c *Card, cs ...Command) { c.Scripts = []Script{{Type: 6, Commands: cs}} }
func availabilityWant(t *testing.T, s *Stack, c *Card, status, entry string) HotspotState {
	t.Helper()
	states, err := HotspotAvailability(s, c)
	if err != nil {
		t.Fatal(err)
	}
	got := states[0]
	if got.Status != status || got.EntryState != entry {
		t.Fatalf("got %+v; want %s / %s", got, status, entry)
	}
	return got
}
func availabilityBLST(t *testing.T, s *Stack, c *Card, records ...[3]uint16) {
	t.Helper()
	b := make([]byte, 2+len(records)*6)
	binary.BigEndian.PutUint16(b, uint16(len(records)))
	for i, r := range records {
		for j, v := range r {
			binary.BigEndian.PutUint16(b[2+i*6+j*2:], v)
		}
	}
	p := filepath.Join(t.TempDir(), "blst")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	r := Resource{ID: c.ID, Size: int64(len(b))}
	a := &Archive{Source: FileSlice{Path: p, Size: int64(len(b))}, Resources: map[string]map[uint16]Resource{"BLST": {c.ID: r}}}
	s.Resources["BLST"] = map[uint16]ResourceRef{c.ID: {Archive: a, Resource: r}}
}

func TestHotspotStoredFlagsDoNotDisableOrdinaryControls(t *testing.T) {
	s, c := availabilityFixture()
	c.Hotspots[0].Flags = 0
	availabilityWant(t, s, c, "enabled", "enabled")
	// No explicit enable command is needed: enabled is the constructor default.
	availabilityLoad(c, availabilityCommand(39, 1))
	availabilityWant(t, s, c, "enabled", "enabled")
}
func TestHotspotZipCannotBecomeGloballyDisabled(t *testing.T) {
	s, c := availabilityFixture()
	c.Hotspots[0].Flags = 1
	availabilityLoad(c, availabilityCommand(10, 1))
	availabilityWant(t, s, c, "conditional", "conditional")
}

func TestHotspotEmptyGeometryIsNotAnActivationFlag(t *testing.T) {
	s, c := availabilityFixture()
	c.Hotspots[0].Rect = [4]int{}
	availabilityWant(t, s, c, "enabled", "enabled")
}

func TestCallerNativeEffectsDoNotProveTargetControlDisabled(t *testing.T) {
	s, owner := availabilityFixture()
	owner.Hotspots[0].Scripts = []Script{{Type: 0, Commands: []Command{availabilityCommand(2, 2), availabilityCommand(17, 0, 0)}}}
	s.Names[3] = []string{"opaque_control"}
	target := &Card{ID: 2, GlobalID: 200, Hotspots: []Hotspot{{ID: 1, Rect: [4]int{0, 0, 608, 392}}}}
	availabilityLoad(target, availabilityCommand(10, 1))
	s.Cards[2] = target
	states, err := HotspotAvailabilityForStacks(map[string]*Stack{"jspit": s})
	if err != nil {
		t.Fatal(err)
	}
	if states["jspit:2"][0].Status != "unknown" {
		t.Fatalf("foreign native call was ignored: %+v", states["jspit:2"][0])
	}
}

func TestCallerEnableAfterEntryRedirectCannotLeaveThirdCardDisabled(t *testing.T) {
	s, owner := availabilityFixture()
	owner.Hotspots[0].Scripts = []Script{{Type: 0, Commands: []Command{availabilityCommand(2, 2), availabilityCommand(9, 1)}}}
	second := &Card{ID: 2, Scripts: []Script{{Type: 9, Commands: []Command{availabilityCommand(2, 3)}}}}
	third := &Card{ID: 3, Hotspots: []Hotspot{{ID: 1, Rect: [4]int{0, 0, 608, 392}}}}
	availabilityLoad(third, availabilityCommand(10, 1))
	s.Cards[2], s.Cards[3] = second, third
	states, err := HotspotAvailabilityForStacks(map[string]*Stack{"jspit": s})
	if err != nil {
		t.Fatal(err)
	}
	if states["jspit:3"][0].Status == "disabled" {
		t.Fatal("entry callback hid a possible caller-owned enable of the third card")
	}
}

func TestForeignDeferredNativeActivationRemainsUnknown(t *testing.T) {
	s, owner := availabilityFixture()
	owner.Hotspots[0].Scripts = []Script{{Type: 0, Commands: []Command{availabilityCommand(2, 2), availabilityCommand(38, 1, 0, 100, 17, 0)}}}
	target := &Card{ID: 2, Hotspots: []Hotspot{{ID: 1, Rect: [4]int{0, 0, 608, 392}}}}
	availabilityLoad(target, availabilityCommand(10, 1))
	s.Cards[2] = target
	states, err := HotspotAvailabilityForStacks(map[string]*Stack{"jspit": s})
	if err != nil {
		t.Fatal(err)
	}
	if states["jspit:2"][0].Status != "unknown" {
		t.Fatal("foreign deferred native effect was ignored")
	}
}

func TestCallerDisableDoesNotLeaveTargetClassifiedAlwaysEnabled(t *testing.T) {
	s, owner := availabilityFixture()
	owner.Hotspots[0].Scripts = []Script{{Type: 0, Commands: []Command{availabilityCommand(2, 2), availabilityCommand(10, 1)}}}
	target := &Card{ID: 2, Hotspots: []Hotspot{{ID: 1, Rect: [4]int{0, 0, 608, 392}}}}
	s.Cards[2] = target
	states, err := HotspotAvailabilityForStacks(map[string]*Stack{"jspit": s})
	if err != nil {
		t.Fatal(err)
	}
	if states["jspit:2"][0].Status != "unknown" || states["jspit:2"][0].CanEnableLater {
		t.Fatalf("foreign disable was lost: %+v", states["jspit:2"][0])
	}
}
func TestHotspotBLSTUsesFirstMatchingRecordAndHotspot(t *testing.T) {
	s, c := availabilityFixture()
	c.Hotspots = append(c.Hotspots, Hotspot{ID: 1, Rect: [4]int{0, 0, 20, 20}})
	availabilityBLST(t, s, c, [3]uint16{4, 0, 1}, [3]uint16{4, 1, 1})
	availabilityLoad(c, availabilityCommand(43, 4))
	availabilityWant(t, s, c, "disabled", "disabled")
	states, err := HotspotAvailability(s, c)
	if err != nil {
		t.Fatal(err)
	}
	if states[1].Status != "enabled" {
		t.Fatal("duplicate hotspot ID did not retain first-match semantics")
	}
	// BLST's enabled word is a comparison to 1, not a nonzero Boolean.
	availabilityBLST(t, s, c, [3]uint16{4, 2, 1})
	availabilityWant(t, s, c, "disabled", "disabled")
}
func TestHotspotConditionalDisableIncludesNoMatchPath(t *testing.T) {
	s, c := availabilityFixture()
	availabilityLoad(c, Command{Opcode: 8, Variable: 0, Branches: []Branch{{Value: 0, Commands: []Command{availabilityCommand(10, 1)}}}})
	got := availabilityWant(t, s, c, "conditional", "conditional")
	if len(got.Changes) != 1 || len(got.Changes[0].Guards) != 1 {
		t.Fatal("lost conditional write evidence")
	}
	// With a disabling default as well, every modeled entry path disables it.
	c.Scripts[0].Commands[0].Branches = append(c.Scripts[0].Commands[0].Branches, Branch{Value: 65535, Commands: []Command{availabilityCommand(10, 1)}})
	availabilityWant(t, s, c, "disabled", "disabled")
}
func TestHotspotLaterEnablePreventsPermanentDisable(t *testing.T) {
	s, c := availabilityFixture()
	availabilityLoad(c, availabilityCommand(10, 1))
	c.Hotspots[0].Scripts = []Script{{Type: 0, Commands: []Command{availabilityCommand(9, 1)}}}
	got := availabilityWant(t, s, c, "conditional", "disabled")
	if !got.CanEnableLater {
		t.Fatal("later enable not retained")
	}
}
func TestHotspotNativeAndDelayedBarriersStayUnknown(t *testing.T) {
	for _, cmd := range []Command{availabilityCommand(17, 0, 0), availabilityCommand(36), availabilityCommand(65000), availabilityCommand(38, 1, 0, 100, 9, 1)} {
		s, c := availabilityFixture()
		availabilityLoad(c, availabilityCommand(10, 1))
		c.Hotspots[0].Scripts = []Script{{Type: 0, Commands: []Command{cmd}}}
		availabilityWant(t, s, c, "unknown", "disabled")
	}
}
func TestHotspotCurrentCardContinuationDoesNotMutateScriptOwner(t *testing.T) {
	s, owner := availabilityFixture()
	availabilityLoad(owner, availabilityCommand(10, 1))
	owner.Hotspots = append(owner.Hotspots, Hotspot{ID: 2, Rect: [4]int{0, 0, 20, 20}, Scripts: []Script{{Type: 0, Commands: []Command{availabilityCommand(2, 2), availabilityCommand(9, 1)}}}})
	target := &Card{ID: 2, GlobalID: 200, Hotspots: []Hotspot{{ID: 1, Rect: [4]int{0, 0, 608, 392}}}}
	availabilityLoad(target, availabilityCommand(10, 1))
	s.Cards[2] = target
	states, err := HotspotAvailabilityForStacks(map[string]*Stack{"jspit": s})
	if err != nil {
		t.Fatal(err)
	}
	if states["jspit:1"][0].CanEnableLater {
		t.Fatal("post-change enable was attributed to owner")
	}
	got := states["jspit:2"][0]
	if got.Status != "unknown" || !got.CanEnableLater {
		t.Fatalf("caller-owned target enable not retained: %+v", got)
	}
	if len(got.Changes) != 2 || got.Changes[1].Caller != "jspit:1" {
		t.Fatalf("lost caller provenance: %+v", got.Changes)
	}
}
func TestHotspotUpdateRunsBeforeEnter(t *testing.T) {
	s, c := availabilityFixture()
	availabilityLoad(c, availabilityCommand(9, 1))
	c.Scripts = append(c.Scripts, Script{Type: 10, Commands: []Command{availabilityCommand(9, 1)}}, Script{Type: 9, Commands: []Command{availabilityCommand(10, 1)}})
	got := availabilityWant(t, s, c, "conditional", "disabled")
	if !got.CanEnableLater {
		t.Fatal("repeatable update enable was ignored")
	}
}
func TestHotspotMalformedActivationCommandsFail(t *testing.T) {
	s, c := availabilityFixture()
	availabilityLoad(c, availabilityCommand(43, 1, 2))
	if _, err := HotspotAvailability(s, c); err == nil {
		t.Fatal("accepted malformed BLST activation")
	}
}
