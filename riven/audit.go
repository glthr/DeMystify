package riven

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/glthr/DeMystify/common"
)

// The audit reads the whole decoded game at once. A card graph says which
// cards name which other cards; the audit adds what only a whole-program view
// can say: which values a variable can ever hold, which branches of the
// scripts can therefore never run, which stored pictures no live branch
// selects, and which cards every route from the start must cross.
//
// It executes nothing. Every result is a statement about the decoded bytes and
// the reviewed native tables below, not about a playthrough.

// InitialVariables are the non-zero values a new game starts with, as set by
// the pinned interpreter. Every other variable starts at zero.
// Source: SourceBase + "riven_vars.cpp#L296-L364".
var InitialVariables = map[string]uint32{
	"ttelescope": 5, "tgatestate": 1, "jbridge1": 1, "jbridge4": 1, "jgallows": 1,
	"jiconcorrectorder": 12068577, "jcrg": 1, "jwharkpos": 1, "bblrvalve": 1, "bblrwtr": 1,
	"bfans": 1, "bytrap": 2, "aatrusbook": 1, "acathbook": 1, "bheat": 1, "blabpage": 1,
	"bidvlv": 1, "bvise": 1, "waterenabled": 1, "ogehnpage": 1, "bblrsw": 1, "ocage": 1,
	"jbeetle": 1, "tdl": 1, "bmagcar": 1, "gnmagcar": 1, "gemagcar": 1, "gimagecurr": 1,
	"gimagemax": 1, "gimagerot": 1, "glkbridge": 1, "grviewpos": 2, "gpinpos": 1,
	"grviewmpos": 1617, "omusicplayer": 1, "tdomeelev": 1, "transitionmode": 1,
}

// NativeVariables are the variables that the pinned interpreter reads or
// writes outside the game's own scripts (every `_vars["name"]` of its Riven
// sources other than the initial values above, plus the three combinations it
// randomizes and the six marble positions). The audit claims nothing about
// the values these can hold.
var NativeVariables = func() map[string]bool {
	names := "aatrusbook acathbook acathstate adomecombo agehn apower atemp atrap atrapbook azip bbait bblrarm " +
		"bblrgrt bblrsw bblrvalve bblrwtr bcratergg bheat bidvlv blabpage bvalve bytram bytramtime bytrap " +
		"bytrapped currentcardid currentstackid domecheck gcathstate gimagecurr gimagemax glkbtns glview " +
		"glviewpos gpinpos gpinup grview grviewpos gscribe gscribetime gupmoov gwhark gwharktime jgallows " +
		"jgirl jiconcorrectorder jiconorder jicons jleftpos jplaybeetle jrbook jrightpos jsunners jsunnertime " +
		"jwharkpos jwmouth ocage odeskbook ogehnpage ostanddrawer pcage pcathcheck pcathtime pcorrectorder " +
		"pelevcombo returncardid returnstackid rrebel rrebelview rrichard rvillagetime tcorrectorder " +
		"tcovercombo themarble transitionmode ttelecover ttelepin ttelescope ttelevalve twaffle waterenabled " +
		"tred torange tyellow tgreen tblue tviolet"
	out := map[string]bool{}
	for _, name := range strings.Fields(names) {
		out[name] = true
	}
	return out
}()

// TemplateHotspots are the seven standard movement zones, with the hotspot
// number and the rectangle they have on an untouched card.
var TemplateHotspots = []struct {
	ID   uint16
	Name string
	Rect [4]int
}{
	{1, "left", [4]int{0, 0, 74, 392}}, {2, "forwardleft", [4]int{73, 0, 207, 392}},
	{3, "forward", [4]int{206, 0, 391, 392}}, {4, "forwardright", [4]int{390, 0, 537, 392}},
	{5, "right", [4]int{536, 0, 608, 392}}, {6, "up", [4]int{0, 0, 608, 41}},
	{7, "down", [4]int{0, 352, 608, 392}},
}

type VariableAudit struct {
	Name string `json:"name"`
	// Initial is the value a new game starts with.
	Initial uint32 `json:"initial"`
	// ScriptValues are the literal values that set_variable commands store.
	ScriptValues []uint16 `json:"scriptValues"`
	WriteSites   int      `json:"writeSites"`
	Increments   int      `json:"incrementSites"`
	// Native is true when the interpreter itself touches the variable: its
	// possible values are then unknown to this audit.
	Native bool `json:"native"`
	// Cases are the values that switch commands compare the variable with.
	Cases     []uint16 `json:"cases"`
	ReadSites int      `json:"readSites"`
	// ReadCards and WriteCards count distinct cards.
	ReadCards  int `json:"readCards"`
	WriteCards int `json:"writeCards"`
	// NeverWritten: no script and no native routine stores a value.
	NeverWritten bool `json:"neverWritten,omitempty"`
	// NeverRead: no script and no native routine tests the value.
	NeverRead bool `json:"neverRead,omitempty"`
	// DeadCases are tested values that the variable can never hold.
	DeadCases []uint16 `json:"deadCases,omitempty"`
}

