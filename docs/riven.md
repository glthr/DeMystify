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

The optional layer controls distinguish native interpreter transitions, previously visited Zip shortcuts and ending resets. Dashed green references have local conditions; gray disabled references and purple automatic continuations appear in the legend when present. The inspector retains each reference's guards, hotspot availability and classification reason. The structural path panel follows the selected layers without executing puzzle state. Saved inventory returns remain caller-sensitive metadata rather than shortcuts to every card.

The bitmap decoder chooses one default PLST picture, rather than executing scripts to reconstruct every possible appearance. Pictures, stored hotspots and raw flags are inspection data; movie compositing, runtime visibility and puzzle changes are not simulated.

## Audit

A card graph says which cards name which other cards. `--audit` adds what only a view of the whole game can say, and writes it to `audit.json` beside the other evidence files:

```bash
go run . --game riven --format dot --audit --output /path/to/local/export "/path/to/Riven CDs"
```

The run ends with a summary. On the verified 1.0 discs:

```text
Variables: 163 (never written: adoit; never read: gcathtime, ostove)
Dead branches: 28 (adoit = 1; bdrwr = 2; gnmagcar = 0)
Picture entries never selected: 19 of 4147 (7 only in dead branches); 152 more on 16 cards with native calls
Hotspots: 8498; inert movement hotspots: 66; blank cards: Gehn:89, Prison:46; empty or inverted rectangles: Temple:371 hotspot 9
Reachable from Temple:155: 2262 cards; Temple:133 stands before 2235 of them
Zip: 606 shortcuts to 189 places; largest shortest distance 125 without, 74 with
```

