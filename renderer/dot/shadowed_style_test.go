package dot

import (
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
)

// Dotted grey, an ordinary grey arrowhead, and a flag in the file: neither the
// dashes and tee of a disabled reference nor a stroke merged with its reverse.
func TestShadowedEdgeHasItsOwnStyle(t *testing.T) {
	g, metadata := newTestGenerator(t, []common.EdgeAttribute{common.IntraAge, common.Shadowed})
	reverse := &common.Edge{Source: metadata.Nodes[1], Target: metadata.Nodes[0], Attributes: []common.EdgeAttribute{common.IntraAge}}
	if err := g.graph.AddEdge(reverse, 1); err != nil {
		t.Fatal(err)
	}
	output, err := g.Generate(nil)
	if err != nil {
		t.Fatal(err)
	}
	var shadowed string
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, `"0" -> "1"`) {
			shadowed = line
		}
	}
	for _, want := range []string{`style="dotted"`, `color="` + ShadowedColor + `"`, `demystify_attributes="shadowed"`, `tooltip="Shadowed reference`} {
		if !strings.Contains(shadowed, want) {
			t.Errorf("shadowed edge lacks %s: %s", want, shadowed)
		}
	}
	for _, unwanted := range []string{"tee", "dashed", "dir=both", "penwidth"} {
		if strings.Contains(shadowed, unwanted) {
			t.Errorf("shadowed edge carries %s: %s", unwanted, shadowed)
		}
	}
	if !strings.Contains(output, `"1" -> "0" [`) {
		t.Errorf("the active reverse edge was merged away:\n%s", output)
	}
}