type DeadBranch struct {
	Card     string `json:"card"`
	Resource string `json:"resource"`
	Event    string `json:"event"`
	Hotspot  string `json:"hotspot,omitempty"`
	Offset   int    `json:"switchOffset"`
	Variable string `json:"variable"`
	// Value is the case value; "default" is the fallback branch.
	Value string `json:"value"`
	// Reason is "value never stored" or "contradicts an enclosing case".
	Reason string `json:"reason"`
}

type UnselectedPicture struct {
	Card   string `json:"card"`
	Index  uint16 `json:"index"`
	Bitmap uint16 `json:"bitmap"`
	Name   string `json:"name"`
	Rect   [4]int `json:"rect"`
	// DeadBranchOnly: a script selects the entry, but only in a dead branch.
	DeadBranchOnly bool `json:"deadBranchOnly"`
	// SameBitmapSelected: another entry of the card with the same bitmap is selected.
	SameBitmapSelected bool `json:"sameBitmapSelected,omitempty"`
}

type PictureAudit struct {
	Records int `json:"records"`
	// Unselected lists entries, other than the default entry 1, that no live
	// branch of any script selects, on cards that call no native routine.
	Unselected []UnselectedPicture `json:"unselected"`
	// NativeCards counts the entries left aside because a script that runs on
	// their card calls a native routine, which may draw any of them.
	NativeEntries int `json:"entriesOnCardsWithNativeCalls"`
	NativeCards   int `json:"cardsWithNativeCalls"`
}

type TemplateUse struct {
	Name         string `json:"name"`
	Hotspots     int    `json:"hotspots"`
	TemplateRect int    `json:"withTemplateRectangle"`
	TemplateID   int    `json:"withTemplateNumber"`
}

type InertHotspot struct {
	Card string `json:"card"`
	Name string `json:"name"`
	ID   uint16 `json:"id"`
	Rect [4]int `json:"rect"`
}

type HotspotAudit struct {
	Total    int            `json:"total"`
	Names    int            `json:"distinctNames"`
	ByName   map[string]int `json:"byName"`
	Template []TemplateUse  `json:"template"`
	// Inert are hotspots named like a movement whose scripts do nothing but
	// set the cursor, or are absent.
	Inert []InertHotspot `json:"inertMovementHotspots"`
	// BlankCards hold the seven template hotspots and no script on any of them.
	BlankCards []string `json:"blankCards"`
	NoScript   int      `json:"withoutAnyScript"`
	// EmptyRectangle lists the hotspots whose rectangle is empty or inverted
	// (left >= right or top >= bottom). The pinned interpreter disables them
	// when it loads them, and no point is inside such a rectangle, so a click
	// can never reach their scripts. Source: SourceBase + "riven_card.cpp#L1273-L1280".
	EmptyRectangle []InertHotspot `json:"emptyOrInvertedRectangle"`
}

type Dominator struct {
	Card      string `json:"card"`
	Dominated int    `json:"dominated"`
}

type LinkClass struct {
	Class       string  `json:"class"`
	Pairs       int     `json:"pairs"`
	WithReverse int     `json:"withReverse"`
	Share       float64 `json:"share"`
}

type ZipAudit struct {
	Records      int      `json:"records"`
	Sources      int      `json:"sourceCards"`
	Destinations int      `json:"destinations"`
	Places       int      `json:"cardsRegisteringAPlace"`
	Untargeted   []string `json:"placesNoHotspotNames"`
	MeanWalk     float64  `json:"meanWalkReplaced"`
	MedianWalk   float64  `json:"medianWalkReplaced"`
	LongestWalk  int      `json:"longestWalkReplaced"`
	LongestFrom  string   `json:"longestWalkFrom"`
	LongestTo    string   `json:"longestWalkTo"`
	// Without and With describe the largest strongly connected component.
	DiameterWithout     int     `json:"diameterWithout"`
	DiameterWith        int     `json:"diameterWith"`
	MeanDistanceWithout float64 `json:"meanDistanceWithout"`
	MeanDistanceWith    float64 `json:"meanDistanceWith"`
	FarthestWithout     int     `json:"farthestFromStartWithout"`
	FarthestWith        int     `json:"farthestFromStartWith"`
}

// ShadowedRecord is one reference that no mouse event can trigger.
type ShadowedRecord struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Kind     string   `json:"baseKind"`
	Hotspots []string `json:"hotspots"`
}

// ShadowedAudit counts the shadowed references and says whether the figures
// of the structure follow them.
type ShadowedAudit struct {
	Total    int              `json:"records"`
	ByKind   map[string]int   `json:"byBaseKind"`
	Followed bool             `json:"followedByRoutes"`
	Records  []ShadowedRecord `json:"list"`
}

