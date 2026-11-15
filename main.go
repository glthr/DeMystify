package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
	"github.com/glthr/DeMystify/hypercard"
	"github.com/glthr/DeMystify/parser"
	pdf "github.com/glthr/DeMystify/renderer"
	"github.com/glthr/DeMystify/renderer/dot"
	pdflayout "github.com/glthr/DeMystify/renderer/layout"
	web "github.com/glthr/DeMystify/renderer/web"
	"github.com/glthr/DeMystify/riven"
)

const graphFilePath = "generated/graph.dot"

const usage = `usage: go run . [options] <game_input_directory_or_file>

Options:
  --game auto|myst|riven     Input engine (default auto; Riven: original five-CD edition)
  --output directory        Output root (Myst: generated; Riven: generated/riven)
  --extract-images directory Decode stored Riven base views into local images.json assets
  --audit                    Riven: also write audit.json (variables, dead branches, unselected pictures, structure)
  --format all|html|pdf|dot   Output formats (default all; DOT is always saved)
  --assets directory         Decoded view images and optional images.json mappings
  --web-output directory     Browser export directory (default generated/web)
  --web-layout auto|pdf|atlas Browser placement (default auto: PDF when Neato is available)
  --layout original|ages     Shared PDF graph arrangement (default original)

HTML exports open offline. The PDF layout requires Neato during generation;
--web-layout atlas uses the built-in layout without Neato.
`

type outputOptions struct {
	format, assetsDir, webDir, webLayout, arrangement, input string
	game, outputRoot, extractImages                          string
	webOutputSet, audit                                      bool
}

