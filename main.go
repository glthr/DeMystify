package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
	"github.com/glthr/DeMystify/parser"
	pdf "github.com/glthr/DeMystify/renderer"
	"github.com/glthr/DeMystify/renderer/dot"
	pdflayout "github.com/glthr/DeMystify/renderer/layout"
	web "github.com/glthr/DeMystify/renderer/web"
)

const graphFilePath = "generated/graph.dot"

const usage = `usage: go run . [options] <hypercard_stacks_directory_path>

Options:
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
}

func parseOutputOptions(args []string) (outputOptions, error) {
	var options outputOptions
	flags := flag.NewFlagSet("demystify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
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

	// parser: extract information from the stacks and cards
	fmt.Println("Parsing stacks and cards...")

	corpus, err := parser.LoadInput(options.input)
	if err != nil {
		return fmt.Errorf("unable to parse stacks and cards: %w", err)
	}
	p, err := parser.NewParserFromCorpus(corpus)
	if err != nil {
		return fmt.Errorf("unable to analyze stacks and cards: %w", err)
	}

	metadata, err := p.Process()
	if err != nil {
		return fmt.Errorf("parser error: %w", err)
	}

	// guard clause (helpful for external contributors...)
	// NOTE: to replace with a more robust mechanism, like hashing the files
	if metadata.TotalStacks != 6 {
		return fmt.Errorf("expected 6 stacks (Ages), got %d", metadata.TotalStacks)
	}

	if metadata.TotalCards != 1355 {
		return fmt.Errorf("expected 1355 cards, got %d", metadata.TotalCards)
	}

	fmt.Printf("Stack count: %d\n", metadata.TotalStacks)
	fmt.Printf("Cards count: %d\n", metadata.TotalCards)

	// graph generation
	fmt.Println("Generating the Myst Graph...")

	g, err := graph.NewGraph(metadata)
	if err != nil {
		return fmt.Errorf("error while instantiating the graph: %w", err)
	}

	g.Process()

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

	if err := os.MkdirAll(filepath.Dir(graphFilePath), 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err := os.WriteFile(graphFilePath, []byte(dotContent), 0644); err != nil {
		return fmt.Errorf("saving DOT file: %w", err)
	}
	fmt.Printf("DOT file: %s\n", graphFilePath)
	var drawing *pdflayout.Drawing
	pdfRendered := false
	if usePDFLayout {
		fmt.Println("Preparing the shared PDF graph layout...")
		var value pdflayout.Drawing
		if options.format == "all" {
			var encoded []byte
			if pinned {
				encoded, err = pdf.RenderPinnedPDFAndLayout(graphFilePath)
			} else {
				encoded, err = pdf.RenderPDFAndLayout(graphFilePath)
			}
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
		report, err := web.Write(options.webDir, metadata, web.Options{AssetsDir: options.assetsDir, InputDir: options.input, Corpus: &corpus, PDFLayout: drawing})
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
		if pinned {
			return pdf.RenderPinnedPDF(graphFilePath)
		}
		return pdf.RenderPDF(graphFilePath)
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
