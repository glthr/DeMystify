package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackgroundScriptsRequireUnambiguousCompleteScope(t *testing.T) {
	for name, edit := range map[string]struct{ declarations, background, owner string }{
		"complete":       {},
		"duplicate ID":   {declarations: `<background id="7" file="background_8.xml"/>`},
		"duplicate file": {declarations: `<background id="8" file="background_7.xml"/>`},
		"missing file":   {declarations: `<background id="9" file="background_9.xml"/>`},
		"wrong ID":       {background: "<background><id>8</id></background>"},
		"malformed":      {background: "<background>"},
		"unknown owner":  {owner: "9"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if edit.background == "" {
				edit.background = "<background><id>7</id><script>rootMessage</script><part><script>partMessage</script></part></background>"
			}
			if edit.owner == "" {
				edit.owner = "7"
			}
			files := map[string]string{
				stackFileName:      `<stack><background id="7" file="background_7.xml"/>` + edit.declarations + "</stack>",
				"background_7.xml": edit.background,
				"background_8.xml": "<background><id>7</id></background>",
				"card_1.xml":       "<card><owner>" + edit.owner + "</owner></card>",
			}
			for path, content := range files {
				if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			stack := &HyperCardStack{Name: "Example"}
			p := &Parser{cards: []*HyperCardCard{{Stack: stack, Filepath: filepath.Join(dir, "card_1.xml")}}}
			scripts, verified := p.backgroundScripts(stack, dir)
			if verified != (name == "complete") {
				t.Fatalf("verified=%v", verified)
			}
			if verified && len(scripts) != 3 {
				t.Fatalf("got %d scripts, want root/part plus the empty extra background script", len(scripts))
			}
		})
	}
}
