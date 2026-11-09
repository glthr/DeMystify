package web

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/glthr/DeMystify/common"
	pdflayout "github.com/glthr/DeMystify/renderer/layout"
)

func syntheticPDFDrawing() pdflayout.Drawing {
	drawing := pdflayout.Drawing{Bounds: pdflayout.Bounds{MaxX: 500, MaxY: 300}}
	for i := 0; i < 3; i++ {
		x, y := 60+float64(i)*160, 120.0
		drawing.Nodes = append(drawing.Nodes, pdflayout.Node{ID: int64(i), X: x, Y: y, Width: 90, Height: 40,
			Fill: "#edf2e2", Stroke: "#123456", FontColor: "#000000", PenWidth: 1, Shape: "ellipse", Style: "filled",
			Draw:   []pdflayout.Operation{{Op: "E", Rect: []float64{x, y, 45, 20}}},
			Labels: []pdflayout.Operation{{Op: "F", Size: 12, Face: "serif"}, {Op: "T", Pt: []float64{x, y}, Align: "c", Width: 50, Text: "PDF label"}}})
	}
	drawing.Edges = []pdflayout.Edge{
		{Source: 0, Target: 1, Color: "#445566", Style: "solid", PenWidth: 1.5,
			Draw: []pdflayout.Operation{{Op: "b", Points: [][]float64{{100, 120}, {130, 95}, {150, 95}, {175, 120}}}},
			Head: []pdflayout.Operation{{Op: "P", Points: [][]float64{{175, 120}, {169, 116}, {169, 124}}}},
			Tail: []pdflayout.Operation{{Op: "P", Points: [][]float64{{100, 120}, {106, 116}, {106, 124}}}}},
		{Source: 1, Target: 2, Color: "#667788", Style: "dashed", PenWidth: 2},
	}
	return drawing
}

func TestPDFDrawingRemainsSeparateFromAtlasAndAllMetadataEdges(t *testing.T) {
	metadata := syntheticGraph(3)
	metadata.Edges = append(metadata.Edges,
		&common.Edge{Source: metadata.Nodes[0], Target: metadata.Nodes[1], Attributes: []common.EdgeAttribute{common.IntraAge}},
		&common.Edge{Source: metadata.Nodes[1], Target: metadata.Nodes[0], Attributes: []common.EdgeAttribute{common.IntraAge}})
	drawing := syntheticPDFDrawing()
	attack := "</script><img src=x onerror=alert(1)>"
	drawing.Nodes[0].Labels[1].Text = attack
	before, _ := json.Marshal(drawing)
	root := t.TempDir()
	atlasReport, err := Write(filepath.Join(root, "atlas"), metadata, Options{})
	if err != nil {
		t.Fatal(err)
	}
	atlas, _ := readExport(t, atlasReport.IndexPath)
	pdfReport, err := Write(filepath.Join(root, "pdf"), metadata, Options{PDFLayout: &drawing})
	if err != nil {
		t.Fatal(err)
	}
	pdf, html := readExport(t, pdfReport.IndexPath)
	if atlas.PDFLayout != nil || atlas.Meta.DefaultLayout != "atlas" || pdf.PDFLayout == nil || pdf.Meta.DefaultLayout != "pdf" {
		t.Fatal("default layout does not follow optional PDF drawing")
	}
	if !reflect.DeepEqual(atlas.Nodes, pdf.Nodes) || !reflect.DeepEqual(atlas.Edges, pdf.Edges) {
		t.Fatal("PDF drawing changed the native atlas or raw edge inventory")
	}
	if len(pdf.Edges) != 4 || len(pdf.PDFLayout.Edges) != 2 || pdfReport.Edges != 4 {
		t.Fatal("merged PDF strokes replaced original edge records")
	}
	if !reflect.DeepEqual(*pdf.PDFLayout, drawing) {
		t.Fatal("routed geometry, styles, labels or arrow operations changed")
	}
	if bytes.Contains(html, []byte(attack)) {
		t.Fatal("PDF label can terminate its embedded script")
	}
	after, _ := json.Marshal(drawing)
	if !bytes.Equal(before, after) {
		t.Fatal("export mutated supplied PDF drawing")
	}
}

