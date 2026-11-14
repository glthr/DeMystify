# Riven (1997) card graph

The optional `riven_graph` package extracts a directed card-reference graph from the original five-CD edition of **Riven: The Sequel to Myst**. It reads ISO9660 images and Mohawk archives directly, disassembles the CARD/HSPT bytecode, and writes a local browser atlas using DeMystify's visual style. The Myst Go application is unchanged.

## Requirements

- Python 3.10 or newer. The extractor and bitmap decoder use the standard library.
- All five original CD images, or the corresponding extracted `*_Data*.mhk` files. DVD, remaster and localized variants have not been validated.
- Optional: Graphviz `sfdp` and `neato` for the organic graph placement and PDF-style browser drawing. Without them, the browser uses a deterministic atlas layout.
- Optional: ReportLab for the printable vector poster. It is not needed for extraction or the browser viewer.

The original inputs are read only. ISO files need not be mounted, and audio/video media are not unpacked. Keep inputs outside Git or in the ignored `riven-input/` directory. Game archives, images and generated game scripts must not be committed.

## Build the local atlas

Run from the repository root, using the directory containing all five ISO files:

```bash
python3 -m riven_graph "/path/to/Riven CDs" --output generated/riven
```

Several ISO paths or a directory of extracted Mohawk data archives may also be supplied. The command requires all eight stacks and applies the CD overlay precedence (`J_DATA3 > J_DATA2 > J_DATA1`, `B_DATA1 > B_DATA`). Open `generated/riven/index.html` directly in a browser. No server, network access, CDN or JavaScript package installation is required.

To add stored card pictures, decode one default PLST view per card and refresh the page:

```bash
python3 -m riven_graph.bitmap "/path/to/Riven CDs" --output generated/riven
python3 -m riven_graph.web_export --output generated/riven
```

The bitmap extractor writes full PNGs, thumbnails and `images-manifest.json`. Unsupported or absent pictures are reported explicitly; unchanged images are reused by resource hash. These are stored base views, not reconstructed movie compositing or executed puzzle states. Cards can contain several picture records; the extractor chooses a default view rather than exporting every runtime appearance.

To generate the optional printable poster after extraction:

```bash
uv run --no-project --with reportlab python -m riven_graph.poster \
  generated/riven/graph.json --output output/pdf/riven-graph.pdf
```

If ReportLab is already installed in the selected Python environment, `python3 -m riven_graph.poster` can be used instead of `uv run` with the same arguments.

## Explore

The page follows the Myst atlas layout: search in the top bar, stack and highlight panels on the map, PDF graph / Image atlas controls, zoom and fit controls, and a selected-card inspector. Search by a label such as `Temple:155`, an internal identity such as `tspit:155`, an original name, picture name or RMAP address. Old `#tspit%3A155` deep links remain supported.

Selecting a card shows its stored picture, hotspot geometry and hotspot scripts, original names and RMAP, complete script disassembly, resource provenance, and incoming/outgoing references with expandable callsite evidence. Keyboard and pointer controls match the Myst viewer. The navigation layers independently toggle native transitions, Zip shortcuts and ending resets. The structural path panel follows the selected layers and does not execute game state.

## Data exports

The default Riven output directory is `generated/riven/`, which is ignored by Git:

| File | Contents |
| --- | --- |
| `index.html` | Offline atlas, with its graph and viewer code embedded |
| `graph.json` | Typed edges, every script callsite, virtual targets, statistics and unresolved saved returns |
| `cards.json` | Parsed script ASTs, NAME tables, RMAPs, picture records and hotspot geometry |
| `manifest.json` | Source containers, absolute byte offsets, resource SHA-256 hashes, overrides and warnings |
| `riven.graphml` | Portable graph for Gephi, Cytoscape and other graph tools |
| `riven.dot` | Graphviz graph, omitting Zip edges for readability |
| `edges.csv` | Typed edges and JSON evidence |
| `scripts/<stack>/<id>.txt` | Readable card/hotspot disassembly, plus explicit missing-target descriptions |
| `analysis.md` | Counts, structural findings, projection rules and limitations |
| `images-manifest.json`, `images/` | Optional decoded default pictures and thumbnails |
| `web-layout.json` | Precomputed PDF-style browser drawing, when Graphviz is available |

On the tested five-CD dataset, the extractor reads 2,307 shipped cards and preserves five referenced but absent CARD resources as virtual nodes. It accounts for all 6,980 opcode-2/27 navigation callsites and produces 6,686 typed edges, including optional Zip and ending-reset layers. These counts identify that dataset rather than all Riven editions.

## Interpretation and limits

A node represents one shipped card, potentially with several rendered appearances, or a marked absent target. A script edge means a CARD/HSPT script contains a navigation command. Nested switch branches are combined, and their local guards are preserved. Repeated edges merge only when source, resolved target and kind agree; every callsite remains in the evidence.

The graph does not execute variables, puzzle state, hotspot enablement, input timing or complete script order. Several sequential navigation commands produce references from the owning card rather than inferred independent clicks. A shortest graph path is not a walkthrough, and an unreached/source/sink card is not proof of an unused or invisible asset.

Native transitions are separately attributed to the pinned ScummVM Mohawk implementation. The 13 destination-time cross-stack redirects preserve their literal requested targets. Caller-sensitive inventory returns remain metadata rather than all-to-all shortcuts. Original disc scripts are not modified by current ScummVM runtime patches. Opcode 36 is parsed and its raw arguments retained, but its purpose remains unknown in the reference engine.

See [the native-navigation audit](riven-native-navigation.md) for the reviewed engine sites and pinned source links. This extracts the data and script bytecode; it does not recover the original interpreter's C++ source.

## Validation

The committed fixtures are synthetic. Run these without game data:

```bash
python3 -m unittest discover -s tests -p 'test_*.py'
node --test tests/test_viewer.cjs
```

Tests that need the five discs or generated metadata skip when those local inputs are absent. Set `RIVEN_TEST_DATA` to a directory of all five CD images to run the optional integration checks for resource counts, overlay precedence, all navigation callsites, RMAP redirects and native attribution. A local `riven-the-sequel-to-myst_202302/` directory is also recognized. Web-data regression tests run when `generated/riven/graph.json` and `cards.json` are present, or `RIVEN_WEB_TEST_DATA` points to another generated Riven directory:

```bash
RIVEN_TEST_DATA="/path/to/Riven CDs" \
RIVEN_WEB_TEST_DATA="/path/to/local/Riven export" \
  python3 -m unittest discover -s tests -p 'test_*.py'
```

The browser canvas tests cover interaction contracts rather than visual browser QA. The local page has also been checked in Safari for layout, search, image/hotspot inspection, navigation layers and structural paths.

## License and references

The package is part of DeMystify under the repository's MIT license. Its adapted frontend retains the DeMystify copyright and license in both source and the generated page. Game-derived local exports are not included in the repository.

The format readers and pixel codec are independently implemented in Python, with facts verified against the [ScummVM Mohawk engine at commit `aed0208e772b128d3f953207a297fc521c32833a`](https://github.com/scummvm/scummvm/tree/aed0208e772b128d3f953207a297fc521c32833a/engines/mohawk).

Background: [Riven Navigator](https://glthr.com/riven-navigator), [Riven Diffs](https://glthr.com/riven-diffs) and [The Myst Graph implementation](https://glthr.com/myst-graph-3).
