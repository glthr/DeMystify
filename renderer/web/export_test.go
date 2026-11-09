package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/hypercard"
)

func syntheticGraph(count int) *common.Metadata {
	metadata := &common.Metadata{TotalCards: uint(count), TotalStacks: 2}
	for i := 0; i < count; i++ {
		stack := "First Age"
		if i%3 == 0 {
			stack = "Second Age"
		}
		metadata.Nodes = append(metadata.Nodes, &common.Node{GraphID: int64(i), Name: fmt.Sprintf("%s:%d", stack, i), StackName: stack, Attributes: []common.NodeAttribute{common.IsCard}})
	}
	for i := 1; i < count; i++ {
		metadata.Edges = append(metadata.Edges, &common.Edge{Source: metadata.Nodes[i-1], Target: metadata.Nodes[i], Attributes: []common.EdgeAttribute{common.IntraAge}})
	}
	return metadata
}

func readExport(t *testing.T, filename string) (graphData, []byte) {
	t.Helper()
	html, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "window.DEMYSTIFY_GRAPH="
	start := bytes.Index(html, []byte(prefix))
	if start < 0 {
		t.Fatal("no embedded graph")
	}
	rest := html[start+len(prefix):]
	end := bytes.Index(rest, []byte(";</script>"))
	if end < 0 {
		t.Fatal("embedded graph script is unterminated")
	}
	var data graphData
	if err := json.Unmarshal(rest[:end], &data); err != nil {
		t.Fatal(err)
	}
	return data, html
}

func TestWriteOfflinePreservesAllEdgeRecordsAndEscapesData(t *testing.T) {
	metadata := syntheticGraph(3)
	attack := "</script><img id=attack src=x onerror=alert(1)>\u2028&"
	metadata.Nodes[0].Name = attack
	metadata.Nodes[0].OriginalName = &attack
	picture := "Original picture label"
	metadata.Nodes[0].SecondaryName = &picture
	metadata.Edges = append(metadata.Edges,
		&common.Edge{Source: metadata.Nodes[0], Target: metadata.Nodes[1], Attributes: []common.EdgeAttribute{common.IntraAge}},
		&common.Edge{Source: metadata.Nodes[1], Target: metadata.Nodes[0], Attributes: []common.EdgeAttribute{common.CrossAge, common.Backtracking}},
		&common.Edge{Source: metadata.Nodes[0], Target: metadata.Nodes[0], Attributes: []common.EdgeAttribute{common.SelfReference, common.Disabled}},
		&common.Edge{Source: metadata.Nodes[1], Target: metadata.Nodes[2], Attributes: []common.EdgeAttribute{common.RestrictiveTransitivityTail}, TransitivityID: 7},
		&common.Edge{Source: metadata.Nodes[2], Target: metadata.Nodes[0], Attributes: []common.EdgeAttribute{common.RestrictiveTransitivityHead}, TransitivityID: 7})
	metadata.Edges = append(metadata.Edges, &common.Edge{Source: metadata.Nodes[2], Target: metadata.Nodes[1], Attributes: []common.EdgeAttribute{common.RestrictiveTransitivityHead}, TransitivityID: 0})
	metadata.Edges = append(metadata.Edges, &common.Edge{Source: metadata.Nodes[0], Target: metadata.Nodes[2], Attributes: []common.EdgeAttribute{common.SequenceContinuation}})
	before, _ := json.Marshal(metadata)
	report, err := Write(filepath.Join(t.TempDir(), "atlas"), metadata, Options{})
	if err != nil {
		t.Fatal(err)
	}
	data, html := readExport(t, report.IndexPath)
	if len(data.Nodes) != 3 || len(data.Edges) != len(metadata.Edges) || data.Meta.Edges != len(metadata.Edges) {
		t.Fatalf("lost graph records: %+v", data.Meta)
	}
	parallel := 0
	for _, edge := range data.Edges {
		if edge.Source == 0 && edge.Target == 1 {
			parallel++
		}
	}
	if parallel != 2 {
		t.Fatalf("parallel edge records collapsed: %d", parallel)
	}
	if data.Nodes[0].Name != attack || data.Nodes[0].OriginalName != attack {
		t.Fatal("name did not round-trip")
	}
	if data.Nodes[0].AssetName != picture {
		t.Fatal("picture label was lost without an artwork directory")
	}
	if bytes.Contains(html, []byte(attack)) || bytes.Contains(html, []byte("{{GRAPH_JSON}}")) {
		t.Fatal("unescaped data or template token in HTML")
	}
	if !bytes.Contains(html, []byte(`\u003c/script\u003e`)) {
		t.Fatal("script terminator not escaped")
	}
	if !bytes.Contains(html, []byte(`"attributes":["restrictiveTransitivityHead"],"transitivityId":0`)) {
		t.Fatal("meaningful zero transitivity ID was lost")
	}
	if !bytes.Contains(html, []byte(`"attributes":["sequenceContinuation"]`)) {
		t.Fatal("ordered navigation continuation was lost")
	}
	if !bytes.HasPrefix(html, []byte(ownershipMarker)) {
		t.Fatal("export ownership marker missing")
	}
	after, _ := json.Marshal(metadata)
	if !bytes.Equal(before, after) {
		t.Fatal("export changed input graph")
	}
	if report.Nodes != 3 || report.Edges != len(metadata.Edges) || report.Images != 0 || report.MissingImages != 3 || data.Meta.MissingImages != 3 {
		t.Fatalf("wrong report: %+v", report)
	}
}

