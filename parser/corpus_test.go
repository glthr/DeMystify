package parser

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/hypercard"
)

func corpusFixture() hypercard.Corpus {
	return hypercard.Corpus{Stacks: []hypercard.Stack{{Name: "Example", BackgroundsComplete: true,
		Cards: []hypercard.Card{{ID: 1, BackgroundID: 7, Script: hypercard.Script{Text: "go card Black"}},
			{ID: 2, Name: "Black", BackgroundID: 7}},
		Backgrounds: []hypercard.Background{{ID: 7}}, CardOrder: []int{2, 1}}}}
}

func TestCorpusValidatesIdentitiesAndAuthoritativeOrder(t *testing.T) {
	for name, change := range map[string]func(*hypercard.Corpus){
		"empty corpus":     func(c *hypercard.Corpus) { c.Stacks = nil },
		"empty stack name": func(c *hypercard.Corpus) { c.Stacks[0].Name = " " },
		"duplicate stacks": func(c *hypercard.Corpus) { c.Stacks = append(c.Stacks, hypercard.Stack{Name: "example"}) },
		"zero card":        func(c *hypercard.Corpus) { c.Stacks[0].Cards[0].ID = 0 },
		"negative card":    func(c *hypercard.Corpus) { c.Stacks[0].Cards[0].ID = -1 },
		"duplicate card":   func(c *hypercard.Corpus) { c.Stacks[0].Cards[1].ID = 1 },
		"duplicate order":  func(c *hypercard.Corpus) { c.Stacks[0].CardOrder = []int{1, 1} },
		"zero order":       func(c *hypercard.Corpus) { c.Stacks[0].CardOrder = []int{0, 1, 2} },
		"missing card":     func(c *hypercard.Corpus) { c.Stacks[0].CardOrder = []int{1, 2, 3} },
		"incomplete order": func(c *hypercard.Corpus) { c.Stacks[0].CardOrder = []int{1} },
	} {
		t.Run(name, func(t *testing.T) {
			corpus := corpusFixture()
			change(&corpus)
			if _, err := NewParserFromCorpus(corpus); err == nil {
				t.Fatal("accepted an invalid corpus")
			}
		})
	}
}

