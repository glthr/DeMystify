# DeMystify

![](./DeMystify.png "DeMystify")

DeMystify is an unofficial open-source project graph-based analysis of the classic Myst game. In other words, it represents Myst as a graph.

## The Myst Graph

The Myst Graph represents the game's cards as a network showing how they are connected. The generated full graph contains 1,364 nodes and 3,232 non-disabled directed connections, before reverse pairs are combined for display. Counts can vary with the input files.

The analysis extracts links from supported HyperTalk commands without executing the scripts. Shortest paths describe the static card graph and exclude disabled and backtracking edges. They do not simulate puzzle conditions, variables, or the HyperCard return stack; restrictive-transitivity IDs associate edges visually rather than enforce stateful navigation. A graph path therefore does not guarantee a playable route through the game.

## Generate the Myst Graph

### Prerequisites

*   A 1993 Myst CD-ROM (Macintosh)
*   [Go](https://go.dev/doc/install)
*   [Neato](https://graphviz.org/docs/layouts/neato/)

Copy the six HyperCard stacks from the CD-ROM's `Myst Files` directory: `Channelwood Age`, `Dunny Age`, `Mechanical Age`, `Myst`, `Selenitic Age` and `Stoneship Age`. DeMystify reads these files directly into Go objects; no conversion tool or intermediate XML is required.

The native reader supports HyperCard 2.x format 10 data forks. It loads card and background identities, original card order, field text and stack/card/background/part HyperTalk scripts. It validates block boundaries and references and reports skipped blocks through `hypercard.Stack.Diagnostics`.

It does not decode artwork, audio or movies, execute scripts, decrypt encrypted stacks, or open ISO/HFS images and MacBinary wrappers. OSA scripts are unsupported. Resource forks are unnecessary for the static graph. Neato is used to lay out and render the PDF.

### Generate the Myst Graph

1.  **Navigate to the DeMystify directory**
2.  **Run DeMystify:**

    ```bash
    $ go run . "/path/to/Myst Files"
    ```

    *   Use the directory containing the six original stacks. Previously converted XML stack directories remain supported and are detected automatically. If both input formats are present, select a directory containing only the format you want to use.
3.  **Wait:** The graph generation process takes several minutes.
4.  **The Myst Graph is generated:** The generated DOT and PDF files will be saved in the `generated` subdirectory.

For programmatic use, `hypercard.ReadFile(path)` returns a decoded stack and `parser.NewParser(path)` accepts either a single native stack or a directory. `parser.LoadInput(path)` exposes the complete input model, including source offsets and diagnostics; `parser.NewParserFromCorpus(corpus)` analyzes an in-memory model without reopening its source files. The command above expects the complete six-stack Myst dataset.

### Add the publication border, header and footer

The app generates the raw vector graph in `generated/graph.pdf`. To reproduce the original publication layout around it:

```bash
python3 -m venv /tmp/demystify-pdf
/tmp/demystify-pdf/bin/python -m pip install -r scripts/requirements-pdf.txt
/tmp/demystify-pdf/bin/python scripts/frame_graph_pdf.py generated/graph.pdf generated/myst_graph.pdf
```

The optional formatter preserves the graph at its native size and adds the original margins, Times-Roman heading, border and clickable `glthr.com` footer. Use `--date YYYY-MM-DD` to set the footer date, or `--subtitle "..."` for a path-specific graph. The raw PDF and DOT remain unchanged.

## Stay Up-to-Date

*   [The Myst Graph: A New Perspective on Myst](https://glthr.com/myst-graph-1)
*   [The Myst Graph, 2: Revealing New Findings](https://glthr.com/myst-graph-2)
*   [The Myst Graph, 3: Creating the Graph with DeMystify](https://glthr.com/myst-graph-3)

## Disclaimer

This project is a personal initiative to analyze and understand the classic Myst game. It is an unofficial and nonlucrative open-source effort created for educational purposes only. This project is not affiliated with Cyan Worlds or the original creators of Myst.

## License and Credits

DeMystify is released under the MIT License, acknowledging the original creators of Myst and Cyan Worlds for inspiring this project.
