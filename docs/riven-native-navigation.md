# Riven native navigation audit

ScummVM reference commit: `aed0208e772b128d3f953207a297fc521c32833a`.
All links below refer to that pinned commit; no source cache is required.
`riven/native.go` records reviewed behavior as explicitly tagged metadata.
This is a supplement to the on-disc bytecode, not a claim that the original
executable has been completely decompiled or emulated.

## Card identifiers and redirects

RMAP is a flat big-endian u32 list. Its index is the stack-local CARD id; its
value is the global id used by opcode 27 and native routines. If duplicates
exist, ScummVM's linear lookup returns the last match.
[Reader](https://github.com/scummvm/scummvm/blob/aed0208e772b128d3f953207a297fc521c32833a/engines/mohawk/riven_stack.cpp#L125-L161).

Opcode 27's first argument indexes **the current stack's NAME/5 list**; it is
not a numeric native stack id. Its two following u16 arguments form the u32
destination RMAP. Resolve the NAME entry, load the target stack, resolve its
RMAP, then apply the same destination redirects as an ordinary card change.
[Opcode](https://github.com/scummvm/scummvm/blob/aed0208e772b128d3f953207a297fc521c32833a/engines/mohawk/riven_scripts.cpp#L934-L966).

The 13 special changes apply to the **requested destination**, before entering
it, not to outgoing navigation from that card. For example, any tspit request
for the CARD whose RMAP is `0x21b69` enters ospit RMAP `0x2e76` instead. Preserve
the literal target as evidence, and preserve every shipped CARD as a node.
[Table and implementation](https://github.com/scummvm/scummvm/blob/aed0208e772b128d3f953207a297fc521c32833a/engines/mohawk/riven.cpp#L588-L630).

## Exhaustive reviewed native change sites

Searches covered `changeToCard`, `changeToStack`, `kRivenCommandChangeCard`,
`RivenStackChangeCommand`, `runEndGame`, and `runCredits` in the engine, script
manager, card, inventory, base stack, and all eight stack implementations.

| Location | Navigation |
| --- | --- |
| `riven.cpp` startup / keyboard / resume | Engine entry to menu; demo startup; menu save/return; optional intro; reload current card. These are engine controls, not CARD/HSPT edges. |
| `riven.cpp` special-change table | 13 destination redirects, handled by `resolveDestination`. |
| `riven_scripts.cpp` switchCard, stack change | Opcodes 2 and 27, extracted directly from game scripts. |
| `riven_scripts.cpp` zipMode | Opcode 45 resolves current hotspot name to a previously visited CARD with the same NAME/1 name. Kept separate from ordinary movement. |
| `riven_card.cpp` property patch | ScummVM CD-only patch adds ospit RMAP `0x2e76` enter-script move to local CARD 2 under `agehn == 1 && atrapbook == 1`; do **not** inject into original-disc graph. Original script transition is already present in its update handler. |
| `aspit.cpp` xasetupcomplete | aspit RMAP `0xe2e`. |
| `aspit.cpp` xaNewGame | aspit CARD 2 then target stack NAME/5 entry 0 (tspit in full game), RMAP `0x6e9a`; ScummVM-added command. |
| `aspit.cpp` three book back commands | `backFromItemScript`: caller-sensitive saved stack/RMAP. |
| `aspit.cpp` xaResumeGame | saved menu caller; ScummVM-added command. |
| `jspit.cpp` xvga1300_carriage | conditional sequence `0x18d4d`, `0x18ab5`, `0x17167`. |
| `jspit.cpp` xhandlecontrolup/down | conditional destination `0x1e374`. |
| `jspit.cpp` xhandlecontrolmid | up `0x1e597`, down `0x1e29c`. |
| `jspit.cpp` sunnersPlayVideo + timers | From RMAP `0x77d6` -> `0x79bd`; `0x79bd` -> `0x7beb`; `0x7beb` -> `0xb6ca`, conditional video interaction. Shoreline timer does not navigate. |
| `jspit.cpp` xjlagoon700_alert, xjlagoon800_alert | The alert video that the enter script of RMAP `0x79bd` and of `0x7beb` plays goes through the same `sunnersPlayVideo`: `0x79bd` -> `0x7beb` and `0x7beb` -> `0xb6ca`, the pairs of those cards' timers. Recorded as a second callsite of the same `native-timer` record. `xjlagoon1500_alert` does not navigate. |
| `ospit.cpp` xbookclick | conditional direct destination RMAP `0x2885`; or, on third refusal, ending credits and menu reset. |
| `ospit.cpp` xorollcredittime | if saved return stack is rspit, special workaround to rspit `0x3338`; otherwise ending credits and menu reset. |
| `rspit.cpp` xrcredittime | ending credits and menu reset. |
| `tspit.cpp` xtexterior300_telescopedown -> xtopenfissure | conditional ending credits and menu reset. |
| `riven_stack.cpp` runCredits | all ending branches eventually reset state and go to aspit CARD 1. Distinct from gameplay movement. |
| `riven_inventory.cpp` checkClick | full-game non-aspit inventory to aspit 5 (Atrus journal), 6 (Catherine journal), 7 (trap book), subject to ownership/visibility; saves caller's stack and global RMAP. Demo exit paths excluded. |
| `riven_inventory.cpp` backFromItemScript | dynamic return using saved stack and global RMAP. |

Sources for native functions:

- [ASpit](https://github.com/scummvm/scummvm/blob/aed0208e772b128d3f953207a297fc521c32833a/engines/mohawk/riven_stacks/aspit.cpp)
- [JSpit](https://github.com/scummvm/scummvm/blob/aed0208e772b128d3f953207a297fc521c32833a/engines/mohawk/riven_stacks/jspit.cpp)
- [OSpit](https://github.com/scummvm/scummvm/blob/aed0208e772b128d3f953207a297fc521c32833a/engines/mohawk/riven_stacks/ospit.cpp)
- [TSpit](https://github.com/scummvm/scummvm/blob/aed0208e772b128d3f953207a297fc521c32833a/engines/mohawk/riven_stacks/tspit.cpp)
- [RSpit](https://github.com/scummvm/scummvm/blob/aed0208e772b128d3f953207a297fc521c32833a/engines/mohawk/riven_stacks/rspit.cpp)
- [Base stack endings](https://github.com/scummvm/scummvm/blob/aed0208e772b128d3f953207a297fc521c32833a/engines/mohawk/riven_stack.cpp#L208-L269)
- [Inventory](https://github.com/scummvm/scummvm/blob/aed0208e772b128d3f953207a297fc521c32833a/engines/mohawk/riven_inventory.cpp#L91-L167)

## Zip and return policy

CARD's second u16 (`zipModePlace`) registers its NAME/1 name and local id when
visited. HSPT's zip field marks a zip hotspot; enabled status depends on the
visited registry and `azip`. Opcode 45 matches the current hotspot's NAME/2 name
against visited CARD names, then requests that local id. The visited registry
is per saved game, not a list of all possible ordinary edges. Potential zip
edges can be resolved by name, but must be tagged `zip` and excluded from normal
path distances.
[Registration](https://github.com/scummvm/scummvm/blob/aed0208e772b128d3f953207a297fc521c32833a/engines/mohawk/riven_card.cpp#L650-L671),
[execution](https://github.com/scummvm/scummvm/blob/aed0208e772b128d3f953207a297fc521c32833a/engines/mohawk/riven_scripts.cpp#L753-L765).

Inventory entry is available globally while the inventory is visible, and the
back command must return to exactly its saved caller. Adding a journal return
edge to every card would create impossible paths through books. The helper
therefore emits `dynamic-return` metadata without inventing endpoints.
Inventory entry rules are provided by `inventory_model()` without expanding
the whole game's ordinary graph.

## Limitations

Native edges derive from ScummVM's reconstruction of the engine. Its additions
and compatibility patches are marked above. Native conditions are annotations,
not satisfiability proofs. Ending reset/menu edges should not count as normal
travel or make every ending a portal back into gameplay. Script sequences can
visit intermediary cards within a single input; a structural-reference graph
does not imply each command is a distinct player action. None of this models
full variable state, ownership, timers, or input timing.
