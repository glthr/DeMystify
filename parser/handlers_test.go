package parser

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
)

const detailHandler = "on openDetail which\nglobal selection\nput which into selection\npush card\ngo card id 2\nend openDetail"
const detailCaller = "on mouseUp\nopenDetail 7\nend mouseUp"

func handlerXMLText(text string) string {
	var out strings.Builder
	if err := xml.EscapeText(&out, []byte(text)); err != nil {
		panic(err)
	}
	return out.String()
}

func handlerFixture(t *testing.T, stack, caller, cardOverride, background string) *Parser {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		stackFileName: "<stack><name>Example</name><background id=\"7\" file=\"background_7.xml\"/><script>" + handlerXMLText(stack) + "</script></stack>",
		"card_1.xml":  "<card><id>1</id><owner>7</owner><part><script>" + handlerXMLText(caller) + "</script></part><script>" + handlerXMLText(cardOverride) + "</script></card>",
		"card_2.xml":  "<card><id>2</id><owner>7</owner><part><script>on mouseUp\npop card\nend mouseUp</script></part></card>",
		"card_3.xml":  "<card><id>3</id><owner>7</owner><part><script>" + handlerXMLText(detailCaller) + "</script></part></card>",
		"card_4.xml":  "<card><id>4</id><owner>7</owner></card>",
	}
	if background != "MISSING" {
		if background == "<background/>" {
			background = "<background><id>7</id></background>"
		} else if strings.Contains(background, "</background>") {
			background = strings.Replace(background, "<background>", "<background><id>7</id>", 1)
		}
		files["background_7.xml"] = background
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := NewParser(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStackHandlerProjectionKeepsCallerReturnContext(t *testing.T) {
	p := handlerFixture(t, detailHandler, strings.Replace(detailCaller, " 7", " 9", 1), "", "<background/>")
	metadata, err := p.Process()
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata.Edges) != 2 {
		t.Fatalf("got %d edges, want the two caller entries", len(metadata.Edges))
	}
	seen := make(map[string]bool)
	for _, edge := range metadata.Edges {
		if edge.Source.IsOfType(common.IsStack) || edge.Target.Name != "Example:2" || !edge.IsOfType(common.Backtracking) {
			t.Errorf("incorrect projected move: %+v", edge)
		}
		seen[edge.Source.Name] = true
	}
	if !seen["Example:1"] || !seen["Example:3"] {
		t.Fatal("shared close-up is missing an actual caller")
	}
}
