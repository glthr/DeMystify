package parser

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/hypercard"
)

func sequenceCorpus(scripts ...string) hypercard.Corpus {
	stack := hypercard.Stack{Name: "Example", BackgroundsComplete: true}
	for id := 1; id <= 4; id++ {
		stack.Cards = append(stack.Cards, hypercard.Card{ID: id})
	}
	stack.Cards[1].Name = "Transit"
	stack.Cards[2].Name = "Destination"
	for index, text := range scripts {
		stack.Cards[0].Parts = append(stack.Cards[0].Parts, hypercard.Part{ID: index + 1, Script: hypercard.Script{Text: text}})
	}
	return hypercard.Corpus{Stacks: []hypercard.Stack{stack}}
}

func sequenceMetadata(t *testing.T, corpus hypercard.Corpus) *common.Metadata {
	t.Helper()
	p, err := NewParserFromCorpus(corpus)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := p.Process()
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}

func sequenceEdges(metadata *common.Metadata) []*common.Edge {
	var edges []*common.Edge
	for _, edge := range metadata.Edges {
		if edge.IsOfType(common.SequenceContinuation) {
			edges = append(edges, edge)
		}
	}
	return edges
}

func TestLiteralSequenceKeepsOwnerReferencesBesideOrderedContinuation(t *testing.T) {
	// Like a doorway preview followed by the actual interior, both literal go
	// commands belong to the original button handler: its card refers to both
	// views, and the preview continues to the interior.
	corpus := sequenceCorpus("on mouseUp\ngo to card id 2\nvs left\ngo to card id 3\nhc\nsoundidle\nopenElevator\nend mouseUp")
	corpus.Stacks[0].Script.Text = "on openElevator\nput \"Elevator Window.MooV\" into movieName\nMovie movieName\nend openElevator"
	before, err := json.Marshal(corpus)
	if err != nil {
		t.Fatal(err)
	}
	metadata := sequenceMetadata(t, corpus)
	if len(metadata.Edges) != 3 {
		t.Fatalf("got %d edges, want the two owner references plus one continuation", len(metadata.Edges))
	}
	entry, direct, continuation := metadata.Edges[0], metadata.Edges[1], metadata.Edges[2]
	if entry.Source.Name != "Example:1" || entry.Target.Name != "Example:2" || entry.IsOfType(common.SequenceContinuation) {
		t.Fatalf("incorrect entry: %+v", entry)
	}
	if direct.Source.Name != "Example:1" || direct.Target.Name != "Example:3" || direct.IsOfType(common.SequenceContinuation) {
		t.Fatalf("the owner lost its own reference to the second view: %+v", direct)
	}
	if continuation.Source.Name != "Example:2" || continuation.Target.Name != "Example:3" || !continuation.IsOfType(common.SequenceContinuation) || continuation.IsOfType(common.Backtracking) {
		t.Fatalf("incorrect static continuation: %+v", continuation)
	}
	for _, edge := range metadata.Edges {
		if edge.Target.Name == "Example:1" {
			t.Fatalf("invented a return to the calling card: %+v", edge)
		}
	}
	after, err := json.Marshal(corpus)
	if err != nil || string(after) != string(before) {
		t.Fatalf("analysis changed original script evidence: %v", err)
	}
}

