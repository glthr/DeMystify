package parser

import (
	"testing"

	"github.com/glthr/DeMystify/hypercard"
)

func TestXMLPreservesInteractiveGeometryAndBooleanForms(t *testing.T) {
	dir := writeCorpusXML(t, map[string]string{
		stackFileName: `<stack><name>Example</name><cardSize><width>544</width><height>332</height></cardSize></stack>`,
		"card_1.xml":  `<card><id>1</id><part><id>4</id><type>button</type><name>Door</name><visible><false/></visible><enabled>false</enabled><rect><top>-4</top><left>10</left><bottom>120</bottom><right>200</right></rect></part><part><id>5</id><type>field</type><visible>true</visible><enabled><true/></enabled></part></card>`,
	})
	corpus, err := LoadXML(dir)
	if err != nil {
		t.Fatal(err)
	}
	stack := corpus.Stacks[0]
	if stack.Width != 544 || stack.Height != 332 {
		t.Fatalf("card size: %dx%d", stack.Width, stack.Height)
	}
	got := stack.Cards[0].Parts[0]
	if got.Name != "Door" || !got.Hidden || got.Enabled || got.Rect != (hypercard.Rectangle{Top: -4, Left: 10, Bottom: 120, Right: 200}) {
		t.Fatalf("geometry/flags: %+v", got)
	}
	if other := stack.Cards[0].Parts[1]; other.Hidden || !other.Enabled {
		t.Fatalf("visible/enabled field: %+v", other)
	}
}

func TestXMLNormalizesUnsignedQuickDrawCoordinates(t *testing.T) {
	got := xmlRectangle(hypercard.Rectangle{Top: 65535, Left: 65532, Bottom: 332, Right: 544})
	if got != (hypercard.Rectangle{Top: -1, Left: -4, Bottom: 332, Right: 544}) {
		t.Fatalf("unsigned rectangle: %+v", got)
	}
}
