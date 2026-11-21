package riven

import (
	"reflect"
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
)

// auditGame builds a one-stack game from cards, with a node per card and the
// given movement links, so that an audit can run without any archive.
func auditGame(names []string, cards map[uint16]*Card, links ...[2]uint16) *Import {
	stack := &Stack{Name: "jspit", Names: map[uint16][]string{4: names, 3: {"xnative"}}, Cards: cards, Resources: map[string]map[uint16]ResourceRef{}}
	md := &common.Metadata{Game: "riven"}
	byID := map[uint16]*common.Node{}
	for _, id := range sortedIDs(cards) {
		cards[id].ID = id
		node := &common.Node{GraphID: int64(len(md.Nodes)), Name: NodeName("jspit", id), StackName: "Jungle", Attributes: []common.NodeAttribute{common.IsCard}, Details: &common.CardDetails{SourceID: SourceID("jspit", id)}}
		byID[id] = node
		md.Nodes = append(md.Nodes, node)
	}
	for _, link := range links {
		md.Edges = append(md.Edges, &common.Edge{Source: byID[link[0]], Target: byID[link[1]], Kind: "script", Attributes: []common.EdgeAttribute{common.IntraAge}, Evidence: []map[string]any{{"hotspot_name": "forward"}}})
	}
	return &Import{Metadata: md, Stacks: map[string]*Stack{"jspit": stack}}
}

func cmd(op uint16, args ...uint16) Command { return Command{Opcode: op, Args: args} }
func sw(variable uint16, branches ...Branch) Command {
	return Command{Opcode: 8, Variable: variable, Branches: branches, Offset: 10}
}
func load(cs ...Command) []Script { return []Script{{Type: 6, Commands: cs}} }
func pictures(n int) []Picture {
	out := make([]Picture, n)
	for i := range out {
		out[i] = Picture{Index: uint16(i + 1), ImageID: uint16(i + 1)}
	}
	return out
}
func variable(t *testing.T, a *Audit, name string) VariableAudit {
	t.Helper()
	for _, v := range a.Variables {
		if v.Name == name {
			return v
		}
	}
	t.Fatalf("no variable %s", name)
	return VariableAudit{}
}

func TestAuditDeadCaseAndPictureOnlyItSelects(t *testing.T) {
	// "car" starts at 0 and is only ever set to 1: the branch for 2 cannot run.
	card := &Card{Pictures: pictures(3), Scripts: load(
		sw(0, Branch{Value: 0, Commands: []Command{cmd(39, 1)}}),
		sw(0, Branch{Value: 1, Commands: []Command{cmd(39, 2)}}),
		sw(0, Branch{Value: 2, Commands: []Command{cmd(39, 3)}}),
	), Hotspots: []Hotspot{{ID: 8, Name: "lever", Scripts: []Script{{Type: 0, Commands: []Command{cmd(7, 0, 1)}}}}}}
	audit, err := auditGame([]string{"car"}, map[uint16]*Card{1: card}).Audit()
	if err != nil {
		t.Fatal(err)
	}
	v := variable(t, audit, "car")
	if !reflect.DeepEqual(v.DeadCases, []uint16{2}) || v.NeverWritten || v.NeverRead {
		t.Fatalf("variable %+v", v)
	}
	if len(audit.DeadBranches) != 1 || audit.DeadBranches[0].Value != "2" || audit.DeadBranches[0].Reason != "value never stored" {
		t.Fatalf("dead branches %+v", audit.DeadBranches)
	}
	if len(audit.Pictures.Unselected) != 1 || audit.Pictures.Unselected[0].Index != 3 || !audit.Pictures.Unselected[0].DeadBranchOnly {
		t.Fatalf("unselected %+v", audit.Pictures.Unselected)
	}
}

func TestAuditNeverWrittenVariable(t *testing.T) {
	card := &Card{Pictures: pictures(1), Scripts: load(sw(0, Branch{Value: 0}), sw(0, Branch{Value: 1}))}
	audit, _ := auditGame([]string{"doit", "unused"}, map[uint16]*Card{1: card}).Audit()
	v := variable(t, audit, "doit")
	if !v.NeverWritten || !reflect.DeepEqual(v.DeadCases, []uint16{1}) || v.ReadSites != 2 || v.ReadCards != 1 {
		t.Fatalf("variable %+v", v)
	}
	if u := variable(t, audit, "unused"); !u.NeverWritten || !u.NeverRead {
		t.Fatalf("unused %+v", u)
	}
}

