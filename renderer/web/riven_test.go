package web

import (
	"bytes"
	"github.com/glthr/DeMystify/common"
	"path/filepath"
	"testing"
)

func TestSharedExporterPreservesEngineCardDetailsAndEvidence(t *testing.T) {
	metadata := syntheticGraph(2)
	metadata.Game = "Riven"
	metadata.StartNodeID = 0
	attack := "</script><img src=x onerror=alert(1)>"
	metadata.Nodes[0].Details = &common.CardDetails{SourceID: "example:1", GlobalID: "0x12345678", Script: attack, ViewWidth: 608, ViewHeight: 392,
		Zones:      []common.InteractionZone{{ID: 7, Name: "forward", Type: "hotspot", Layer: "card", Script: attack, X: 5, Y: 8, Width: 100, Height: 200, Enabled: true, RawFlags: 3, Availability: "conditional", AvailabilityReason: "Switch door controls BLST 7"}},
		Provenance: map[string]any{"byteOffset": 123}, Unresolved: []map[string]any{{"kind": "saved-return"}}, PictureRecords: []map[string]any{{"index": 1, "imageID": 12}}}
	metadata.Edges[0].Kind = "native-timer"
	metadata.Edges[0].Attributes = append(metadata.Edges[0].Attributes, common.Native)
	metadata.Edges[0].Evidence = []map[string]any{{"byteOffset": 42, "guard": attack}}
	report, err := Write(filepath.Join(t.TempDir(), "atlas"), metadata, Options{})
	if err != nil {
		t.Fatal(err)
	}
	data, html := readExport(t, report.IndexPath)
	if data.Meta.Game != "Riven" || data.Nodes[0].SourceID != "example:1" || data.Nodes[0].RMAP != "0x12345678" {
		t.Fatalf("lost engine identity: %+v", data)
	}
	if data.Nodes[0].ScriptText != attack || data.Nodes[0].Zones[0].Script != attack || data.Nodes[0].Zones[0].RawFlags != 3 {
		t.Fatal("lost script/zone details")
	}
	if data.Nodes[0].Zones[0].Availability != "conditional" || data.Nodes[0].Zones[0].AvailabilityReason != "Switch door controls BLST 7" {
		t.Fatal("lost static hotspot availability proof")
	}
	if data.Edges[0].Kind != "native-timer" || data.Edges[0].Evidence[0]["guard"] != attack {
		t.Fatal("lost edge kind or evidence")
	}
	if bytes.Contains(html, []byte(attack)) {
		t.Fatal("engine detail escaped its script-safe JSON envelope")
	}
}
