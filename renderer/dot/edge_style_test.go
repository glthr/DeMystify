package dot

import (
	"github.com/glthr/DeMystify/common"
	"math"
	"strings"
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

func restrictiveStyles(tailAttrs, headAttrs []common.EdgeAttribute) (edgeStyle, edgeStyle) {
	tail := &common.Edge{Source: &common.Node{StackName: "source"}, TransitivityID: 1, Attributes: append([]common.EdgeAttribute{common.RestrictiveTransitivityTail}, tailAttrs...)}
	head := &common.Edge{Source: &common.Node{StackName: "target"}, TransitivityID: 1, Attributes: append([]common.EdgeAttribute{common.RestrictiveTransitivityHead}, headAttrs...)}
	g := &Generator{metadata: &common.Metadata{Edges: []*common.Edge{tail, head}}, stackColors: map[string]nodeColors{"source": {borderColor: "#123456"}, "target": {borderColor: "#654321"}}}
	return g.applyEdgeStyle(tail, false), g.applyEdgeStyle(head, false)
}

func TestRestrictiveHeadSharesTailAppearance(t *testing.T) {
	for _, attrs := range [][]common.EdgeAttribute{nil, {common.CrossAge}} {
		tail, head := restrictiveStyles(attrs, attrs)
		if head.color != tail.color || head.penWidth != tail.penWidth {
			t.Fatalf("appearance differs: tail=%+v head=%+v", tail, head)
		}
		if !strings.Contains(head.tooltip, "Head ID1") && !strings.Contains(head.tooltip, "Head ID 1") {
			t.Fatalf("missing head ID: %s", head.tooltip)
		}
		if strings.Contains(head.tooltip, "Tail") {
			t.Fatalf("tail tooltip leaked: %s", head.tooltip)
		}
	}
}

func TestRestrictiveHeadKeepsOwnState(t *testing.T) {
	for _, attr := range []common.EdgeAttribute{common.Disabled, common.Backtracking} {
		_, head := restrictiveStyles(nil, []common.EdgeAttribute{attr})
		arrow, tooltip := "tee", "disabled"
		if attr == common.Backtracking {
			arrow, tooltip = "inv", "pop"
		}
		if head.arrowhead != arrow || !strings.Contains(strings.ToLower(head.tooltip), tooltip) {
			t.Fatalf("head state lost: %+v", head)
		}
	}
}

func TestRestrictiveHeadDoesNotInheritTailState(t *testing.T) {
	for _, attr := range []common.EdgeAttribute{common.Disabled, common.Backtracking} {
		_, head := restrictiveStyles([]common.EdgeAttribute{attr}, nil)
		tooltip := strings.ToLower(head.tooltip)
		if head.arrowhead != "normal" || head.arrowtail != "" || strings.Contains(tooltip, "disabled") || strings.Contains(tooltip, "pop") {
			t.Fatalf("tail state leaked: %+v", head)
		}
	}
}

func TestSequenceContinuationUsesHollowArrowheadAndExplanation(t *testing.T) {
	g, metadata := newTestGenerator(t)
	edge := &common.Edge{Source: metadata.Nodes[0], Target: metadata.Nodes[1], Attributes: []common.EdgeAttribute{common.IntraAge, common.SequenceContinuation}}
	style := g.applyEdgeStyle(edge, false)
	if !strings.Contains(style.tooltip, "Ordered literal continuation") || !strings.Contains(style.tooltip, "not followed by path search") || style.arrowtail != "" || style.arrowhead != "onormal" || style.bidirectional {
		t.Fatalf("an automatic step was not marked as one: %+v", style)
	}
	if got := g.buildEdgeStyleString(style); !strings.Contains(got, `arrowhead="onormal"`) || strings.Contains(got, "dir=") {
		t.Fatalf("unexpected DOT attributes for a continuation: %s", got)
	}
	edge.Attributes = append(edge.Attributes, common.Disabled)
	style = g.applyEdgeStyle(edge, true)
	if style.arrowhead != "tee" || !strings.Contains(style.tooltip, "disabled") || !strings.Contains(style.tooltip, "Ordered literal continuation") {
		t.Fatalf("path/disabled state lost its explanation or tee: %+v", style)
	}
}

func TestPathHighlightPrefersTheOrdinaryReferenceOverAContinuation(t *testing.T) {
	g, metadata := newTestGenerator(t)
	a, b := metadata.Nodes[0], metadata.Nodes[1]
	// The intermediate card's own reference and a caller's continuation can
	// join the same two cards; only the first is a route.
	metadata.Edges = []*common.Edge{
		{Source: a, Target: b, Attributes: []common.EdgeAttribute{common.IntraAge, common.SequenceContinuation}},
		{Source: a, Target: b, Attributes: []common.EdgeAttribute{common.IntraAge}},
	}
	output, err := g.Generate([]int64{a.GraphID, b.GraphID})
	if err != nil {
		t.Fatal(err)
	}
	lines := edgeLines(output)
	if len(lines) != 2 {
		t.Fatalf("parallel records lost: %v", lines)
	}
	for _, line := range lines {
		if strings.Contains(line, "Path edge") == strings.Contains(line, "onormal") {
			t.Fatalf("the highlight was not given to the ordinary reference alone: %v", lines)
		}
	}
}

func TestSequenceContinuationsAreNotMergedIntoBidirectionalMoves(t *testing.T) {
	g, metadata := newTestGenerator(t)
	a, b := metadata.Nodes[0], metadata.Nodes[1]
	metadata.Edges = []*common.Edge{
		{Source: a, Target: b, Attributes: []common.EdgeAttribute{common.IntraAge, common.SequenceContinuation}},
		{Source: b, Target: a, Attributes: []common.EdgeAttribute{common.IntraAge, common.SequenceContinuation}},
	}
	output, err := g.Generate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(edgeLines(output)) != 2 || strings.Contains(output, "dir=both") {
		t.Fatalf("opposite ordered steps were merged: %s", output)
	}
}
