package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func writeXMLFixture(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCardsRequirePositiveIDs(t *testing.T) {
	for _, content := range []string{"<card/>", "<card><id>0</id></card>", "<card><id>-1</id></card>"} {
		path := writeXMLFixture(t, "card.xml", content)
		if _, err := ParseSimpleCard(&HyperCardStack{Name: "Example"}, path, nil); err == nil {
			t.Errorf("accepted missing or nonpositive card ID in %s", content)
		}
	}
}

func TestCardsRequireAParentStack(t *testing.T) {
	path := writeXMLFixture(t, "card.xml", "<card><id>1</id></card>")
	if _, err := ParseSimpleCard(nil, path, nil); err == nil {
		t.Fatal("accepted a card without a parent stack")
	}
}

func TestStacksRejectAmbiguousIdentities(t *testing.T) {
	for _, content := range []string{
		"<stack/>",
		"<stack><name> </name></stack>",
		`<stack><name>Example</name><card id="0"/></stack>`,
		`<stack><name>Example</name><card id="1"/><card id="1"/></stack>`,
	} {
		path := writeXMLFixture(t, "stack.xml", content)
		if _, _, err := ParseStackFile(path); err == nil {
			t.Errorf("accepted invalid stack or card identity in %s", content)
		}
	}
}
