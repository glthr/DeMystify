package riven

import (
	"fmt"
	"os"
	"reflect"
	"testing"
)

func sequenceFixture() (*Stack, *Card, map[string]*Stack) {
	stack := &Stack{Name: "tspit", Cards: map[uint16]*Card{}, Names: map[uint16][]string{4: {"door"}}}
	for id := uint16(1); id <= 7; id++ {
		stack.Cards[id] = &Card{ID: id}
	}
	return stack, stack.Cards[1], map[string]*Stack{"tspit": stack}
}

func sequenceMove(id uint16, offset int) Command {
	return Command{Opcode: 2, Args: []uint16{id}, Offset: offset, End: offset + 6}
}

func sequenceHotspot(card *Card, commands ...Command) {
	card.Hotspots = []Hotspot{{ID: 3, Name: "forward", Rect: [4]int{10, 20, 300, 390}, Scripts: []Script{{Type: 0, Commands: commands}}}}
}

func TestSequenceKeepsCallerAndBothInstructionOffsets(t *testing.T) {
	s, c, stacks := sequenceFixture()
	sequenceHotspot(c, sequenceMove(2, 0x4c), Command{Opcode: 20, Offset: 0x52}, Command{Opcode: 18, Offset: 0x56}, sequenceMove(3, 0x6a))
	before := append([]Command(nil), c.Hotspots[0].Scripts[0].Commands...)
	refs, err := sequenceReferences(s, c, stacks)
	if err != nil || len(refs) != 1 {
		t.Fatalf("references=%+v, error=%v", refs, err)
	}
	r := refs[0]
	if r.sourceID != 2 || r.targetID != 3 || r.evidence["sequence_owner"] != "tspit:1" || r.evidence["resource"] != "HSPT:1" || r.evidence["previous_byte_offset"] != 0x4c || r.evidence["byte_offset"] != 0x6a || r.evidence["event"] != "mouse_down" || r.evidence["hotspot_blst_id"] != uint16(3) {
		t.Fatalf("lost caller/instruction context: %+v", r)
	}
	if !reflect.DeepEqual(r.evidence["between_opcodes"], []uint16{20, 18}) || !reflect.DeepEqual(before, c.Hotspots[0].Scripts[0].Commands) {
		t.Fatal("changed the original requests or lost separating commands")
	}
}

func TestSequenceDoesNotJoinMutuallyExclusiveBranchesOrJoins(t *testing.T) {
	s, c, stacks := sequenceFixture()
	sequenceHotspot(c, sequenceMove(2, 10), Command{Opcode: 8, Variable: 0, Offset: 20, Branches: []Branch{
		{Value: 0, Commands: []Command{sequenceMove(3, 30), sequenceMove(4, 36)}},
		{Value: 65535, Commands: []Command{sequenceMove(5, 42), sequenceMove(6, 48)}},
	}}, sequenceMove(7, 54))
	refs, err := sequenceReferences(s, c, stacks)
	if err != nil || len(refs) != 2 {
		t.Fatalf("references=%+v, error=%v", refs, err)
	}
	for i, want := range [][2]uint16{{3, 4}, {5, 6}} {
		if refs[i].sourceID != want[0] || refs[i].targetID != want[1] {
			t.Fatalf("invented a switch boundary/join: %+v", refs)
		}
		guards := refs[i].evidence["guards"].([]map[string]any)
		if len(guards) != 1 || guards[0]["variable_name"] != "door" || guards[0]["switch_offset"] != 20 {
			t.Fatalf("lost branch guard: %+v", guards)
		}
	}
	defaultGuard := refs[1].evidence["guards"].([]map[string]any)[0]
	if !reflect.DeepEqual(defaultGuard["excluded_values"], []uint16{0}) {
		t.Fatal("default branch lost its excluded value")
	}
}

func TestSequenceDoesNotJoinSeparateEventsOrHotspots(t *testing.T) {
	s, c, stacks := sequenceFixture()
	c.Hotspots = []Hotspot{
		{ID: 1, Scripts: []Script{{Type: 0, Commands: []Command{sequenceMove(2, 10)}}, {Type: 2, Commands: []Command{sequenceMove(3, 20)}}}},
		{ID: 2, Scripts: []Script{{Type: 0, Commands: []Command{sequenceMove(4, 30)}}}},
	}
	refs, err := sequenceReferences(s, c, stacks)
	if err != nil || len(refs) != 0 {
		t.Fatalf("joined independent handler contexts: %+v, %v", refs, err)
	}
}