func TestAuditClaimsNothingAboutNativeOrIncrementedVariables(t *testing.T) {
	card := &Card{Pictures: pictures(1), Scripts: load(
		sw(0, Branch{Value: 4}), // agehn: the interpreter stores 4 itself
		sw(1, Branch{Value: 7}), // counter: incremented below
	), Hotspots: []Hotspot{{ID: 8, Name: "button", Scripts: []Script{{Type: 0, Commands: []Command{cmd(24, 1, 1)}}}}}}
	audit, _ := auditGame([]string{"agehn", "counter"}, map[uint16]*Card{1: card}).Audit()
	if len(audit.DeadBranches) != 0 {
		t.Fatalf("dead branches %+v", audit.DeadBranches)
	}
	if v := variable(t, audit, "agehn"); !v.Native || v.NeverWritten || len(v.DeadCases) != 0 {
		t.Fatalf("agehn %+v", v)
	}
}

func TestAuditInitialValueIsAPossibleValue(t *testing.T) {
	// gnmagcar starts at 1 and nothing stores 0: the case for 0 is dead, the case for 1 is not.
	card := &Card{Pictures: pictures(2), Scripts: load(
		sw(0, Branch{Value: 0, Commands: []Command{cmd(39, 2)}}),
		sw(0, Branch{Value: 1, Commands: []Command{cmd(39, 1)}}),
	)}
	audit, _ := auditGame([]string{"gnmagcar"}, map[uint16]*Card{1: card}).Audit()
	if v := variable(t, audit, "gnmagcar"); v.Initial != 1 || !reflect.DeepEqual(v.DeadCases, []uint16{0}) {
		t.Fatalf("gnmagcar %+v", v)
	}
}

func TestAuditContradictoryNestedCaseAndDefault(t *testing.T) {
	card := &Card{Pictures: pictures(1), Scripts: load(
		sw(0, Branch{Value: 0, Commands: []Command{sw(0, Branch{Value: 1, Commands: []Command{cmd(39, 1)}})}}),
		// Both possible values have their own case: the default cannot run.
		sw(0, Branch{Value: 0}, Branch{Value: 1}, Branch{Value: 65535}),
	), Hotspots: []Hotspot{{ID: 8, Name: "button", Scripts: []Script{{Type: 0, Commands: []Command{cmd(7, 0, 1)}}}}}}
	audit, _ := auditGame([]string{"door"}, map[uint16]*Card{1: card}).Audit()
	if len(audit.DeadBranches) != 2 {
		t.Fatalf("dead branches %+v", audit.DeadBranches)
	}
	reasons := map[string]string{}
	for _, d := range audit.DeadBranches {
		reasons[d.Value] = d.Reason
	}
	if reasons["1"] != "contradicts an enclosing case" || reasons["default"] != "value never stored" {
		t.Fatalf("reasons %+v", reasons)
	}
}

func TestAuditSelectionAfterAChangeOfCardBelongsToTheNewCard(t *testing.T) {
	// The handler of card 1 enters card 2 and then selects its second picture.
	caller := &Card{Pictures: pictures(2), Hotspots: []Hotspot{{ID: 3, Name: "forward", Scripts: []Script{{Type: 0, Commands: []Command{cmd(2, 2), cmd(39, 2)}}}}}}
	target := &Card{Pictures: pictures(2)}
	audit, _ := auditGame(nil, map[uint16]*Card{1: caller, 2: target}, [2]uint16{1, 2}).Audit()
	if len(audit.Pictures.Unselected) != 1 || audit.Pictures.Unselected[0].Card != "Jungle:1" {
		t.Fatalf("unselected %+v", audit.Pictures.Unselected)
	}
}

func TestAuditLeavesAsideCardsWithNativeCalls(t *testing.T) {
	card := &Card{Pictures: pictures(3), Scripts: load(cmd(17, 0, 0))}
	audit, _ := auditGame(nil, map[uint16]*Card{1: card}).Audit()
	if len(audit.Pictures.Unselected) != 0 || audit.Pictures.NativeEntries != 2 || audit.Pictures.NativeCards != 1 {
		t.Fatalf("pictures %+v", audit.Pictures)
	}
}

