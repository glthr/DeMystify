package dot

import (
	"github.com/glthr/DeMystify/common"
	"math"
	"testing"
)

func TestRestrictiveTailSupportsNegativeIDs(t *testing.T) {
	g := &Generator{}
	for _, id := range []int64{-1, -7, math.MinInt64} {
		edge := &common.Edge{Attributes: []common.EdgeAttribute{common.RestrictiveTransitivityTail}, TransitivityID: id}
		style := g.applyEdgeStyle(edge, false)
		if style.color == "" || style.arrowhead != "none" || style.penWidth != 1.2 {
			t.Fatalf("ID%d: invalid tail style %+v", id, style)
		}
	}
}