func TestMissingImageCountExcludesStackAndVirtualNodes(t *testing.T) {
	metadata := syntheticGraph(3)
	metadata.Nodes[1].Attributes = []common.NodeAttribute{common.IsStack}
	metadata.Nodes[2].Attributes = []common.NodeAttribute{common.IsCard, common.IsVirtual}
	report, err := Write(filepath.Join(t.TempDir(), "atlas"), metadata, Options{})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := readExport(t, report.IndexPath)
	if report.MissingImages != 1 || data.Meta.MissingImages != 1 || report.Images != 0 {
		t.Fatalf("missing-image eligibility is incorrect: %+v / %+v", report, data.Meta)
	}
}

func TestLayoutIsDeterministicAndRectanglesNeverOverlap(t *testing.T) {
	metadata := syntheticGraph(120)
	first, err := collectData(metadata, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range first.Nodes {
		if i%2 == 0 {
			first.Nodes[i].Height = 132
		}
	}
	layout(&first)
	for a, node := range first.Nodes {
		if node.X-node.Width/2 < first.Meta.Layout.MinX || node.Y-node.Height/2 < first.Meta.Layout.MinY || node.X+node.Width/2 > first.Meta.Layout.MaxX || node.Y+node.Height/2 > first.Meta.Layout.MaxY {
			t.Fatalf("node outside declared bounds: %+v", node)
		}
		for b := a + 1; b < len(first.Nodes); b++ {
			other := first.Nodes[b]
			if abs(node.X-other.X) < (node.Width+other.Width)/2 && abs(node.Y-other.Y) < (node.Height+other.Height)/2 {
				t.Fatalf("overlapping node rectangles %d and %d", node.ID, other.ID)
			}
		}
	}
	for i, j := 0, len(metadata.Nodes)-1; i < j; i, j = i+1, j-1 {
		metadata.Nodes[i], metadata.Nodes[j] = metadata.Nodes[j], metadata.Nodes[i]
	}
	for i, j := 0, len(metadata.Edges)-1; i < j; i, j = i+1, j-1 {
		metadata.Edges[i], metadata.Edges[j] = metadata.Edges[j], metadata.Edges[i]
	}
	second, err := collectData(metadata, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range second.Nodes {
		if i%2 == 0 {
			second.Nodes[i].Height = 132
		}
	}
	layout(&second)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("layout depends on input enumeration order")
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestWriteRejectsInvalidGraphBeforeCreatingOutput(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*common.Metadata)
	}{
		{"duplicate IDs", func(m *common.Metadata) { m.Nodes[1].GraphID = m.Nodes[0].GraphID }},
		{"missing endpoint", func(m *common.Metadata) { m.Edges[0].Target = &common.Node{GraphID: 99} }},
		{"nil node", func(m *common.Metadata) { m.Nodes[0] = nil }},
		{"nil edge", func(m *common.Metadata) { m.Edges[0] = nil }},
		{"unsafe numeric ID", func(m *common.Metadata) { m.Nodes[0].GraphID = 1 << 54 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := syntheticGraph(3)
			test.change(m)
			target := filepath.Join(t.TempDir(), "output")
			if _, err := Write(target, m, Options{}); err == nil {
				t.Fatal("invalid graph accepted")
			}
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatal("invalid graph created output")
			}
		})
	}
}