func parseOutputOptions(args []string) (outputOptions, error) {
	var options outputOptions
	flags := flag.NewFlagSet("demystify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.game, "game", "auto", "input engine")
	flags.StringVar(&options.outputRoot, "output", "", "output root")
	flags.StringVar(&options.extractImages, "extract-images", "", "decode Riven card pictures")
	flags.BoolVar(&options.audit, "audit", false, "write the Riven state and structure audit")
	flags.StringVar(&options.format, "format", "all", "output formats")
	flags.StringVar(&options.assetsDir, "assets", "", "decoded view images")
	flags.StringVar(&options.webDir, "web-output", "generated/web", "browser export directory")
	flags.StringVar(&options.webLayout, "web-layout", "auto", "browser placement")
	flags.StringVar(&options.arrangement, "layout", "original", "shared PDF graph arrangement")
	if err := flags.Parse(args); err != nil {
		return options, fmt.Errorf("%s\n%w", usage, err)
	}
	if flags.NArg() != 1 {
		return options, fmt.Errorf("%s", usage)
	}
	switch options.format {
	case "all", "html", "pdf", "dot":
	default:
		return options, fmt.Errorf("%s\nunknown output format %q", usage, options.format)
	}
	if options.webDir == "" {
		return options, fmt.Errorf("%s\nweb output directory is empty", usage)
	}
	switch options.webLayout {
	case "auto", "pdf", "atlas":
	default:
		return options, fmt.Errorf("%s\nunknown web layout %q", usage, options.webLayout)
	}
	switch options.arrangement {
	case "ages", "original":
	default:
		return options, fmt.Errorf("%s\nunknown graph arrangement %q", usage, options.arrangement)
	}
	if options.game != "auto" && options.game != "myst" && options.game != "riven" {
		return options, fmt.Errorf("%s\nunknown game %q", usage, options.game)
	}
	if options.extractImages != "" && options.assetsDir != "" {
		return options, fmt.Errorf("--extract-images and --assets cannot be combined")
	}
	flags.Visit(func(option *flag.Flag) {
		if option.Name == "web-output" {
			options.webOutputSet = true
		}
	})
	options.input = flags.Arg(0)
	return options, nil
}

func needsPDFLayout(options outputOptions) (bool, error) {
	if options.format != "all" && options.format != "html" || options.webLayout == "atlas" {
		return false, nil
	}
	err := CheckNeatoInstalled()
	if options.webLayout == "pdf" && err != nil {
		return false, err
	}
	return err == nil, nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Print(usage)
		return nil
	}
	options, err := parseOutputOptions(args)
	if err != nil {
		return err
	}
	if options.format == "all" || options.format == "pdf" {
		if err := CheckNeatoInstalled(); err != nil {
			return err
		}
	}
	usePDFLayout, err := needsPDFLayout(options)
	if err != nil {
		return err
	}

	game, err := inputGame(options.input, options.game)
	if err != nil {
		return err
	}
	graphPath := graphFilePath
	if game == "riven" && options.outputRoot == "" {
		options.outputRoot = "generated/riven"
	}
	if options.outputRoot != "" {
		graphPath = filepath.Join(options.outputRoot, "graph.dot")
		if !options.webOutputSet {
			options.webDir = filepath.Join(options.outputRoot, "web")
		}
	}
	if options.extractImages != "" && game != "riven" {
		return fmt.Errorf("--extract-images requires Riven input")
	}
	if options.audit && game != "riven" {
		return fmt.Errorf("--audit requires Riven input")
	}
	var corpus *hypercard.Corpus
	var imported *riven.Import
	var metadata *common.Metadata
	fmt.Println("Parsing stacks and cards...")
	if game == "riven" {
		for _, output := range []string{filepath.Dir(graphPath), options.webDir, options.extractImages} {
			if output != "" {
				if err := protectInputDirectory(options.input, output); err != nil {
					return err
				}
			}
		}
		imported, err = riven.LoadInput(options.input)
		if err != nil {
			return fmt.Errorf("loading Riven input: %w", err)
		}
		defer imported.Close()
		metadata = imported.Metadata
		if options.extractImages != "" {
			options.assetsDir, err = imported.ExtractImages(options.extractImages)
			if err != nil {
				return fmt.Errorf("extracting Riven pictures: %w", err)
			}
		}
	} else {
		value, err := parser.LoadInput(options.input)
		if err != nil {
			return fmt.Errorf("unable to parse stacks and cards: %w", err)
		}
		corpus = &value
		p, err := parser.NewParserFromCorpus(value)
		if err != nil {
			return fmt.Errorf("unable to analyze stacks and cards: %w", err)
		}
		metadata, err = p.Process()
		if err != nil {
			return fmt.Errorf("parser error: %w", err)
		}
		if metadata.TotalStacks != 6 {
			return fmt.Errorf("expected 6 stacks (Ages), got %d", metadata.TotalStacks)
		}
		if metadata.TotalCards != 1355 {
			return fmt.Errorf("expected 1355 cards, got %d", metadata.TotalCards)
		}
	}

	fmt.Printf("Stack count: %d\n", metadata.TotalStacks)
	fmt.Printf("Cards count: %d\n", metadata.TotalCards)

	// graph generation
	fmt.Printf("Generating the %s Graph...\n", map[string]string{"myst": "Myst", "riven": "Riven"}[game])

	g, err := graph.NewGraph(metadata)
	if err != nil {
		return fmt.Errorf("error while instantiating the graph: %w", err)
	}

	if metadata.StartNodeName != "" {
		if id, ok := g.GetNodeID(metadata.StartNodeName); ok {
			metadata.StartNodeID = id
		}
	}
	g.Process()
	if imported != nil {
		if err := imported.WriteEvidence(filepath.Dir(graphPath)); err != nil {
			return fmt.Errorf("saving Riven evidence: %w", err)
		}
	}

	if imported != nil && options.audit {
		audit, err := imported.WriteAudit(filepath.Dir(graphPath))
		if err != nil {
			return fmt.Errorf("saving Riven audit: %w", err)
		}
		defer fmt.Print(audit.Summary())
	}

	fmt.Printf("Nodes count: %d\n", metadata.TotalNodes)
	fmt.Printf("Edges count: %d\n", metadata.TotalEdges)
	fmt.Print(analysisSummary(metadata))

	// calculate the shortest path between start (Myst:8336) and end (Dunny Age:11088) of the game
	// shortestPath, err := ComputeShortestPath(p, g, "Myst", 8336, "Dunny Age", 11088)
	// if err != nil {
	// 	return fmt.Errorf("error while computing the shortest path: %w", err)
	// }

	// graph rendering
	fmt.Println("Generating the DOT file...")

	dotGenerator := dot.NewGenerator(g, metadata)

	// NOTE: replace `nil` (no path) with `shortestPath.Path` (shortest path) or with any other path
	// like `metadata.Stats.MostSeparatedNodes.Path` (most separated nodes)
	dotContent, err := dotGenerator.Generate(nil)
	if err != nil {
		return fmt.Errorf("error generating the graph rendering: %w", err)
	}
	pinned := options.arrangement == "ages" && (usePDFLayout || options.format == "all" || options.format == "pdf")
	if pinned {
		fmt.Println("Arranging Ages and reducing within-Age crossings...")
		flatDOT, err := dotGenerator.GenerateFlat(nil)
		if err != nil {
			return fmt.Errorf("preparing Age layout: %w", err)
		}
		ctx := context.Background()
		original, err := pdflayout.PositionOnly(ctx, dotContent)
		if err != nil {
			return fmt.Errorf("preparing original placement: %w", err)
		}
		flat, err := pdflayout.PositionOnly(ctx, flatDOT)
		if err != nil {
			return fmt.Errorf("preparing alternative placement: %w", err)
		}
		groups := make(map[int64]string, len(original.Nodes))
		for _, node := range metadata.Nodes {
			group := node.StackName
			if group == "" && node.IsOfType(common.IsStack) {
				group = node.Name
			}
			groups[node.GraphID] = group
		}
		arranged, err := pdflayout.ArrangeAges(ctx, original, flat, groups)
		if err != nil {
			return fmt.Errorf("arranging Ages: %w", err)
		}
		dotContent, err = dotGenerator.GeneratePositioned(nil, arranged.Positions)
		if err != nil {
			return fmt.Errorf("saving Age placement: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(graphPath), 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err := os.WriteFile(graphPath, []byte(dotContent), 0644); err != nil {
		return fmt.Errorf("saving DOT file: %w", err)
	}
	fmt.Printf("DOT file: %s\n", graphPath)
	var drawing *pdflayout.Drawing
	pdfRendered := false
	if usePDFLayout {
		fmt.Println("Preparing the shared PDF graph layout...")
		var value pdflayout.Drawing
		if options.format == "all" {
			encoded, renderErr := pdf.RenderToFile(graphPath, filepath.Join(filepath.Dir(graphPath), "graph.pdf"), true, pinned)
			err = renderErr
			if err == nil {
				value, err = pdflayout.FromJSON(encoded)
			}
			pdfRendered = err == nil
		} else {
			if pinned {
				value, err = pdflayout.RunPinned(dotContent)
			} else {
				value, err = pdflayout.Run(dotContent)
			}
		}
		if err != nil {
			return fmt.Errorf("preparing PDF-style browser layout (use --web-layout atlas for the built-in layout): %w", err)
		}
		drawing = &value
	}

	if options.format == "all" || options.format == "html" {
		fmt.Println("Generating the browser viewer...")
		report, err := web.Write(options.webDir, metadata, web.Options{AssetsDir: options.assetsDir, InputDir: options.input, Corpus: corpus, PDFLayout: drawing})
		if err != nil {
			return fmt.Errorf("generating browser viewer: %w", err)
		}
		fmt.Printf("Browser viewer: %s\nCards with images: %d/%d\n", report.IndexPath, report.Images, metadata.TotalCards)
		for i, warning := range report.Warnings {
			if i == 3 {
				fmt.Printf("%d additional image warnings\n", len(report.Warnings)-i)
				break
			}
			fmt.Printf("Image note: %s\n", warning)
		}
	}
	if (options.format == "all" || options.format == "pdf") && !pdfRendered {
		fmt.Println("Generating the PDF file...")
		_, err := pdf.RenderToFile(graphPath, filepath.Join(filepath.Dir(graphPath), "graph.pdf"), false, pinned)
		return err
	}
	return nil
}

// analysisSummary reports what the analysis found, so that the figures the
// project publishes can be read from a run. "Edges count" above is the number
// of non-disabled directed pairs; a pair can hold several records.
func analysisSummary(metadata *common.Metadata) string {
	var returns, steps, disabled int
	for _, edge := range metadata.Edges {
		if edge.IsOfType(common.Backtracking) {
			returns++
		}
		if edge.IsOfType(common.SequenceContinuation) {
			steps++
		}
		if edge.IsOfType(common.Disabled) {
			disabled++
		}
	}
	stats := metadata.Stats
	summary := fmt.Sprintf("Edge records: %d (saved returns: %d, automatic next steps: %d, disabled: %d)\n", len(metadata.Edges), returns, steps, disabled)
	summary += fmt.Sprintf("Source / sink / isolated flags: %d / %d / %d\n", len(stats.NodesWithNoIncoming), len(stats.NodesWithNoOutgoing), len(stats.IsolatedNodes))
	summary += fmt.Sprintf("Nodes with an active reference to themselves: %d\n", len(stats.NodesWithSelfLoops))
	if farthest := stats.MostSeparatedNodes; len(farthest.Path) > 1 {
		summary += fmt.Sprintf("Largest finite shortest distance: %.0f (%s to %s)\n", farthest.Distance, farthest.Source.Name, farthest.Target.Name)
	}
	return summary
}

func ComputeShortestPath(
	p *parser.Parser,
	g *graph.MystGraph,
	fromStack string,
	fromID int,
	toStack string,
	toID int,
) (*common.ShortestPathInfo, error) {
	from, err := p.GetCardByStackAndID(fromStack, fromID)
	if err != nil {
		return nil, err
	}

	fromNode, ok := g.GetNodeID(from.Name)
	if !ok {
		return nil, common.NodeNotFoundErr
	}

	to, err := p.GetCardByStackAndID(toStack, toID)
	if err != nil {
		return nil, err
	}

	toNode, ok := g.GetNodeID(to.Name)
	if !ok {
		return nil, common.NodeNotFoundErr
	}

	return g.ComputeShortestPath(fromNode, toNode, nil)
}

func CheckNeatoInstalled() error {
	if _, err := exec.LookPath("neato"); err != nil {
		return fmt.Errorf("neato is not installed: %w", err)
	}

	return nil
}

// inputGame keeps native HyperCard/XML input as the default and recognizes the
// original Riven ISO/Mohawk containers without adding a second application.
func inputGame(input, requested string) (string, error) {
	if requested != "auto" {
		return requested, nil
	}
	info, err := os.Stat(input)
	if err != nil {
		return "", fmt.Errorf("inspecting game input: %w", err)
	}
	isMohawk := func(name string) bool {
		lower := strings.ToLower(name)
		return strings.HasSuffix(lower, ".iso") || strings.HasSuffix(lower, ".mhk")
	}
	if !info.IsDir() {
		if isMohawk(input) {
			return "riven", nil
		}
		return "myst", nil
	}
	found := false
	err = filepath.WalkDir(input, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && isMohawk(entry.Name()) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("inspecting input directory: %w", err)
	}
	if found {
		return "riven", nil
	}
	return "myst", nil
}

func protectInputDirectory(input, output string) error {
	info, err := os.Stat(input)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}
	source, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(source, target)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("output directory %q overlaps original input %q", output, input)
	}
	return nil
}
