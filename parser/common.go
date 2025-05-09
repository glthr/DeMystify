package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/glthr/DeMystify/common"
)

// NOTE: perhaps refactor, depending on how the original files are decompiled
const stackFileName = "stack_-1.xml"

type Parser struct {
	stacksDir string
	stacks    []*HyperCardStack
	cards     []*HyperCardCard
	links     []*HyperCardLink
}

type HyperCardLink struct {
	Source, Target   any // HyperCard stack or card
	IsCrossAges      bool
	IsNotImplemented bool // points to from another card, but does not exist
	IsDisabled       bool // commented out script line
	IsBacktracking   bool

	// Transitivity
	TransitivityRank common.TransitivityRank
	TransitivityID   int64
}

func NewParser(stacksDir string) (*Parser, error) {
	p := &Parser{
		stacksDir: stacksDir,
	}

	stacksPaths, err := p.getPaths()
	if err != nil {
		return nil, err
	}

	if err = p.parseStacksAndCards(stacksPaths); err != nil {
		return nil, err
	}

	if err = p.identifyLinks(); err != nil {
		return nil, err
	}

	return p, nil
}

type Paths struct {
	stackRootDir   string
	stackFilepath  string
	cardsFilepaths []string
}

func (p *Parser) getPaths() ([]Paths, error) {
	var results []Paths

	err := filepath.Walk(p.stacksDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == p.stacksDir || !info.IsDir() {
			return nil
		}

		stackFilePath := filepath.Join(path, stackFileName)
		if _, err := os.Stat(stackFilePath); err == nil {
			cardPaths, err := findCardFiles(path)
			if err != nil {
				return err
			}
			results = append(results, Paths{
				stackRootDir:   path,
				stackFilepath:  stackFilePath,
				cardsFilepaths: cardPaths,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return results, nil
}

func findCardFiles(dir string) ([]string, error) {
	var cardPaths []string
	cardPattern := regexp.MustCompile(`^card_\d+\.xml$`)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() && cardPattern.MatchString(entry.Name()) {
			cardPaths = append(cardPaths, filepath.Join(dir, entry.Name()))
		}
	}

	return cardPaths, nil
}

func (p *Parser) parseStacksAndCards(stacksPaths []Paths) error {
	for _, stackPath := range stacksPaths {
		stack, cardIdNameMap, err := ParseStackFile(stackPath.stackFilepath)
		if err != nil {
			return err
		}
		p.stacks = append(p.stacks, stack)

		for _, cardPath := range stackPath.cardsFilepaths {
			card, parseErr := ParseSimpleCard(stack, cardPath, cardIdNameMap)
			if parseErr != nil {
				return parseErr
			}

			p.cards = append(p.cards, card)
		}
	}
	return nil
}

func (p *Parser) identifyLinks() error {
	for _, stack := range p.stacks {
		for _, script := range stack.Script {
			for _, line := range script.lines {
				if link, err := p.parseCardCommand(stack, line.Line); err != nil {
					return fmt.Errorf("cannot get HyperCardLink: %w", err)
				} else if link != nil {
					p.links = append(p.links, link)
				}
			}
		}
	}

	for i, card := range p.cards {
		for _, script := range card.Scripts {
			var (
				goToCards []*HyperCardCard
				links     []*HyperCardLink
			)
			for _, line := range script.lines {
				if link, err := p.parseCardCommand(card, line.Line); err != nil {
					return fmt.Errorf("cannot get HyperCardLink: %w", err)
				} else if link != nil {
					if link.TransitivityRank == common.DefaultTransitivity {
						goToCards = append(goToCards, link.Target.(*HyperCardCard))
					}
					link.IsDisabled = line.IsDisabled
					links = append(links, link)
				}
			}

			// identify transitive cards
			var filteredLinks []*HyperCardLink
			for j, link := range links {
				if link.TransitivityRank != common.DefaultTransitivity {
					// split link into two

					// source to transitive card
					filteredLinks = append(filteredLinks, &HyperCardLink{
						Source:           link.Source,
						Target:           goToCards[j],
						TransitivityRank: common.Tail,
						TransitivityID:   int64(i),
						IsCrossAges:      link.IsCrossAges,
						IsNotImplemented: link.IsNotImplemented,
						IsDisabled:       link.IsDisabled,
						IsBacktracking:   false,
					})

					// transitive card to target
					filteredLinks = append(filteredLinks, &HyperCardLink{
						Source:           goToCards[j],
						Target:           link.Target,
						TransitivityRank: common.Head,
						TransitivityID:   int64(i),
						IsCrossAges:      link.IsCrossAges,
						IsNotImplemented: link.IsNotImplemented,
						IsDisabled:       link.IsDisabled,
						IsBacktracking:   false,
					})
				} else {
					filteredLinks = append(filteredLinks, link)
				}
			}

			p.links = append(p.links, filteredLinks...)
		}
	}

	// remove duplicates
	getName := func(item any) string {
		switch s := item.(type) {
		case *HyperCardCard:
			return s.Name
		case *HyperCardStack:
			return s.Name
		}
		return ""
	}

	// add transitivity property to existing links
	for i, linkA := range p.links {
		if linkA.TransitivityRank != common.DefaultTransitivity {
			continue
		}

		sourceAName := getName(linkA.Source)
		targetAName := getName(linkA.Target)

		for j, linkB := range p.links {
			if i == j {
				continue
			}

			sourceBName := getName(linkB.Source)
			if sourceAName != sourceBName {
				continue
			}

			targetBName := getName(linkB.Target)
			if targetAName != targetBName {
				continue
			}

			p.links[i].TransitivityRank = linkB.TransitivityRank
			p.links[i].TransitivityID = linkB.TransitivityID
			break
		}
	}

	// identify backtracking
	for _, card := range p.cards {
		if card.IsPushCard {
			for _, link := range p.links {
				if link.Source == card && link.Target != nil && link.TransitivityRank == common.DefaultTransitivity {
					if link.Target.(*HyperCardCard).IsPopCard {
						link.IsBacktracking = true
					}
				}
			}
		}
	}

	return nil
}

func splitScript(script string) []string {
	if script == "" {
		return []string{}
	}
	return strings.Split(strings.ReplaceAll(script, "\r\n", "\n"), "\n")
}

func (p *Parser) GetAllStacks() []*HyperCardStack {
	return p.stacks
}

func (p *Parser) GetAllCards() []*HyperCardCard {
	return p.cards
}

func (p *Parser) GetAllPopCards() []*HyperCardCard {
	var popCards []*HyperCardCard
	for _, card := range p.cards {
		if card.IsPopCard {
			popCards = append(popCards, card)
		}
	}
	return popCards
}

func (p *Parser) GetCardByStackAndName(stackName, cardName string) (*HyperCardCard, error) {
	for _, card := range p.cards {
		if strings.EqualFold(card.Name, cardName) && strings.EqualFold(card.Stack.Name, stackName) {
			return card, nil
		}

		if card.OriginalName != nil && strings.EqualFold(*card.OriginalName, cardName) && strings.EqualFold(card.Stack.Name, stackName) {
			return card, nil
		}
	}
	return nil, fmt.Errorf("card with name %s and stack %s not found", cardName, stackName)
}

func (p *Parser) GetCardByStackAndID(stackName string, ID int) (*HyperCardCard, error) {
	for _, card := range p.cards {
		if card.ID == ID && strings.EqualFold(card.Stack.Name, stackName) {
			return card, nil
		}
	}
	return nil, fmt.Errorf("HyperCardCard with ID %d and stack %s not found", ID, stackName)
}

func (p *Parser) GetStackByName(stackName string) (*HyperCardStack, error) {
	for _, stack := range p.stacks {
		if strings.EqualFold(stack.Name, stackName) {
			return stack, nil
		}
	}
	return nil, fmt.Errorf("stack with name %s not found", stackName)
}
