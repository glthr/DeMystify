package layout

import (
	"strings"
	"testing"
)

func TestGraphvizDrawingPreservesOptionalNavigationKind(t *testing.T) {
	fixture := strings.Replace(drawingFixture, `"penwidth":"2","style":"dashed"`, `"penwidth":"2","style":"dashed","demystify_kind":"native-timer"`, 1)
	drawing, err := FromJSON([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(drawing.Edges) != 1 || drawing.Edges[0].Kind != "native-timer" {
		t.Fatalf("lost navigation kind needed by shared viewer layer filtering: %+v", drawing.Edges)
	}
}
