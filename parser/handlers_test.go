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

func TestStackHandlerProjectionRejectsUnprovenContexts(t *testing.T) {
	for name, edit := range map[string]struct{ stack, caller, local, background string }{
		"card override":            {local: "on openDetail which\nend openDetail"},
		"part override":            {caller: detailCaller + "\non openDetail which\nend openDetail"},
		"primitive override":       {local: "on push what\nend push"},
		"background override":      {background: "<background><script>on openDetail which\nend openDetail</script></background>"},
		"background part override": {background: "<background><part><script>on openDetail which\nend openDetail</script></part></background>"},
		"unverified background":    {background: "MISSING"},
		"malformed background":     {background: "<background>"},
		"dynamic argument":         {caller: "on mouseUp\nopenDetail variable\nend mouseUp"},
		"computed argument":        {caller: "on mouseUp\nopenDetail 2 + offset\nend mouseUp"},
		"commented caller":         {caller: "--on mouseUp\nopenDetail 7\nend mouseUp"},
		"conditional navigation":   {stack: "on openDetail which\nif which is 7 then\npush card\ngo card id 2\nend if\nend openDetail"},
		"parameter target":         {stack: "on openDetail which\npush card\ngo card id which\nend openDetail"},
		"nested call":              {stack: strings.Replace(detailHandler, "push card", "anotherHandler which\npush card", 1)},
		"two moves":                {stack: strings.Replace(detailHandler, "end openDetail", "go card id 4\nend openDetail", 1)},
		"duplicate declaration":    {stack: detailHandler + "\n" + detailHandler},
		"commented declaration":    {stack: "--" + detailHandler},
		"primitive stack override": {stack: detailHandler + "\non push what\nend push"},
	} {
		t.Run(name, func(t *testing.T) {
			if edit.stack == "" {
				edit.stack = detailHandler
			}
			if edit.caller == "" {
				edit.caller = detailCaller
			}
			if edit.background == "" {
				edit.background = "<background/>"
			}
			p := handlerFixture(t, edit.stack, edit.caller, edit.local, edit.background)
			for _, link := range p.links {
				if source, ok := link.Source.(*HyperCardCard); ok && source.ID == 1 && link.Target.(*HyperCardCard).ID == 2 {
					t.Fatal("unproven call acquired an entry edge")
				}
			}
		})
	}
}
