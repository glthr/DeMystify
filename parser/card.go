package parser

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
)

type HyperCardCard struct {
	ID           int
	OriginalName *string // depends on Stack
	Name         string
	Filepath     string
	Stack        *HyperCardStack
	Background   *string
	Scripts      []HyperTalk
	IsPushCard   bool
	IsPopCard    bool
	HasBluePage  bool
	HasRedPage   bool
	HasWhitePage bool
}

// SimplePart contains only the script from a part
type simplePart struct {
	ScriptRaw string `xml:"script"`
	Script    []string
}

type simpleContent struct {
	Layer string `xml:"layer"`
	ID    int    `xml:"id"`
	Text  string `xml:"text"`
}

// ParseSimpleCard parses a card XML file and returns only the needed data
func ParseSimpleCard(parentStack *HyperCardStack, filepath string, cardIdNameMap map[int]string) (*HyperCardCard, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	type simpleCard struct {
		Stack          string
		ID             int             `xml:"id"`
		Parts          []simplePart    `xml:"part"`
		Contents       []simpleContent `xml:"content"`
		ScriptRaw      string          `xml:"script"`
		BackgroundText string          // extracted from content with layer=background and id=1
	}

	var c simpleCard

	err = xml.Unmarshal(content, &c)
	if err != nil {
		return nil, err
	}

	// process script for each part
	var scripts []HyperTalk
	processRawScript := func(rawScript string) {
		scriptLines := HyperTalk{}
		for _, line := range splitScript(rawScript) {
			line = strings.TrimSpace(line)
			scriptLines.lines = append(scriptLines.lines, ScriptLine{
				Line:       line,
				IsDisabled: strings.HasPrefix(line, "--"),
			})
		}

		scripts = append(scripts, scriptLines)
	}

	for _, part := range c.Parts {
		processRawScript(part.ScriptRaw)
	}

	processRawScript(c.ScriptRaw)

	// extract image name from the content with layer=background and id=1
	var background *string
	for _, ct := range c.Contents {
		if ct.Layer == "background" && ct.ID == 1 {
			background = &ct.Text
			break
		}
	}

	var name *string
	if n, ok := cardIdNameMap[c.ID]; ok {
		name = &n
	}

	return &HyperCardCard{
		Name:         fmt.Sprintf("%s:%d", parentStack.Name, c.ID),
		Filepath:     filepath,
		Stack:        parentStack,
		ID:           c.ID,
		OriginalName: name,
		Background:   background,
		Scripts:      scripts,
	}, nil
}
