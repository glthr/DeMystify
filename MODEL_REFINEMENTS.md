# Model refinements after review of the illustrated audit

This round starts from the merged audit, `eb28ddd`. It corrects the project's source definition, projects supported stack-handler calls into actual card relationships, and resolves a further named-card syntax. The illustrated PDF is also revised: its former source-node correction is withdrawn, and each of the 17 self-reference additions has individual image/script evidence.

## Sources and the crumpled note

A source has no non-disabled incoming edge **from another node**. A self-loop does not provide an entrance from another card. Myst:8631 therefore remains a source, as the original article said; the first audit's contrary conclusion used the wrong definition for this project.

Myst:8631 is the only card reclassified by this definition change. Its image and self-reference remain, and it is still unreachable from the dock under either backtracking policy. Source totals continue to include isolates; sink and isolate definitions are unchanged.

## Shared valve views belong to their callers

Temple 3-e (Channelwood Age:100047) contains no valve-handler call. Its original ownership of three arrows was accidental. The first audit moved those definitions to the stack but left their callers unexpanded, creating a misleading detached four-node group.

The revised extractor recognizes a narrow, verified handler shape: optional parameter-to-global state assignment, a saved current caller, and one unconditional fixed same-stack destination. Actual literal-argument calls are expanded only with complete background scope and no relevant local/background override. Conditional, computed, nested or ambiguous cases remain unsupported.

| Shared target | Actual calling cards in Channelwood |
| --- | --- |
| 86839 / SwitchClose-Input | 13703, 19143, 20137, 21498, 26492, 33578 |
| 87418 / SwitchClose-LeftOutput | 14360, 19870, 20934, 22077, 27330, 35578 |
| 87249 / SwitchClose-RightOutput | 17667, 32368, 2465, 24265, 29197, 34505 |

These 18 caller entries replace three definition-only arrows. They retain contextual backtracking classification; no arbitrary close-up-to-all-callers return edges are introduced. The three close-ups join the main weak component and the stack becomes an isolated abstraction. This reveals reuse of three views at six valve positions, rather than a detached temple area.

## Additional script forms

Four unquoted `go to card black` statements on Myst:72560 and 77588 recover two distinct references to existing Myst:23444. HyperTalk permits an unbound single-word literal; the resolver requires an existing original card name and conservatively rejects imported identifier bindings, incomplete background scope, reserved syntax and numeric-word ordinals.

The XML sweep found 51 statements unsupported by literal navigation patterns: four unquoted Black statements, 44 bare `go card` statements, and three relative/variable statements in the Myst:23279 media-path utility. Shared player code explains conditional destinations for the 21 q-button bare statements; the other 23 do not identify fixed destinations from their owning object alone. The utility rewrites media paths and is not evidence of ordinary gameplay routes. Event sends, simulated clicks and image substitution also require execution context.

## Conditional Zip navigation omitted from the six-Age import

The root player application contains a shared `on go` wrapper, activated through the player/ALLRes startup scripts. Options > Zip Mode toggles `Quick`. For a q-named runtime target, the wrapper can redirect a go command to the same-Age card named by its first word, when that name is in the Age's visited list. Otherwise it passes the ordinary command on. Local visibility and script conditions still apply.

The independently reconciled inventory contains 525 direct-go q buttons and 527 go statements. 524 buttons resolve to 510 distinct candidate pairs; 497 are absent from the ordinary graph and 13 already occur there. Additional pairs by Age: Channelwood 150, Selenitic 116, Myst 99, Stoneship 78, Mechanical 54. Four buttons are initially hidden. One target, Channelwood:2153 part 7 `qbri1`, does not resolve; the similar `qbr1` is not silently substituted.

[The complete 510-pair catalogue](audit/conditional-zip-connections.csv) records this separate conditional layer. It is an analysis result, not a stateful implementation or an addition to the default ordinary-mode graph. Actual calls depend on runtime message target, Zip setting, visit history and local conditions.

