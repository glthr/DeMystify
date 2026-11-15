package riven

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
)

func auditNumber(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case float64:
		return int(n)
	}
	return -1
}

type auditCallsite struct {
	source, resource, event string
	offset, hotspot         int
}

func auditKey(source string, ev map[string]any) auditCallsite {
	h := -1
	if n, ok := ev["hotspot_ordinal"]; ok {
		h = auditNumber(n)
	}
	return auditCallsite{source, fmt.Sprint(ev["resource"]), fmt.Sprint(ev["event"]), auditNumber(ev["byte_offset"]), h}
}
func auditJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestSuppliedDiscCommandOffsetsAndNativeAttribution(t *testing.T) {
	input, profile := fixtureCorpus(t)
	i, err := LoadInput(input)
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	commands := map[auditCallsite]Command{}
	count := 0
	for key, stack := range i.Stacks {
		for id, card := range stack.Cards {
			contexts := []struct {
				tag     string
				ordinal int
				scripts []Script
			}{{"CARD", -1, card.Scripts}}
			for ordinal, h := range card.Hotspots {
				contexts = append(contexts, struct {
					tag     string
					ordinal int
					scripts []Script
				}{"HSPT", ordinal, h.Scripts})
			}
			for _, context := range contexts {
				ref := stack.Resources[context.tag][id]
				raw, err := ref.Archive.Read(context.tag, id)
				if err != nil {
					t.Fatal(err)
				}
				for _, script := range context.scripts {
					if script.Offset < 0 || script.Offset+2 > len(raw) || be16(raw[script.Offset:]) != script.Type {
						t.Fatalf("%s:%d %s event offset %d is wrong", key, id, context.tag, script.Offset)
					}
					var walk func([]Command)
					walk = func(cs []Command) {
						for _, c := range cs {
							count++
							if c.Offset < 0 || c.End <= c.Offset || c.End > len(raw) || c.Offset+4 > len(raw) || be16(raw[c.Offset:]) != c.Opcode {
								t.Fatalf("%s:%d %s command offset %d/op%d does not identify its actual bytes", key, id, context.tag, c.Offset, c.Opcode)
							}
							n := int(be16(raw[c.Offset+2:]))
							if c.Opcode == 8 {
								if n != 2 || be16(raw[c.Offset+4:]) != c.Variable {
									t.Fatal("switch payload misaligned")
								}
							} else {
								if n != len(c.Args) {
									t.Fatal("argument count changed")
								}
								for j, arg := range c.Args {
									if c.Offset+6+j*2 > len(raw) || be16(raw[c.Offset+4+j*2:]) != arg {
										t.Fatalf("%s:%d command%d argument%d differs from source bytes", key, id, c.Offset, j)
									}
								}
							}
							identity := auditCallsite{SourceID(key, id), fmt.Sprintf("%s:%d", context.tag, id), EventName(script.Type), c.Offset, context.ordinal}
							if _, ok := commands[identity]; ok {
								t.Fatalf("duplicate command identity %+v", identity)
							}
							commands[identity] = c
							for _, branch := range c.Branches {
								walk(branch.Commands)
							}
						}
					}
					walk(script.Commands)
				}
			}
		}
	}
	if count != profile.commands {
		t.Fatalf("decoded %d commands, want %d for edition %s", count, profile.commands, profile.edition)
	}
	seen := map[auditCallsite]bool{}
	for _, edge := range i.Metadata.Edges {
		for _, ev := range edge.Evidence {
			if edge.Kind == "script" {
				identity := auditKey(edge.Source.Details.SourceID, ev)
				cmd, ok := commands[identity]
				if !ok || (cmd.Opcode != 2 && cmd.Opcode != 27) || auditNumber(ev["opcode"]) != int(cmd.Opcode) {
					t.Fatalf("edge %+v does not identify an actual navigation opcode", identity)
				}
				if seen[identity] {
					t.Fatalf("navigation instruction emitted twice: %+v", identity)
				}
				seen[identity] = true
			}
			if strings.HasPrefix(edge.Kind, "native") {
				if url, ok := ev["engine_source"].(string); !ok || !strings.HasPrefix(url, SourceBase) {
					t.Fatalf("native edge lacks pinned attribution: %#v", ev)
				}
			}
		}
	}
	if len(seen) != 6980 {
		t.Fatalf("navigation instructions reconciled=%d", len(seen))
	}
	for identity, c := range commands {
		if (c.Opcode == 2 || c.Opcode == 27) && !seen[identity] {
			t.Fatalf("unrepresented literal navigation %+v", identity)
		}
	}
	if len(i.Unresolved) != 3 {
		t.Fatal("saved return callsites changed")
	}
	for _, ev := range i.Unresolved {
		if !strings.HasPrefix(fmt.Sprint(ev["source"]), "aspit:") || !strings.HasPrefix(fmt.Sprint(ev["engine_source"]), SourceBase) {
			t.Fatalf("saved return attribution corrupted: %#v", ev)
		}
	}
	t.Logf("reconciled all %d command offsets with original CARD/HSPT bytes and all %d literal navigation callsites", count, len(seen))
}

