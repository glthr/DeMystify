# Graph changes

The audit uses the original six Myst stacks and 1,355 converted cards. The reference code is `0276f8c`, and its app-generated DOT exactly reproduces the committed `generated/graph.dot`.

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

The published `generated/myst_graph.pdf` adds a title, border, and footer after the app renders its graph. DOT is the primary comparison; PDF framing and Graphviz layout changes are assessed separately from graph content.
