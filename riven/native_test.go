package riven

import (
	"reflect"
	"strings"
	"testing"
)

func redirectFixture() map[string]*Stack {
	stacks := map[string]*Stack{}
	for _, redirect := range SpecialRedirects {
		for _, identity := range []struct {
			stack  string
			global uint32
		}{{redirect.SourceStack, redirect.SourceRMap}, {redirect.TargetStack, redirect.TargetRMap}} {
			if stacks[identity.stack] == nil {
				stacks[identity.stack] = &Stack{Name: identity.stack}
			}
			found := false
			for _, value := range stacks[identity.stack].RMap {
				found = found || value == identity.global
			}
			if !found {
				stacks[identity.stack].RMap = append(stacks[identity.stack].RMap, identity.global)
			}
		}
	}
	return stacks
}

func TestNativeRMapLastMatchAndZeroIndex(t *testing.T) {
	stacks := map[string]*Stack{"test": {RMap: []uint32{0, 0xffffffff, 0x12345, 0xffffffff}}}
	for _, tt := range []struct {
		global uint32
		want   uint16
	}{{0, 0}, {0xffffffff, 3}, {0x12345, 2}} {
		got, err := LocalCardID("test", tt.global, stacks)
		if err != nil || got != tt.want {
			t.Fatalf("RMAP 0x%x: got %d, %v; want %d", tt.global, got, err, tt.want)
		}
	}
	if _, err := LocalCardID("missing", 0, stacks); err == nil {
		t.Fatal("missing stack accepted")
	}
	if _, err := LocalCardID("test", 7, stacks); err == nil {
		t.Fatal("missing RMAP accepted")
	}
}

func TestAllNativeRedirectsPreserveRequestedTargetEvidence(t *testing.T) {
	if len(SpecialRedirects) != 13 {
		t.Fatal("expected 13 native redirects")
	}
	stacks := redirectFixture()
	for _, redirect := range SpecialRedirects {
		t.Run(redirect.SourceStack+"-"+redirect.Reason, func(t *testing.T) {
			literal, err := LocalCardID(redirect.SourceStack, redirect.SourceRMap, stacks)
			if err != nil {
				t.Fatal(err)
			}
			want, err := LocalCardID(redirect.TargetStack, redirect.TargetRMap, stacks)
			if err != nil {
				t.Fatal(err)
			}
			stack, card, chain, err := ResolveDestination(strings.ToUpper(redirect.SourceStack), literal, stacks)
			if err != nil || stack != redirect.TargetStack || card != want || len(chain) != 1 {
				t.Fatalf("got %s:%d chain=%v err=%v; want %s:%d", stack, card, chain, err, redirect.TargetStack, want)
			}
			ev := chain[0]
			if ev["source_stack"] != redirect.SourceStack || ev["source_card_id"] != literal || ev["source_rmap"] != redirect.SourceRMap || ev["target_rmap"] != redirect.TargetRMap {
				t.Fatalf("literal or resolved identity lost: %#v", ev)
			}
			if ev["source"] != SourceBase+"riven.cpp#L588-L630" {
				t.Fatal("unpinned redirect evidence")
			}
		})
	}
}

func TestUnrelatedEditionAndMissingMatchedDestination(t *testing.T) {
	stacks := map[string]*Stack{"tspit": {RMap: []uint32{9}}}
	stack, card, chain, err := ResolveDestination("TSPIT", 7, stacks)
	if err != nil || stack != "tspit" || card != 7 || len(chain) != 0 || chain == nil {
		t.Fatalf("unrelated source changed: %s %d %v %v", stack, card, chain, err)
	}
	stacks["tspit"].RMap = []uint32{0x21b69}
	if _, _, _, err := ResolveDestination("tspit", 0, stacks); err == nil {
		t.Fatal("matched redirect with missing destination accepted")
	}
}

