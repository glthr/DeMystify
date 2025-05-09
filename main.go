package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"

	"github.com/glthr/DeMystify/common"
	"github.com/glthr/DeMystify/graph"
	"github.com/glthr/DeMystify/parser"
	pdf "github.com/glthr/DeMystify/renderer"
	"github.com/glthr/DeMystify/renderer/dot"
)

const graphFilePath = "generated/graph.dot"

func main() {
	// ensure that Neato is installed (to generate the PDF file)
	// NOTE: should be refactored by using a library to create the PDF file,
	// or render the graph in a different format
	err := CheckNeatoInstalled()
	if err != nil {
		log.Fatalf("Neato is not installed: %v", err)
	}

	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go <xml_hypercard_files_directory_path>")
	}

	stacksDir := os.Args[1]

	// parser: extract information from the stacks and cards
	fmt.Println("Parsing stacks and cards...")

	p, err := parser.NewParser(stacksDir)
	if err != nil {
		log.Fatalf("unable to parse stacks and cards: %v", err)
	}

	metadata, err := p.Process()
	if err != nil {
		log.Fatalf("parser error: %v", err)
	}

	// guard clause (helpful for external contributors...)
	// NOTE: to replace with a more robust mechanism, like hashing the files
	if metadata.TotalStacks != 6 {
		log.Fatalf("expected 6 stacks (Ages), got %d", metadata.TotalStacks)
	}

	if metadata.TotalCards != 1355 {
		log.Fatalf("expected 1355 cards, got %d", metadata.TotalCards)
	}

	fmt.Printf("Stack count: %d\n", metadata.TotalStacks)
	fmt.Printf("Cards count: %d\n", metadata.TotalCards)

	// graph generation
	fmt.Println("Generating the Myst Graph...")

	g, err := graph.NewGraph(metadata)
	if err != nil {
		log.Fatalf("error while instantiating the graph: %v", err)
	}

	g.Process()

	fmt.Printf("Nodes count: %d\n", metadata.TotalNodes)
	fmt.Printf("Edges count: %d\n", metadata.TotalEdges)

	// calculate the shortest path between start (Myst:8336) and end (Dunny Age:11088) of the game
	// shortestPath, err := ComputeShortestPath(p, g, "Myst", 8336, "Dunny Age", 11088)
	// if err != nil {
	// 	log.Fatalf("error while computing the shortest path: %v", err)
	// }

	// graph rendering
	fmt.Println("Generating the DOT file...")

	dotGenerator := dot.NewGenerator(g, metadata)

	// NOTE: replace `nil` (no path) with `shortestPath.Path` (shortest path) or with any other path
	// like `metadata.Stats.MostSeparatedNodes.Path` (most separated nodes)
	dotContent, err := dotGenerator.Generate(nil)
	if err != nil {
		log.Fatalf("error generating the graph rendering: %v", err)
	}

	if err := os.WriteFile(graphFilePath, []byte(dotContent), 0644); err != nil {
		fmt.Printf("Error saving file: %v\n", err)
		return
	}

	fmt.Println("Generating the PDF file...")

	pdf.RenderPDF(graphFilePath)
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
		log.Fatalf("error while getting card: %v", err)
	}

	toNode, ok := g.GetNodeID(to.Name)
	if !ok {
		return nil, common.NodeNotFoundErr
	}

	return g.ComputeShortestPath(fromNode, toNode, nil)
}

func CheckNeatoInstalled() error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("where", "neato")
	} else {
		cmd = exec.Command("which", "neato")
	}

	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return fmt.Errorf("Neato is not installed")
	}

	return nil
}
