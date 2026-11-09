package main

import (
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

HTML exports open offline. The PDF layout requires Neato during generation;
--web-layout atlas uses the built-in layout without Neato.
`

type outputOptions struct {
	format, assetsDir, webDir, webLayout, input string
}

func parseOutputOptions(args []string) (outputOptions, error) {
	var options outputOptions
	flags := flag.NewFlagSet("demystify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.format, "format", "all", "output formats")
	flags.StringVar(&options.assetsDir, "assets", "", "decoded view images")
	flags.StringVar(&options.webDir, "web-output", "generated/web", "browser export directory")
	flags.StringVar(&options.webLayout, "web-layout", "auto", "browser placement")
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

	if err := os.MkdirAll(filepath.Dir(graphFilePath), 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err := os.WriteFile(graphFilePath, []byte(dotContent), 0644); err != nil {
		return fmt.Errorf("saving DOT file: %w", err)
	}
	var drawing *pdflayout.Drawing
	pdfRendered := false
	if usePDFLayout {
		fmt.Println("Preparing the shared PDF graph layout...")
		var value pdflayout.Drawing
		if options.format == "all" {
			var encoded []byte
			encoded, err = pdf.RenderPDFAndLayout(graphFilePath)
			if err == nil {
				value, err = pdflayout.FromJSON(encoded)
			}
			pdfRendered = err == nil
		} else {
			value, err = pdflayout.Run(dotContent)
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
		return pdf.RenderPDF(graphFilePath)
	}
	return nil
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