func TestWriteProtectsOriginalDirectoriesAndUnrelatedOutput(t *testing.T) {
	base := t.TempDir()
	input := filepath.Join(base, "originals")
	if err := os.Mkdir(input, 0755); err != nil {
		t.Fatal(err)
	}
	for _, options := range []Options{{InputDir: input}, {AssetsDir: input}} {
		for _, target := range []string{input, filepath.Join(input, "output"), base} {
			if _, err := Write(target, syntheticGraph(2), options); err == nil || !strings.Contains(err.Error(), "overlap") {
				t.Fatalf("unsafe output %s: %v", target, err)
			}
		}
	}
	unrelated := filepath.Join(base, "unrelated")
	if err := os.Mkdir(unrelated, 0755); err != nil {
		t.Fatal(err)
	}
	original := []byte("<h1>User document</h1>")
	index := filepath.Join(unrelated, "index.html")
	if err := os.WriteFile(index, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(unrelated, syntheticGraph(2), Options{}); err == nil {
		t.Fatal("unrelated index overwritten")
	}
	if actual, _ := os.ReadFile(index); !bytes.Equal(actual, original) {
		t.Fatal("user index changed")
	}
	link := filepath.Join(base, "output-link")
	if err := os.Symlink(input, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(link, syntheticGraph(2), Options{}); err == nil {
		t.Fatal("symlink output accepted")
	}
	alias := filepath.Join(base, "original-alias")
	if err := os.Symlink(input, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(filepath.Join(input, "nested"), syntheticGraph(2), Options{InputDir: alias}); err == nil {
		t.Fatal("symlink alias bypassed original protection")
	}
}

func TestWriteRefreshIsDeterministicAndFailureKeepsPriorIndex(t *testing.T) {
	target := filepath.Join(t.TempDir(), "atlas")
	report, err := Write(target, syntheticGraph(4), Options{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(report.IndexPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Write(target, syntheticGraph(4), Options{}); err != nil {
		t.Fatal(err)
	}
	if second, _ := os.ReadFile(report.IndexPath); !bytes.Equal(first, second) {
		t.Fatal("same graph produced different HTML")
	}
	if _, err := Write(target, syntheticGraph(4), Options{AssetsDir: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("missing image source accepted")
	}
	if after, _ := os.ReadFile(report.IndexPath); !bytes.Equal(first, after) {
		t.Fatal("failed refresh replaced working index")
	}
}

func TestImageURLsAreLocalAndRequireRealFiles(t *testing.T) {
	root := t.TempDir()
	name := "name with # and ?.png"
	if err := os.WriteFile(filepath.Join(root, name), []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := imageURL(root, "images-hash", name)
	if err != nil {
		t.Fatal(err)
	}
	if got != "images-hash/name%20with%20%23%20and%20%3F.png" {
		t.Fatalf("image URL = %q", got)
	}
	for _, bad := range []string{"", "../outside.png", "/absolute.png", "x/../inside.png", "x\\evil.png", "missing.png"} {
		if _, err := imageURL(root, "images-hash", bad); err == nil {
			t.Errorf("unsafe/missing image accepted: %q", bad)
		}
	}
	if err := os.Symlink(filepath.Join(root, name), filepath.Join(root, "link.png")); err != nil {
		t.Fatal(err)
	}
	if _, err := imageURL(root, "images-hash", "link.png"); err == nil {
		t.Fatal("image symlink accepted")
	}
}

func TestZonesUseOwnedBackgroundAndOriginalViewCoordinates(t *testing.T) {
	metadata := syntheticGraph(2)
	metadata.Nodes[0].Name = "First Age:10"
	metadata.Nodes[0].StackName = "First Age"
	corpus := hypercard.Corpus{Stacks: []hypercard.Stack{{Name: "First Age", Width: 544, Height: 333,
		Backgrounds: []hypercard.Background{{ID: 2, Parts: []hypercard.Part{{ID: 1, Type: "button", Name: "Shared", Rect: hypercard.Rectangle{Top: 10, Left: 20, Bottom: 50, Right: 80}, Enabled: true}}}, {ID: 3, Parts: []hypercard.Part{{ID: 99, Type: "button", Rect: hypercard.Rectangle{Bottom: 10, Right: 10}}}}},
		Cards:       []hypercard.Card{{ID: 10, BackgroundID: 2, Parts: []hypercard.Part{{ID: 1, Type: "field", Name: "Card field", Rect: hypercard.Rectangle{Top: 100, Left: 200, Bottom: 130, Right: 260}, Script: hypercard.Script{Text: "on mouseUp\ngo this card\nend mouseUp"}, Hidden: true, Enabled: false}, {ID: 9, Rect: hypercard.Rectangle{Top: 10, Bottom: 5}}}}}}}}
	data, err := collectData(metadata, &corpus)
	if err != nil {
		t.Fatal(err)
	}
	node := data.Nodes[0]
	if node.ViewWidth != 544 || node.ViewHeight != 333 || len(node.Zones) != 2 || data.Meta.Zones != 2 {
		t.Fatalf("geometry inventory = %+v", node)
	}
	shared, card := node.Zones[0], node.Zones[1]
	if shared.Layer != "background" || shared.X != 20 || shared.Y != 10 || shared.Width != 60 || shared.Height != 40 || shared.Name != "Shared" || !shared.Enabled {
		t.Fatalf("wrong background zone: %+v", shared)
	}
	if card.Layer != "card" || card.ID != 1 || card.X != 200 || !card.Hidden || card.Enabled || card.Script == "" {
		t.Fatalf("wrong card zone: %+v", card)
	}
}

func TestWriteCopiesVersionedImagesAndKeepsEarlierViewsUsable(t *testing.T) {
	metadata := syntheticGraph(2)
	base := t.TempDir()
	source, target := filepath.Join(base, "original-pictures"), filepath.Join(base, "atlas")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := struct {
		Images []map[string]string `json:"images"`
	}{Images: []map[string]string{
		{"path": "view.png", "node": metadata.Nodes[0].Name}, {"path": "view.png", "node": metadata.Nodes[1].Name},
	}}
	encoded, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(source, "images.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	makePicture := func(c color.RGBA) []byte {
		picture := image.NewRGBA(image.Rect(0, 0, 64, 40))
		for y := 0; y < 40; y++ {
			for x := 0; x < 64; x++ {
				picture.SetRGBA(x, y, c)
			}
		}
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, picture); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "view.png"), buffer.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}
	firstOriginal := makePicture(color.RGBA{R: 200, A: 255})
	report, err := Write(target, metadata, Options{AssetsDir: source})
	if err != nil {
		t.Fatal(err)
	}
	first, firstHTML := readExport(t, report.IndexPath)
	if report.Images != 2 || report.MissingImages != 0 || first.Meta.Images != 2 {
		t.Fatalf("artwork coverage: %+v", report)
	}
	if first.Nodes[0].Image == "" || first.Nodes[0].Image != first.Nodes[1].Image || first.Nodes[0].Thumbnail == "" || first.Nodes[0].ImageWidth != 64 || first.Nodes[0].ImageHeight != 40 {
		t.Fatalf("image metadata: %+v", first.Nodes)
	}
	localPath := func(imageURL string) string {
		decoded, err := url.PathUnescape(imageURL)
		if err != nil {
			t.Fatal(err)
		}
		return filepath.Join(target, filepath.FromSlash(decoded))
	}
	oldFull := localPath(first.Nodes[0].Image)
	if actual, err := os.ReadFile(oldFull); err != nil || !bytes.Equal(actual, firstOriginal) {
		t.Fatalf("full artwork bytes changed: %v", err)
	}
	if actual, _ := os.ReadFile(filepath.Join(source, "view.png")); !bytes.Equal(actual, firstOriginal) {
		t.Fatal("source artwork changed during export")
	}
	if _, err := Write(target, metadata, Options{AssetsDir: source}); err != nil {
		t.Fatalf("reusing an immutable artwork version: %v", err)
	}
	if actual, _ := os.ReadFile(report.IndexPath); !bytes.Equal(actual, firstHTML) {
		t.Fatal("unchanged artwork changed the exported HTML")
	}
	secondOriginal := makePicture(color.RGBA{B: 220, A: 255})
	report, err = Write(target, metadata, Options{AssetsDir: source})
	if err != nil {
		t.Fatal(err)
	}
	second, priorHTML := readExport(t, report.IndexPath)
	if first.Nodes[0].Image == second.Nodes[0].Image {
		t.Fatal("changed artwork reused a mutable URL")
	}
	if actual, _ := os.ReadFile(oldFull); !bytes.Equal(actual, firstOriginal) {
		t.Fatal("refresh replaced artwork used by prior index")
	}
	if actual, err := os.ReadFile(localPath(second.Nodes[0].Image)); err != nil || !bytes.Equal(actual, secondOriginal) {
		t.Fatalf("new artwork missing: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "view.png"), []byte("invalid image"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(target, metadata, Options{AssetsDir: source}); err == nil {
		t.Fatal("invalid image accepted")
	}
	if actual, _ := os.ReadFile(report.IndexPath); !bytes.Equal(actual, priorHTML) {
		t.Fatal("failed artwork refresh changed prior index")
	}
}