func TestLiteralSequenceIgnoresRequestsThatAreNotSteps(t *testing.T) {
	for name, tc := range map[string]struct {
		script string
		want   [][2]string
	}{
		// The owner's own references already record these requests.
		"owner first":   {"go card id 1\nvs left\ngo card id 2", nil},
		"same view":     {"go card id 2\ngo card id 2", nil},
		"owner between": {"go card id 2\ngo card id 1\ngo card id 3", [][2]string{{"Example:2", "Example:1"}}},
		"later step":    {"go card id 1\ngo card id 2\ngo card id 3", [][2]string{{"Example:2", "Example:3"}}},
	} {
		t.Run(name, func(t *testing.T) {
			var got [][2]string
			for _, edge := range sequenceEdges(sequenceMetadata(t, sequenceCorpus("on mouseUp\n"+tc.script+"\nend mouseUp"))) {
				got = append(got, [2]string{edge.Source.Name, edge.Target.Name})
			}
			if len(got) != len(tc.want) || len(got) == 1 && got[0] != tc.want[0] {
				t.Fatalf("got continuations %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLiteralSequenceSupportsResolvedNamesCommentsAndVisualEffects(t *testing.T) {
	for name, script := range map[string]string{
		"three steps":   "go card id 2\ngo card id 3\ngo card id 4",
		"quoted names":  "go card \"Transit\"\n-- ordinary explanation\n\nvisual effect dissolve fast\ngo card \"Destination\"",
		"static names":  "go card Transit\nvs right\ngo card Destination",
		"trailing note": "go card id 2 -- entered here\nvs left -- visual-only convention\ngo to card id 3",
		// Both spellings pass the same literal direction to the same command.
		"quoted direction": "go card id 2\nvs \"left\"\ngo to card id 3",
		"quoted and cased": "go card id 2\nVS \"Right\" -- visual-only convention\ngo to card id 3",
	} {
		t.Run(name, func(t *testing.T) {
			metadata := sequenceMetadata(t, sequenceCorpus("on mouseUp\n"+script+"\nend mouseUp"))
			edges := sequenceEdges(metadata)
			want := 1
			if name == "three steps" {
				want = 2
			}
			if len(edges) != want || edges[0].Source.Name != "Example:2" || edges[0].Target.Name != "Example:3" {
				t.Fatalf("unexpected continuations: %+v", edges)
			}
		})
	}
}

func TestLiteralSequenceRejectsControlFlowAndUnresolvedContexts(t *testing.T) {
	for name, body := range map[string]string{
		"branches":     "if ready then\ngo card id 2\nelse\ngo card id 3\nend if",
		"inline":       "if ready then go card id 2\ngo card id 3",
		"loop":         "repeat with x = 1 to 2\ngo card id 2\ngo card id 3\nend repeat",
		"join":         "go card id 2\nif ready then\nput 1 into state\nend if\ngo card id 3",
		"dynamic go":   "global destination\ngo card id 2\ngo card destination\ngo card id 3",
		"computed go":  "go card id 2\ngo card id offset + 2\ngo card id 3",
		"current card": "go card id 2\ngo this card\ngo card id 3",
		"remote stack": "go card id 2 of stack \"Example\"\ngo card id 3",
		"continuation": "go card id 2\nput 1 ¬\ninto variable\ngo card id 3",
		"terminal":     "go card id 2\nexit mouseUp\ngo card id 3",
		"return":       "go card id 2\nreturn 1\ngo card id 3",
	} {
		t.Run(name, func(t *testing.T) {
			metadata := sequenceMetadata(t, sequenceCorpus("on mouseUp\n"+body+"\nend mouseUp"))
			if len(sequenceEdges(metadata)) != 0 {
				t.Fatal("unproven control flow acquired a continuation")
			}
		})
	}
}

// A handler may redisplay its own card before it moves on, as the Myst
// fireplace button does. That request names no destination: it ends a run and
// hides nothing else. Any other request that is not a literal card still hides
// the handler, because it may leave the stack.
func TestLiteralSequenceRedrawEndsARunWithoutHidingTheHandler(t *testing.T) {
	step := func(from, to string) [][2]string { return [][2]string{{"Example:" + from, "Example:" + to}} }
	for name, tc := range map[string]struct {
		body string
		want [][2]string
	}{
		"redraw first":       {"play \"click\"\ngo card\nplay stop\ngo card id 2\ngo card id 3", step("2", "3")},
		"this card first":    {"go this card\ngo card id 2\ngo card id 3", step("2", "3")},
		"to this card first": {"go to this card\ngo card id 2\ngo card id 3", step("2", "3")},
		"redraw last":        {"go card id 2\ngo card id 3\ngo card", step("2", "3")},
		"redraw between":     {"go card id 2\ngo card\ngo card id 3", nil},
		"redraw then a pair": {"go card id 2\ngo card\ngo card id 3\ngo card id 4", step("3", "4")},
		"stack first":        {"go card id 4 of stack \"Example\"\ngo card id 2\ngo card id 3", nil},
		"ordinal first":      {"go next\ngo card id 2\ngo card id 3", nil},
	} {
		t.Run(name, func(t *testing.T) {
			var got [][2]string
			for _, edge := range sequenceEdges(sequenceMetadata(t, sequenceCorpus("on mouseUp\n"+tc.body+"\nend mouseUp"))) {
				got = append(got, [2]string{edge.Source.Name, edge.Target.Name})
			}
			if len(got) != len(tc.want) || len(got) == 1 && got[0] != tc.want[0] {
				t.Fatalf("got continuations %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLiteralSequenceUnknownCommandsBreakRuns(t *testing.T) {
	for _, command := range []string{"navigateSomewhere", `do "go card id 4"`, `send "navigate" to this card`, "vs direction", `vs "direction"`, `vs "left" & suffix`, `vs "left", 2`, "put navigator() into state",
		// A parenthesized call could run a function of the imported stacks.
		"wait until ready()", "play chooser()", "set cursor to pick(1)", `HTVisual effect("wipe"),,,1`, `playQT "" & loop()`,
		// Not among the player's display, sound and wait commands.
		"hc", "ww", "xCIcon3 it,4001", "openCard", "put 1 into state", "get the mouse", "lock screen", `answer "ready"`} {
		t.Run(command, func(t *testing.T) {
			metadata := sequenceMetadata(t, sequenceCorpus("on mouseUp\ngo card id 2\n"+command+"\ngo card id 3\ngo card id 4\nend mouseUp"))
			edges := sequenceEdges(metadata)
			if len(edges) != 1 || edges[0].Source.Name != "Example:3" || edges[0].Target.Name != "Example:4" {
				t.Fatalf("opaque call was crossed, or the following explicit run was lost: %+v", edges)
			}
		})
	}
}

func TestLiteralSequenceCrossesPlayerDisplaySoundAndWaitCommands(t *testing.T) {
	for _, command := range []string{`play "click"`, "play stop", "wait 20", `wait until the sound is "done"`, "wait until ready", "set cursor to none",
		"vd", `dplay "latch"`, `playQT "room tone",,loop,40`, `playQT "",,"loop",volume * 2,,,,,true`, "soundidle", "SoundIdle",
		`HTVisual "wipe up",,,2,16`, `htvisual "scroll down",,the rect of card button "panel",2,8`, "htlock true", "htlock forcefalse",
		// Parentheses inside a quoted name are text, not a call.
		`htaddpict "panel (lit)",the rect of card button "panel","srccopy"`,
		"play \"hum\"\nplay \"click\"\nvs \"left\"\nset cursor to none -- trailing note"} {
		t.Run(command, func(t *testing.T) {
			metadata := sequenceMetadata(t, sequenceCorpus("on mouseUp\ngo card id 2\n"+command+"\ngo card id 3\nend mouseUp"))
			edges := sequenceEdges(metadata)
			if len(edges) != 1 || edges[0].Source.Name != "Example:2" || edges[0].Target.Name != "Example:3" {
				t.Fatalf("a display, sound or wait command ended the run: %+v", edges)
			}
			if len(metadata.Edges) != 3 {
				t.Fatalf("got %d records, want both owner references and the continuation", len(metadata.Edges))
			}
		})
	}
}

func TestLiteralSequenceImportedHandlersOverrideTransparentCommands(t *testing.T) {
	for name := range sequenceTransparent {
		for _, scope := range []string{"stack", "source-card", "target-card"} {
			t.Run(name+"/"+scope, func(t *testing.T) {
				corpus := sequenceCorpus("on mouseUp\ngo card id 2\n" + name + " something\ngo card id 3\nend mouseUp")
				script := hypercard.Script{Text: "on " + name + " value\ngo card id 4\nend " + name}
				switch scope {
				case "stack":
					corpus.Stacks[0].Script = script
				case "source-card":
					corpus.Stacks[0].Cards[0].Script = script
				case "target-card":
					corpus.Stacks[0].Cards[1].Script = script
				}
				if len(sequenceEdges(sequenceMetadata(t, corpus))) != 0 {
					t.Fatal("an imported handler of the same name was ignored")
				}
			})
		}
	}
}

func TestLiteralSequenceDoesNotCrossPartsHandlersOrDisabledNavigation(t *testing.T) {
	for name, scripts := range map[string][]string{
		"parts":               {"on mouseUp\ngo card id 2\nend mouseUp", "on mouseUp\ngo card id 3\nend mouseUp"},
		"handlers":            {"on first\ngo card id 2\nend first\non second\ngo card id 3\nend second"},
		"commented handler":   {"--on mouseUp\ngo card id 2\ngo card id 3\nend mouseUp"},
		"incomplete handler":  {"on mouseUp\ngo card id 2\ngo card id 3"},
		"function":            {"function destination\ngo card id 2\ngo card id 3\nend destination"},
		"disabled navigation": {"on mouseUp\ngo card id 2\n--go card id 4\ngo card id 3\nend mouseUp"},
	} {
		t.Run(name, func(t *testing.T) {
			metadata := sequenceMetadata(t, sequenceCorpus(scripts...))
			if len(sequenceEdges(metadata)) != 0 {
				t.Fatal("independent or disabled navigation acquired a continuation")
			}
			if name == "disabled navigation" {
				found := false
				for _, edge := range metadata.Edges {
					if edge.Target.Name == "Example:4" {
						found = edge.Source.Name == "Example:1" && edge.IsOfType(common.Disabled)
					}
				}
				if !found {
					t.Fatal("disabled source reference was lost or moved")
				}
			}
		})
	}
}

func TestLiteralSequenceRefusesVisualOverridesAndUnverifiedScope(t *testing.T) {
	for _, scope := range []string{"source-part", "source-card", "target-card", "background", "background-part", "stack", "unverified", "go-source", "go-target", "visual"} {
		t.Run(scope, func(t *testing.T) {
			corpus := sequenceCorpus("on mouseUp\ngo card id 2\nvs left\ngo card id 3\nend mouseUp")
			script := hypercard.Script{Text: "on vs direction\ngo card id 4\nend vs"}
			switch scope {
			case "source-part":
				corpus.Stacks[0].Cards[0].Parts = append(corpus.Stacks[0].Cards[0].Parts, hypercard.Part{ID: 2, Script: script})
			case "source-card":
				corpus.Stacks[0].Cards[0].Script = script
			case "target-card":
				corpus.Stacks[0].Cards[1].Script = script
			case "stack":
				corpus.Stacks[0].Script = script
			case "background", "background-part":
				for i := range corpus.Stacks[0].Cards {
					corpus.Stacks[0].Cards[i].BackgroundID = 7
				}
				background := hypercard.Background{ID: 7}
				if scope == "background" {
					background.Script = script
				} else {
					background.Parts = []hypercard.Part{{ID: 1, Script: script}}
				}
				corpus.Stacks[0].Backgrounds = []hypercard.Background{background}
			case "unverified":
				corpus.Stacks[0].BackgroundsComplete = false
			case "go-source", "go-target":
				index := 0
				if scope == "go-target" {
					index = 1
				}
				corpus.Stacks[0].Cards[index].Script.Text = "on go destination\nend go"
			case "visual":
				corpus.Stacks[0].Cards[0].Parts[0].Script.Text = strings.Replace(corpus.Stacks[0].Cards[0].Parts[0].Script.Text, "vs left", "visual effect dissolve", 1)
				corpus.Stacks[0].Script.Text = "on visual effect\nend visual"
			}
			if len(sequenceEdges(sequenceMetadata(t, corpus))) != 0 {
				t.Fatal("overridden or unavailable command scope acquired a continuation")
			}
		})
	}
}

func TestLiteralSequencePreservesSavedReturnAndRestrictivePairs(t *testing.T) {
	corpus := sequenceCorpus("on mouseUp\npush card\ngo card id 2\nend mouseUp")
	corpus.Stacks[0].Cards[1].Parts = []hypercard.Part{{ID: 1, Script: hypercard.Script{Text: "on mouseUp\nvd\npop card\nend mouseUp"}}}
	metadata := sequenceMetadata(t, corpus)
	if len(metadata.Edges) != 1 || !metadata.Edges[0].IsOfType(common.Backtracking) || len(sequenceEdges(metadata)) != 0 {
		t.Fatal("ordinary push/pop entry changed")
	}
	corpus = sequenceCorpus("on mouseUp\npush card id 4 of stack \"Example\"\ngo card id 2\ngo card id 3\nend mouseUp")
	metadata = sequenceMetadata(t, corpus)
	if len(metadata.Edges) != 3 || len(sequenceEdges(metadata)) != 0 || !metadata.Edges[0].IsOfType(common.RestrictiveTransitivityTail) || !metadata.Edges[1].IsOfType(common.RestrictiveTransitivityHead) {
		t.Fatal("saved return pair changed")
	}
}

func TestLiteralSequenceDoesNotDeduplicateOrdinaryAndOrderedReferences(t *testing.T) {
	corpus := sequenceCorpus("on mouseUp\ngo card id 2\ngo card id 3\nend mouseUp")
	corpus.Stacks[0].Cards[1].Script.Text = "on mouseUp\ngo card id 3\nend mouseUp"
	metadata := sequenceMetadata(t, corpus)
	// 1 -> 2 and 1 -> 3 from the owner, then 2 -> 3 twice: the continuation
	// and the intermediate card's own reference.
	if len(metadata.Edges) != 4 || len(sequenceEdges(metadata)) != 1 {
		t.Fatal("ordinary and ordered references with the same endpoints were conflated")
	}
}
