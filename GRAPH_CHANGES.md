# Graph changes

This first-audit ledger uses the supplied XML snapshot of six Myst stacks and 1,355 converted cards. The reference code is `0276f8c`, and its app-generated DOT exactly reproduces [`generated/graph.dot` at that revision](https://github.com/glthr/DeMystify/blob/0276f8c/generated/graph.dot).

A [second audit and fresh raw-stack conversion](AUDIT_ROUND_2.md) subsequently verified the same-input DOT byte-for-byte and recovered one further Dunny relationship from the fresh input (3,215 pairs / 3,228 records). The tables below retain the first audit's controlled comparison.

A [later model review](MODEL_REFINEMENTS.md) restores Myst:8631's source label under the clarified definition, connects the 18 real valve callers, and documents further script forms. The measurements below describe the first audit, before those refinements.

## Maintenance verification

The maintenance code checkpoint is `da4e2bb`. It preserves every node, label, attribute, edge, and declaration order in the reference graph. The complete DOT files match after sorting attributes within each statement and ignoring blank lines. Attribute formatting changes do not change the represented graph.

| Check | Reference | Maintenance |
| --- | ---: | ---: |
| Game nodes | 1,364 | 1,364 |
| Active directed endpoint pairs | 3,189 | 3,189 |
| Edge records including disabled and parallel links | 3,198 | 3,198 |

The reference DOT SHA256 is `9a60b38daacc89ef7b9f5859461a76c100ebf63df7ec49e4be7be302572c0d09`. The maintenance DOT SHA256 is `8201f5bdbcd65b5e4dff91b72a7f6274cf0579d6487c8c289ee0a816e97587f0`.

Maintenance covers input validation, command recognition, rendering robustness, error propagation, and a polynomial shortest-route solver. The default graph rendering does not request a shortest route, so solver changes do not alter this reference graph.

`go test ./...` and `go vet ./...` passed before each commit. The combined maintenance checkpoint also passes race detection and the declared Go 1.24.1 minimum. Original game files and test renderings remain local.

The original-app and maintenance-app PDFs also render to identical pixels at a 2,600-pixel long edge with the same Graphviz 16.1.0 toolchain.

The published `generated/myst_graph.pdf` adds a title, border, and footer after the app renders its graph. DOT is the primary comparison; PDF framing and Graphviz layout changes are assessed separately from graph content.

## Corrections to the represented graph

| Measurement | Original | Corrected |
| --- | ---: | ---: |
| Game nodes | 1,364 | 1,364 |
| Non-disabled directed endpoint pairs | 3,189 | 3,214 |
| Edge records including disabled and parallel links | 3,198 | 3,227 |
| Sources | 37 | 36 |
| Sinks | 51 | 62 |
| Isolated nodes | 19 | 18 |
| Largest weak component | 1,342 | 1,339 |
| Start to ending shortest distance | 24 | 24 |
| Longest finite shortest distance | 130 | 130 |

Endpoint pairs combine parallel records and exclude disabled links. They retain contextual backtracking links in the visualization; shortest-path calculations exclude those links. All 1,364 original node labels remain unchanged.

### Missing named card transitions

Eight distinct transitions to named Black cards were missing. These cards represent screen transitions, so their arrows add intermediate scene connections rather than new locations.

| Source | Target |
| --- | --- |
| Channelwood Age:100894 | Channelwood Age:26005 |
| Dunny Age:11088 | Dunny Age:2203 |
| Mechanical Age:46649 | Mechanical Age:47045 |
| Myst:28552, Myst:55938, Myst:77008 | Myst:23444 |
| Selenitic Age:59926 | Selenitic Age:62357 |
| Stoneship Age:53570 | Stoneship Age:25833 |

### Refreshes and stack handlers

Seventeen active self-loops and four disabled self-loop records were missing because explicit current-card refreshes were ignored. Refreshes stay in the same scene and do not shorten routes between distinct cards. The self-loop on Myst:8631 also removes its incorrect source highlighting.

Three Channelwood handler-definition arrows incorrectly started at Channelwood Age:100047, Temple 3-e. They now start at the Channelwood stack node and lead to cards 86839, 87249, and 87418. Removing those false card connections separates the handler-definition component from the main card component. The stack is no longer isolated and now displays its handler-definition outgoing arrows with source highlighting.

### Sink and edge classifications

Incoming backtracking links incorrectly prevented cards from being classified as sinks. Twelve card nodes gain the missing sink highlighting: Dunny Age:11283; Myst:41677, 66100, 70175, 70509, 70908, 78339; Stoneship Age:40420, 52299, 52563, 54156, 54808. The Channelwood stack loses its former sink status, giving a net increase of eleven sinks. A sink in this static graph does not necessarily mean a gameplay dead end: returning can depend on the HyperCard return stack.

Five return-edge records from Myst:44018 to Myst:46439 were incorrectly classified as cross-age links. Both endpoints are in Myst; their metadata now uses the intra-age classification. Rendered return heads continue to inherit the paired cross-age tail styling, preserving the visual association of each route. Restrictive-transitivity IDs are unique per push/go pair and their tooltip numbers become 0 through 4; the paired colors remain unchanged in this dataset.

### Incorrect shortest routes

The original graph adapter did not expose edge weights to Dijkstra. Backtracking links assigned infinite cost were consequently treated as ordinary one-step moves. Myst:28679 to Myst:44018 was reported as one step; the permitted alternative takes fourteen. Dunny Age:7365 to Dunny Age:11283 was also reported as one step; no permitted route exists under the no-backtracking policy. These corrections affect path analysis and highlighted paths rather than removing contextual arrows from the default visualization.

Independent breadth-first searches confirm the unchanged 24-step start-to-ending route and 130-step diameter. The old single-route solver enumerated equivalent alternatives; the replacement chooses a deterministic route without enumerating them.

## Verification and limits

Synthetic regression tests cover malformed inputs, script extraction, parallel edges, labels, pathfinding, and PDF subprocess errors. Independent small-graph references check distances and route selection. The opt-in real-data test checks graph counts, recovered named routes, classifications, and the corrected path examples:

```bash
DEMYSTIFY_TEST_DATA="$MYST_DATA" go test -run TestOriginalMystImport .
go test -race ./...
go vet ./...
```

DOT is the primary comparison. Corrected graph topology can move many nodes in Neato even when only a few arrows change; movement alone is not another game-related correction. The [original published PDF](https://github.com/glthr/DeMystify/blob/0276f8c/generated/myst_graph.pdf) remains an archived reference in Git history. The current full-graph artifacts are regenerated from the fresh conversion; see [their provenance](generated/README.md).

Extraction remains static. Eighteen calling-card routes through three Channelwood custom stack handlers are not expanded yet. Variable-based card names, dynamic expressions, and the HyperCard return stack are not executed. In particular, a shortest graph path is not a guaranteed playable walkthrough.
