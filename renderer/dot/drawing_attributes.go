package dot

import (
	"strings"

	"github.com/glthr/DeMystify/common"
)

// Drawing flags travel through Graphviz with the routed geometry. Consumers
// must not infer traversal rules from a line's color or merged endpoints.
func drawingEdgeAttributes(edge *common.Edge) string {
	var attributes []string
	for _, flag := range []struct {
		attribute common.EdgeAttribute
		name      string
	}{
		{common.Disabled, "disabled"},
		{common.SequenceContinuation, "sequenceContinuation"},
		{common.Backtracking, "backtracking"},
		{common.RestrictiveTransitivityTail, "restrictiveTransitivityTail"},
		{common.RestrictiveTransitivityHead, "restrictiveTransitivityHead"},
		{common.Conditional, "conditional"},
	} {
		if edge.IsOfType(flag.attribute) {
			attributes = append(attributes, flag.name)
		}
	}
	return strings.Join(attributes, ",")
}
