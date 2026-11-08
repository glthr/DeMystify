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
	var transitivityID int64
	for _, stack := range p.stacks {
		for _, script := range stack.Script {
			links, err := p.identifyScriptLinks(stack, script, &transitivityID)
			if err != nil {
				return fmt.Errorf("stack %s: %w", stack.Name, err)
			}
			p.links = append(p.links, links...)
		}
	}

	for _, card := range p.cards {
		for _, script := range card.Scripts {
			links, err := p.identifyScriptLinks(card, script, &transitivityID)
			if err != nil {
				return fmt.Errorf("card %s: %w", card.Name, err)
			}
			p.links = append(p.links, links...)
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
			if i == j || linkB.TransitivityRank == common.DefaultTransitivity || linkA.IsDisabled != linkB.IsDisabled {
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

func (p *Parser) identifyScriptLinks(source any, script HyperTalk, transitivityID *int64) ([]*HyperCardLink, error) {
	var links []*HyperCardLink
	var handlers []int
	handlerID := 0
	handlerName := ""
	for _, line := range script.lines {
		// Handler declarations delimit independent sequences of commands.
		// Nested `end if`/`end repeat` statements do not end a handler.
		fields := strings.Fields(line.Line)
		if !line.IsDisabled && len(fields) >= 2 {
			switch strings.ToLower(fields[0]) {
			case "on", "function":
				handlerID++
				handlerName = fields[1]
			case "end":
				if handlerName != "" && strings.EqualFold(fields[1], handlerName) {
					handlerID++
					handlerName = ""
				}
			}
		}
		link, err := p.parseCardCommand(source, line.Line)
		if err != nil {
			return nil, fmt.Errorf("cannot parse command %q: %w", line.Line, err)
		}
		if link != nil {
			link.IsDisabled = line.IsDisabled
			links = append(links, link)
			handlers = append(handlers, handlerID)
		}
	}

	var result []*HyperCardLink
	for i, link := range links {
		if link.TransitivityRank == common.DefaultTransitivity {
			result = append(result, link)
			continue
		}

		// A pushed return card is reached through the following go command.
		// Indexing a separate list of go targets loses their positions when a
		// script contains several pushes or ordinary go commands.
		var goLink *HyperCardLink
		for j := i + 1; j < len(links); j++ {
			if handlers[j] != handlers[i] {
				break
			}
			candidate := links[j]
			if candidate.TransitivityRank == common.DefaultTransitivity && candidate.IsDisabled == link.IsDisabled {
				goLink = candidate
				break
			}
		}
		if goLink == nil {
			return nil, fmt.Errorf("push card command has no following go card command")
		}

		result = append(result, &HyperCardLink{
			Source:           link.Source,
			Target:           goLink.Target,
			TransitivityRank: common.Tail,
			TransitivityID:   *transitivityID,
			IsCrossAges:      goLink.IsCrossAges,
			IsNotImplemented: goLink.IsNotImplemented,
			IsDisabled:       link.IsDisabled,
		}, &HyperCardLink{
			Source:           goLink.Target,
			Target:           link.Target,
			TransitivityRank: common.Head,
			TransitivityID:   *transitivityID,
			IsCrossAges:      link.IsCrossAges,
			IsNotImplemented: link.IsNotImplemented,
			IsDisabled:       link.IsDisabled,
		})
		(*transitivityID)++
	}
	return result, nil
}

func splitScript(script string) []string {
	if script == "" {
		return []string{}
	}
	script = strings.ReplaceAll(script, "\r\n", "\n")
	return strings.Split(strings.ReplaceAll(script, "\r", "\n"), "\n")
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