- **Variables.** For each of the game's variables: the value a new game starts with, the literal values that scripts store, the values that `switch` commands compare it with, and the cards that do either. A variable that nothing stores is `neverWritten`; one that nothing tests is `neverRead`.
- **Dead branches.** A `switch` case is dead when it tests a value that the variable can never hold: not its initial value, and not a value that any script stores. A nested case that contradicts the case enclosing it is dead too. This is the nearest thing Riven's bytecode has to a commented-out line of Myst.
- **Pictures.** Every picture-list entry other than entry 1 (the interpreter's default) that no live branch of any script selects, with the bitmap it names. A selection made after a change of card in the same handler counts for the card just entered.
- **Hotspots.** How many of the hotspots named `left`, `forwardleft`, `forward`, `forwardright`, `right`, `up` and `down` still have the number (1 to 7) and the rectangle of the template they come from; the movement hotspots that do nothing; the cards that are that template and nothing else; and the hotspots whose rectangle is empty or inverted, which the interpreter disables when it loads them (one in the 1.0 game: Temple:371, hotspot 9, a Zip hotspot).
- **Structure.** The cards that every route from the start to many others must cross (dominators), the unavoidable cards on the way to the farthest one, how often a link owned by a turn or by a forward hotspot has its reverse, the groups of cards joined by turns, and what Zip mode does to distances when every shortcut is available.

The audit executes nothing, and it is deliberately cautious. A variable that the pinned interpreter reads or writes natively, or that a script increments, is assumed able to hold any value, so none of its cases is reported dead. Picture entries on a card whose scripts call a native routine are counted apart, since that routine may draw any of them. The initial values and the list of natively used variables are tables in `riven/audit.go`, taken from the same pinned ScummVM revision as the other native facts. A dead branch or an unselected picture is a fact about the decoded scripts; it does not say why it is so, and it is not a playthrough.

## Verified CD editions

Two English Windows five-CD sets have been checked independently:

| Set | Disc 1 README | Bundled Windows interpreter | Standalone Mohawk archives |
| --- | --- | --- | --- |
| Initial 1.0 | Version 1.0, September 15, 1997 | `Riven.exe` 1.0, fixed version 1.0.0.0 | 26; no `B_DATA1.MHK` or `J_DATA3.MHK` |
| Updated 1.02 | Version 1.02, November 12, 1997 | `Riven.exe` 1.02, fixed version 1.0.2.0 | 28; includes those two overlays |

The 26 common standalone archives have identical full SHA-256 hashes in the two verified sets. The separately compressed, installed `Extras.MHK` also differs between these editions; it is outside this archive comparison. The overlays replace Boiler `HSPT:284`, Jungle `CARD:255`, Jungle `CARD:538` and Jungle `HSPT:255`, among other resources. Their animation and effect handling differs; the literal navigation commands and local branch guards in these four script resources are unchanged. Keeping the original overlays preserves the supplied edition's scripts and byte offsets. Removing them from a later copy is not a substitute for establishing an independent 1.0 source.

Use the Disc 1 README and the actual interpreter's version resource to identify these sets. Folder names, upload dates, generic installer `ProductVersion = 1.0` fields and an `A_DATA.MHK` detector hash do not establish the interpreter version: that data archive is shared by both sets. DeMystify imports whichever archives are supplied and records their provenance; it does not infer or execute the original interpreter version. The Macintosh application and localized or DVD editions are not covered by these Windows version checks.

## Interpretation

A graph node represents one shipped card, potentially with several rendered states, or an explicitly marked absent target. A script edge means a CARD/HSPT script contains a navigation command. All nested switch branches are combined; their local guards and offsets remain in the evidence. A handler requesting several cards retains every literal reference from its owner.

On each validated five-CD dataset there are 2,307 shipped cards and five virtual absent targets. The original 6,686 `(source, target, kind)` combinations are preserved. Classification separates 14 parallel records whose conditions differ, yielding 6,700 original records; 21 derived automatic-continuation records bring the export to **6,721 typed records**. All 6,980 opcode-2/27 callsites are retained exactly once, including eight references to those five absent targets. The 606 potential Zip shortcuts and four ending-reset records are distinct optional layers. Typed records can exceed the shared graph engine's unique-pair count.

Hotspot availability is reported separately as `enabled`, `disabled`, `conditional` or `unknown`. The analysis models ordinary hotspots' enabled default, first-match BLST/direct activation writes, entry phases, later events and caller-owned writes after a card change. Stored HSPT flags are not an off default. Zip depends on its runtime option and visited-card registry. Native code, unresolved callbacks and ambiguous current-card effects prevent a permanent disabled proof. Empty geometry alone does not prove that a stored handler cannot run. The verified corpora produce **zero proven permanently disabled records** and 835 conditional records; unknown activation remains explicitly unknown. The disabled renderer is covered by synthetic fixtures.

A `Conditional` reference retains its local switch guards, reviewed native condition or conditional hotspot activation. It is a structural possibility, not proof that the conditions can be satisfied together. This conservative bytecode analysis does not solve every combination of puzzle variables, trace the complete game state or prove global gameplay reachability.

The 21 `sequence-continuation` references describe two ordered card requests in the same handler and the same straight-line branch. For example, Garden 9 requests card 11 and then card 12; the original 9→11 and 9→12 instructions remain, with a derived 11→12 continuation identifying the original caller and both byte offsets. The inference stops at opaque/native/deferred effects, uncertain callbacks and branch boundaries. These steps describe an automatic script continuation, not a separate player click. They are not mathematical all-to-all transitive closure, and the importer does not remove arbitrary direct references just because a longer route exists.

Ordinary shared graph path search excludes disabled references, automatic continuations, Zip shortcuts and ending resets, along with Myst's existing saved-return exclusions. Native movement is included but separately attributed. The browser and PDF drawing preserve these flags on parallel records. Node source/sink flags summarize extracted references, including displayed special edges; they are not proofs of gameplay accessibility.

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

`RIVEN_TEST_EDITION` defaults to `1.02` for existing test commands. Unknown values fail; the tests do not guess an edition from a shared data hash. `RIVEN_TEST_INPUT` remains an alias for the test-data path. Each profile checks the exact archive count and full archive SHA-256 fingerprints, the selected source archives of the four replaced script resources, its decoded command count (45,502 for 1.0; 45,491 for 1.02), all 6,980 navigation callsites and their original byte offsets, absent targets, native attribution and all 2,307 default bitmap views. Both sets produce the same classified endpoint graph and representative decoded picture hashes. The 6,686 original typed endpoint combinations, all 7,609 original evidence identities, and the 21 derived continuations are checked separately; splitting a parallel record cannot duplicate or discard an instruction. If `RIVEN_REFERENCE_DATA` is also set, point it at an independent pre-classification extraction of the same edition so byte-offset comparisons use the matching scripts. Only the new derived continuation kind is excluded from that original-reference comparison. The viewer's DOM/canvas tests exercise interaction contracts rather than replacing visual browser review.

The existing Myst integration tests remain available through their documented environment variables. The Myst Go entry point, original frontend controls and repository-generated Myst files are preserved.
