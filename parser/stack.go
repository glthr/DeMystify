package parser

import (
	"github.com/glthr/DeMystify/hypercard"
)

type HyperCardStack struct {
	Directory   string
	Name        string
	Script      []HyperTalk
	Cards       []HyperCardCard
	IsPushStack bool
	CardOrder   []int // Authoritative input order, separate from graph enumeration.
	Source      hypercard.Source
	cardIDs     map[int]bool // explicit XML manifest; empty permits a partial import
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
	stack, names, _, err := readXMLStack(filepath)
	if err != nil {
		return nil, nil, err
	}
	parsed := analysisStack(stack)
	return parsed, names, nil
}