func TestAuditTemplateInertAndBlankCards(t *testing.T) {
	var blank, used []Hotspot
	for _, zone := range TemplateHotspots {
		blank = append(blank, Hotspot{ID: zone.ID, Name: zone.Name, Rect: zone.Rect})
	}
	used = append(used,
		Hotspot{ID: 1, Name: "left", Rect: TemplateHotspots[0].Rect, Scripts: []Script{{Type: 0, Commands: []Command{cmd(2, 1)}}}},
		// A cursor change alone leaves the hotspot inert.
		Hotspot{ID: 6, Name: "up", Rect: [4]int{0, 0, 608, 60}, Scripts: []Script{{Type: 4, Commands: []Command{cmd(13, 3004)}}}})
	audit, _ := auditGame(nil, map[uint16]*Card{1: {Hotspots: blank}, 2: {Hotspots: used}}).Audit()
	if len(audit.Hotspots.EmptyRectangle) != 0 {
		t.Fatalf("empty rectangles %+v", audit.Hotspots.EmptyRectangle)
	}
	if !reflect.DeepEqual(audit.Hotspots.BlankCards, []string{"Jungle:1"}) || len(audit.Hotspots.Inert) != 8 {
		t.Fatalf("hotspots %+v", audit.Hotspots)
	}
	left := audit.Hotspots.Template[0]
	if left.Hotspots != 2 || left.TemplateRect != 2 || left.TemplateID != 2 {
		t.Fatalf("left %+v", left)
	}
	if up := audit.Hotspots.Template[5]; up.Hotspots != 2 || up.TemplateRect != 1 {
		t.Fatalf("up %+v", up)
	}
}

func TestAuditEmptyAndInvertedRectangles(t *testing.T) {
	card := &Card{Hotspots: []Hotspot{
		{ID: 1, Name: "ok", Rect: [4]int{0, 0, 10, 10}},
		{ID: 2, Name: "inverted", Rect: [4]int{274, 107, 227, 162}},
		{ID: 3, Name: "flat", Rect: [4]int{5, 5, 50, 5}},
		{ID: 4, Name: "empty"},
	}}
	audit, _ := auditGame(nil, map[uint16]*Card{1: card}).Audit()
	var got []string
	for _, h := range audit.Hotspots.EmptyRectangle {
		got = append(got, h.Name)
	}
	if !reflect.DeepEqual(got, []string{"inverted", "flat", "empty"}) {
		t.Fatalf("empty rectangles %v", got)
	}
}

func TestAuditDominatorsAndZip(t *testing.T) {
	// 1 -> 2 -> {3, 4} -> 5 -> 1, and a Zip shortcut 1 -> 5. Card 2 stands
	// before 3, 4 and 5; neither 3 nor 4 stands before 5.
	cards := map[uint16]*Card{}
	for id := uint16(1); id <= 5; id++ {
		cards[id] = &Card{}
	}
	game := auditGame(nil, cards, [2]uint16{1, 2}, [2]uint16{2, 3}, [2]uint16{2, 4}, [2]uint16{3, 5}, [2]uint16{4, 5}, [2]uint16{5, 1})
	nodes := game.Metadata.Nodes
	game.Metadata.Edges = append(game.Metadata.Edges, &common.Edge{Source: nodes[0], Target: nodes[4], Kind: "zip", Attributes: []common.EdgeAttribute{common.IntraAge, common.Zip}})
	audit, _ := game.Audit()
	s := audit.Structure
	if s.Reachable != 5 || s.FarthestCard != "Jungle:5" || s.FarthestDistance != 3 || !reflect.DeepEqual(s.FarthestChain, []string{"Jungle:1", "Jungle:2", "Jungle:5"}) {
		t.Fatalf("structure %+v", s)
	}
	if len(s.Dominators) != 0 { // below the reporting threshold in a five-card game
		t.Fatalf("dominators %+v", s.Dominators)
	}
	z := s.Zip
	if z.Records != 1 || z.LongestWalk != 3 || z.DiameterWithout != 4 || z.DiameterWith != 4 || z.FarthestWithout != 3 || z.FarthestWith != 2 {
		t.Fatalf("zip %+v", z)
	}
	if s.Links[1].Class != "forward" || s.Links[1].Pairs != 6 || s.Links[1].WithReverse != 0 {
		t.Fatalf("links %+v", s.Links)
	}
}

