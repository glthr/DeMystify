# Notice

## License

DeMystify was released under the MIT License up to and including commit `03f3fea` (pull request #19). From the
change that adds the Exile, Revelation and End of Ages extractors, it is released under the GNU General Public License, version 3
or (at your option) any later version: see [LICENSE](LICENSE). The copies of DeMystify already published under the MIT
License remain available under it. All the code so far is by the author listed in [AUTHORS.md](AUTHORS.md).

The license changes because the Myst III and Myst V extractors follow ScummVM and libhsplasma, which are licensed under the GNU
General Public License, version 3 or later, as described below.

## Work this project relies on

**Myst III: Exile** (`exile/`)

- The opcode names and argument signatures of `exile/exile_graph/names.py` follow the ScummVM Myst III engine,
  [`engines/myst3/script.cpp` and `state.cpp`](https://github.com/scummvm/scummvm/tree/aed0208e772b128d3f953207a297fc521c32833a/engines/myst3)
  at commit `aed0208e772b128d3f953207a297fc521c32833a`, where the meaning of each opcode was worked out.
- The layout of the tables inside the executable and the decoding of its SafeDisc 2.10 sections, with the key
  published for the 1.0 executable, follow the reader that ResidualVM used before the scripts were moved to
  `myst3.dat`: [`engines/myst3/database.cpp` at `9cc0513f`](https://github.com/scummvm/scummvm/blob/9cc0513f6908508ca76141ba9e6181aa87c6b606/engines/myst3/database.cpp).
- The engine transitions added to the graph are attributed, by link, to ScummVM's `engines/myst3` sources at the
  commit above. The Python code is an independent implementation.

**Myst IV: Revelation** (`revelation/`)

- The archive format follows [myst4tools](https://github.com/coderjo/myst4tools/blob/master/m4b-format.md) and
  [the Tomahna project](https://tomahnaproject.github.io/tomahna-project-wiki/myst4/file-reference/).
- The tile layout, the `.zap` header and the observation that the engine never reads `nodedb.ndb` follow
  [m4fix](https://github.com/rsheasby/m4fix/blob/main/docs/TECHNICAL.md); the layout of `node.bin` follows
  [myst-disclosure](https://github.com/exodustx0/myst-disclosure).
- No open-source engine exists for this game. The compiled libraries are read, never run.

**Myst V: End of Ages** (`endofages/`)

- The encryption of the game's files (AES-128, the published key), its strings, the page format of version 6 with
  Myst V's class numbers, and the objects read (scene objects, coordinate interfaces, scripts' parameters,
  responders and their link messages, ladders, collision meshes) follow
  [libhsplasma](https://github.com/H-uru/libhsplasma) (GNU GPL v3) at commit
  `ea1faeb4bf03e52ceaf63e216f09a73b487c8a09`: `core/Stream/plEncryptedStream.cpp`, `core/Stream/hsStream.cpp`,
  `core/PRP/plPageInfo.cpp`, `core/ResManager/plResManager.cpp` and `pdUnifiedTypeMap.cpp` (copied as data into
  `endofages_graph/eoa_types.json`), and the readers of the classes named under `core/PRP/`.
- The Classic-mode files (`.node`), the reading of the compiled Python 2.3 scripts and the graph are this project's
  own work. The Python code is an independent implementation.

**Riven** (`riven/`)

- The native transitions are reviewed against ScummVM's Mohawk engine at commit
  `aed0208e772b128d3f953207a297fc521c32833a`; [docs/riven-native-navigation.md](docs/riven-native-navigation.md)
  links each one to its source.

## Game data

Myst, Riven, Myst III: Exile, Myst IV: Revelation and Myst V: End of Ages belong to their creators and publishers. Nothing of the games is
in this repository: no disc image, archive, executable, library, picture, movie, sound, script or generated listing.
The extractors read the user's own original discs, in place, and write their outputs to ignored folders.
