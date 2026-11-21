# Myst IV: Revelation node graph

A local, reproducible extractor for **Myst IV: Revelation, version 1.0 (2004, two DVDs)**. It reads the first DVD image directly, follows the game's archives down to each node, decodes the commands and the parameters of the compiled classes, reads the constant destinations of the compiled libraries, and writes a directed graph of the nodes with the evidence of every edge.

Open [the graph explorer](generated/index.html). Its CSS, JavaScript and graph data are embedded. Pictures load from the adjacent `images` folder; no server or network connection is required.

## Rebuild

Python 3.10 or newer. The graph itself needs no third-party package. The pictures need [Pillow](https://python-pillow.org/) (to put the tiles together) and [FFmpeg](https://ffmpeg.org/) (to unroll the cubes). The compiled libraries are read with `objdump` (part of the system's developer tools): without it, the graph is built without their constants and says so. Graphviz (`neato`) places the nodes of the explorer.

```sh
cd revelation
python3 -m pip install pillow   # only for the pictures
python3 -m revelation_graph /path/to/DVD1.bin --output generated
python3 -m unittest discover -s tests -v
DEMYSTIFY_REVELATION_DISC=/path/to/DVD1.bin python3 -m unittest discover -s tests -v   # adds the checks on the 1.0 release
```

The input is a DVD image (`.bin`, `.iso`) or a folder holding `data.m4b` and the `m4_ai_*.dll` libraries (an installed game). Inputs are read in place and never written to. `--no-images` and `--no-web` skip the pictures and the explorer.

## Output

- `generated/index.html`: offline explorer (DeMystify's viewer): graph and atlas layouts, Age filters, pictures, listings, evidence and structural paths.
- `generated/graph.json`: nodes (name, world, zone, type, stored position, hotspots, picture layers, classes), typed edges with their evidence, the analysis, the Zip places, the compiled destinations, the stale neighbour table, the parameters whose name and value disagree.
- `generated/scripts/w<world>/<node>.txt`: one readable listing per node: position, hotspots, class parameters, command blocks and what starts each of them.
- `generated/images/`: one picture per node that stores a default cube: a panorama (equirectangular, the front face at the centre) or, for a fixed view, its single face. `images-manifest.json` records the tiles and their hash.
- `generated/edges.csv`, `generated/revelation.dot`, `generated/manifest.json`, `generated/analysis.md`.

## How the game is stored

`data.m4b` is an archive of archives ("BigFile", signature `UBI_BF_SIG`): a tree of names, sizes and offsets, then the files, uncompressed. It holds one archive per node, `global/w<world>/z<zone>/n<node>/w…z…n….m4b`, and two shared archives. A node archive holds:

- `node.bin`: free or fixed viewport, three numbers that are the node's position in its world (the third one up), a camera, and the direction to face on arrival from named nodes;
- `cube/layer_default.m4b`: the picture, six faces of 6 x 6 tiles of 128 pixels (`.jpg`, or `.zap`: a JPEG of colours and a JPEG of transparency); transparent tiles are not stored;
- `command_block/*.bin`: named lists of commands, each a line of text such as `ContextChange(1,01,021)`;
- `ai/<class>/*.ai`: the parameters, in text, of the compiled classes that run in the node;
- `graphic/hotspots.m4b`: the hotspots, each with its offset from the node.

Strings in the binary files have the two bits of every pair exchanged; the transform is its own inverse.

## Interpretation

A graph node is one stored node, or an explicitly marked virtual node for a target that no archive holds. An edge is a reference to another node:

| Kind | Meaning |
|---|---|
| `move` | A `ContextChange` in a block that a click on a hotspot starts, or an `aitransition` (hotspot, movie, destination). |
| `auto` | A `ContextChange` in a block started by something other than a click. |
| `return` | A fixed view goes back: to the node it records (`aisubnode`), to the node of an amulet vision, or, inferred, to the node that enters it. |
| `engine` | The destination is data and a compiled class makes the move: a hotspot named `h_to_<node>`, three parameters (world, zone, node), a transition named `tr_<node>`, an arrival record of the destination, a constant in a library. |
| `amulet` | The vision that the amulet opens (`aiamuletstaticvisualsource`). |
| `unreferenced` | A `ContextChange` in a block that nothing stored starts. Path search ignores it. |

Inferred edges carry `inferred: true` in their evidence (63 on the 1.0 disc). A return inferred for a fixed view that several nodes enter is `callerOwned`: it belongs to the node the player came from, and path search ignores it. Conditions are kept as text and never evaluated. Positions of nodes that store no picture are copies of other nodes' positions.

The libraries are read, not run: `native.py` finds the calls to `ai::ContextChange` and to the constructor of `ai::Transition` whose destination is three pushed constants (55 on the 1.0 disc; 57 other calls take computed values, which come from the parameters). A constant is attributed to a node when the same routine names resources of that node. `revelation_graph/compiled_by_hand.json` adds the constant destinations read by hand in the disassembly, with their addresses: those that other instructions separate from their call, and those whose routine is a callback handed over by a method of a class (sixteen, among them the passage from day to night, `w5_z01_n110` to `w1_z01_n310`, `M4AIW5RD.DLL` 0x10016d3b); its list `wrong` names two calls that the automatic rule gives to the wrong node.

A class parameter that names a transition of its node (`<node>.tr_<target>`) becomes an edge only when the node stores that transition, panel or movie; otherwise it is listed under `danglingTransitions`. `m_subnodeName` makes an edge for the rail classes that open the close-up they name; the incubator class `w5_z04_incub` only compares it with the last node visited (`M4AIW5RD.DLL` 0x10003930), and its names are listed under `lastNodeChecks`. Command blocks are also read from the misspelled folders `commandblock/` and `command_blocks/` of four nodes.

## The disc

Use the image of your own first DVD of the English 1.0 release (September 2004). The one the extractor was built on has the SHA-1 of the [Redump](http://redump.org/disc/19849/) record of disc 1, `6d83e2c92c804698f2d6e4156e137d7f3ab56d1c` (volume `MYST4_1`, created 2004-09-05). The second DVD holds movies and translations and is not needed. An installed game (a folder holding `data.m4b` and the libraries) can be read too.

## Tools

- `tools/export_faces.py DISC OUT NODE...`: writes the six faces of a node's cube, for figures that show a single face.
- `tools/corpus.py`: gathers every small file of every node into one JSON file, for study.

## References

Part of [DeMystify](../README.md), with [The Myst Graph](https://glthr.com/myst-graph-1) and The Riven Graph; the viewer and layout code are those of the Exile extractor (`../exile`).

The archive format follows [myst4tools](https://github.com/coderjo/myst4tools/blob/master/m4b-format.md) and [the Tomahna project](https://tomahnaproject.github.io/tomahna-project-wiki/myst4/file-reference/); the tile layout, the `.zap` header and the observation that the engine never reads `nodedb.ndb` follow [m4fix](https://github.com/rsheasby/m4fix/blob/main/docs/TECHNICAL.md); the layout of `node.bin` follows [myst-disclosure](https://github.com/exodustx0/myst-disclosure). No open-source engine exists for this game: what a class does with its parameters is not known beyond what their names say.

See [NOTICE.md](../NOTICE.md). The supplied disc image remains unchanged. Generated outputs include game-derived listings and pictures: `generated/` is ignored by Git; keep them local.
