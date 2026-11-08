package dot

import (
	"regexp"
	"testing"

	"github.com/glthr/DeMystify/common"
)

func TestNameMappingsCannotHideOrSwapGraphNodes(t *testing.T) {
	for _, names := range []map[int64]string{{0: "Alias"}, {0: "B", 1: "A"}} {
		g, _ := newTestGenerator(t, []common.EdgeAttribute{common.IntraAge})
		g.graph.SetNameMapping(names)
		output, err := g.Generate(nil)
		if err != nil {
			t.Fatal(err)
		}
		if !regexp.MustCompile(`(?m)^  "0" \[.*label="A".*\];$`).MatchString(output) ||
			!regexp.MustCompile(`(?m)^  "1" \[.*label="B".*\];$`).MatchString(output) {
			t.Fatalf("mapping %v hid canonical graph labels", names)
		}
		if _, exists := g.stackColors["Myst"]; !exists {
			t.Fatal("name mapping removed the stack color")
		}
	}
}
