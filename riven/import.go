package riven

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/glthr/DeMystify/common"
)

var StackDisplay = map[string]string{"aspit": "Menu", "bspit": "Boiler", "gspit": "Garden", "jspit": "Jungle", "ospit": "Gehn", "pspit": "Prison", "rspit": "Rebel", "tspit": "Temple"}

func NodeName(stack string, id uint16) string { return fmt.Sprintf("%s:%d", StackDisplay[stack], id) }
func SourceID(stack string, id uint16) string { return fmt.Sprintf("%s:%d", stack, id) }

type Import struct {
	Metadata                                         *common.Metadata
	Stacks                                           map[string]*Stack
	Archives                                         []*Archive
	Manifest                                         map[string]any
	Unresolved                                       []map[string]any
	Warnings                                         []map[string]any
	CommandCount, NavigationCount, MissingReferences int
}

func (i *Import) Close() error { return nil } // FileSlice reads open only bounded ranges and close immediately.
func sortedIDs[T any](m map[uint16]T) []uint16 {
	ids := make([]uint16, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	return ids
}
func provenance(ref ResourceRef, tag string) (map[string]any, error) {
	b, e := ref.Archive.Read(tag, ref.Resource.ID)
	if e != nil {
		return nil, e
	}
	sum := sha256.Sum256(b)
	s := ref.Archive.Source
	return map[string]any{"container": s.Path, "path": s.Name, "archiveOffset": s.Offset, "resource": fmt.Sprintf("%s:%d", tag, ref.Resource.ID), "resourceOffset": ref.Resource.Offset, "resourceSize": ref.Resource.Size, "absoluteOffset": s.Offset + ref.Resource.Offset, "sha256": hex.EncodeToString(sum[:])}, nil
}

func LoadInput(path string) (*Import, error) {
	archives, e := DiscoverArchives(path)
	if e != nil {
		return nil, e
	}
	i := &Import{Metadata: &common.Metadata{Game: "riven"}, Stacks: map[string]*Stack{}, Archives: archives, Manifest: map[string]any{}}
	type candidate struct {
		rank    int
		stack   string
		archive *Archive
	}
	var candidates []candidate
	pattern := regexp.MustCompile(`^([abgjoprt])_data([0-9]*)\.mhk$`)
	for _, a := range archives {
		m := pattern.FindStringSubmatch(strings.ToLower(filepath.Base(a.Source.Name)))
		if m == nil {
			continue
		}
		rank, _ := strconv.Atoi(m[2])
		candidates = append(candidates, candidate{rank, m[1] + "spit", a})
	}
	sort.Slice(candidates, func(a, b int) bool {
		if candidates[a].rank != candidates[b].rank {
			return candidates[a].rank < candidates[b].rank
		}
		return candidates[a].archive.Source.Name < candidates[b].archive.Source.Name
	})
	var overrides []map[string]any
	for _, c := range candidates {
		s := i.Stacks[c.stack]
		if s == nil {
			s = &Stack{Name: c.stack, Names: map[uint16][]string{}, Cards: map[uint16]*Card{}, Resources: map[string]map[uint16]ResourceRef{}}
			i.Stacks[c.stack] = s
		}
		for tag, rs := range c.archive.Resources {
			if s.Resources[tag] == nil {
				s.Resources[tag] = map[uint16]ResourceRef{}
			}
			for id, r := range rs {
				if old, ok := s.Resources[tag][id]; ok {
					overrides = append(overrides, map[string]any{"stack": s.Name, "tag": tag, "id": id, "overridden": old.Archive.Source.Name, "selected": c.archive.Source.Name})
				}
				s.Resources[tag][id] = ResourceRef{c.archive, r}
			}
		}
	}
	if len(i.Stacks) != 8 {
		return nil, fmt.Errorf("Riven requires all eight stacks; found %d", len(i.Stacks))
	}
	stackKeys := make([]string, 0, 8)
	for key := range i.Stacks {
		stackKeys = append(stackKeys, key)
	}
	sort.Strings(stackKeys)
	var selected []map[string]any
	for _, key := range stackKeys {
		s := i.Stacks[key]
		read := func(tag string, id uint16) ([]byte, error) {
			ref, ok := s.Resources[tag][id]
			if !ok {
				return nil, fmt.Errorf("%s lacks %s:%d", key, tag, id)
			}
			return ref.Archive.Read(tag, id)
		}
		b, err := read("RMAP", 1)
		if err != nil {
			return nil, err
		}
		s.RMap, err = ParseRMap(b)
		if err != nil {
			return nil, err
		}
		for _, id := range sortedIDs(s.Resources["NAME"]) {
			b, err = read("NAME", id)
			if err != nil {
				return nil, err
			}
			s.Names[id], err = ParseNames(b)
			if err != nil {
				return nil, err
			}
		}
		for _, tag := range []string{"CARD", "HSPT", "NAME", "RMAP", "PLST", "BLST"} {
			for _, id := range sortedIDs(s.Resources[tag]) {
				p, err := provenance(s.Resources[tag][id], tag)
				if err != nil {
					return nil, err
				}
				p["stack"] = key
				selected = append(selected, p)
			}
		}
		for _, id := range sortedIDs(s.Resources["CARD"]) {
			b, err = read("CARD", id)
			if err != nil {
				return nil, err
			}
			card, err := ParseCard(b)
			if err != nil {
				return nil, fmt.Errorf("%s CARD %d: %w", key, id, err)
			}
			if int(id) >= len(s.RMap) {
				return nil, fmt.Errorf("%s CARD %d lacks RMAP identity", key, id)
			}
			card.ID = id
			card.Name = NameAt(s.Names, 1, card.NameID)
			card.GlobalID = s.RMap[id]
			card.Sources = map[string]map[string]any{}
			b, err = read("HSPT", id)
			if err != nil {
				return nil, err
			}
			card.Hotspots, err = ParseHotspots(b)
			if err != nil {
				return nil, fmt.Errorf("%s HSPT %d: %w", key, id, err)
			}
			for index := range card.Hotspots {
				h := &card.Hotspots[index]
				h.Name = NameAt(s.Names, 2, h.NameID)
			}
			if _, ok := s.Resources["PLST"][id]; ok {
				b, err = read("PLST", id)
				if err != nil {
					return nil, err
				}
				card.Pictures, err = ParsePictures(b)
				if err != nil {
					return nil, err
				}
			}
			for _, tag := range []string{"CARD", "HSPT"} {
				p, err := provenance(s.Resources[tag][id], tag)
				if err != nil {
					return nil, err
				}
				card.Sources[tag] = p
			}
			s.Cards[id] = card
		}
	}
	i.Manifest = map[string]any{"scummvmCommit": ScummVMCommit, "archives": archives, "overrides": overrides, "selectedResources": selected, "policy": "SHA-256 of consumed metadata resources; input archives are not modified"}
	if err := i.buildMetadata(stackKeys); err != nil {
		return nil, err
	}
	return i, nil
}

func (i *Import) buildMetadata(stackKeys []string) error {
	md := i.Metadata
	nodes := map[string]*common.Node{}
	edges := map[string]*common.Edge{}
	for _, key := range stackKeys {
		s := i.Stacks[key]
		for _, id := range sortedIDs(s.Cards) {
			c := s.Cards[id]
			original := c.Name
			details := &common.CardDetails{SourceID: SourceID(key, id), GlobalID: fmt.Sprintf("0x%08x", c.GlobalID), ViewWidth: 608, ViewHeight: 392, Provenance: map[string]any{"CARD": c.Sources["CARD"], "HSPT": c.Sources["HSPT"]}}
			var text strings.Builder
			fmt.Fprintf(&text, "// %s RMAP 0x%x\n// CARD:%d\n%s", NodeName(key, id), c.GlobalID, id, FormatScripts(c.Scripts, s.Names))
			for _, h := range c.Hotspots {
				fmt.Fprintf(&text, "\n// HSPT:%d hotspot %d: %s rect=%v flags=%d\n%s", id, h.ID, h.Name, h.Rect, h.Flags, FormatScripts(h.Scripts, s.Names))
				w, height := h.Rect[2]-h.Rect[0], h.Rect[3]-h.Rect[1]
				if w > 0 && height > 0 {
					details.Zones = append(details.Zones, common.InteractionZone{ID: int(h.ID), Name: h.Name, Type: "hotspot", Layer: "card", Script: FormatScripts(h.Scripts, s.Names), X: h.Rect[0], Y: h.Rect[1], Width: w, Height: height, Enabled: true, RawFlags: h.Flags})
				}
			}
			details.Script = text.String()
			for _, p := range c.Pictures {
				details.PictureRecords = append(details.PictureRecords, map[string]any{"index": p.Index, "imageID": p.ImageID, "rect": p.Rect})
			}
			node := &common.Node{GraphID: int64(len(md.Nodes)), Name: NodeName(key, id), StackName: StackDisplay[key], OriginalName: &original, Attributes: []common.NodeAttribute{common.IsCard}, Details: details}
			nodes[details.SourceID] = node
			md.Nodes = append(md.Nodes, node)
			md.TotalCards++
		}
	}
	var add func(string, string, uint16, string, map[string]any) error
	add = func(source, stack string, id uint16, kind string, evidence map[string]any) error {
		dstStack, dstID, chain, err := ResolveDestination(stack, id, i.Stacks)
		if err != nil {
			return err
		}
		targetKey := SourceID(dstStack, dstID)
		evidence["literal_target"] = SourceID(stack, id)
		if len(chain) > 0 {
			evidence["native_redirects"] = chain
		}
		target := nodes[targetKey]
		if target == nil {
			evidence["missing_target"] = true
			i.MissingReferences++
			details := &common.CardDetails{SourceID: targetKey, Script: "// Referenced CARD resource is absent; this is a virtual target.", Provenance: map[string]any{"status": "missing CARD resource"}}
			if int(dstID) < len(i.Stacks[dstStack].RMap) {
				details.GlobalID = fmt.Sprintf("0x%08x", i.Stacks[dstStack].RMap[dstID])
			}
			target = &common.Node{GraphID: int64(len(md.Nodes)), Name: NodeName(dstStack, dstID), StackName: StackDisplay[dstStack], Attributes: []common.NodeAttribute{common.IsVirtual}, Details: details}
			nodes[targetKey] = target
			md.Nodes = append(md.Nodes, target)
		} else if target.IsOfType(common.IsVirtual) {
			evidence["missing_target"] = true
			i.MissingReferences++
		}
		src := nodes[source]
		edgeKey := source + ">" + targetKey + ":" + kind
		edge := edges[edgeKey]
		if edge == nil {
			attrs := []common.EdgeAttribute{common.IntraAge}
			if src.StackName != target.StackName {
				attrs = []common.EdgeAttribute{common.CrossAge}
			}
			if src == target {
				attrs = append(attrs, common.SelfReference)
			}
			if target.IsOfType(common.IsVirtual) {
				attrs = append(attrs, common.NotImplemented)
			}
			if kind == "zip" {
				attrs = append(attrs, common.Zip)
			}
			if strings.HasPrefix(kind, "native") {
				attrs = append(attrs, common.Native)
			}
			if kind == "native-ending" || kind == "native-new-game" {
				attrs = append(attrs, common.EndingReset)
			}
			edge = &common.Edge{Source: src, Target: target, Kind: kind, Attributes: attrs}
			edges[edgeKey] = edge
		}
		edge.Evidence = append(edge.Evidence, evidence)
		return nil
	}
	nativeEdge := func(source string, tr NativeTransition, ev map[string]any) error {
		ev["engine_source"] = tr.Source
		ev["condition"] = tr.Condition
		if tr.Sequence != nil {
			ev["sequence"] = *tr.Sequence
		}
		if tr.Stack == "" {
			ev["source"] = source
			ev["kind"] = tr.Kind
			ev["reason"] = "caller-sensitive saved return"
			i.Unresolved = append(i.Unresolved, ev)
			nodes[source].Details.Unresolved = append(nodes[source].Details.Unresolved, ev)
			return nil
		}
		id := uint16(tr.CardID)
		var err error
		if tr.CardID < 0 {
			id, err = LocalCardID(tr.Stack, tr.GlobalID, i.Stacks)
			if err != nil {
				return err
			}
		}
		kind := tr.Kind
		if !strings.HasPrefix(kind, "native") {
			kind = "native-" + kind
		}
		return add(source, tr.Stack, id, kind, ev)
	}
	for _, key := range stackKeys {
		s := i.Stacks[key]
		for _, id := range sortedIDs(s.Cards) {
			c := s.Cards[id]
			source := SourceID(key, id)
			process := func(ss []Script, h *Hotspot, ordinal int) error {
				for _, script := range ss {
					var walk func([]Command, []map[string]any) error
					walk = func(cs []Command, guards []map[string]any) error {
						for _, cmd := range cs {
							i.CommandCount++
							if _, known := opNames[cmd.Opcode]; !known || cmd.Opcode == 36 {
								i.Warnings = append(i.Warnings, map[string]any{"source": source, "byte_offset": cmd.Offset, "opcode": cmd.Opcode, "warning": "opcode semantics unknown; raw arguments retained"})
							}
							ev := map[string]any{"resource": fmt.Sprintf("CARD:%d", id), "event": EventName(script.Type), "byte_offset": cmd.Offset, "opcode": cmd.Opcode, "guards": guards}
							if h != nil {
								ev["resource"] = fmt.Sprintf("HSPT:%d", id)
								ev["hotspot_ordinal"] = ordinal
								ev["hotspot_blst_id"] = h.ID
								ev["hotspot_name"] = h.Name
								ev["hotspot_rect"] = h.Rect
							}
							switch cmd.Opcode {
							case 8:
								values := []uint16{}
								for _, br := range cmd.Branches {
									if br.Value != 65535 {
										values = append(values, br.Value)
									}
								}
								for _, br := range cmd.Branches {
									excluded := []uint16{}
									if br.Value == 65535 {
										excluded = values
									}
									guard := map[string]any{"variable": cmd.Variable, "variable_name": NameAt(s.Names, 4, int(cmd.Variable)), "value": br.Value, "switch_offset": cmd.Offset, "excluded_values": excluded}
									next := append(append([]map[string]any{}, guards...), guard)
									if err := walk(br.Commands, next); err != nil {
										return err
									}
								}
							case 2:
								if len(cmd.Args) != 1 {
									return fmt.Errorf("%s malformed card change", source)
								}
								i.NavigationCount++
								if err := add(source, key, cmd.Args[0], "script", ev); err != nil {
									return err
								}
							case 27:
								i.NavigationCount++
								dest := strings.ToLower(NameAt(s.Names, 5, int(cmd.StackNameID)))
								target, err := LocalCardID(dest, cmd.GlobalCardID, i.Stacks)
								if err != nil {
									return err
								}
								if err = add(source, dest, target, "script", ev); err != nil {
									return err
								}
							case 17:
								if len(cmd.Args) < 2 || int(cmd.Args[1]) != len(cmd.Args)-2 {
									return fmt.Errorf("%s malformed external call", source)
								}
								external := NameAt(s.Names, 3, int(cmd.Args[0]))
								for _, tr := range ExternalTransitions(key, external) {
									copy := map[string]any{}
									for k, v := range ev {
										copy[k] = v
									}
									copy["external"] = external
									copy["external_args"] = cmd.Args[2:]
									if err := nativeEdge(source, tr, copy); err != nil {
										return err
									}
								}
							case 45:
								if h == nil || h.Name == "" {
									return fmt.Errorf("%s Zip call has no named hotspot", source)
								}
								for _, targetID := range sortedIDs(s.Cards) {
									target := s.Cards[targetID]
									if target.ZipModePlace != 0 && target.Name == h.Name {
										copy := map[string]any{}
										for k, v := range ev {
											copy[k] = v
										}
										copy["zip_name"] = h.Name
										copy["condition"] = "Zip enabled; destination previously visited"
										if err := add(source, key, targetID, "zip", copy); err != nil {
											return err
										}
									}
								}
							case 38:
								if len(cmd.Args) >= 5 && cmd.Args[3] == 2 {
									if err := add(source, key, cmd.Args[4], "script-deferred", ev); err != nil {
										return err
									}
								}
							}
						}
						return nil
					}
					if err := walk(script.Commands, []map[string]any{}); err != nil {
						return err
					}
				}
				return nil
			}
			if err := process(c.Scripts, nil, -1); err != nil {
				return err
			}
			for index := range c.Hotspots {
				if err := process(c.Hotspots[index].Scripts, &c.Hotspots[index], index); err != nil {
					return err
				}
			}
			for _, tr := range TimerTransitions(key, c.GlobalID) {
				if err := nativeEdge(source, tr, map[string]any{"event": "native_timer"}); err != nil {
					return err
				}
			}
		}
	}
	edgeKeys := make([]string, 0, len(edges))
	for key := range edges {
		edgeKeys = append(edgeKeys, key)
	}
	sort.Strings(edgeKeys)
	for _, key := range edgeKeys {
		md.Edges = append(md.Edges, edges[key])
	}
	md.TotalNodes = uint(len(md.Nodes))
	md.TotalEdges = uint(len(md.Edges))
	md.TotalStacks = uint(len(i.Stacks))
	start, err := LocalCardID("tspit", 0x6e9a, i.Stacks)
	if err != nil {
		return err
	}
	md.StartNodeID = nodes[SourceID("tspit", start)].GraphID
	md.StartNodeName = NodeName("tspit", start)
	i.Manifest["commandCount"] = i.CommandCount
	i.Manifest["navigationCallsites"] = i.NavigationCount
	i.Manifest["missingReferences"] = i.MissingReferences
	i.Manifest["warnings"] = i.Warnings
	i.Manifest["inventory"] = InventoryModel()
	return nil
}

func (i *Import) WriteEvidence(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	write := func(name string, v any) error {
		b, e := json.MarshalIndent(v, "", "  ")
		if e != nil {
			return e
		}
		return os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0644)
	}
	ns := make([]map[string]any, 0, len(i.Metadata.Nodes))
	for _, n := range i.Metadata.Nodes {
		ns = append(ns, map[string]any{"id": n.Details.SourceID, "name": n.Name, "stack": n.StackName, "graphID": n.GraphID, "virtual": n.IsOfType(common.IsVirtual), "details": n.Details})
		parts := strings.Split(n.Details.SourceID, ":")
		path := filepath.Join(dir, "scripts", parts[0])
		if e := os.MkdirAll(path, 0755); e != nil {
			return e
		}
		if e := os.WriteFile(filepath.Join(path, parts[1]+".txt"), []byte(n.Details.Script), 0644); e != nil {
			return e
		}
	}
	es := make([]map[string]any, 0, len(i.Metadata.Edges))
	for _, e := range i.Metadata.Edges {
		es = append(es, map[string]any{"source": e.Source.Details.SourceID, "target": e.Target.Details.SourceID, "kind": e.Kind, "evidence": e.Evidence, "attributes": e.Attributes})
	}
	for name, v := range map[string]any{"graph.json": map[string]any{"game": "riven", "nodes": ns, "edges": es, "stats": i.Metadata.Stats, "typedEdges": len(es), "cards": i.Metadata.TotalCards, "startNodeID": i.Metadata.StartNodeID, "unresolved": i.Unresolved}, "cards.json": i.Stacks, "manifest.json": i.Manifest} {
		if err := write(name, v); err != nil {
			return err
		}
	}
	return nil
}