func TestInvalidPDFDrawingFailsBeforeCreatingOrReplacingOutput(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*pdflayout.Drawing)
	}{
		{"missing node", func(d *pdflayout.Drawing) { d.Nodes = d.Nodes[:2] }},
		{"unknown node", func(d *pdflayout.Drawing) { d.Nodes[2].ID = 99 }},
		{"duplicate node", func(d *pdflayout.Drawing) { d.Nodes[2].ID = 1 }},
		{"missing edge endpoint", func(d *pdflayout.Drawing) { d.Edges[0].Target = 99 }},
		{"empty bounds", func(d *pdflayout.Drawing) { d.Bounds.MaxX = 0 }},
		{"nonfinite bounds", func(d *pdflayout.Drawing) { d.Bounds.MaxY = math.Inf(1) }},
		{"overflowing bounds span", func(d *pdflayout.Drawing) { d.Bounds.MinX = -math.MaxFloat64; d.Bounds.MaxX = math.MaxFloat64 }},
		{"nonfinite node", func(d *pdflayout.Drawing) { d.Nodes[1].X = math.NaN() }},
		{"nonpositive node width", func(d *pdflayout.Drawing) { d.Nodes[1].Width = 0 }},
		{"node outside bounds", func(d *pdflayout.Drawing) { d.Nodes[1].Y = -100 }},
		{"nonfinite route", func(d *pdflayout.Drawing) { d.Edges[0].Draw[0].Points[2][0] = math.NaN() }},
		{"nonfinite arrow", func(d *pdflayout.Drawing) { d.Edges[0].Head[0].Points[0][1] = math.Inf(-1) }},
		{"nonfinite label", func(d *pdflayout.Drawing) { d.Nodes[0].Labels[1].Pt[0] = math.Inf(1) }},
		{"nonfinite style", func(d *pdflayout.Drawing) { d.Nodes[0].PenWidth = math.NaN() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			metadata := syntheticGraph(3)
			drawing := syntheticPDFDrawing()
			test.change(&drawing)
			root := t.TempDir()
			fresh := filepath.Join(root, "new", "atlas")
			if _, err := Write(fresh, metadata, Options{PDFLayout: &drawing}); err == nil {
				t.Fatal("invalid PDF drawing accepted")
			}
			if _, err := os.Stat(filepath.Join(root, "new")); !os.IsNotExist(err) {
				t.Fatal("invalid drawing created output parent")
			}
			report, err := Write(filepath.Join(root, "working"), metadata, Options{})
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(report.IndexPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Write(report.Dir, metadata, Options{PDFLayout: &drawing}); err == nil {
				t.Fatal("invalid drawing replaced existing atlas")
			}
			if after, _ := os.ReadFile(report.IndexPath); !bytes.Equal(before, after) {
				t.Fatal("failed drawing refresh changed prior index")
			}
		})
	}
}

func TestPDFBezierControlPolygonMayExtendOutsideDrawingBounds(t *testing.T) {
	data, err := collectData(syntheticGraph(3), nil)
	if err != nil {
		t.Fatal(err)
	}
	drawing := syntheticPDFDrawing()
	drawing.Edges[0].Draw[0].Points[1][1] = -5
	if err := attachPDFLayout(&data, &drawing); err != nil {
		t.Fatalf("finite off-bounds Bezier control point rejected: %v", err)
	}
}