func TestSequenceOpaqueOperationsInvalidateTheCurrentStack(t *testing.T) {
	for _, opcode := range []uint16{17, 19, 27, 36, 38, 45, 500} {
		t.Run(fmt.Sprint(opcode), func(t *testing.T) {
			s, c, stacks := sequenceFixture()
			sequenceHotspot(c, sequenceMove(2, 10), Command{Opcode: opcode, Offset: 20}, sequenceMove(3, 30), sequenceMove(4, 40))
			refs, err := sequenceReferences(s, c, stacks)
			if err != nil || len(refs) != 0 {
				t.Fatalf("continued through opaque opcode %d: %+v, %v", opcode, refs, err)
			}
		})
	}
}

func TestSequencePrimitiveStateChangeEndsOnlyTheCurrentRun(t *testing.T) {
	s, c, stacks := sequenceFixture()
	sequenceHotspot(c, sequenceMove(2, 10), Command{Opcode: 7, Args: []uint16{0, 1}, Offset: 20}, sequenceMove(3, 30), Command{Opcode: 18}, sequenceMove(4, 40))
	refs, err := sequenceReferences(s, c, stacks)
	if err != nil || len(refs) != 1 || refs[0].sourceID != 3 || refs[0].targetID != 4 {
		t.Fatalf("incorrect primitive state barrier: %+v, %v", refs, err)
	}
}

func TestSequenceRejectsOpaqueEntryUpdateAndLeaveCallbacks(t *testing.T) {
	for _, event := range []uint16{6, 7, 9, 10} {
		for _, opcode := range []uint16{2, 17, 19, 27, 38, 45} {
			s, c, stacks := sequenceFixture()
			stacks["tspit"].Cards[2].Scripts = []Script{{Type: event, Commands: []Command{{Opcode: opcode, Args: []uint16{4}, Offset: 10}}}}
			sequenceHotspot(c, sequenceMove(2, 10), sequenceMove(3, 20))
			refs, err := sequenceReferences(s, c, stacks)
			if err != nil || len(refs) != 0 {
				t.Fatalf("callback event %d/op%d bridged a sequence: %+v, %v", event, opcode, refs, err)
			}
		}
	}
}

func TestSequenceRejectsOpaqueOwnerLeaveAndBranchHistory(t *testing.T) {
	s, c, stacks := sequenceFixture()
	c.Scripts = []Script{{Type: 7, Commands: []Command{{Opcode: 17, Args: []uint16{0, 0}}}}}
	sequenceHotspot(c, sequenceMove(2, 10), sequenceMove(3, 20))
	if refs, err := sequenceReferences(s, c, stacks); err != nil || len(refs) != 0 {
		t.Fatalf("opaque owner leave callback was ignored: %+v, %v", refs, err)
	}
	c.Scripts = nil
	sequenceHotspot(c, Command{Opcode: 8, Branches: []Branch{
		{Value: 0, Commands: []Command{{Opcode: 17, Args: []uint16{0, 0}}}},
		{Value: 1},
	}}, sequenceMove(2, 10), sequenceMove(3, 20))
	if refs, err := sequenceReferences(s, c, stacks); err != nil || len(refs) != 0 {
		t.Fatalf("unknown stack after an opaque branch was assumed restored: %+v, %v", refs, err)
	}
}

func TestSequenceRejectsMissingSourcesOwnerReentryAndRepeatedViews(t *testing.T) {
	for _, targets := range [][]uint16{{99, 2}, {1, 2}, {2, 2}} {
		s, c, stacks := sequenceFixture()
		sequenceHotspot(c, sequenceMove(targets[0], 10), sequenceMove(targets[1], 20))
		refs, err := sequenceReferences(s, c, stacks)
		if err != nil || len(refs) != 0 {
			t.Fatalf("invalid source sequence %v: %+v, %v", targets, refs, err)
		}
	}
}

