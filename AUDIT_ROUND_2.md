# Second code audit and fresh source conversion

This audit starts at `4335f9d`. It tests both the supplied XML snapshot and a fresh conversion of the archived HyperCard stacks. The additional code fixes preserve the default DOT byte-for-byte on either input. The input snapshots themselves differ by one navigation relationship.

## Fresh conversion provenance

Six actual HyperCard stack data forks were extracted selectively from the supplied emulator archives. Their SHA256 hashes agree between the ZIP and 7z copies. Original files remain unchanged. Available Channelwood resource-fork and FinderInfo sidecars were preserved on the working copies; the other five archived stacks provide no such sidecars.

The converter is unmodified [stackimport](https://github.com/uliwitness/stackimport) at `878664b2122abc1caec25b5f71d2222574f4f3c8`, with snd2wav submodule `d6900ad35ba4da1fb488fe006f64a7977d98b32c`. It was built natively with Apple clang 21, `-std=gnu++11`, Carbon and CoreServices, using upstream Mac resource support and QuickTime disabled. Each conversion used `--noprogress`.

| Stack | Raw data-fork SHA256 |
| --- | --- |
| Channelwood Age | `0ba2b4e29433016c3f02edc80d93657b659f24f5f85fbaae7ae933686a3541b5` |
| Dunny Age | `26788a6974873affa8d115cc640d14b11d48814bd97527025eefe634aead0918` |
| Mechanical Age | `fa12c5e5201acfc9144f19172c4bc9df007b347169abe540a533fa3c0fefabd1` |
| Myst | `00c596f35546d76f2080bbfed4ac446952c7e7f0fa4dc0bb3ad60291417ab91e` |
| Selenitic Age | `c601f2b5184757588310c080bb9de82bb4bc7f13e2e1062ccac61874bee54bc6` |
| Stoneship Age | `053713e20834be91e43b2e4bb8471f349fc8469e6cc9a669612625e056d77379` |

All six converter runs exited successfully. The 1,355 exported card IDs exactly match the raw stack manifests (376 / 30 / 213 / 330 / 223 / 183). All 1,373 XML files parse. Logs, per-file hashes, raw-block literal offsets and conversion manifests are retained locally with the original data; no game files enter Git.

Warnings remain explicit: 23 unsupported MAST/PRFT/PRNT/PRST blocks, five unavailable resource forks, and one Channelwood PAGE-boundary warning. The last page-table case fills its block with 253 IDs; a second page supplies 123, reproducing all 376 declared IDs. This validates the card inventory, not lossless decoding of every unsupported block.

## Input difference and graph impact

Eleven XML files differ from the saved conversion, mostly in whitespace. Three script differences are substantive: Dunny card 5200 part 2 has `go to card id 5119` instead of bare `5119`; the Mechanical stack and Myst card 11521 use `xMemory(1)` instead of `Memory(1)`. Raw binary block evidence confirms the fresh literals originate in the archived stacks. The cause of the saved-XML discrepancy is not established, so it must not be described as a Go-parser correction or a historical Cyan revision.

| Measurement | Saved XML | Fresh raw-stack conversion |
| --- | ---: | ---: |
| Graph nodes | 1,364 | 1,364 |
| Non-disabled directed pairs | 3,214 | 3,215 |
| Edge records | 3,227 | 3,228 |
| Source / sink / isolated flags | 36 / 62 / 18 | 36 / 62 / 18 |
| Weak components | 21 | 21 |
| Dock-to-ending distance | 24 | 24 |
| Longest finite shortest distance | 130 | 130 |

The sole added relationship is `Dunny Age:5200` (Father-Left) to `Dunny Age:5119` (Father-In). Its distance falls from three edges to one. Five ordered-pair distances improve, all starting at card 5200; no pair becomes newly reachable and no distance worsens. Component memberships, all node records and dock reachability stay unchanged.

This restores a local orientation connection between existing views. It does not reveal a new location, prove a developer changed the room, or alter the two headline distances in the articles.

## Additional programmatic fixes

- Parser: reject partial literal matches and unsupported dynamic scopes instead of inventing destinations; keep pushed-return context and restrictive goto pairing within the relevant handler; validate missing, duplicate and nonpositive identities; support direct stack roots and root aliases; distinguish original card names from generated graph labels; use structured edge keys; avoid treating quoted numeric card positions as names.
- Graph: synchronize public node/edge mutations with metadata and totals; honor disabled/backtracking flags on insertion; clear obsolete classifications during reanalysis; keep single-query, all-pairs and waypoint cost accumulation consistent; compare complete attribute multisets so active and disabled parallels cannot collapse into one record.
- Renderer: respect highlight and numeric-label options; preserve page borders under analysis styling; resolve canonical graph IDs independently of optional name aliases; normalize negative transitivity palette IDs; keep restrictive-head appearance paired while honoring its own disabled/backtracking state; preserve the prior PDF when Neato fails and reject empty successful outputs.

The regressions were reproduced against the pre-fix code. Each committed snapshot passed `go test ./...` and `go vet ./...` before commit, with about 100 changed lines or fewer. Final checks cover both real-data profiles, independent path references, race detection, fuzzing, Go 1.24.1 compatibility and vulnerability scanning.

## Controlled DOT comparison

The saved-XML and fresh-XML app runs are compared separately. Code changes must not be confused with input changes:

| Input | Before/after code DOT SHA256 | Bytes |
| --- | --- | ---: |
| Saved XML | `20f330ddb7a837bc4018fb5b8b575e61ca0f3576ef0053382251ae3cff0381ac` | 233,515 |
| Fresh XML | `5de8b7c3a30dd372f3e18692b09da9bc0ff8fa316a444448fb7a05a88654fe93` | 233,523 |

Both same-input comparisons are byte-identical. The saved-versus-fresh difference is the Dunny connection described above. The full app also renders PDFs from the fresh conversion; DOT, rather than layout movement or PDF timestamps, is the primary regression check.

To repeat the Go checks with either verified conversion:

```bash
DEMYSTIFY_TEST_DATA="$MYST_DATA" go test -race ./...
go vet ./...
go build -o "$RUN/demystify" .
(cd "$RUN" && ./demystify "$MYST_DATA")
```

The opt-in test independently reads Dunny card 5200 part 2 to select the known input profile; it does not infer expected counts from the graph being tested. Set `MYST_DATA` to the converted six-stack directory and use a separate writable output directory for `RUN`.

Remaining limits are explicit: no full HyperTalk interpreter, variable resolution, general handler-call expansion or saved-return-stack simulation. Quoted card ordinals are left unsupported rather than fabricated as named destinations. Native converter warnings and absent resource forks also remain coverage limits. Floating-point route selection follows the forward Dijkstra tight-prefix model; it is not exact real-number arithmetic. The graph is an analysis of references, not a guaranteed playable walkthrough.