func TestNormalizedGraphvizCoordinatesSurviveHTMLExport(t *testing.T) {
	// Nonzero original bounds and unrelated _gvid values make both the
	// coordinate transform and endpoint identity mapping observable.
	raw := []byte(`{"bb":"10,20,510,320","objects":[
	 {"_gvid":10,"name":"0","pos":"60,200","width":"1.25","height":"0.55555556","_draw_":[{"op":"e","rect":[60,200,45,20]}],"_ldraw_":[{"op":"T","pt":[60,196],"align":"c","width":30,"text":"Dock"}]},
	 {"_gvid":20,"name":"1","pos":"260,200","width":"1.25","height":"0.55555556"},
	 {"_gvid":30,"name":"2","pos":"420,200","width":"1.25","height":"0.55555556"}],
	 "edges":[{"tail":10,"head":20,"_draw_":[{"op":"b","points":[[105,200],[140,225],[180,225],[215,200]]}]}]}`)
	drawing, err := pdflayout.FromJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Write(filepath.Join(t.TempDir(), "pdf"), syntheticGraph(3), Options{PDFLayout: &drawing})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := readExport(t, report.IndexPath)
	got := data.PDFLayout
	if got.Bounds != (pdflayout.Bounds{MaxX: 500, MaxY: 300}) || got.Nodes[0].ID != 0 || got.Nodes[0].X != 50 || got.Nodes[0].Y != 120 || got.Nodes[0].Width != 90 {
		t.Fatalf("point normalization changed: %+v", got)
	}
	if !reflect.DeepEqual(got.Nodes[0].Draw[0].Rect, []float64{50, 120, 45, 20}) || !reflect.DeepEqual(got.Nodes[0].Labels[0].Pt, []float64{50, 124}) {
		t.Fatal("ellipse radii or text baseline coordinates changed")
	}
	if got.Edges[0].Source != 0 || got.Edges[0].Target != 1 || !reflect.DeepEqual(got.Edges[0].Draw[0].Points[1], []float64{130, 95}) {
		t.Fatal("Bezier route or Graphviz endpoint identity changed")
	}
}

func TestLargeGraphvizRoundingDoesNotRejectValidNodeBoxes(t *testing.T) {
	// The five-significant-digit pos/bb fields are coarser than the draw paths
	// once coordinates exceed 10,000 points. Both center and bounds can round in
	// opposite directions; width/height are independently rounded in inches.
	raw := []byte(`{"bb":"0,0,13643,13935","objects":[
 {"_gvid":0,"name":"0","pos":"60,200","width":"1.25","height":"0.55555556"},
 {"_gvid":1,"name":"1","pos":"13593,10350","width":"1.3979","height":"0.43333"},
 {"_gvid":2,"name":"2","pos":"420,200","width":"1.25","height":"0.55555556"}]}`)
	drawing, err := pdflayout.FromJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	data, err := collectData(syntheticGraph(3), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := attachPDFLayout(&data, &drawing); err != nil {
		t.Fatalf("valid large-coordinate rounding rejected: %v", err)
	}
	// Exercise independent rounding on the other axis as well.
	drawing.Nodes[1].Y = 15
	if err := attachPDFLayout(&data, &drawing); err != nil {
		t.Fatalf("valid top-bound rounding rejected: %v", err)
	}
	drawing.Nodes[1].X += 2
	if err := attachPDFLayout(&data, &drawing); err == nil {
		t.Fatal("real out-of-bounds node accepted under rounding tolerance")
	}
}

func TestPDFNodeCornerOverflowCannotBeHiddenByRoundingTolerance(t *testing.T) {
	data, err := collectData(syntheticGraph(3), nil)
	if err != nil {
		t.Fatal(err)
	}
	drawing := syntheticPDFDrawing()
	drawing.Bounds.MaxX = math.MaxFloat64
	drawing.Nodes[1].X = math.MaxFloat64
	drawing.Nodes[1].Width = math.MaxFloat64
	if err := attachPDFLayout(&data, &drawing); err == nil {
		t.Fatal("overflowing node corners accepted")
	}
}