func TestNativeExternalsKeepReturnsAndResetsDistinct(t *testing.T) {
	for _, name := range []string{"xaatrusbookback", "xacathbookback", "xtrapbookback", "xaResumeGame"} {
		transitions := ExternalTransitions("ASpit", name)
		if len(transitions) != 1 || transitions[0].Stack != "" || transitions[0].Kind != "dynamic-return" || transitions[0].CardID != -1 {
			t.Fatalf("return fabricated a destination: %+v", transitions)
		}
	}
	newGame := ExternalTransitions("aspit", "xaNewGame")
	if len(newGame) != 2 || newGame[0].Stack != "aspit" || newGame[0].CardID != 2 || newGame[1].GlobalID != 0x6e9a || newGame[0].Sequence == nil || *newGame[0].Sequence != 0 || *newGame[1].Sequence != 1 {
		t.Fatalf("new-game sequence lost: %+v", newGame)
	}
	book := ExternalTransitions("ospit", "xbookclick")
	if len(book) != 2 || book[0].GlobalID != 0x2885 || book[0].Kind != "native" || book[1].Kind != "ending" || book[1].CardID != 1 {
		t.Fatalf("book click branches: %+v", book)
	}
	if got := ExternalTransitions("aspit", "xhandlecontrolup"); len(got) != 0 {
		t.Fatalf("external accepted in wrong stack: %+v", got)
	}
	if got := ExternalTransitions("jspit", "not_a_command"); len(got) != 0 {
		t.Fatal("unknown external invented navigation")
	}
}

func TestNativeCarriageAndHandleBranches(t *testing.T) {
	carriage := ExternalTransitions("jspit", "xvga1300_carriage")
	want := []uint32{0x18d4d, 0x18ab5, 0x17167}
	if len(carriage) != len(want) {
		t.Fatalf("carriage transitions: %+v", carriage)
	}
	for i, transition := range carriage {
		if transition.GlobalID != want[i] || transition.CardID != -1 || transition.Sequence == nil || *transition.Sequence != i {
			t.Fatalf("carriage branch %d: %+v", i, transition)
		}
	}
	up, down, mid := ExternalTransitions("jspit", "xhandlecontrolup"), ExternalTransitions("jspit", "xhandlecontroldown"), ExternalTransitions("jspit", "xhandlecontrolmid")
	if up[0].Condition != "handle dragged down" || down[0].Condition != "handle dragged up" || mid[0].GlobalID != 0x1e597 || mid[1].GlobalID != 0x1e29c {
		t.Fatal("handle direction or branch changed")
	}
	for _, pair := range [][2]string{{"aspit", "xasetupcomplete"}, {"aspit", "xanewgame"}, {"jspit", "xhandlecontrolmid"}, {"ospit", "xorollcredittime"}, {"rspit", "xrcredittime"}, {"tspit", "xtexterior300_telescopedown"}} {
		for _, transition := range ExternalTransitions(pair[0], pair[1]) {
			if !strings.HasPrefix(transition.Source, SourceBase) {
				t.Fatalf("native source not pinned: %+v", transition)
			}
		}
	}
}

func TestSunnerTimersAndInventoryPolicy(t *testing.T) {
	for source, target := range map[uint32]uint32{0x77d6: 0x79bd, 0x79bd: 0x7beb, 0x7beb: 0xb6ca} {
		got := TimerTransitions("JSPIT", source)
		if len(got) != 1 || got[0].GlobalID != target || got[0].Kind != "native-timer" || got[0].Stack != "jspit" || !strings.Contains(got[0].Condition, "jsunners == 0") {
			t.Fatalf("timer %#x: %+v", source, got)
		}
	}
	if len(TimerTransitions("tspit", 0x77d6)) != 0 || len(TimerTransitions("jspit", 1)) != 0 {
		t.Fatal("timer applied outside its card")
	}
	items := InventoryModel()["items"].([]map[string]any)
	got := []int{}
	for _, item := range items {
		got = append(got, item["target_card_id"].(int))
	}
	if !reflect.DeepEqual(got, []int{5, 6, 7}) {
		t.Fatalf("inventory destinations: %v", got)
	}
}

func TestLagoonAlertsNavigateLikeTheTimersOfTheirCards(t *testing.T) {
	// xjlagoon700_alert runs on entering RMAP 0x79bd and xjlagoon800_alert on
	// entering 0x7beb: each must name the destination of that card's timer.
	for name, card := range map[string]uint32{"xjlagoon700_alert": 0x79bd, "xjLagoon800_Alert": 0x7beb} {
		alert, timer := ExternalTransitions("jspit", name), TimerTransitions("jspit", card)
		if len(alert) != 1 || len(timer) != 1 {
			t.Fatalf("%s: %d alert and %d timer transitions", name, len(alert), len(timer))
		}
		if alert[0].Stack != timer[0].Stack || alert[0].GlobalID != timer[0].GlobalID || alert[0].Kind != timer[0].Kind || alert[0].Condition == "" || alert[0].Source == timer[0].Source {
			t.Errorf("%s: %+v does not match the timer %+v", name, alert[0], timer[0])
		}
	}
	if got := ExternalTransitions("jspit", "xjlagoon1500_alert"); got != nil {
		t.Errorf("the beach alert does not navigate: %+v", got)
	}
}
