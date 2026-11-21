# Myst V: End of Ages, the Classic-mode graph

A local, reproducible extractor for **Myst V: End of Ages (2005)**, read from the Windows files of the GOG release
2.0.0.9. Myst V is real-time 3D with three modes of navigation: Classic, Classic Plus and Free Roam. Classic moves the
player between fixed points, and the game stores them: one `dat/<Age>.node` file per playable Age, 2,729 points and
6,878 links. The extractor reads them, adds every other way the data moves the player (links to Ages and spawn
points, scripted links, scenes, warps, ladders, moving platforms), and writes a directed graph with the evidence of
every edge, its measures, and a comparison with the floors that Free Roam walks on.

## Rebuild

Python 3.10 or newer, standard library only, and an `openssl` command (LibreSSL or OpenSSL) for AES.

```sh
cd endofages
python3 -m endofages_graph "/path/to/Myst V" --output generated
python3 -m unittest discover -s tests -v
DEMYSTIFY_ENDOFAGES_DIR="/path/to/Myst V" python3 -m unittest discover -s tests -v   # adds the checks on the game
```

The input is the game's folder, the one that holds `dat/`, `Python/`, `SDL/` and `avi/` (in the GOG Mac
application: `Contents/SharedSupport/myst_v/support/myst_v/drive_c/Myst V`). It is read in place and never written
to. `--no-freeroam` skips the floor comparison (about thirty seconds).

## Output

- `generated/graph.json`: nodes (id `<Age>:<index>`, label `<Abbr>:<index>`, position, piece, nearby spawn points,
  scenes), typed edges with evidence, placement distances and conditions, spawn points, node regions.
- `generated/analysis.json`: per Age and between Ages; reach, distances, largest shortest distance, hubs, one-way
  parts, in three readings (every edge; without the links that can never open; without any state of the game).
- `generated/freeroam.json`: per Age, the floor area, the floor a walker reaches from the Classic points, the floor
  farther than 15 units from every point (largest patches), shortcut candidates, and the height of the points above
  the floor.

## How the game is stored

Most files of `dat/` and `Python/python.pak` begin with `0x0D874288`, the plain size, then AES-128 in ECB mode under
a fixed key (`crypt.py`). Names are XORed with `mystnerd`. `<Age>.node` (version 5): a count, then per point three
floats (x, y, z; Z up) and its links, each a target index, a type byte and a vector (`nodes.py`). The pages (`.prp`,
version 6) hold the scene objects, their positions, spawn points, scripts' parameters, responders with their link
messages, ladders and collision meshes (`prp.py`, `physics.py`). `python.pak` holds 180 scripts compiled for Python
2.3 (`pak.py`); `pydis.py` lists their instructions, which is how the Bahro handler's links are known.

## Interpretation

| Kind | Meaning |
|---|---|
| `classic` | A link stored in `<Age>.node`, with its type (`classicType`, 0 to 4). |
| `link` | A link to a spawn point: a link message in a responder (a book, a pedestal button) or a link a script builds on a click (the Bahro handler). The source is the node nearest to the clickable, the target the node nearest to the spawn point. |
| `script-link` | A link a script makes without a click of its own: K'veer to Descent after the intro; a Keep to K'veer once the four slates are on their Keeps. |
| `cutscene` | A recorded scene that moves the player to its warp point (xNPCSpawner when "do Player warp?" is set; the conclusions). Scenes that do not move the player are listed on their node. |
| `warp` | A script moves the player within the Age without a scene (Descent's elevators, Tahgira's crack and the fall through it). After a ride, a fall or a scene, the game itself sends the player to the closest Classic point (`PtClosestNode`), which is the rule the extractor follows. |
| `ladder` | A ladder, climbed up or down. |
| `ride` | A moving platform, inferred from positions (an elevator's axis, the levels of a cage, the tram's stops): `inferred: true`. |

Conditions are kept as text and never evaluated. Links whose condition can never be met (a pedestal whose symbol is
named `DoNotGoHere` or `NotUsed`) carry `never`. Targets that the data names but the Age lacks are `absent` nodes;
Releeshan, the place of the final scene, has no Classic point and is a `place` node.

## The game

GOG 2.0.0.9: 2,732 nodes (2,729 points), 7,024 edges (classic 6,876, link 85, script-link 5, cutscene 8, warp 6,
ladder 30, ride 14); from K'veer's arrival, 2,724 nodes reached, the farthest at 59 moves, the largest shortest
distance 88; Myst has no way back. The points stand 3.0 units above the collision floor in every Age.

## Credits

The formats follow libhsplasma (H-uru/libhsplasma, GNU GPL v3), read at commit `ea1faeb`; see
[NOTICE.md](../NOTICE.md). `eoa_types.json` holds Myst V's class numbers and names from its
`pdUnifiedTypeMap.cpp`. The `.node` format, the Bahro handler's links and the Python 2.3 reading are this project's
own work. The Python code is an independent implementation.
