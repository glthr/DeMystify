package parser

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
)

// backgroundScripts verifies the complete background scope before exposing scripts.
func (p *Parser) backgroundScripts(stack *HyperCardStack, dir string) ([]HyperTalk, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	content, err := os.ReadFile(filepath.Join(dir, stackFileName))
	var manifest struct {
		Backgrounds []Background `xml:"background"`
	}
	if err != nil || xml.Unmarshal(content, &manifest) != nil || len(manifest.Backgrounds) == 0 {
		return nil, false
	}
	expected := make(map[string]int)
	backgroundIDs := make(map[int]bool)
	for _, background := range manifest.Backgrounds {
		if background.ID <= 0 || background.File == "" || filepath.Base(background.File) != background.File {
			return nil, false
		}
		if expected[background.File] != 0 || backgroundIDs[background.ID] {
			return nil, false
		}
		expected[background.File] = background.ID
		backgroundIDs[background.ID] = true
	}
	var scripts []HyperTalk
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "background_") || !strings.HasSuffix(entry.Name(), ".xml") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		var background struct {
			XMLName xml.Name     `xml:"background"`
			ID      int          `xml:"id"`
			Script  string       `xml:"script"`
			Parts   []simplePart `xml:"part"`
		}
		if err != nil || xml.Unmarshal(content, &background) != nil {
			return nil, false
		}
		if id, declared := expected[entry.Name()]; declared && id != background.ID {
			return nil, false
		}
		delete(expected, entry.Name())
		texts := []string{background.Script}
		for _, part := range background.Parts {
			texts = append(texts, part.ScriptRaw)
		}
		for _, text := range texts {
			var script HyperTalk
			for _, line := range splitScript(text) {
				script.lines = append(script.lines, ScriptLine{Line: line})
			}
			scripts = append(scripts, script)
		}
	}
	if len(expected) != 0 {
		return nil, false
	}
	for _, card := range p.cards {
		if card.Stack != stack {
			continue
		}
		content, err := os.ReadFile(card.Filepath)
		var context struct {
			Owner int `xml:"owner"`
		}
		if err != nil || xml.Unmarshal(content, &context) != nil || !backgroundIDs[context.Owner] {
			return nil, false
		}
	}
	return scripts, true
}
