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
	stacksDir     string
	stacks        []*HyperCardStack
	cards         []*HyperCardCard
	links         []*HyperCardLink
	pushedLinks   map[*HyperCardLink]bool
	stackHandlers map[*HyperCardStack]map[string]stackHandlerSummary
	backgrounds   map[*HyperCardStack]backgroundScope
}

type HyperCardLink struct {
	Source, Target         any // HyperCard stack or card
	IsCrossAges            bool
	IsNotImplemented       bool // points to from another card, but does not exist
	IsDisabled             bool // commented out script line
	IsBacktracking         bool
	IsSequenceContinuation bool

	// Transitivity
	TransitivityRank common.TransitivityRank
	TransitivityID   int64
}

// NewParser detects native HyperCard data forks or an existing XML export.
func NewParser(stacksDir string) (*Parser, error) {
	corpus, err := LoadInput(stacksDir)
	if err != nil {
		return nil, err
	}
	p, err := NewParserFromCorpus(corpus)
	if err == nil {
		p.stacksDir = stacksDir
	}
	return p, err
}

type Paths struct {
	stackRootDir   string
	stackFilepath  string
	cardsFilepaths []string
}

func (p *Parser) getPaths() ([]Paths, error) {
	var results []Paths

	root, err := filepath.EvalSymlinks(p.stacksDir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("stack input must be a directory: %s", p.stacksDir)
	}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
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
	if len(results) == 0 {
		return nil, fmt.Errorf("no %s files found in %s", stackFileName, p.stacksDir)
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

func (p *Parser) identifyLinks() error {
	var transitivityID int64
	for _, stack := range p.stacks {
		for _, script := range stack.Script {
			links, err := p.identifyScriptLinks(stack, p.withoutHandlerDefinitions(stack, script), &transitivityID)
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
			p.links = append(p.links, p.projectHandlerCalls(card, script)...)
		}
	}

	// identify backtracking
	for _, card := range p.cards {
		if card.IsPushCard {
			for _, link := range p.links {
				if link.Source == card && p.pushedLinks[link] && link.Target != nil && link.TransitivityRank == common.DefaultTransitivity {
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
	if p.pushedLinks == nil {
		p.pushedLinks = make(map[*HyperCardLink]bool)
	}
	var links []*HyperCardLink
	var handlers []int
	sequence := p.newLiteralSequence(source, script)
	handlerID := 0
	handlerName := ""
	pushed := false
	for lineIndex, line := range script.lines {
		// Handler declarations delimit independent sequences of commands.
		// Nested `end if`/`end repeat` statements do not end a handler.
		fields := strings.Fields(line.Line)
		if !line.IsDisabled && len(fields) >= 2 {
			switch strings.ToLower(fields[0]) {
			case "on", "function":
				handlerID++
				pushed = false
				handlerName = fields[1]
			case "end":
				if handlerName != "" && strings.EqualFold(fields[1], handlerName) {
					handlerID++
					pushed = false
					handlerName = ""
				}
			}
		}
		command, disabled := normalizeCommand(line.Line)
		if !disabled && !line.IsDisabled && findCommandMatch(pushCardPattern, command) != nil {
			pushed = true
		}
		link, err := p.parseCardCommand(source, line.Line)
		if err != nil {
			// A comment is not a command. Text that only resembles one, such
			// as a note quoting an old go line, records nothing.
			if !disabled && !line.IsDisabled {
				return nil, fmt.Errorf("cannot parse command %q: %w", line.Line, err)
			}
			link = nil
		}
		continuation := sequence.observe(lineIndex, line, link)
		if link != nil {
			link.IsDisabled = line.IsDisabled
			if pushed && !link.IsDisabled {
				p.pushedLinks[link] = true
			}
			links = append(links, link)
			handlers = append(handlers, handlerID)
		}
		if continuation != nil {
			links = append(links, continuation)
			handlers = append(handlers, handlerID)
		}
	}

	var result []*HyperCardLink
	consumed := make(map[*HyperCardLink]bool)
	for i, link := range links {
		if link.TransitivityRank == common.DefaultTransitivity {
			if !consumed[link] {
				result = append(result, link)
			}
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
			// A commented-out push with no commented-out go after it is a
			// leftover or a note, not an incomplete command.
			if link.IsDisabled {
				continue
			}
			return nil, fmt.Errorf("push card command has no following go card command")
		}

		consumed[goLink] = true
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

// Script names refer to original HyperCard names, not generated graph labels.
func (p *Parser) getScriptCardByName(stackName, cardName string) (*HyperCardCard, error) {
	for _, card := range p.cards {
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