func TestCorpusBackgroundProofDoesNotTrustCompletenessFlagAlone(t *testing.T) {
	for name, change := range map[string]func(*hypercard.Stack){
		"complete":        func(s *hypercard.Stack) {},
		"unknown":         func(s *hypercard.Stack) { s.BackgroundsComplete = false },
		"unknown owner":   func(s *hypercard.Stack) { s.Cards[0].BackgroundID = 99 },
		"negative owner":  func(s *hypercard.Stack) { s.Cards[0].BackgroundID = -1 },
		"duplicate IDs":   func(s *hypercard.Stack) { s.Backgrounds = append(s.Backgrounds, hypercard.Background{ID: 7}) },
		"zero ID":         func(s *hypercard.Stack) { s.Backgrounds[0].ID = 0 },
		"background root": func(s *hypercard.Stack) { s.Backgrounds[0].Script.Text = "global black" },
		"background part": func(s *hypercard.Stack) {
			s.Backgrounds[0].Parts = []hypercard.Part{{Script: hypercard.Script{Text: "global black"}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			corpus := corpusFixture()
			change(&corpus.Stacks[0])
			p, err := NewParserFromCorpus(corpus)
			if err != nil {
				t.Fatal(err)
			}
			if (len(p.links) == 1) != (name == "complete") {
				t.Fatalf("unproven background scope acquired a bare-name reference: %v", p.links)
			}
		})
	}
}

func TestCorpusDistinguishesVerifiedEmptyFromUnknownBackgrounds(t *testing.T) {
	for _, complete := range []bool{false, true} {
		corpus := corpusFixture()
		stack := &corpus.Stacks[0]
		stack.Backgrounds = nil
		stack.BackgroundsComplete = complete
		for i := range stack.Cards {
			stack.Cards[i].BackgroundID = 0
		}
		p, err := NewParserFromCorpus(corpus)
		if err != nil {
			t.Fatal(err)
		}
		if (len(p.links) == 1) != complete {
			t.Fatalf("complete=%v: got %v", complete, p.links)
		}
	}
}

func TestCorpusDoesNotAliasCallerOwnedInput(t *testing.T) {
	corpus := corpusFixture()
	corpus.Stacks[0].Cards[0].Contents = []hypercard.Content{{Layer: "background", ID: 1, Text: "Original label"}}
	corpus.Stacks[0].Backgrounds[0].Parts = []hypercard.Part{{Script: hypercard.Script{Text: "put 1 into other"}}}
	p, err := NewParserFromCorpus(corpus)
	if err != nil {
		t.Fatal(err)
	}
	corpus.Stacks[0].CardOrder[0] = 999
	corpus.Stacks[0].Cards[0].Contents[0].Text = "Changed label"
	corpus.Stacks[0].Cards[0].Script.Text = "global black"
	corpus.Stacks[0].Cards[1].Name = "Changed name"
	corpus.Stacks[0].Backgrounds[0].Parts[0].Script.Text = "global black"
	if !reflect.DeepEqual(p.stacks[0].CardOrder, []int{2, 1}) || *p.cards[0].Background != "Original label" || *p.cards[1].OriginalName != "Black" {
		t.Fatal("caller mutation changed the loaded analysis objects")
	}
	if p.cards[0].Scripts[0].RawText != "go card Black" {
		t.Fatal("raw script evidence aliases the caller's script")
	}
	link, err := p.parseCardCommand(p.cards[0], "go card Black")
	if err != nil || link == nil || link.Target != p.cards[1] {
		t.Fatalf("caller mutation changed the background binding proof: %+v, %v", link, err)
	}
}

func writeCorpusXML(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestXMLCorpusPreservesRawDataAndIndependentOrder(t *testing.T) {
	dir := writeCorpusXML(t, map[string]string{
		stackFileName:      `<stack><name>Example</name><background id="7" file="background_7.xml" name="Declared background"/><card id="2"/><card id="1" name="Manifest name"/><card id="10"/><script>  first&#13;-- second&#10;third  </script></stack>`,
		"card_1.xml":       `<card><id>1</id><name>Local name</name><owner>7</owner><part><id>4</id><type>button</type><script>  part script  </script></part><script>  root script  </script><content><layer>background</layer><id>1</id><text>Visual <span>Label</span><a> Link</a></text></content></card>`,
		"card_10.xml":      `<card><id>10</id><owner>7</owner></card>`,
		"card_2.xml":       `<card><id>2</id><owner>7</owner></card>`,
		"background_7.xml": `<background><id>7</id><name>Actual background</name><part><id>8</id><type>field</type><script>background part</script></part><content><layer>background</layer><id>2</id><text>Background text</text></content><script>background root</script></background>`,
	})
	corpus, err := LoadXML(dir)
	if err != nil {
		t.Fatal(err)
	}
	stack := corpus.Stacks[0]
	if !stack.BackgroundsComplete || !reflect.DeepEqual(stack.CardOrder, []int{2, 1, 10}) {
		t.Fatalf("manifest order/completeness lost: %+v", stack)
	}
	if got := []int{stack.Cards[0].ID, stack.Cards[1].ID, stack.Cards[2].ID}; !reflect.DeepEqual(got, []int{1, 10, 2}) {
		t.Fatalf("graph enumeration changed: %v", got)
	}
	card, background := stack.Cards[0], stack.Backgrounds[0]
	if card.Name != "Manifest name" || card.BackgroundID != 7 || card.Contents[0].Text != "Visual Label Link" || card.Parts[0].ID != 4 || card.Parts[0].Type != "button" {
		t.Fatalf("card data lost: %+v", card)
	}
	if background.Name != "Actual background" || background.Parts[0].Type != "field" || background.Contents[0].Text != "Background text" {
		t.Fatalf("background data lost: %+v", background)
	}
	stackPath, err := filepath.EvalSymlinks(filepath.Join(dir, stackFileName))
	if err != nil {
		t.Fatal(err)
	}
	if stack.Script.Text != "  first\r-- second\nthird  " || stack.Script.Source.Path != stackPath || stack.Script.Source.Format != "xml" {
		t.Fatalf("raw script/source lost: %+v", stack.Script)
	}
	p, err := NewParserFromCorpus(corpus)
	if err != nil {
		t.Fatal(err)
	}
	if p.cards[0].Scripts[0].RawText != "  part script  " || p.cards[0].Scripts[1].RawText != "  root script  " || p.cards[0].Source != card.Source || p.cards[0].BackgroundID != 7 {
		t.Fatal("analysis lost part/root order, raw text, source or owner")
	}
	scripts, verified := p.backgroundScripts(p.stacks[0])
	if !verified || scripts[0].RawText != "background root" || scripts[1].RawText != "background part" {
		t.Fatal("background root/part order changed")
	}
}

func TestLoadedCorpusAnalysisDoesNotReopenXML(t *testing.T) {
	dir := writeCorpusXML(t, map[string]string{
		stackFileName:      `<stack><name>Example</name><background id="7" file="background_7.xml"/><card id="1"/><card id="2"/><card id="3" name="Black"/><card id="4"/><script>` + handlerXMLText(detailHandler) + `</script></stack>`,
		"card_1.xml":       `<card><id>1</id><owner>7</owner><part><script>` + handlerXMLText(detailCaller) + `</script></part></card>`,
		"card_2.xml":       `<card><id>2</id><owner>7</owner><script>on mouseUp&#10;pop card&#10;end mouseUp</script></card>`,
		"card_3.xml":       `<card><id>3</id><owner>7</owner><script>` + handlerXMLText(detailCaller) + `</script></card>`,
		"card_4.xml":       `<card><id>4</id><owner>7</owner><script>go card Black</script></card>`,
		"background_7.xml": `<background><id>7</id><script>put "black" into other</script></background>`,
	})
	corpus, err := LoadXML(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	p, err := NewParserFromCorpus(corpus)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := p.Process()
	if err != nil || len(metadata.Edges) != 3 {
		t.Fatalf("loaded corpus lost references after XML removal: %+v, %v", metadata, err)
	}
	projected := 0
	for _, edge := range metadata.Edges {
		if edge.Source.IsOfType(common.IsStack) {
			t.Fatal("helper definition became a stack-level world move")
		}
		if edge.Target.Name == "Example:2" && edge.IsOfType(common.Backtracking) {
			projected++
		}
	}
	if projected != 2 {
		t.Fatalf("lost caller attribution: %d", projected)
	}
	filtered := p.withoutHandlerDefinitions(p.stacks[0], p.stacks[0].Script[0])
	if filtered.RawText != detailHandler || filtered.Source != corpus.Stacks[0].Script.Source {
		t.Fatal("handler filtering discarded original source evidence")
	}
}

func TestMalformedXMLOwnerLeavesScopeUnknownWithoutDiscardingCards(t *testing.T) {
	dir := writeCorpusXML(t, map[string]string{
		stackFileName:      `<stack><name>Example</name><background id="7" file="background_7.xml"/></stack>`,
		"card_1.xml":       `<card><id>1</id><owner>not-an-id</owner></card>`,
		"background_7.xml": `<background><id>7</id></background>`,
	})
	corpus, err := LoadXML(dir)
	if err != nil {
		t.Fatal(err)
	}
	if corpus.Stacks[0].BackgroundsComplete || len(corpus.Stacks[0].Cards) != 1 || len(corpus.Stacks[0].Diagnostics) == 0 {
		t.Fatal("malformed owner acquired a complete scope or discarded readable cards")
	}
	p, err := NewParserFromCorpus(corpus)
	if err != nil {
		t.Fatal(err)
	}
	if _, verified := p.backgroundScripts(p.stacks[0]); verified {
		t.Fatal("malformed owner became verified during analysis")
	}
}

func TestPublicXMLReadersPreserveScriptEvidence(t *testing.T) {
	dir := writeCorpusXML(t, map[string]string{
		stackFileName: `<stack><name>Example</name><card id="1" name="Gate"/><script>  stack text  </script></stack>`,
		"card_1.xml":  `<card><id>1</id><script>  card text  </script></card>`,
	})
	stack, names, err := ParseStackFile(filepath.Join(dir, stackFileName))
	if err != nil {
		t.Fatal(err)
	}
	card, err := ParseSimpleCard(stack, filepath.Join(dir, "card_1.xml"), names)
	if err != nil {
		t.Fatal(err)
	}
	if *card.OriginalName != "Gate" || stack.Script[0].RawText != "  stack text  " || card.Scripts[0].RawText != "  card text  " || !strings.HasSuffix(card.Source.Path, "card_1.xml") {
		t.Fatal("compatibility readers lost original data")
	}
}

func TestUnloadedPathContextDoesNotAcquireBareNameProof(t *testing.T) {
	p, source, _ := unquotedNameFixture()
	// A direct test object may be outside p.cards. Its file path still cannot
	// stand in for an explicitly loaded and verified background scope.
	p.cards = p.cards[1:]
	source.Filepath = filepath.Join(t.TempDir(), "card_1.xml")
	if link, err := p.parseCardCommand(source, "go card Black"); err != nil || link != nil {
		t.Fatalf("unloaded file-bearing context acquired a name proof: %+v, %v", link, err)
	}
}

func TestXMLOriginalNamesRemainManifestAuthoritative(t *testing.T) {
	dir := writeCorpusXML(t, map[string]string{
		stackFileName:      `<stack><name>Example</name><background id="7" file="background_7.xml"/><card id="1"/><card id="2"/></stack>`,
		"card_1.xml":       `<card><id>1</id><owner>7</owner><script>go card Black</script></card>`,
		"card_2.xml":       `<card><id>2</id><name>Black</name><owner>7</owner></card>`,
		"background_7.xml": `<background><id>7</id></background>`,
	})
	corpus, err := LoadXML(dir)
	if err != nil {
		t.Fatal(err)
	}
	if corpus.Stacks[0].Cards[1].Name != "" {
		t.Fatal("a local card name replaced the unnamed manifest entry")
	}
	p, err := NewParserFromCorpus(corpus)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.links) != 0 || p.cards[1].OriginalName != nil {
		t.Fatal("local XML name acquired an unintended bare-name reference")
	}
	link, err := p.parseCardCommand(p.cards[0], `go card "Black"`)
	if err != nil || link == nil || !link.IsNotImplemented || link.Target == p.cards[1] {
		t.Fatalf("local XML name changed quoted-name resolution: %+v, %v", link, err)
	}
	for _, names := range []map[int]string{nil, {}, {2: ""}, {2: "Manifest name"}} {
		card, err := ParseSimpleCard(p.stacks[0], filepath.Join(dir, "card_2.xml"), names)
		if err != nil {
			t.Fatal(err)
		}
		want := names[2]
		if want == "" && card.OriginalName != nil || want != "" && (card.OriginalName == nil || *card.OriginalName != want) {
			t.Fatalf("map=%v: unexpected original name %v", names, card.OriginalName)
		}
	}
}
