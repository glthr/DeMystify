package parser

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/glthr/DeMystify/hypercard"
)

type xmlPart struct {
	ID     string `xml:"id"`
	Type   string `xml:"type"`
	Script string `xml:"script"`
}

type xmlContent struct {
	Layer string  `xml:"layer"`
	ID    int     `xml:"id"`
	Text  xmlText `xml:"text"`
}

// Content text may contain styled spans/links. Their characters remain text;
// presentation markup is not part of the neutral field value.
type xmlText string

func (text *xmlText) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	var value strings.Builder
	depth := 1
	for depth != 0 {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			value.Write(token)
		}
	}
	*text = xmlText(value.String())
	return nil
}

func xmlSource(path string) hypercard.Source {
	return hypercard.Source{Path: path, Format: "xml"}
}

func xmlParts(parts []xmlPart, source hypercard.Source) []hypercard.Part {
	var result []hypercard.Part
	for _, part := range parts {
		// Part identity was optional in older XML exports. Missing/unsupported
		// IDs do not turn an otherwise readable script into an import error.
		id, _ := strconv.Atoi(strings.TrimSpace(part.ID))
		result = append(result, hypercard.Part{ID: id, Type: part.Type, Source: source,
			Script: hypercard.Script{Text: part.Script, Source: source}})
	}
	return result
}

func xmlContents(contents []xmlContent) []hypercard.Content {
	var result []hypercard.Content
	for _, content := range contents {
		result = append(result, hypercard.Content{Layer: content.Layer, ID: content.ID, Text: string(content.Text)})
	}
	return result
}

func readXMLStack(path string) (hypercard.Stack, map[int]string, []Background, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return hypercard.Stack{}, nil, nil, err
	}
	var input struct {
		XMLName     xml.Name     `xml:"stack"`
		Name        string       `xml:"name"`
		ID          int          `xml:"id"`
		CardCount   int          `xml:"cardCount"`
		CardID      int          `xml:"cardID"`
		ListID      int          `xml:"listID"`
		CantModify  bool         `xml:"cantModify,omitempty"`
		CantDelete  bool         `xml:"cantDelete,omitempty"`
		CantAbort   bool         `xml:"cantAbort,omitempty"`
		Script      string       `xml:"script"`
		Backgrounds []Background `xml:"background"`
		Cards       []CardInfo   `xml:"card"`
	}
	if err := xml.Unmarshal(content, &input); err != nil {
		return hypercard.Stack{}, nil, nil, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return hypercard.Stack{}, nil, nil, fmt.Errorf("stack in %s requires a name", path)
	}
	source := xmlSource(path)
	stack := hypercard.Stack{Name: input.Name, Source: source, Script: hypercard.Script{Text: input.Script, Source: source}}
	names, seen := make(map[int]string), make(map[int]bool)
	for _, card := range input.Cards {
		if card.ID <= 0 || seen[card.ID] {
			return hypercard.Stack{}, nil, nil, fmt.Errorf("invalid or repeated card ID %d in stack %q", card.ID, input.Name)
		}
		seen[card.ID] = true
		stack.CardOrder = append(stack.CardOrder, card.ID)
		if card.Name != "" {
			names[card.ID] = card.Name
		}
	}
	return stack, names, input.Backgrounds, nil
}

func readXMLCard(path string) (hypercard.Card, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return hypercard.Card{}, false, err
	}
	var input struct {
		XMLName  xml.Name     `xml:"card"`
		ID       int          `xml:"id"`
		Name     string       `xml:"name"`
		Owner    string       `xml:"owner"`
		Parts    []xmlPart    `xml:"part"`
		Contents []xmlContent `xml:"content"`
		Script   string       `xml:"script"`
	}
	if err := xml.Unmarshal(content, &input); err != nil {
		return hypercard.Card{}, false, err
	}
	if input.ID <= 0 {
		return hypercard.Card{}, false, fmt.Errorf("invalid card ID %d in %s: expected a positive integer", input.ID, path)
	}
	source := xmlSource(path)
	owner, ownerErr := strconv.Atoi(strings.TrimSpace(input.Owner))
	return hypercard.Card{ID: input.ID, Name: input.Name, BackgroundID: owner, Source: source,
		Parts: xmlParts(input.Parts, source), Contents: xmlContents(input.Contents),
		Script: hypercard.Script{Text: input.Script, Source: source}}, ownerErr == nil, nil
}