As an illustrative sensitivity check, adding every candidate at once gives 3,729 pairs, dock-to-ending 19 and diameter 74. These are **stateless union results**, not a playable new-game route. The article's 24/130 measurements should be labeled as ordinary-graph distances with Zip links omitted, rather than mode-independent game-travel bounds.

## What the self-reference evidence establishes

The 17 additions are **17 extracted self-reference pairs**, from 19 uncommented `go this card` lines. Eight lines occur within explicit active handler declarations. Eleven marker fragments have commented opening declarations; their invocation is not established solely by line syntax. Calling all 17 proven runtime refresh interactions overstated the evidence.

| Card | Referenced base asset | Script context |
| --- | --- | --- |
| Mechanical Age:34002 | S-Throne Throne | Cache-door handlers |
| Myst:10238 | Woodpath4-S BoatUp | Commented marker header |
| Myst:10316 | StairsGear2-E | Commented marker header |
| Myst:13071 | SpaceShipPath2-E | Commented marker header |
| Myst:16774 | Garden4-E | Commented marker header |
| Myst:17825 | Forest3-S | Commented marker header |
| Myst:25971 | Woodpath1-N note | Commented marker header |
| Myst:3604 | Woodpath2-N | Commented marker header |
| Myst:45802 | TreeRoom3-S | TreeUpdate handler |
| Myst:71480 | TreeGateLookDown | TreeUpdate handler |
| Myst:8079 | Woodpath1-N | Commented marker header |
| Myst:8336 | Dock1-E | Commented marker header |
| Myst:8631 | Woodpath1-N Crumpled | Commented marker header |
| Myst:8794 | Woodpath3-E | Commented marker header |
| Stoneship Age:13801 | Lighthouse2-S up | Hatch-lock handler |
| Stoneship Age:19307 | Lighthouse2-S down up | Chest-close handler |
| Stoneship Age:27714 | LighthouseTop1-S | Hatch handlers |

The illustrated catalogue identifies all 17 original base PICTs and 33 related variants/patches by bank, resource ID and name, and pairs each case with its command, location and relationship. Original pixels are decoded; scenes are not synthesized or reconstructed into assumed runtime states. The assets and full game scripts remain outside Git.

## Distances, comparison and validation

Article 3 states: “As a result, backtracking edges are de facto excluded.” ([Path analysis](https://glthr.com/myst-graph-3).) The original weighted adapter failed to enforce that policy. Myst:28679 (Library1-NW) to 44018 (LibraryUp) is still one up-button action; 14 is the solver's alternative after excluding that direct saved-return interaction, not a required player walk. The PDF now explains this distinction with the original quote and button commands.

| Measurement | Before refinements | After refinements |
| --- | ---: | ---: |
| Nodes | 1,364 | 1,364 |
| Saved XML pairs / records | 3,214 / 3,227 | 3,231 / 3,244 |
| Fresh conversion XML pairs / records | 3,215 / 3,228 | 3,232 / 3,245 |
| Source / sink / isolated flags | 36 / 62 / 18 | 37 / 63 / 19 |
| Largest weak component | 1,339 | 1,342 |
| Dock-to-ending / diameter | 24 / 130 | 24 / 130 |

Independent breadth-first searches confirm both headline distances. With backtracking included, the caller entries make 3,900 ordered pairs newly reachable into the three existing close-ups; the 3 removed stack-origin routes were definition artifacts. The two Black references shorten 111 ordered-pair distances. No new nodes are added.

All six stack data forks match the archived copies byte-for-byte. Re-conversion with the pinned tool in [the second audit](AUDIT_ROUND_2.md), with resource forks available, produces identical 1,367 core card/stack/background XML files; all 1,373 XML files parse. Unsupported-block and page-table warnings remain documented.

The full stack-to-XML-to-app pipeline renders DOT and PDF successfully. Final DOT SHA256: `0d752962bde14e29265b35b1f7e411d05e5299e98caed7703b8427e537903814`. Every small code snapshot passed tests and vet before commit; final checks cover both real-data profiles with race detection, Go 1.24.1, staticcheck, fuzzing and vulnerability scanning. A separately reproduced public `AddEdge` panic for a new disabled edge is also fixed with regression coverage.
