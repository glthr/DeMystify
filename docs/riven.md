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

The importer reads ISO9660 directories and requested archive ranges directly. The images need not be mounted, audio/video are not unpacked, and original files remain read only. Available CD overlays are resolved in engine precedence: `J_DATA3 > J_DATA2 > J_DATA1` and `B_DATA1 > B_DATA`. The verified initial 1.0 set has neither optional patch archive `B_DATA1.MHK` nor `J_DATA3.MHK`; `J_DATA2.MHK` is part of that initial set.

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

## Verified CD editions

Two English Windows five-CD sets have been checked independently:

| Set | Disc 1 README | Bundled Windows interpreter | Mohawk archives |
| --- | --- | --- | --- |
| Initial 1.0 | Version 1.0, September 15, 1997 | `Riven.exe` 1.0, fixed version 1.0.0.0 | 26; no `B_DATA1.MHK` or `J_DATA3.MHK` |
| Updated 1.02 | Version 1.02, November 12, 1997 | `Riven.exe` 1.02, fixed version 1.0.2.0 | 28; includes those two overlays |

The 26 common archives have identical full SHA-256 hashes in the two verified sets. The overlays replace Boiler `HSPT:284`, Jungle `CARD:255`, Jungle `CARD:538` and Jungle `HSPT:255`, among other resources. Their animation and effect handling differs; the literal navigation commands and local branch guards in these four script resources are unchanged. Keeping the original overlays preserves the supplied edition's scripts and byte offsets. Removing them from a later copy is not a substitute for establishing an independent 1.0 source.

Use the Disc 1 README and the actual interpreter's version resource to identify these sets. Folder names, upload dates, generic installer `ProductVersion = 1.0` fields and an `A_DATA.MHK` detector hash do not establish the interpreter version: that data archive is shared by both sets. DeMystify imports whichever archives are supplied and records their provenance; it does not infer or execute the original interpreter version. The Macintosh application and localized or DVD editions are not covered by these Windows version checks.

## Interpretation

A graph node represents one shipped card, potentially with several rendered states, or an explicitly marked absent target. A script edge means a CARD/HSPT script contains a navigation command. All nested switch branches are combined; their local guards and offsets remain in the evidence. A handler requesting several cards contributes references from its owner, rather than inferred independent player actions.

On the validated five-CD dataset there are 2,307 shipped cards, five virtual absent targets and 6,686 typed directed edges. All 6,980 opcode-2/27 callsites are retained, including eight references to those five absent targets. The 606 potential Zip shortcuts and four ending resets are distinct optional layers. Typed edge counts can exceed the shared graph engine's unique-pair count.

Ordinary shared graph path search excludes Zip shortcuts and ending resets, along with Myst's existing saved-return and automatic-continuation exclusions. Native movement is included but separately attributed. Node source/sink flags summarize extracted references, including displayed special edges; they are not proofs of gameplay accessibility.

Native facts are reviewed against a pinned ScummVM Mohawk implementation. The 13 destination-time redirects preserve their literal requested targets. Original disc bytecode is not modified by current ScummVM runtime patches. Opcode 36 is structurally decoded with raw arguments retained, while its purpose remains unknown in the reference engine.

This imports the data and bytecode; it does not recover the original interpreter's C++ source or run the complete game. A graph path is not a walkthrough. An unreached, source or sink card does not by itself prove that an asset is unused or invisible. The two five-CD sets above are validated; DVD, remaster and localized variants are not verified.

See [the native-navigation audit](riven-native-navigation.md) for engine-source links and the reviewed native navigation sites.

## Validate

The normal repository checks cover the import, shared graph integration and viewer using synthetic fixtures:

```bash
go test -race ./...
go vet ./...
node --test renderer/web/viewer_test.cjs
```

Original game inputs are optional local test data and must never be committed. Set `RIVEN_TEST_DATA` to the directory containing the five CDs and explicitly select the verified edition:

```bash
RIVEN_TEST_DATA="/path/to/1.0 CDs" RIVEN_TEST_EDITION=1.0 go test -v ./riven
RIVEN_TEST_DATA="/path/to/1.02 CDs" RIVEN_TEST_EDITION=1.02 go test -v ./riven
```

`RIVEN_TEST_EDITION` defaults to `1.02` for existing test commands. Unknown values fail; the tests do not guess an edition from a shared data hash. `RIVEN_TEST_INPUT` remains an alias for the test-data path. Each profile checks the exact archive count and full archive SHA-256 fingerprints, the selected source archives of the four replaced script resources, its decoded command count (45,502 for 1.0; 45,491 for 1.02), all 6,980 navigation callsites and their original byte offsets, absent targets, native attribution and all 2,307 default bitmap views. Both sets produce the same typed graph and representative decoded picture hashes. If `RIVEN_REFERENCE_DATA` is also set, point it at an independent extraction of the same edition so byte-offset comparisons use the matching scripts. The viewer's DOM/canvas tests exercise interaction contracts rather than replacing visual browser review.

The existing Myst integration tests remain available through their documented environment variables. The Myst Go entry point, original frontend controls and repository-generated Myst files are preserved.
