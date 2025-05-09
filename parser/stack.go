package parser

import (
	"encoding/xml"
	"os"
	"strings"
)

type HyperCardStack struct {
	Directory   string
	Name        string
	Script      []HyperTalk
	Cards       []HyperCardCard
	IsPushStack bool
}

// Background is used to get the image name
type Background struct {
	ID   int    `xml:"id,attr"`
	File string `xml:"file,attr"`
	Name string `xml:"name,attr"`
}

// CardInfo represents a card entry in the stack
type CardInfo struct {
	ID     int    `xml:"id,attr"`
	File   string `xml:"file,attr"`
	Marked bool   `xml:"marked,attr"`
	Name   string `xml:"name,attr"`
	Owner  int    `xml:"owner,attr"`
}

// ParseStackFile parses a stack XML file and returns the HyperCard stack structure
func ParseStackFile(filepath string) (*HyperCardStack, map[int]string, error) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, nil, err
	}

	type stackInfo struct {
		XMLName    xml.Name   `xml:"stack"`
		Name       string     `xml:"name"`
		ID         int        `xml:"id"`
		CardCount  int        `xml:"cardCount"`
		CardID     int        `xml:"cardID"`
		ListID     int        `xml:"listID"`
		CantModify bool       `xml:"cantModify,omitempty"`
		CantDelete bool       `xml:"cantDelete,omitempty"`
		CantAbort  bool       `xml:"cantAbort,omitempty"`
		ScriptRaw  string     `xml:"script"`
		Background Background `xml:"background"`
		CardsInfo  []CardInfo `xml:"card"`
	}

	var s stackInfo
	err = xml.Unmarshal(content, &s)
	if err != nil {
		return nil, nil, err
	}

	// process script for each part
	scripts := make([]HyperTalk, 0, len(s.ScriptRaw))
	var script HyperTalk
	for _, line := range splitScript(s.ScriptRaw) {
		line = strings.TrimSpace(line)
		script.lines = append(script.lines, ScriptLine{
			Line:       line,
			IsDisabled: strings.HasPrefix(line, "--"),
		})
	}

	scripts = append(scripts, script)

	// create a map associating the cards IDs with their names,
	// as HyperCard cards do not contain their own names
	cardsIdNameMap := make(map[int]string, len(s.CardsInfo))
	for _, c := range s.CardsInfo {
		if c.Name != "" {
			cardsIdNameMap[c.ID] = c.Name
		}
	}

	return &HyperCardStack{
			Name:   s.Name,
			Script: scripts,
		},
		cardsIdNameMap,
		nil
}
