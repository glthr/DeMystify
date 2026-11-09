# DeMystify

![](./DeMystify.png "DeMystify")

DeMystify is an unofficial open-source project graph-based analysis of the classic Myst game. In other words, it represents Myst as a graph.

## The Myst Graph

The Myst Graph represents the game's cards as a network showing how they are connected. The generated full graph contains 1,364 nodes and 3,225 non-disabled directed connections, before reverse pairs are combined for display. Counts can vary with the input files.

Supported straight-line navigation sequences are represented step by step. For example, two successive literal `go` commands through an intermediate view become `start → intermediate → destination`, rather than two departures from the script's starting card. These continuations are distinct from saved push/pop returns. Branches, dynamic navigation and opaque calls limit which sequences can be resolved statically. Shared intermediate cards can combine continuations from different callers; path search does not retain that caller context. A graph step counts a card change, including automatic transitions, rather than a player click.

The analysis extracts links from supported HyperTalk commands without executing the scripts. Shortest paths describe the static card graph and exclude disabled and backtracking edges. They do not simulate puzzle conditions, variables, or the HyperCard return stack; restrictive-transitivity IDs associate edges visually rather than enforce stateful navigation. A graph path therefore does not guarantee a playable route through the game.

## Generate the Myst Graph

### Prerequisites

*   A 1993 Myst CD-ROM (Macintosh)
*   [Go](https://go.dev/doc/install)
*   [Neato](https://graphviz.org/docs/layouts/neato/) for PDF output and PDF-style browser placement; the built-in image atlas and DOT output work without it

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
4.  **The Myst Graph is generated:** DOT and PDF are saved in `generated`; open `generated/web/index.html` for the browser viewer.

For programmatic use, `hypercard.ReadFile(path)` returns a decoded stack and `parser.NewParser(path)` accepts either a single native stack or a directory. `parser.LoadInput(path)` exposes the complete input model, including source offsets and diagnostics; `parser.NewParserFromCorpus(corpus)` analyzes an in-memory model without reopening its source files. The command above expects the complete six-stack Myst dataset.

### Explore in a browser

Generate the browser output with your decoded view images:

```bash
go run . --format html --assets "/path/to/views" "/path/to/Myst Files"
```

Open `generated/web/index.html` directly in your browser. The viewer works offline, with no server, CDN or JavaScript package installation. Scroll or pinch to zoom, drag to pan, use the zoom/fit controls, filter by Age, or search for a card ID, name or picture name. Zooming into the graph reveals view thumbnails inside its nodes. Selecting a card opens its larger view and incoming/outgoing connections.

When Neato is installed, the browser starts with the PDF graph's organic placement, routed connections, arrow shapes and colors. Switch between **PDF graph** and **Image atlas** in the viewer. In the PDF view, the **Images** control switches between compact thumbnails and the original graph labels, retaining the same node boxes and positions. Card pictures and interaction zones remain available in the side panel in both views.

Saved-return entries use the PDF's inverted arrowheads at both ends, including in the image atlas. They mark extracted push/pop navigation, such as entering a close-up with a saved return; they do not add an unconditional reverse connection to the graph.

Use **Highlight nodes** to emphasize sources, sinks, isolated nodes, active self-references, page cards, absent targets or stacks. Multiple categories combine with OR. Other nodes stay in place and turn gray, along with their unrelated connections; they remain selectable and searchable. Clearing the highlights restores the normal colors. Age visibility controls remain independent.

`--web-layout auto` is the default: use the PDF drawing when Neato is available, otherwise the built-in image atlas. Choose `--web-layout pdf` to require the PDF-style graph or `--web-layout atlas` to generate HTML without Neato. For example:

```bash
go run . --format html --web-layout pdf --assets "/path/to/views" "/path/to/Myst Files"
```

The default `--layout ages` arrangement groups cards by Age and chooses between two organic placements to reduce crossings within each Age. Small disconnected components stay near their own Age. Use `--layout original` to retain the original placement. This option controls the PDF and the browser's **PDF graph** together; the built-in **Image atlas** keeps its separate placement. DOT-only generation skips placement.

When HTML and PDF are generated together, both use the same Graphviz layout pass, including node boxes, curved connections and arrowheads. The PDF contains labels; the browser can display pictures inside those same boxes. The browser stores its drawing locally and needs no Graphviz installation when opened.

Enable **Interactive zones** on the selected view to inspect card and background buttons and fields. Select a zone to read its stored name, visibility/enabled flags and HyperTalk script. These are the geometry and flags saved in the stack; scripts can change them at runtime. The viewer inspects the static graph and does not execute the game.

`--format all` (the default) produces HTML, PDF and DOT. `--format pdf` produces PDF and DOT; `--format dot` produces DOT alone. Use `--web-output /path/to/viewer` to choose the browser export directory. Without `--assets`, the viewer still provides the graph and interactive geometry, with missing views explicitly identified.

The default `generated/web` export is ignored by Git because it contains local game imagery and scripts. Original assets are not included in the repository. Keep custom export/image directories outside Git or in an ignored local directory.

The image directory can contain PNG, JPEG or GIF files. Stack subdirectories scope picture filenames and `card_<id>` filenames. For exact mappings, add an `images.json` file; its paths are relative to that directory:

```json
{"images":[
  {"node":"Example Age:1","path":"views/start.png"},
  {"stack":"Example Age","picture":"Courtyard-N","path":"views/courtyard.png"}
]}
```

When a manifest is present, it defines the mappings. Ambiguous images stay unmapped and are reported. Matching pictures are copied and deduplicated into the browser export, with smaller thumbnails loaded as they enter the viewport. The originals remain unchanged.

### Extract original view images

The optional image extractor uses Python's standard library and [FFmpeg](https://ffmpeg.org/). Point it at the original `Myst Graphics` resource banks with their resource forks preserved, or at extracted `.rsrc`, MacBinary or AppleDouble bank files:

```bash
python3 scripts/extract_myst_images.py "/path/to/Myst Graphics" --output "/path/to/decoded views"
go run . --format html --assets "/path/to/decoded views" "/path/to/Myst Files"
```

It decodes named PICT resources containing a single QuickTime SMC image and writes PNGs, an `images.json` catalogue and an extraction report. Source banks remain unchanged. Unsupported picture formats are reported. These images are stored base views; QuickDraw compositing, movies and runtime puzzle changes are not reconstructed. FFmpeg is needed only for this extraction step, not for graph import or the browser viewer.

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