// The corpus check pins the published figures of the verified original 1.0 discs.
func TestAuditOriginalCorpus(t *testing.T) {
	input, profile := fixtureCorpus(t)
	if profile.edition != "1.0" {
		t.Skip("audit figures are pinned for the 1.0 corpus")
	}
	imported, err := LoadInput(input)
	if err != nil {
		t.Fatal(err)
	}
	audit, err := imported.WriteAudit(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dead := map[string]int{}
	for _, d := range audit.DeadBranches {
		dead[d.Variable+"="+d.Value]++
	}
	if !reflect.DeepEqual(dead, map[string]int{"adoit=1": 7, "bdrwr=2": 1, "gnmagcar=0": 20}) {
		t.Fatalf("dead branches %v", dead)
	}
	deadOnly := 0
	for _, p := range audit.Pictures.Unselected {
		if p.DeadBranchOnly {
			deadOnly++
		}
	}
	if len(audit.Variables) != 163 || audit.Pictures.Records != 4147 || len(audit.Pictures.Unselected) != 19 || deadOnly != 7 || audit.Pictures.NativeEntries != 152 {
		t.Fatalf("variables %d, pictures %+v", len(audit.Variables), audit.Pictures)
	}
	if r := audit.Hotspots.EmptyRectangle; len(r) != 1 || r[0].Card != "Temple:371" || r[0].ID != 9 || r[0].Rect != [4]int{274, 107, 227, 162} {
		t.Fatalf("empty rectangles %+v", r)
	}
	if audit.Hotspots.Total != 8498 || len(audit.Hotspots.Inert) != 66 || !reflect.DeepEqual(audit.Hotspots.BlankCards, []string{"Gehn:89", "Prison:46"}) {
		t.Fatalf("hotspots %d inert %d blank %v", audit.Hotspots.Total, len(audit.Hotspots.Inert), audit.Hotspots.BlankCards)
	}
	s := audit.Structure
	// The ten shadowed references are left out: Boiler:304 is no longer
	// reached, one pair of turns is gone (Garden:167 to 169) and eight Zip
	// shortcuts no longer count.
	if s.Reachable != 2261 || s.Dominators[1] != (Dominator{"Temple:133", 2234}) || s.Dominators[6] != (Dominator{"Temple:113", 2217}) || s.Dominators[7] != (Dominator{"Gehn:1", 125}) || len(s.FarthestChain) != 37 {
		t.Fatalf("structure %+v", s.Dominators[:8])
	}
	if s.Links[0] != (LinkClass{"turn", 3459, 3164, 91.5}) || s.Links[1] != (LinkClass{"forward", 1584, 146, 9.2}) || s.Places != 908 {
		t.Fatalf("links %+v places %d", s.Links, s.Places)
	}
	if z := s.Zip; z.Records != 598 || z.Sources != 555 || z.Destinations != 189 || z.DiameterWithout != 125 || z.DiameterWith != 75 || z.MeanDistanceWithout != 51.52 || z.MeanDistanceWith != 30.99 || z.FarthestWith != 59 {
		t.Fatalf("zip %+v", z)
	}
	if sh := s.Shadowed; sh.Total != 10 || sh.ByKind["script"] != 2 || sh.ByKind["zip"] != 8 || sh.Followed || len(sh.Records) != 10 {
		t.Fatalf("shadowed %+v", sh)
	}
	// Following them gives back the figures of the export that did not know them.
	imported.Metadata.IncludeShadowed = true
	followed, err := imported.Audit()
	if err != nil {
		t.Fatal(err)
	}
	s = followed.Structure
	if s.Reachable != 2262 || s.Dominators[1] != (Dominator{"Temple:133", 2235}) || s.Dominators[6] != (Dominator{"Temple:113", 2218}) || s.Links[0] != (LinkClass{"turn", 3460, 3164, 91.4}) || !s.Shadowed.Followed {
		t.Fatalf("followed structure %+v links %+v", s.Dominators[:8], s.Links)
	}
	if z := s.Zip; z.Records != 606 || z.Sources != 562 || z.DiameterWith != 74 || z.MeanDistanceWithout != 51.51 || z.MeanDistanceWith != 30.85 {
		t.Fatalf("followed zip %+v", z)
	}
}

func TestAuditNativeTimerDoesNotOwnALink(t *testing.T) {
	// Jungle:627 to 629: a forward step, and the sunners' alert video that
	// the enter script starts. The pair stays a step forward.
	cards := map[uint16]*Card{1: {}, 2: {}}
	game := auditGame(nil, cards, [2]uint16{1, 2})
	nodes := game.Metadata.Nodes
	game.Metadata.Edges = append(game.Metadata.Edges, &common.Edge{Source: nodes[0], Target: nodes[1], Kind: "native-timer", Attributes: []common.EdgeAttribute{common.IntraAge, common.Native, common.Conditional},
		Evidence: []map[string]any{{"event": "native_timer"}, {"event": "card_enter", "external": "xjlagoon700_alert"}}})
	audit, err := game.Audit()
	if err != nil {
		t.Fatal(err)
	}
	for _, class := range audit.Structure.Links {
		if want := map[string]int{"forward": 1}[class.Class]; class.Pairs != want {
			t.Errorf("class %q has %d pairs, want %d", class.Class, class.Pairs, want)
		}
	}
}

func TestAuditLeavesShadowedReferencesOutUnlessTheRunFollowsThem(t *testing.T) {
	// 1 -> 2 -> 3 -> 1; card 4 is entered only through a shadowed reference,
	// and the only Zip shortcut is shadowed too.
	run := func(include bool) *Audit {
		cards := map[uint16]*Card{1: {}, 2: {}, 3: {}, 4: {}}
		game := auditGame(nil, cards, [2]uint16{1, 2}, [2]uint16{2, 3}, [2]uint16{3, 1})
		nodes := game.Metadata.Nodes
		game.Metadata.IncludeShadowed = include
		game.Metadata.Edges = append(game.Metadata.Edges,
			&common.Edge{Source: nodes[1], Target: nodes[3], Kind: ShadowedKind, BaseKind: "script", Attributes: []common.EdgeAttribute{common.IntraAge, common.Shadowed}, Evidence: []map[string]any{{"hotspot_name": "egg"}}},
			&common.Edge{Source: nodes[0], Target: nodes[2], Kind: ShadowedKind, BaseKind: "zip", Attributes: []common.EdgeAttribute{common.IntraAge, common.Zip, common.Shadowed}, Evidence: []map[string]any{{"hotspot_name": "place"}}})
		audit, err := game.Audit()
		if err != nil {
			t.Fatal(err)
		}
		return audit
	}
	without, with := run(false), run(true)
	for _, audit := range []*Audit{without, with} {
		sh := audit.Structure.Shadowed
		if sh.Total != 2 || sh.ByKind["script"] != 1 || sh.ByKind["zip"] != 1 || len(sh.Records) != 2 || sh.Records[0].From != "Jungle:2" || sh.Records[0].To != "Jungle:4" || sh.Records[0].Hotspots[0] != "egg" {
			t.Fatalf("shadowed records are not counted on their own: %+v", sh)
		}
	}
	if without.Structure.Shadowed.Followed || !with.Structure.Shadowed.Followed {
		t.Fatal("the audit does not say whether it followed the shadowed references")
	}
	if with.Structure.Reachable != without.Structure.Reachable+1 {
		t.Errorf("reachable: %d without, %d with", without.Structure.Reachable, with.Structure.Reachable)
	}
	if without.Structure.Zip.Records != 0 || with.Structure.Zip.Records != 1 {
		t.Errorf("Zip records: %d without, %d with", without.Structure.Zip.Records, with.Structure.Zip.Records)
	}
	if !strings.Contains(without.Summary(), "Shadowed: 2 records (script: 1, zip: 1); left out of routes, dominators and Zip figures\n") || !strings.Contains(with.Summary(), "Shadowed: 2 records (script: 1, zip: 1); followed (--include-shadowed)\n") {
		t.Errorf("summaries:\n%s\n%s", without.Summary(), with.Summary())
	}
}