// LoadXML loads the existing converted export completely before analysis. Its
// graph enumeration remains filesystem-lexicographic; CardOrder preserves the
// manifest's independent order. No analysis later reopens these files.
func LoadXML(dir string) (hypercard.Corpus, error) {
	paths, err := (&Parser{stacksDir: dir}).getPaths()
	if err != nil {
		return hypercard.Corpus{}, err
	}
	var corpus hypercard.Corpus
	for _, paths := range paths {
		stack, names, backgrounds, err := readXMLStack(paths.stackFilepath)
		if err != nil {
			return hypercard.Corpus{}, err
		}
		for _, previous := range corpus.Stacks {
			if strings.EqualFold(previous.Name, stack.Name) {
				return hypercard.Corpus{}, fmt.Errorf("duplicate stack name %q", stack.Name)
			}
		}
		seen := make(map[int]bool, len(paths.cardsFilepaths))
		declared := make(map[int]bool, len(stack.CardOrder))
		for _, id := range stack.CardOrder {
			declared[id] = true
		}
		ownersReadable := true
		for _, path := range paths.cardsFilepaths {
			card, ownerReadable, err := readXMLCard(path)
			if err != nil {
				return hypercard.Corpus{}, err
			}
			if seen[card.ID] {
				return hypercard.Corpus{}, fmt.Errorf("duplicate card ID %d in stack %q", card.ID, stack.Name)
			}
			if len(declared) != 0 && !declared[card.ID] {
				return hypercard.Corpus{}, fmt.Errorf("card ID %d is absent from stack %q manifest", card.ID, stack.Name)
			}
			seen[card.ID] = true
			card.Name = names[card.ID]
			ownersReadable = ownersReadable && ownerReadable
			stack.Cards = append(stack.Cards, card)
		}
		for _, id := range stack.CardOrder {
			if !seen[id] {
				return hypercard.Corpus{}, fmt.Errorf("missing card ID %d declared in stack %q manifest", id, stack.Name)
			}
		}
		loadXMLBackgrounds(&stack, paths.stackRootDir, backgrounds, ownersReadable)
		corpus.Stacks = append(corpus.Stacks, stack)
	}
	return corpus, nil
}

func loadXMLBackgrounds(stack *hypercard.Stack, dir string, declarations []Background, ownersReadable bool) {
	stack.BackgroundsComplete = len(declarations) != 0 && ownersReadable
	diagnostic := func(id int, message string) {
		stack.BackgroundsComplete = false
		stack.Diagnostics = append(stack.Diagnostics, hypercard.Diagnostic{BlockType: "XML background scope", ID: id, Message: message})
	}
	if len(declarations) == 0 {
		diagnostic(0, "background inventory is unavailable")
	}
	if !ownersReadable {
		diagnostic(0, "one or more card background owners are unreadable")
	}
	expected, declaredIDs := make(map[string]Background), make(map[int]bool)
	for _, background := range declarations {
		_, duplicateFile := expected[background.File]
		if background.ID <= 0 || background.File == "" || filepath.Base(background.File) != background.File || duplicateFile || declaredIDs[background.ID] {
			diagnostic(background.ID, "invalid or repeated background declaration")
		}
		expected[background.File] = background
		declaredIDs[background.ID] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		diagnostic(0, err.Error())
		return
	}
	actualIDs := make(map[int]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "background_") || !strings.HasSuffix(entry.Name(), ".xml") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		var input struct {
			XMLName  xml.Name     `xml:"background"`
			ID       int          `xml:"id"`
			Name     string       `xml:"name"`
			Parts    []xmlPart    `xml:"part"`
			Contents []xmlContent `xml:"content"`
			Script   string       `xml:"script"`
		}
		if err == nil {
			err = xml.Unmarshal(content, &input)
		}
		if err != nil {
			diagnostic(0, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		if declared, ok := expected[entry.Name()]; ok {
			if declared.ID != input.ID {
				diagnostic(input.ID, "background file identity differs from its declaration")
			}
			if input.Name == "" {
				input.Name = declared.Name
			}
			delete(expected, entry.Name())
		}
		if input.ID <= 0 || actualIDs[input.ID] {
			diagnostic(input.ID, "invalid or repeated background file identity")
		}
		actualIDs[input.ID] = true
		source := xmlSource(path)
		stack.Backgrounds = append(stack.Backgrounds, hypercard.Background{ID: input.ID, Name: input.Name,
			Source: source, Script: hypercard.Script{Text: input.Script, Source: source},
			Parts: xmlParts(input.Parts, source), Contents: xmlContents(input.Contents)})
	}
	if len(expected) != 0 {
		diagnostic(0, "one or more declared background files are missing")
	}
	for _, card := range stack.Cards {
		if !actualIDs[card.BackgroundID] || !declaredIDs[card.BackgroundID] {
			diagnostic(card.BackgroundID, "card background owner is not declared and present")
		}
	}
}
