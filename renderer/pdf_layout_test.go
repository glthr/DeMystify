package pdf

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	pdflayout "github.com/glthr/DeMystify/renderer/layout"
)

func TestPinnedPDFAndBrowserPreserveTheSamePreparedArrangement(t *testing.T) {
	if _, err := exec.LookPath("neato"); err != nil {
		t.Skip("Neato is not installed")
	}
	t.Chdir(t.TempDir())
	const source = `digraph G {
 graph[splines=true]; node[shape=box];
 "1"[pos="40,-80!"]; "2"[pos="540,-280!"];
 "1"->"2"[dir=both,arrowtail=odot];
 }`
	if err := os.WriteFile("placed.dot", []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := RenderPinnedPDFAndLayout("placed.dot")
	if err != nil {
		t.Fatal(err)
	}
	shared, err := pdflayout.FromJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	standalone, err := pdflayout.RunPinned(source)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(shared, standalone) {
		t.Fatal("PDF and HTML have different pinned drawing geometry")
	}
	first, second := shared.Nodes[0], shared.Nodes[1]
	if math.Abs(second.X-first.X-500) > .01 || math.Abs(second.Y-first.Y-200) > .01 {
		t.Fatalf("prepared relative positions changed: %+v %+v", first, second)
	}
	if len(shared.Edges) != 1 || len(shared.Edges[0].Head) == 0 || len(shared.Edges[0].Tail) == 0 {
		t.Fatal("pinned routing lost an edge or arrow")
	}
}

func TestRenderPDFAndLayoutShareOneNeatoInvocation(t *testing.T) {
	if _, err := exec.LookPath("neato"); err != nil {
		t.Skip("Neato is not installed")
	}
	t.Chdir(t.TempDir())
	const source = `digraph G { "1" [shape=box,fillcolor="#ffffba",style=filled]; "1" -> "2" [dir=both]; }`
	if err := os.WriteFile("graph.dot", []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := RenderPDFAndLayout("graph.dot")
	if err != nil {
		t.Fatal(err)
	}
	var drawing struct {
		BB      string `json:"bb"`
		Objects []struct {
			Name string `json:"name"`
			Pos  string `json:"pos"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(data, &drawing); err != nil {
		t.Fatal(err)
	}
	if drawing.BB == "" || len(drawing.Objects) != 2 || drawing.Objects[0].Pos == "" {
		t.Fatalf("missing shared layout geometry: %+v", drawing)
	}
	shared, err := pdflayout.FromJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	standalone, err := pdflayout.Run(source)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(shared, standalone) {
		t.Fatal("HTML-only and shared PDF+HTML rendering produce different drawing geometry")
	}
	pdf, err := os.ReadFile(pdfFilePath)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("missing PDF: %v", err)
	}
	for _, pattern := range []string{"generated/.graph-*.pdf", "generated/.graph-*.json"} {
		if leftovers, err := filepath.Glob(pattern); err != nil || len(leftovers) != 0 {
			t.Fatalf("temporary files remain: %v %v", leftovers, err)
		}
	}
}

func TestInvalidLayoutPreservesExistingPDF(t *testing.T) {
	previous := previousPDF(t)
	fakeNeato(t, `format=""
for arg do
  if [ "$arg" = "-Tpdf" ]; then format=pdf; fi
  if [ "$arg" = "-Tjson" ]; then format=json; fi
  if [ "$next" = yes ]; then
    if [ "$format" = pdf ]; then printf 'new PDF' > "$arg"; else printf 'invalid json' > "$arg"; fi
    next=no
  fi
  if [ "$arg" = "-o" ]; then next=yes; fi
done
`)
	if _, err := RenderPDFAndLayout("input.dot"); err == nil {
		t.Fatal("accepted an invalid drawing")
	}
	if got, err := os.ReadFile(pdfFilePath); err != nil || !bytes.Equal(got, previous) {
		t.Fatalf("previous PDF changed: %v", err)
	}
}