type StructureAudit struct {
	Start     string `json:"start"`
	Reachable int    `json:"reachableFromStart"`
	// Dominators lists the cards that every route from the start to at least
	// MinDominated other cards must cross, most dominating first.
	Dominators   []Dominator `json:"dominators"`
	MinDominated int         `json:"dominatorThreshold"`
	// FarthestChain is the dominator chain of the farthest card: the cards no
	// route to it can avoid, in order.
	FarthestCard     string      `json:"farthestCard"`
	FarthestDistance int         `json:"farthestDistance"`
	FarthestChain    []string    `json:"farthestDominatorChain"`
	Links            []LinkClass `json:"linksByHotspotName"`
	Places           int         `json:"placesJoinedByTurns"`
	PlaceSizes       map[int]int `json:"placeSizes"`
	Zip              ZipAudit    `json:"zip"`
	// Shadowed references are left out of everything above unless Followed.
	Shadowed ShadowedAudit `json:"shadowed"`
}

type Audit struct {
	Policy       string          `json:"policy"`
	Variables    []VariableAudit `json:"variables"`
	DeadBranches []DeadBranch    `json:"deadBranches"`
	Pictures     PictureAudit    `json:"pictures"`
	Hotspots     HotspotAudit    `json:"hotspots"`
	Structure    StructureAudit  `json:"structure"`
}

const auditPolicy = "Static audit of the decoded scripts; nothing is executed. A variable touched by a native " +
	"routine or incremented by a script is assumed able to hold any value. Entries on cards whose scripts call a " +
	"native routine are not reported as unselected. Movement excludes Zip, ending resets and automatic next steps. " +
	"A shadowed reference (no mouse event can reach its hotspot) is left out of movement and of the Zip figures " +
	"unless the run follows them."

type variableUse struct {
	display            string
	values             map[uint16]int
	cases              map[uint16]int
	increments         int
	readCards, wrCards map[string]bool
}

func cardLabel(source string) string {
	parts := strings.SplitN(source, ":", 2)
	if len(parts) != 2 {
		return source
	}
	return StackDisplay[parts[0]] + ":" + parts[1]
}

func sortedU16(m map[uint16]int) []uint16 {
	out := make([]uint16, 0, len(m))
	for v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}

