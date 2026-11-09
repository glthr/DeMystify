package parser

import (
	"fmt"

	"github.com/glthr/DeMystify/hypercard"
)

type HyperCardCard struct {
	ID           int
	OriginalName *string
	Name         string
	Filepath     string // Retained for callers; analysis never reopens this path.
	Stack        *HyperCardStack
	Background   *string
	BackgroundID int
	Source       hypercard.Source
	Scripts      []HyperTalk
	IsPushCard   bool
	IsPopCard    bool
	HasBluePage  bool
	HasRedPage   bool
	HasWhitePage bool
}

// ParseSimpleCard is the compatibility XML reader for one card. Corpus loading
// also verifies the stack inventory and background scope.
func ParseSimpleCard(parentStack *HyperCardStack, filepath string, cardIdNameMap map[int]string) (*HyperCardCard, error) {
	if parentStack == nil {
		return nil, fmt.Errorf("card requires a parent stack")
	}
	card, _, err := readXMLCard(filepath)
	if err != nil {
		return nil, err
	}
	card.Name = cardIdNameMap[card.ID]
	return analysisCard(parentStack, card), nil
}
