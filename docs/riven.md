# Riven (1997) through the DeMystify pipeline

The original five-CD edition of **Riven: The Sequel to Myst** uses Mohawk archives and script bytecode rather than Myst's HyperCard stacks. DeMystify's Go `riven` package reads those inputs and builds the existing `common.Metadata` model. The command then uses the same Gonum graph, analysis, DOT/PDF generators, asset preparation and browser viewer as Myst.

## Run

Use Go as specified by `go.mod` and all five original CD images, or the corresponding extracted `*_Data*.mhk` archives. Keep inputs outside Git or in ignored `riven-input/`.

```bash
go run . --game riven --format html \
  --extract-images generated/riven-assets \
  "/path/to/Riven CDs"
```

`--game auto` is the default: ISO/Mohawk input selects Riven; native HyperCard or converted XML input retains Myst. `--game myst` or `--game riven` selects an engine explicitly. No Python application or intermediate conversion is needed for Riven.

The importer reads ISO9660 directories and requested archive ranges directly. The images need not be mounted, audio/video are not unpacked, and original files remain read only. The original CD overlays are resolved in engine precedence: `J_DATA3 > J_DATA2 > J_DATA1` and `B_DATA1 > B_DATA`.

The example decodes one stored default view per card into an ordinary `images.json` asset directory, then passes it through the same asset pipeline as `--assets`. Existing decoded pictures can instead be supplied with `--assets /path/to/views`; that option cannot be combined with `--extract-images`.

## Outputs and layouts

Riven defaults to the ignored `generated/riven/` output root, with its browser export in `generated/riven/web/`. Open `web/index.html` directly: no server, CDN or JavaScript package installation is needed.

The existing output flags also apply:

```bash
# Topology and evidence only; does not require Graphviz.
go run . --game riven --format dot --output /path/to/local/export "/path/to/Riven CDs"

# Offline atlas using the built-in layout, without Neato.
go run . --game riven --format html --web-layout atlas "/path/to/Riven CDs"

# Shared PDF/browser graph and decoded default card views.
go run . --game riven --format all \
  --extract-images generated/riven-assets "/path/to/Riven CDs"
```

`--output` selects the DOT, PDF and evidence directory. Unless `--web-output` is explicit, the browser is written to its `web/` subdirectory. `--web-layout auto|pdf|atlas` and `--layout original|ages` retain the existing placement choices. Neato is required for PDF output and the PDF-style browser drawing. The generated browser stores its drawing locally and does not require Graphviz when opened.

Riven evidence exports contain the card/script ASTs, original names and RMAP identities, resource byte offsets and SHA-256 hashes, local branch guards, native-source links, absent targets and explicitly unresolved returns. They are generated locally from the supplied game files. No such game-derived exports are committed.

## Explore

Riven uses the same top bar, typography, colors, map controls, stack filters, image/PDF layouts and inspector as the Myst viewer. Its game metadata changes the branding and entry card, and enables the relevant navigation-layer and source-inspection panels.

Search by a label such as `Temple:155`, an internal identity such as `tspit:155`, an original name, picture name or RMAP address. Selecting a card shows its stored picture, hotspot geometry, complete script disassembly, provenance and incoming/outgoing references with callsite evidence. Source-ID deep links remain supported.

The optional layer controls distinguish native interpreter transitions, previously visited Zip shortcuts and ending resets. The structural path panel follows the selected layers without executing puzzle state. Saved inventory returns remain caller-sensitive metadata rather than shortcuts to every card.

The bitmap decoder chooses one default PLST picture, rather than executing scripts to reconstruct every possible appearance. Pictures, stored hotspots and raw flags are inspection data; movie compositing, runtime visibility and puzzle changes are not simulated.

## Interpretation

A graph node represents one shipped card, potentially with several rendered states, or an explicitly marked absent target. A script edge means a CARD/HSPT script contains a navigation command. All nested switch branches are combined; their local guards and offsets remain in the evidence. A handler requesting several cards contributes references from its owner, rather than inferred independent player actions.

On the validated five-CD dataset there are 2,307 shipped cards, five virtual absent targets and 6,686 typed directed edges. All 6,980 opcode-2/27 callsites are retained, including eight references to those five absent targets. The 606 potential Zip shortcuts and four ending resets are distinct optional layers. Typed edge counts can exceed the shared graph engine's unique-pair count.

Ordinary shared graph path search excludes Zip shortcuts and ending resets, along with Myst's existing saved-return and automatic-continuation exclusions. Native movement is included but separately attributed. Node source/sink flags summarize extracted references, including displayed special edges; they are not proofs of gameplay accessibility.

Native facts are reviewed against a pinned ScummVM Mohawk implementation. The 13 destination-time redirects preserve their literal requested targets. Original disc bytecode is not modified by current ScummVM runtime patches. Opcode 36 is structurally decoded with raw arguments retained, while its purpose remains unknown in the reference engine.

This imports the data and bytecode; it does not recover the original interpreter's C++ source or run the complete game. A graph path is not a walkthrough. An unreached, source or sink card does not by itself prove that an asset is unused or invisible. The original five-CD edition is validated; DVD, remaster and localized variants are not verified.

See [the native-navigation audit](riven-native-navigation.md) for engine-source links and the reviewed native navigation sites.

## Validate

The normal repository checks cover the import, shared graph integration and viewer using synthetic fixtures:

```bash
go test -race ./...
go vet ./...
node --test renderer/web/viewer_test.cjs
```

Original game inputs are optional local test data and must never be committed. Set `RIVEN_TEST_DATA` to the directory containing the five CDs to run dataset regressions when available. Those tests reconcile resource counts, overlay precedence, all navigation callsites, absent targets, native attribution and bitmap decoding. The viewer's DOM/canvas tests exercise interaction contracts rather than replacing visual browser review.

The existing Myst integration tests remain available through their documented environment variables. The Myst Go entry point, original frontend controls and repository-generated Myst files are preserved.