func sortedStrings(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// possibleValues returns the values a variable can hold, or nil when the
// audit cannot bound them (native use, or an increment).
func possibleValues(name string, use *variableUse) map[uint32]bool {
	if NativeVariables[name] || use.increments > 0 {
		return nil
	}
	out := map[uint32]bool{InitialVariables[name]: true}
	for v := range use.values {
		out[uint32(v)] = true
	}
	return out
}

// Audit computes the whole-game report from an imported game.
func (i *Import) Audit() (*Audit, error) {
	stackKeys := make([]string, 0, len(i.Stacks))
	for key := range i.Stacks {
		stackKeys = append(stackKeys, key)
	}
	sort.Strings(stackKeys)
	audit := &Audit{Policy: auditPolicy}

	// ---- 1. every write and every test of every variable
	uses := map[string]*variableUse{}
	use := func(stack *Stack, index uint16) (string, *variableUse) {
		display := NameAt(stack.Names, 4, int(index))
		name := strings.ToLower(display)
		u := uses[name]
		if u == nil {
			u = &variableUse{display: display, values: map[uint16]int{}, cases: map[uint16]int{}, readCards: map[string]bool{}, wrCards: map[string]bool{}}
			uses[name] = u
		}
		return name, u
	}
	type owner struct {
		stack   *Stack
		source  string
		card    *Card
		hotspot *Hotspot
		script  Script
	}
	var owners []owner
	for _, key := range stackKeys {
		stack := i.Stacks[key]
		for index := range stack.Names[4] {
			use(stack, uint16(index))
		}
		for _, id := range sortedIDs(stack.Cards) {
			card := stack.Cards[id]
			source := SourceID(key, id)
			for _, script := range card.Scripts {
				owners = append(owners, owner{stack, source, card, nil, script})
			}
			for h := range card.Hotspots {
				for _, script := range card.Hotspots[h].Scripts {
					owners = append(owners, owner{stack, source, card, &card.Hotspots[h], script})
				}
			}
		}
	}
	var scan func(o owner, cs []Command)
	scan = func(o owner, cs []Command) {
		for _, c := range cs {
			switch c.Opcode {
			case 7, 24:
				if len(c.Args) != 2 {
					continue
				}
				_, u := use(o.stack, c.Args[0])
				u.wrCards[o.source] = true
				if c.Opcode == 7 {
					u.values[c.Args[1]]++
				} else {
					u.increments++
				}
			case 8:
				_, u := use(o.stack, c.Variable)
				u.readCards[o.source] = true
				for _, br := range c.Branches {
					u.cases[br.Value]++
					scan(o, br.Commands)
				}
			}
		}
	}
	for _, o := range owners {
		scan(o, o.script.Commands)
	}
	possible := map[string]map[uint32]bool{}
	names := make([]string, 0, len(uses))
	for name, u := range uses {
		names = append(names, name)
		possible[name] = possibleValues(name, u)
	}
	sort.Strings(names)
	for _, name := range names {
		u := uses[name]
		v := VariableAudit{Name: u.display, Initial: InitialVariables[name], ScriptValues: sortedU16(u.values), Increments: u.increments, Native: NativeVariables[name], ReadCards: len(u.readCards), WriteCards: len(u.wrCards)}
		for _, n := range u.values {
			v.WriteSites += n
		}
		for value, n := range u.cases {
			v.ReadSites += n
			if value != 65535 {
				v.Cases = append(v.Cases, value)
			}
		}
		sort.Slice(v.Cases, func(a, b int) bool { return v.Cases[a] < v.Cases[b] })
		v.NeverWritten = len(u.values) == 0 && u.increments == 0 && !v.Native
		v.NeverRead = len(u.cases) == 0 && !v.Native
		if set := possible[name]; set != nil {
			for _, value := range v.Cases {
				if !set[uint32(value)] {
					v.DeadCases = append(v.DeadCases, value)
				}
			}
		}
		audit.Variables = append(audit.Variables, v)
	}

	// ---- 2. which branches can run, and what each live or dead branch selects
	redirect := func(stack string, id uint16) string {
		dstStack, dstID, _, err := ResolveDestination(stack, id, i.Stacks)
		if err != nil {
			return SourceID(stack, id)
		}
		return SourceID(dstStack, dstID)
	}
	type selection struct{ live, dead int }
	selected := map[string]map[uint16]*selection{} // card -> picture-list index
	nativeCalls := map[string]bool{}               // cards on which a script calls a native routine
	selectPicture := func(contexts map[string]bool, index uint16, live bool) {
		for card := range contexts {
			if selected[card] == nil {
				selected[card] = map[uint16]*selection{}
			}
			s := selected[card][index]
			if s == nil {
				s = &selection{}
				selected[card][index] = s
			}
			if live {
				s.live++
			} else {
				s.dead++
			}
		}
	}
	var run func(o owner, cs []Command, contexts map[string]bool, env map[string]uint16, live bool) map[string]bool
	run = func(o owner, cs []Command, contexts map[string]bool, env map[string]uint16, live bool) map[string]bool {
		for _, c := range cs {
			switch c.Opcode {
			case 8:
				name, _ := use(o.stack, c.Variable)
				explicit := map[uint16]bool{}
				for _, br := range c.Branches {
					if br.Value != 65535 {
						explicit[br.Value] = true
					}
				}
				merged := map[string]bool{}
				for k := range contexts {
					merged[k] = true
				}
				for _, br := range c.Branches {
					branchLive, reason := live, ""
					known, hasKnown := env[name]
					set := possible[name]
					if br.Value != 65535 {
						if set != nil && !set[uint32(br.Value)] {
							branchLive, reason = false, "value never stored"
						} else if hasKnown && known != br.Value {
							branchLive, reason = false, "contradicts an enclosing case"
						}
					} else {
						covered := set != nil
						for value := range set {
							if value > 65535 || !explicit[uint16(value)] {
								covered = false
							}
						}
						if covered {
							branchLive, reason = false, "value never stored"
						} else if hasKnown && explicit[known] {
							branchLive, reason = false, "contradicts an enclosing case"
						}
					}
					if live && !branchLive {
						d := DeadBranch{Card: cardLabel(o.source), Resource: "CARD", Event: EventName(o.script.Type), Offset: c.Offset, Variable: uses[name].display, Value: fmt.Sprint(br.Value), Reason: reason}
						if br.Value == 65535 {
							d.Value = "default"
						}
						if o.hotspot != nil {
							d.Resource, d.Hotspot = "HSPT", o.hotspot.Name
						}
						audit.DeadBranches = append(audit.DeadBranches, d)
					}
					inner := map[string]uint16{}
					for k, v := range env {
						inner[k] = v
					}
					if br.Value != 65535 {
						inner[name] = br.Value
					}
					branchContexts := map[string]bool{}
					for k := range contexts {
						branchContexts[k] = true
					}
					for k := range run(o, br.Commands, branchContexts, inner, branchLive) {
						merged[k] = true
					}
				}
				contexts = merged
			case 7:
				if len(c.Args) == 2 {
					name, _ := use(o.stack, c.Args[0])
					env[name] = c.Args[1]
				}
			case 24:
				if len(c.Args) == 2 {
					name, _ := use(o.stack, c.Args[0])
					delete(env, name)
				}
			case 2:
				// Later commands of the same handler run on the card just entered.
				if len(c.Args) == 1 {
					contexts = map[string]bool{redirect(o.stack.Name, c.Args[0]): true}
				}
			case 27:
				dest := strings.ToLower(NameAt(o.stack.Names, 5, int(c.StackNameID)))
				if target, err := LocalCardID(dest, c.GlobalCardID, i.Stacks); err == nil {
					contexts = map[string]bool{redirect(dest, target): true}
				}
			case 17:
				for card := range contexts {
					nativeCalls[card] = true
				}
				for k := range env { // a native routine may change any variable
					delete(env, k)
				}
			case 19:
				for k := range env {
					delete(env, k)
				}
			case 39:
				if len(c.Args) >= 1 {
					selectPicture(contexts, c.Args[0], live)
				}
			}
		}
		return contexts
	}
	for _, o := range owners {
		run(o, o.script.Commands, map[string]bool{o.source: true}, map[string]uint16{}, true)
	}
	sort.SliceStable(audit.DeadBranches, func(a, b int) bool {
		x, y := audit.DeadBranches[a], audit.DeadBranches[b]
		if x.Variable != y.Variable {
			return x.Variable < y.Variable
		}
		if x.Value != y.Value {
			return x.Value < y.Value
		}
		return false // keep the order of the cards within one variable and value
	})

	// ---- 3. stored pictures that no live branch selects
	nativeCards := map[string]bool{}
	for _, key := range stackKeys {
		stack := i.Stacks[key]
		for _, id := range sortedIDs(stack.Cards) {
			card := stack.Cards[id]
			source := SourceID(key, id)
			audit.Pictures.Records += len(card.Pictures)
			liveBitmaps := map[uint16]bool{}
			for _, p := range card.Pictures {
				if s := selected[source][p.Index]; p.Index == 1 || (s != nil && s.live > 0) {
					liveBitmaps[p.ImageID] = true
				}
			}
			for _, p := range card.Pictures {
				s := selected[source][p.Index]
				if p.Index == 1 || (s != nil && s.live > 0) {
					continue // entry 1 is the interpreter's default picture
				}
				if nativeCalls[source] {
					audit.Pictures.NativeEntries++
					nativeCards[source] = true
					continue
				}
				name := ""
				if ref, ok := stack.Resources["tBMP"][p.ImageID]; ok {
					name = ref.Resource.Name
				}
				audit.Pictures.Unselected = append(audit.Pictures.Unselected, UnselectedPicture{Card: cardLabel(source), Index: p.Index, Bitmap: p.ImageID, Name: name, Rect: p.Rect, DeadBranchOnly: s != nil && s.dead > 0, SameBitmapSelected: liveBitmaps[p.ImageID]})
			}
		}
	}
	audit.Pictures.NativeCards = len(nativeCards)
	if audit.Pictures.Unselected == nil {
		audit.Pictures.Unselected = []UnselectedPicture{}
	}

	// ---- 4. hotspots: the template they come from, and the ones left inert
	audit.Hotspots.ByName = map[string]int{}
	templateByName := map[string]int{}
	for index, t := range TemplateHotspots {
		templateByName[t.Name] = index
		audit.Hotspots.Template = append(audit.Hotspots.Template, TemplateUse{Name: t.Name})
	}
	movement := map[string]bool{"afl": true, "afr": true, "back": true}
	for _, t := range TemplateHotspots {
		movement[t.Name] = true
	}
	for _, key := range stackKeys {
		stack := i.Stacks[key]
		for _, id := range sortedIDs(stack.Cards) {
			card := stack.Cards[id]
			blank := 0
			for _, h := range card.Hotspots {
				audit.Hotspots.Total++
				audit.Hotspots.ByName[h.Name]++
				acts := false
				for _, script := range h.Scripts {
					var does func([]Command) bool
					does = func(cs []Command) bool {
						for _, c := range cs {
							if c.Opcode == 8 {
								for _, br := range c.Branches {
									if does(br.Commands) {
										return true
									}
								}
							} else if c.Opcode != 13 { // change_cursor alone does nothing
								return true
							}
						}
						return false
					}
					if does(script.Commands) {
						acts = true
					}
				}
				if len(h.Scripts) == 0 {
					audit.Hotspots.NoScript++
				}
				if index, ok := templateByName[h.Name]; ok {
					t := TemplateHotspots[index]
					u := &audit.Hotspots.Template[index]
					u.Hotspots++
					if h.Rect == t.Rect {
						u.TemplateRect++
					}
					if h.ID == t.ID {
						u.TemplateID++
					}
					if !acts && h.Rect == t.Rect && h.ID == t.ID {
						blank++
					}
				}
				if h.Rect[0] >= h.Rect[2] || h.Rect[1] >= h.Rect[3] {
					audit.Hotspots.EmptyRectangle = append(audit.Hotspots.EmptyRectangle, InertHotspot{Card: NodeName(key, id), Name: h.Name, ID: h.ID, Rect: h.Rect})
				}
				if movement[h.Name] && !acts {
					audit.Hotspots.Inert = append(audit.Hotspots.Inert, InertHotspot{Card: NodeName(key, id), Name: h.Name, ID: h.ID, Rect: h.Rect})
				}
			}
			if blank == len(TemplateHotspots) && len(card.Hotspots) == len(TemplateHotspots) {
				audit.Hotspots.BlankCards = append(audit.Hotspots.BlankCards, NodeName(key, id))
			}
		}
	}
	audit.Hotspots.Names = len(audit.Hotspots.ByName)

	// ---- 5. structure of the movement graph
	audit.Structure = i.structureAudit()
	return audit, nil
}

// hotspotClass sorts the names of the hotspots that own a link.
func hotspotClass(names map[string]bool) string {
	turn, forward, back, vertical, other := 0, 0, 0, 0, 0
	for name := range names {
		switch name {
		case "left", "right", "afl", "afr":
			turn++
		case "forward", "forwardleft", "forwardright":
			forward++
		case "back":
			back++
		case "up", "down":
			vertical++
		default:
			other++
		}
	}
	switch {
	case turn > 0 && forward+back+vertical+other == 0:
		return "turn"
	case forward > 0 && turn+back+vertical+other == 0:
		return "forward"
	case back > 0 && turn+forward+vertical+other == 0:
		return "back"
	case vertical > 0 && turn+forward+back+other == 0:
		return "up or down"
	}
	return "other"
}

func (i *Import) structureAudit() StructureAudit {
	md := i.Metadata
	n := len(md.Nodes)
	out := make([]map[int]bool, n)
	in := make([]map[int]bool, n)
	for k := range out {
		out[k], in[k] = map[int]bool{}, map[int]bool{}
	}
	type pair struct{ a, b int }
	owners := map[pair]map[string]bool{}
	var zips []pair
	shadowed := ShadowedAudit{ByKind: map[string]int{}, Followed: md.IncludeShadowed, Records: []ShadowedRecord{}}
	for _, e := range md.Edges {
		a, b := int(e.Source.GraphID), int(e.Target.GraphID)
		if e.IsOfType(common.Shadowed) {
			names := map[string]bool{}
			for _, ev := range e.Evidence {
				if name, ok := ev["hotspot_name"].(string); ok {
					names[name] = true
				}
			}
			shadowed.Total++
			shadowed.ByKind[e.BaseKind]++
			shadowed.Records = append(shadowed.Records, ShadowedRecord{From: e.Source.Name, To: e.Target.Name, Kind: e.BaseKind, Hotspots: sortedStrings(names)})
			if !md.IncludeShadowed {
				continue
			}
		}
		if e.IsOfType(common.Zip) {
			zips = append(zips, pair{a, b})
			continue
		}
		if e.IsOfType(common.EndingReset) || e.IsOfType(common.SequenceContinuation) || e.IsOfType(common.Disabled) || a == b {
			continue
		}
		out[a][b], in[b][a] = true, true
		if owners[pair{a, b}] == nil {
			owners[pair{a, b}] = map[string]bool{}
		}
		if e.Kind == "native-timer" {
			// The sunners' timers and alert videos are not owned by a hotspot,
			// even when the enter script of the card starts the video.
			continue
		}
		for _, ev := range e.Evidence {
			if name, ok := ev["hotspot_name"].(string); ok {
				owners[pair{a, b}][name] = true
			} else if event, _ := ev["event"].(string); strings.HasPrefix(event, "card") {
				owners[pair{a, b}]["<card script>"] = true
			} // a native timer has no owner among the hotspots
		}
	}
	neighbours := func(adj []map[int]bool) [][]int {
		lists := make([][]int, len(adj))
		for k, set := range adj {
			for v := range set {
				lists[k] = append(lists[k], v)
			}
			sort.Ints(lists[k])
		}
		return lists
	}
	succ, pred := neighbours(out), neighbours(in)
	bfs := func(adj [][]int, start int) []int {
		dist := make([]int, n)
		for k := range dist {
			dist[k] = -1
		}
		dist[start] = 0
		queue := []int{start}
		for len(queue) > 0 {
			u := queue[0]
			queue = queue[1:]
			for _, v := range adj[u] {
				if dist[v] < 0 {
					dist[v] = dist[u] + 1
					queue = append(queue, v)
				}
			}
		}
		return dist
	}
	name := func(k int) string { return md.Nodes[k].Name }
	start := int(md.StartNodeID)
	s := StructureAudit{Start: name(start), MinDominated: 50, PlaceSizes: map[int]int{}}
	fromStart := bfs(succ, start)
	farthest := start
	for k, d := range fromStart {
		if d >= 0 {
			s.Reachable++
			if d > fromStart[farthest] || (d == fromStart[farthest] && name(k) < name(farthest)) {
				farthest = k
			}
		}
	}

	// Dominators (Cooper, Harvey and Kennedy), over the cards reachable from the start.
	var order []int
	seen := make([]bool, n)
	type frame struct{ node, next int }
	stack := []frame{{start, 0}}
	seen[start] = true
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		if top.next < len(succ[top.node]) {
			v := succ[top.node][top.next]
			top.next++
			if !seen[v] {
				seen[v] = true
				stack = append(stack, frame{v, 0})
			}
			continue
		}
		order = append(order, top.node)
		stack = stack[:len(stack)-1]
	}
	rpo := make([]int, len(order))
	index := make([]int, n)
	for k := range order {
		rpo[k] = order[len(order)-1-k]
		index[rpo[k]] = k
	}
	idom := make([]int, n)
	for k := range idom {
		idom[k] = -1
	}
	idom[start] = start
	intersect := func(a, b int) int {
		for a != b {
			for index[a] > index[b] {
				a = idom[a]
			}
			for index[b] > index[a] {
				b = idom[b]
			}
		}
		return a
	}
	for changed := true; changed; {
		changed = false
		for _, v := range rpo[1:] {
			candidate := -1
			for _, p := range pred[v] {
				if idom[p] < 0 {
					continue
				}
				if candidate < 0 {
					candidate = p
				} else {
					candidate = intersect(p, candidate)
				}
			}
			if candidate >= 0 && idom[v] != candidate {
				idom[v], changed = candidate, true
			}
		}
	}
	dominated := make([]int, n)
	for k := len(rpo) - 1; k > 0; k-- { // children come after their dominator in this order
		v := rpo[k]
		dominated[idom[v]] += dominated[v] + 1
	}
	for _, v := range rpo {
		if dominated[v] >= s.MinDominated {
			s.Dominators = append(s.Dominators, Dominator{name(v), dominated[v]})
		}
	}
	sort.SliceStable(s.Dominators, func(a, b int) bool { return s.Dominators[a].Dominated > s.Dominators[b].Dominated })
	s.FarthestCard, s.FarthestDistance = name(farthest), fromStart[farthest]
	for v := farthest; ; v = idom[v] {
		s.FarthestChain = append([]string{name(v)}, s.FarthestChain...)
		if v == start {
			break
		}
	}

	// Links by the name of the hotspot that owns them, and how often the reverse exists.
	classes := map[string]*LinkClass{}
	for _, class := range []string{"turn", "forward", "back", "up or down", "other"} {
		classes[class] = &LinkClass{Class: class}
	}
	turns := make([]map[int]bool, n)
	for k := range turns {
		turns[k] = map[int]bool{}
	}
	for p, names := range owners {
		class := classes[hotspotClass(names)]
		class.Pairs++
		if out[p.b][p.a] {
			class.WithReverse++
		}
		if class.Class == "turn" {
			turns[p.a][p.b], turns[p.b][p.a] = true, true
		}
	}
	for _, class := range []string{"turn", "forward", "back", "up or down", "other"} {
		c := *classes[class]
		if c.Pairs > 0 {
			c.Share = float64(int(float64(c.WithReverse)/float64(c.Pairs)*1000+0.5)) / 10
		}
		s.Links = append(s.Links, c)
	}
	placed := make([]bool, n)
	for k, node := range md.Nodes {
		if placed[k] || node.IsOfType(common.IsVirtual) {
			continue
		}
		size, queue := 0, []int{k}
		placed[k] = true
		for len(queue) > 0 {
			u := queue[0]
			queue = queue[1:]
			size++
			for v := range turns[u] {
				if !placed[v] {
					placed[v] = true
					queue = append(queue, v)
				}
			}
		}
		s.Places++
		s.PlaceSizes[size]++
	}

	// Zip mode: what one shortcut replaces, and what all of them do to distances.
	z := ZipAudit{Records: len(zips)}
	sources, destinations := map[int]bool{}, map[int]bool{}
	withZip := make([]map[int]bool, n)
	for k := range withZip {
		withZip[k] = map[int]bool{}
		for v := range out[k] {
			withZip[k][v] = true
		}
	}
	var walks []int
	cache := map[int][]int{}
	for _, p := range zips {
		sources[p.a], destinations[p.b] = true, true
		if p.a != p.b {
			withZip[p.a][p.b] = true
		}
		if cache[p.a] == nil {
			cache[p.a] = bfs(succ, p.a)
		}
		if d := cache[p.a][p.b]; d >= 0 {
			walks = append(walks, d)
			if d > z.LongestWalk {
				z.LongestWalk, z.LongestFrom, z.LongestTo = d, name(p.a), name(p.b)
			}
		}
	}
	z.Sources, z.Destinations = len(sources), len(destinations)
	for _, key := range sortedStackKeys(i.Stacks) {
		for _, id := range sortedIDs(i.Stacks[key].Cards) {
			if i.Stacks[key].Cards[id].ZipModePlace != 0 {
				z.Places++
			}
		}
	}
	for k, node := range md.Nodes {
		if node.Details == nil || destinations[k] {
			continue
		}
		parts := strings.SplitN(node.Details.SourceID, ":", 2)
		if stack := i.Stacks[parts[0]]; stack != nil && len(parts) == 2 {
			var id uint16
			fmt.Sscan(parts[1], &id)
			if card := stack.Cards[id]; card != nil && card.ZipModePlace != 0 {
				z.Untargeted = append(z.Untargeted, name(k))
			}
		}
	}
	sort.Strings(z.Untargeted)
	if len(walks) > 0 {
		sort.Ints(walks)
		total := 0
		for _, w := range walks {
			total += w
		}
		z.MeanWalk = float64(int(float64(total)/float64(len(walks))*100+0.5)) / 100
		z.MedianWalk = float64(walks[len(walks)/2])
		if len(walks)%2 == 0 {
			z.MedianWalk = float64(walks[len(walks)/2-1]+walks[len(walks)/2]) / 2
		}
	}
	// The largest strongly connected component: mutual reachability with the start's component.
	toStart := bfs(pred, start)
	var giant []int
	for k := 0; k < n; k++ {
		if fromStart[k] >= 0 && toStart[k] >= 0 {
			giant = append(giant, k)
		}
	}
	inGiant := make([]bool, n)
	for _, k := range giant {
		inGiant[k] = true
	}
	measure := func(adj [][]int) (int, float64) {
		diameter, total, count := 0, 0, 0
		for _, k := range giant {
			for v, d := range bfs(adj, k) {
				if d > 0 && inGiant[v] {
					total += d
					count++
					if d > diameter {
						diameter = d
					}
				}
			}
		}
		if count == 0 {
			return 0, 0
		}
		return diameter, float64(int(float64(total)/float64(count)*100+0.5)) / 100
	}
	zipSucc := neighbours(withZip)
	z.DiameterWithout, z.MeanDistanceWithout = measure(succ)
	z.DiameterWith, z.MeanDistanceWith = measure(zipSucc)
	z.FarthestWithout = fromStart[farthest]
	for _, d := range bfs(zipSucc, start) {
		if d > z.FarthestWith {
			z.FarthestWith = d
		}
	}
	s.Zip = z
	s.Shadowed = shadowed
	return s
}

