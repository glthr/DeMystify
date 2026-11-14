package layout

import (
	"reflect"
	"strings"
	"testing"
)

func TestGraphvizDrawingRetainsTraversalFlags(t *testing.T) {
	fixture := strings.Replace(drawingFixture, `"penwidth":"2","style":"dashed"`, `"penwidth":"2","style":"dashed","demystify_attributes":"disabled,sequenceContinuation,conditional"`, 1)
	drawing, err := FromJSON([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(drawing.Edges) != 1 || !reflect.DeepEqual(drawing.Edges[0].Attributes, []string{"disabled", "sequenceContinuation", "conditional"}) {
		t.Fatalf("lost traversal flags: %+v", drawing.Edges)
	}
}
