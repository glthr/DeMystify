package layout

import (
	"context"
	"encoding/json"
	"math"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

const drawingFixture = `{
 "bb":"10,20,210,120", "_subgraph_cnt":1,
 "objects":[
  {"_gvid":40,"name":"cluster_hidden","nodes":[5,9],"style":"invis"},
  {"_gvid":99,"name":"__TOP__","style":"invis"},
  {"_gvid":5,"name":"0","pos":"60,80","width":"1","height":"0.5","shape":"ellipse","penwidth":"2",
   "_draw_":[{"op":"c","color":"#123456"},{"op":"C","color":"#aaffaa"},{"op":"E","rect":[60,80,36,18]}],
   "_ldraw_":[{"op":"F","face":"Arial","size":10},{"op":"c","color":"#112233"},{"op":"T","pt":[60,75],"align":"c","width":24,"text":"Dock"}]},
  {"_gvid":9,"name":"1","pos":"170,50","width":"1","height":"0.5"}
 ],
 "edges":[
  {"tail":99,"head":5,"style":"invis"},
  {"tail":5,"head":9,"style":"invis"},
  {"tail":5,"head":9,"penwidth":"2","style":"dashed",
   "_draw_":[{"op":"c","color":"#123456"},{"op":"b","points":[[96,80],[108,85],[123,55],[134,50]]}],
   "_hdraw_":[{"op":"P","points":[[130,47],[140,50],[130,53]]}],
   "_tdraw_":[{"op":"e","rect":[96,80,3,3]}],
   "_ldraw_":[{"op":"T","pt":[110,70],"align":"l","width":20,"text":"route"}]}
 ]
}`

func TestFromJSONPreservesRoutesLabelsAndGraphIdentity(t *testing.T) {
	drawing, err := FromJSON([]byte(drawingFixture))
	if err != nil {
		t.Fatal(err)
	}
	if drawing.Bounds != (Bounds{MaxX: 200, MaxY: 100}) || len(drawing.Nodes) != 2 || len(drawing.Edges) != 1 {
		t.Fatalf("incorrect bounds/inventory: %+v", drawing)
	}
	node := drawing.Nodes[0]
	if node.ID != 0 || node.X != 50 || node.Y != 40 || node.Width != 72 || node.Height != 36 {
		t.Fatalf("node point/inch conversion: %+v", node)
	}
	if node.Fill != "#aaffaa" || node.Stroke != "#123456" || node.FontColor != "#112233" || node.PenWidth != 2 {
		t.Fatalf("lost PDF paint: %+v", node)
	}
	if !reflect.DeepEqual(node.Draw[2].Rect, []float64{50, 40, 36, 18}) || !reflect.DeepEqual(node.Labels[2].Pt, []float64{50, 45}) {
		t.Fatal("ellipse radii or label baseline were transformed incorrectly")
	}
	edge := drawing.Edges[0]
	if edge.Source != 0 || edge.Target != 1 || edge.Style != "dashed" || edge.PenWidth != 2 || edge.Color != "#123456" {
		t.Fatalf("edge endpoints must resolve via _gvid, not array indices: %+v", edge)
	}
	if edge.Draw[1].Op != "b" || !reflect.DeepEqual(edge.Draw[1].Points, [][]float64{{86, 40}, {98, 35}, {113, 65}, {124, 70}}) {
		t.Fatal("Bezier controls or opcode case changed")
	}
	if edge.Head[0].Op != "P" || !reflect.DeepEqual(edge.Tail[0].Rect, []float64{86, 40, 3, 3}) || edge.Labels[0].Text != "route" {
		t.Fatal("arrowheads, tail marker or edge label were dropped")
	}
}

func TestFromJSONRejectsMalformedOrUnsafeDrawings(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
	}{
		{"nonfinite bounds", `10,20,210,120`, `0,0,Inf,120`},
		{"empty bounds", `10,20,210,120`, `0,0,0,120`},
		{"missing bounds", `"bb":"10,20,210,120",`, ``},
		{"nonfinite position", `"60,80"`, `"NaN,80"`},
		{"zero width", `"width":"1"`, `"width":"0"`},
		{"overflowed point width", `"width":"1"`, `"width":"1e308"`},
		{"noncanonical node ID", `"name":"0"`, `"name":"00"`},
		{"visible nonnumeric ID", `"name":"0"`, `"name":"anchor"`},
		{"duplicate graph ID", `"name":"1"`, `"name":"0"`},
		{"duplicate Graphviz ID", `"_gvid":9,`, `"_gvid":5,`},
		{"unknown endpoint", `"head":9,"penwidth"`, `"head":500,"penwidth"`},
		{"negative pen width", `"penwidth":"2"`, `"penwidth":"-1"`},
		{"bad curve", `[[96,80],[108,85],[123,55],[134,50]]`, `[[96,80],[108,85],[123,55]]`},
		{"bad point arity", `[[96,80],[108,85]`, `[[96],[108,85]`},
		{"negative radius", `[60,80,36,18]`, `[60,80,-36,18]`},
		{"external image", `{"op":"E","rect":[60,80,36,18]}`, `{"op":"I","rect":[0,0,50,50],"name":"https://example.invalid/image.png"}`},
		{"unknown operation", `"op":"E"`, `"op":"Z"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := strings.Replace(drawingFixture, tc.before, tc.after, 1)
			if data == drawingFixture {
				t.Fatal("test did not change fixture")
			}
			if _, err := FromJSON([]byte(data)); err == nil {
				t.Fatal("accepted malformed drawing")
			}
		})
	}
}

func TestFromJSONDoesNotClampBezierControlsAndFlipsGradients(t *testing.T) {
	data := strings.Replace(drawingFixture, `[108,85]`, `[-100,500]`, 1)
	data = strings.Replace(data, `{"op":"C","color":"#aaffaa"}`, `{"op":"C","grad":"radial","p0":[60,80,4],"p1":[70,70,20],"stops":[{"frac":0,"color":"#ffffff"},{"frac":1,"color":"#000000"}]}`, 1)
	drawing, err := FromJSON([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(drawing.Edges[0].Draw[1].Points[1], []float64{-110, -380}) {
		t.Fatal("legitimate off-bounds Bezier control point was clamped")
	}
	gradient := drawing.Nodes[0].Draw[1]
	if !reflect.DeepEqual(gradient.P0, []float64{50, 40, 4}) || !reflect.DeepEqual(gradient.P1, []float64{60, 50, 20}) {
		t.Fatal("gradient centers/radii transformed incorrectly")
	}
	if _, err := json.Marshal(drawing); err != nil {
		t.Fatal(err)
	}
}

func TestRunUsesInstalledNeatoRoutesAndBothArrows(t *testing.T) {
	if _, err := exec.LookPath("neato"); err != nil {
		t.Skip("neato is not installed")
	}
	drawing, err := Run(`digraph G {
 node[shape=box,style="filled,rounded",fillcolor="#d9fcc5",fontsize=10];
 "__TOP__"[style=invis];"__TOP__"->0[style=invis];
 0[label="Dock"];1[label="Library"];
 0->1[dir=both,arrowtail=odot,arrowhead=vee,color="#123456",penwidth=2];
 1->1[style=dashed];
 }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(drawing.Nodes) != 2 || len(drawing.Edges) != 2 || drawing.Bounds.MaxX <= 0 || drawing.Bounds.MaxY <= 0 {
		t.Fatalf("wrong real Neato inventory: %+v", drawing)
	}
	for _, node := range drawing.Nodes {
		if len(node.Draw) == 0 || len(node.Labels) == 0 || node.Width <= 0 || node.Height <= 0 || math.IsNaN(node.X) {
			t.Fatalf("incomplete real node geometry: %+v", node)
		}
	}
	paired := drawing.Edges[0]
	if paired.Source != 0 || paired.Target != 1 || len(paired.Head) == 0 || len(paired.Tail) == 0 {
		t.Fatal("Neato's merged reciprocal arrow geometry was lost")
	}
	if _, err := Run("digraph {"); err == nil {
		t.Fatal("malformed DOT unexpectedly succeeded")
	}
}

func TestRunContextCancellationAndBoundedOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RunContext(ctx, "digraph {0}"); err == nil {
		t.Fatal("ignored canceled layout context")
	}
	output := cappedOutput{maximum: 4}
	if _, err := output.Write([]byte("12345")); err == nil || output.buffer.Len() != 4 {
		t.Fatal("layout output was not bounded")
	}
}