func sortedStackKeys(stacks map[string]*Stack) []string {
	keys := make([]string, 0, len(stacks))
	for key := range stacks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// WriteAudit saves the audit beside the other evidence files.
func (i *Import) WriteAudit(dir string) (*Audit, error) {
	audit, err := i.Audit()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(audit, "", "  ")
	if err != nil {
		return nil, err
	}
	return audit, os.WriteFile(filepath.Join(dir, "audit.json"), append(encoded, '\n'), 0644)
}

// Summary is the short text printed after a run.
func (a *Audit) Summary() string {
	var b strings.Builder
	var neverWritten, neverRead, dead []string
	for _, v := range a.Variables {
		if v.NeverWritten {
			neverWritten = append(neverWritten, v.Name)
		}
		if v.NeverRead {
			neverRead = append(neverRead, v.Name)
		}
		for _, value := range v.DeadCases {
			dead = append(dead, fmt.Sprintf("%s = %d", v.Name, value))
		}
	}
	fmt.Fprintf(&b, "Variables: %d (never written: %s; never read: %s)\n", len(a.Variables), strings.Join(neverWritten, ", "), strings.Join(neverRead, ", "))
	fmt.Fprintf(&b, "Dead branches: %d (%s)\n", len(a.DeadBranches), strings.Join(dead, "; "))
	deadOnly := 0
	for _, p := range a.Pictures.Unselected {
		if p.DeadBranchOnly {
			deadOnly++
		}
	}
	fmt.Fprintf(&b, "Picture entries never selected: %d of %d (%d only in dead branches); %d more on %d cards with native calls\n", len(a.Pictures.Unselected), a.Pictures.Records, deadOnly, a.Pictures.NativeEntries, a.Pictures.NativeCards)
	var invalid []string
	for _, h := range a.Hotspots.EmptyRectangle {
		invalid = append(invalid, fmt.Sprintf("%s hotspot %d", h.Card, h.ID))
	}
	fmt.Fprintf(&b, "Hotspots: %d; inert movement hotspots: %d; blank cards: %s; empty or inverted rectangles: %s\n", a.Hotspots.Total, len(a.Hotspots.Inert), strings.Join(a.Hotspots.BlankCards, ", "), strings.Join(invalid, ", "))
	for _, d := range a.Structure.Dominators {
		if d.Card != a.Structure.Start {
			fmt.Fprintf(&b, "Reachable from %s: %d cards; %s stands before %d of them\n", a.Structure.Start, a.Structure.Reachable, d.Card, d.Dominated)
			break
		}
	}
	if sh := a.Structure.Shadowed; sh.Total != 0 {
		kinds := make([]string, 0, len(sh.ByKind))
		for kind := range sh.ByKind {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		for k, kind := range kinds {
			kinds[k] = fmt.Sprintf("%s: %d", kind, sh.ByKind[kind])
		}
		policy := "left out of routes, dominators and Zip figures"
		if sh.Followed {
			policy = "followed (--include-shadowed)"
		}
		fmt.Fprintf(&b, "Shadowed: %d records (%s); %s\n", sh.Total, strings.Join(kinds, ", "), policy)
	}
	z := a.Structure.Zip
	fmt.Fprintf(&b, "Zip: %d shortcuts to %d places; largest shortest distance %d without, %d with\n", z.Records, z.Destinations, z.DiameterWithout, z.DiameterWith)
	return b.String()
}