func TestSequenceRejectsDestinationTimeCrossStackRedirect(t *testing.T) {
	s, c, stacks := sequenceFixture()
	s.RMap = make([]uint32, 3)
	s.RMap[2] = 0x21b69 // Temple dome placeholder redirects before entry.
	stacks["ospit"] = &Stack{Name: "ospit", RMap: []uint32{0x2e76}, Cards: map[uint16]*Card{0: {ID: 0}}}
	sequenceHotspot(c, sequenceMove(2, 10), sequenceMove(3, 20), sequenceMove(4, 30))
	refs, err := sequenceReferences(s, c, stacks)
	if err != nil || len(refs) != 0 {
		t.Fatalf("continued using owner-stack IDs after redirect: %+v, %v", refs, err)
	}
}

func TestSequenceRejectsMalformedCardChanges(t *testing.T) {
	s, c, stacks := sequenceFixture()
	sequenceHotspot(c, Command{Opcode: 2, Offset: 10})
	if _, err := sequenceReferences(s, c, stacks); err == nil {
		t.Fatal("accepted malformed sequence request")
	}
}

func TestSequenceStoredMovieOpcodeIsJudgedByTheCommandItStores(t *testing.T) {
	stored := func(command uint16, extra ...uint16) Command {
		return Command{Opcode: 38, Args: append([]uint16{1, 0, 0, command}, extra...), Offset: 20}
	}
	for _, test := range []struct {
		name    string
		command Command
		joined  bool
	}{
		{"activates a sound list", stored(40, 2), true},
		{"activates a picture list", stored(39, 2), true},
		{"changes card", stored(2, 5), false},
		{"changes stack", stored(27, 5), false},
		{"runs an external command", stored(17, 0), false},
		{"refreshes the card", stored(19, 0), false},
		{"has too few arguments to tell", Command{Opcode: 38, Args: []uint16{1, 0, 0, 40}, Offset: 20}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, c, stacks := sequenceFixture()
			sequenceHotspot(c, sequenceMove(2, 10), test.command, sequenceMove(3, 30))
			refs, err := sequenceReferences(s, c, stacks)
			if err != nil || (len(refs) == 1) != test.joined {
				t.Fatalf("stored movie opcode %+v: got %+v, %v; want joined=%v", test.command, refs, err, test.joined)
			}
		})
	}
}

func TestSequenceCallbacksAcceptOnlyHarmlessStoredMovieOpcodes(t *testing.T) {
	harmless := &Card{ID: 2, Scripts: []Script{{Type: 9, Commands: []Command{{Opcode: 38, Args: []uint16{1, 0, 0, 40, 2}}}}}}
	if !sequenceCallbacksStable(harmless) {
		t.Fatal("a stored sound-list activation made the callbacks unstable")
	}
	risky := &Card{ID: 2, Scripts: []Script{{Type: 9, Commands: []Command{{Opcode: 38, Args: []uint16{1, 0, 0, 2, 5}}}}}}
	if sequenceCallbacksStable(risky) {
		t.Fatal("a stored card change left the callbacks stable")
	}
}

func TestSequenceOriginalDiscCorpus(t *testing.T) {
	input := os.Getenv("RIVEN_TEST_DATA")
	if input == "" {
		t.Skip("set RIVEN_TEST_DATA to an original English five-CD corpus")
	}
	imported, err := LoadInput(input)
	if err != nil {
		t.Fatal(err)
	}
	defer imported.Close()
	count := 0
	seen := map[string]bool{}
	for name, stack := range imported.Stacks {
		for _, card := range stack.Cards {
			refs, err := sequenceReferences(stack, card, imported.Stacks)
			if err != nil {
				t.Fatal(err)
			}
			count += len(refs)
			for _, ref := range refs {
				key := fmt.Sprintf("%s %d>%d", ref.evidence["sequence_owner"], ref.sourceID, ref.targetID)
				seen[key] = true
				if name != stack.Name {
					t.Fatal("sequence changed stack identity")
				}
			}
		}
	}
	if count != 22 {
		t.Fatalf("got %d ordered sequence callsites, want 22", count)
	}
	for _, key := range []string{"gspit:9 11>12", "gspit:17 11>13", "jspit:360 361>362", "tspit:10 20>6", "tspit:10 20>7", "bspit:455 465>427", "jspit:151 150>148"} {
		if !seen[key] {
			t.Errorf("lost verified caller-owned continuation %s", key)
		}
	}
	t.Logf("verified %d caller-owned display sequences", count)
}
