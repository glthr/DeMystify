# Myst III: Exile node graph

A local, reproducible extractor for **Myst III: Exile, version 1.0 (2001, four CDs)**. It reads the raw CD images directly, decodes the SafeDisc-protected executable that holds the game's scripts, follows the navigation instructions of every node and writes an interactive graph with portable data exports.

Open [the graph explorer](generated/index.html). Its CSS, JavaScript and graph data are embedded. Pictures load from the adjacent `images` folder; no server or network connection is required. The page reuses DeMystify's viewer and offers both the PDF-style graph and a picture atlas. Search by `LEIS:12`. Select a node for its panorama, its hotspot zones, its script listing and the evidence of each edge. Zip shortcuts are an optional layer.

## Rebuild

Python 3.10 or newer; the extractor has **no third-party Python dependencies**. [FFmpeg](https://ffmpeg.org/) assembles the panoramas. Graphviz (`sfdp`, `neato`) is optional for the layout; a fallback is used without it.

```sh
cd exile
python3 -m exile_graph /path/to/discs --output generated
python3 -m unittest discover -s tests -v
DEMYSTIFY_EXILE_DISCS=/path/to/discs python3 -m unittest discover -s tests -v   # adds the checks on the 1.0 release
```

The input is one or several paths: raw or cooked CD images (`.img`, `.bin`, `.iso`), or a folder holding `M3.exe` and the `*nodes.m3a` archives (an installed game, or the later DVD edition, 1.27, for comparison). Inputs are read only; nothing is unpacked. `--no-images` skips the pictures and `--no-web` the explorer.

## Output

- `generated/index.html`: offline explorer with graph and atlas layouts, age and highlight filters, hotspot overlays, scripts, evidence and structural paths.
- `generated/images/`: one picture per stored view. A panoramic node stores six cube faces; they are assembled into one equirectangular picture (heading 0 at the centre). Flat frames are copied as stored.
- `generated/images-manifest.json`: archive, resource offsets, sizes and hashes of each picture.
- `generated/graph.json`: typed directed graph, evidence for each edge, statistics, destinations taken from variables and engine metadata.
- `generated/nodes.json`: every script block as stored: node lists, conditions, hotspot rectangles, opcodes with their arguments and file offsets.
- `generated/scripts/<ROOM>/<node>.txt`: one readable listing per scripted node.
- `generated/exile.dot`, `generated/exile.graphml`, `generated/edges.csv`: for Graphviz, Gephi, Cytoscape or a spreadsheet.
- `generated/manifest.json`: edition, disc volumes, executable sections and table offsets, archive locations.
- `generated/analysis.md`: measured results, projection rules and limits.

## How the game is stored

Exile does not keep its scripts on the discs as data files. Ages, rooms, nodes, hotspots and scripts sit in the data section of `M3.exe`; the `.m3a` archives hold the pictures and movies of each room. The retail 1.0 executable is wrapped by SafeDisc 2: its `.text` and `.data` sections are decoded here with the key that the ResidualVM project published for this version.

A room has a four-letter name (`LEIS`, `ENPP`, `MATO`…) and a number; rooms are grouped into ages. A node belongs to a room. Most nodes are **panoramas**: the player turns freely inside them, so a turn is not a move. Others are flat **frames** (close-ups, panels) or menu screens. A third of the scripted nodes are never visited: they are helper scripts that other scripts call.

## Interpretation

A graph node is one place: a scripted node that draws a view, a script-only stop that navigation enters, or an explicitly marked virtual node for a target that no room stores. An edge is a structural reference:

| Kind | Meaning |
|---|---|
| `move` | A hotspot script of the source requests the target. |
| `auto` | A script that runs on arrival, or on every frame, requests the target. |
| `zip` | A Zip shortcut, offered only once its target has been visited. |
| `continuation` | One script requests two places in a row; the edge joins the first to the second. Path search ignores it. |
| `native` | A transition performed by the engine, attributed to ScummVM. |

The 11 engine transitions are the menu's (MENU:100 to its pages, and back through MENU 599) and the five frames of the Edanna amber puzzle (ENEM 114, 116, 118, 120, 122 return to node number % 100: ScummVM sets the return variable in [`puzzles.cpp`, lines 263 to 293](https://github.com/scummvm/scummvm/blob/aed0208e772b128d3f953207a297fc521c32833a/engines/myst3/puzzles.cpp#L263-L293)).

Helper scripts are followed: when a hotspot calls a script node that performs the move, the edge starts at the place that owns the hotspot and its evidence names the helper. Tests found before an instruction are kept as text beside the edge; they are never evaluated, so a path does not prove a playable route. Instructions that reload the current node (usually after a movie) are counted but are not edges. Destinations read from a variable, such as the place saved when a book or the menu is opened, are listed under `dynamicNavigation` and are not expanded into edges from or to every place.

This decodes the game's **data and script bytecode**; it does not recover the source of the original interpreter. Puzzle logic written in native code (`runPuzzle`) is listed per node and not interpreted. Movies, sounds and alternate states of a view are not replayed.

## Tools

- `tools/verify_nodes.py DISCS DVD`: reads the 1.0 and 1.27 executables with the same code and compares their script lists with each other and with ScummVM's `myst3.dat`. Result (2026-11-16): the 92 lists of 1.27 are byte for byte those of `myst3.dat`; 1.0 has the same rooms (without `LOGO`), no unread byte between its lists, and 78 of its 91 lists say what 1.27 says. The 13 others differ in 85 blocks; `LEIS 10077`, `LEIS 10078` and `MENU 530` were added after 1.0.
- `tools/export_faces.py DISCS GAME NODE...`: writes the six cube faces of a panorama as stored, for figures that compare single faces (`LEIS:1`, `LIFO:47`, `LIFO:147`).

## The discs

Use the images of your own original discs. The extractor was built and checked on the English 1.0 release on four CDs (May 2001). Discs 2, 3 and 4 of a good dump are identical to the [Redump](http://redump.org/disc/12873/) records of the original pressing (610427-CD2 to CD4). Disc 1 (Redump record 12872) carries deliberately unreadable sectors (SafeDisc), so two dumps of it differ; the extractor identifies the 1.0 `M3.EXE` (mastered on 10 April 2001) by ScummVM's detection fingerprint, the MD5 of its first 5,000 bytes, `f8ab52da2815c1342eeb42b1bcad5441`.

## References

Part of [DeMystify](../README.md), with [The Myst Graph](https://glthr.com/myst-graph-1) and The Riven Graph.

File layouts, opcode meanings and engine behaviour are cross-checked against the [ScummVM Myst III engine at commit aed0208e772b128d3f953207a297fc521c32833a](https://github.com/scummvm/scummvm/tree/aed0208e772b128d3f953207a297fc521c32833a/engines/myst3). The layout of the tables inside the executable and the SafeDisc decoding follow the reader that ResidualVM used before the scripts were moved to `myst3.dat` ([database.cpp at 9cc0513f](https://github.com/scummvm/scummvm/blob/9cc0513f6908508ca76141ba9e6181aa87c6b606/engines/myst3/database.cpp)). The opcode and variable names in `exile_graph/names.py` are those of ScummVM. The extractor is independently implemented in Python; see [NOTICE.md](../NOTICE.md).

The supplied disc images remain unchanged. Generated outputs include game-derived scripts and pictures: `generated/` is ignored by Git; keep them local.