func TestSuppliedDiscGraphAgainstVerifiedReference(t *testing.T) {
	referenceDir := os.Getenv("RIVEN_REFERENCE_DATA")
	if referenceDir == "" {
		t.Skip("set RIVEN_REFERENCE_DATA to the independently verified generated/ directory")
	}
	i, err := LoadInput(fixtureInput(t))
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	var reference struct {
		Nodes []struct {
			ID      string `json:"id"`
			Virtual bool   `json:"virtual"`
		} `json:"nodes"`
		Edges []struct {
			Source, Target, Kind string
			Evidence             []map[string]any
		} `json:"edges"`
	}
	b, err := os.ReadFile(filepath.Join(referenceDir, "graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &reference); err != nil {
		t.Fatal(err)
	}
	if len(reference.Nodes) != 2312 || len(reference.Edges) != 6686 {
		t.Fatal("unexpected reference graph")
	}
	virtual := map[string]bool{}
	for _, n := range i.Metadata.Nodes {
		if n.IsOfType(common.IsVirtual) {
			virtual[n.Details.SourceID] = true
		}
	}
	wantVirtual := map[string]bool{}
	for _, n := range reference.Nodes {
		if n.Virtual {
			wantVirtual[n.ID] = true
		}
	}
	if !reflect.DeepEqual(virtual, wantVirtual) {
		t.Fatalf("virtual targets differ: %v / %v", virtual, wantVirtual)
	}
	wantEdges := map[string]bool{}
	wantCalls := map[auditCallsite]map[string]any{}
	// Aggregate evidence across classification-split records. An evidence
	// identity belongs to its original typed endpoint and original instruction;
	// neither a new flag nor a parallel record may duplicate or discard it.
	type evidenceIdentity struct {
		endpoint string
		callsite auditCallsite
	}
	wantEvidence := map[evidenceIdentity]map[string]any{}
	for _, edge := range reference.Edges {
		endpoint := edge.Source + ">" + edge.Target + ":" + edge.Kind
		wantEdges[endpoint] = true
		for _, ev := range edge.Evidence {
			identity := evidenceIdentity{endpoint, auditKey(edge.Source, ev)}
			if _, duplicate := wantEvidence[identity]; duplicate {
				t.Fatalf("duplicate verified reference evidence: %+v", identity)
			}
			wantEvidence[identity] = ev
		}
		if edge.Kind == "script" {
			for _, ev := range edge.Evidence {
				identity := auditKey(edge.Source, ev)
				if _, duplicate := wantCalls[identity]; duplicate {
					t.Fatalf("duplicate verified literal callsite: %+v", identity)
				}
				wantCalls[identity] = ev
			}
		}
	}
	gotEdges := map[string]bool{}
	gotEvidence := map[evidenceIdentity]map[string]any{}
	gotCalls := map[auditCallsite]bool{}
	derivedRecords, derivedCallsites := 0, 0
	for _, edge := range i.Metadata.Edges {
		// This is the sole derived kind. Every literal, Zip and native record
		// remains part of the independent pre-classification audit.
		if edge.Kind == "sequence-continuation" {
			if !edge.IsOfType(common.SequenceContinuation) {
				t.Fatal("derived continuation lacks its explicit attribute")
			}
			derivedRecords++
			derivedCallsites += len(edge.Evidence)
			continue
		}
		source, target := edge.Source.Details.SourceID, edge.Target.Details.SourceID
		endpoint := source + ">" + target + ":" + edge.Kind
		gotEdges[endpoint] = true
		for _, ev := range edge.Evidence {
			identity := evidenceIdentity{endpoint, auditKey(source, ev)}
			if _, duplicate := gotEvidence[identity]; duplicate {
				t.Fatalf("original evidence emitted in multiple classified records: %+v", identity)
			}
			gotEvidence[identity] = ev
			want, ok := wantEvidence[identity]
			if !ok {
				t.Fatalf("original evidence absent from verified reference: %+v", identity)
			}
			for _, field := range []string{"opcode", "literal_target", "guards", "missing_target", "native_redirects", "engine_source", "condition", "external", "external_args", "zip_name", "sequence"} {
				if auditJSON(ev[field]) != auditJSON(want[field]) {
					t.Fatalf("%+v %s differs: Go=%s reference=%s", identity, field, auditJSON(ev[field]), auditJSON(want[field]))
				}
			}
		}
		if edge.Kind != "script" {
			continue
		}
		for _, ev := range edge.Evidence {
			identity := auditKey(source, ev)
			if _, ok := wantCalls[identity]; !ok {
				t.Fatalf("callsite absent from verified reference: %+v", identity)
			}
			if gotCalls[identity] {
				t.Fatalf("literal instruction emitted twice: %+v", identity)
			}
			gotCalls[identity] = true
		}
	}
	if !reflect.DeepEqual(gotEdges, wantEdges) {
		for k := range wantEdges {
			if !gotEdges[k] {
				t.Errorf("missing edge %s", k)
			}
		}
		for k := range gotEdges {
			if !wantEdges[k] {
				t.Errorf("extra edge %s", k)
			}
		}
	}
	if len(gotEdges) != 6686 || len(wantEdges) != 6686 {
		t.Fatalf("original typed endpoint union %d/%d, want 6686", len(gotEdges), len(wantEdges))
	}
	if len(gotCalls) != 6980 || len(wantCalls) != 6980 {
		t.Fatalf("reference callsites %d/%d", len(gotCalls), len(wantCalls))
	}
	if len(gotEvidence) != 7609 || len(wantEvidence) != 7609 {
		t.Fatalf("all original evidence %d/%d, want 7609", len(gotEvidence), len(wantEvidence))
	}
	if derivedRecords != 22 || derivedCallsites != 22 {
		t.Fatalf("derived automatic continuations %d records/%d callsites, want 22/22", derivedRecords, derivedCallsites)
	}
	t.Logf("all %d original typed endpoints, %d evidence identities and %d literal navigation guards/destinations match verified reference; %d derived continuations audited separately", len(gotEdges), len(gotEvidence), len(gotCalls), derivedRecords)
}
