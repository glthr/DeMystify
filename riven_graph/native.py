"""Reviewed Riven navigation implemented outside the CARD/HSPT bytecode.

These facts are derived from the pinned ScummVM implementation, not guessed
from card names. They are kept separate from edges actually encoded on disc.
The source URLs below allow every native edge to be audited. This is not an
emulator: conditions are preserved as annotations rather than evaluated.
"""

from __future__ import annotations

from typing import Mapping, Sequence

SCUMMVM_COMMIT = "aed0208e772b128d3f953207a297fc521c32833a"
SOURCE_BASE = f"https://github.com/scummvm/scummvm/blob/{SCUMMVM_COMMIT}/engines/mohawk/"
STACK_IDS = {1: "ospit", 2: "pspit", 3: "rspit", 4: "tspit", 5: "bspit", 6: "gspit", 7: "jspit", 8: "aspit"}

# This table is applied when a CARD is requested, before the card is entered.
# In particular, it is NOT an outgoing edge from the redirect card itself.
SPECIAL_REDIRECTS = (
    ("aspit", 0x1F04, "ospit", 0x44AD, "Trap book"),
    ("bspit", 0x1C0E7, "ospit", 0x2E76, "Dome linking book"),
    ("gspit", 0x111B1, "ospit", 0x2E76, "Dome linking book"),
    ("jspit", 0x28A18, "rspit", 0x0F94, "Tay linking book"),
    ("jspit", 0x26228, "ospit", 0x2E76, "Dome linking book"),
    ("ospit", 0x5F0D, "pspit", 0x3BF0, "Return from 233rd Age"),
    ("ospit", 0x470A, "jspit", 0x1508E, "Return from 233rd Age"),
    ("ospit", 0x5C52, "gspit", 0x10BEA, "Return from 233rd Age"),
    ("ospit", 0x5D68, "bspit", 0x1ADFD, "Return from 233rd Age"),
    ("ospit", 0x5E49, "tspit", 0x0E87, "Return from 233rd Age"),
    ("pspit", 0x4108, "ospit", 0x2E76, "Dome linking book"),
    ("rspit", 0x32D8, "jspit", 0x1C474, "Return from Tay"),
    ("tspit", 0x21B69, "ospit", 0x2E76, "Dome linking book"),
)


def local_card_id(stack: str, global_id: int, rmaps: Mapping[str, Sequence[int]]) -> int:
    """Resolve RMAP identity exactly as ScummVM does (last matching index)."""
    matches = [i for i, value in enumerate(rmaps[stack]) if value == global_id]
    if not matches:
        raise ValueError(f"{stack}: RMAP 0x{global_id:x} has no CARD index")
    return matches[-1]


def resolve_destination(stack: str, card_id: int, rmaps: Mapping[str, Sequence[int]]) -> dict:
    """Apply the full game's native destination redirects in engine order.

    Keep the literal destination, since redirect cards can still exist in the
    archives and deserve their own nodes in an archival graph.
    """
    stack = stack.lower()
    result = {"literal_stack": stack, "literal_card_id": card_id, "redirect_chain": []}
    for src_stack, src_rmap, dst_stack, dst_rmap, reason in SPECIAL_REDIRECTS:
        if stack != src_stack:
            continue
        # A missing source RMAP is not an error for unrelated editions/stacks.
        if src_rmap not in rmaps.get(stack, ()):
            continue
        if card_id == local_card_id(stack, src_rmap, rmaps):
            destination_id = local_card_id(dst_stack, dst_rmap, rmaps)
            result["redirect_chain"].append({
                "source_stack": stack, "source_card_id": card_id,
                "source_rmap": src_rmap, "target_stack": dst_stack,
                "target_card_id": destination_id, "target_rmap": dst_rmap,
                "reason": reason, "source": SOURCE_BASE + "riven.cpp#L588-L630",
            })
            stack, card_id = dst_stack, destination_id
    result.update(stack=stack, card_id=card_id, redirected=bool(result["redirect_chain"]))
    return result


def _transition(stack, *, rmap=None, card=None, condition="", kind="native", source="", **extra):
    result = {"target_stack": stack, "kind": kind, "condition": condition, "source": SOURCE_BASE + source}
    if rmap is not None:
        result["target_rmap"] = rmap
    if card is not None:
        result["target_card_id"] = card
    result.update(extra)
    return result


def external_transitions(stack: str, external_name: str) -> list[dict]:
    """Potential native changes caused by an opcode-17 external function.

    A dynamic-return record deliberately has no destination: connecting it to
    every card would destroy caller-sensitive return semantics. Some menu
    functions below were added by ScummVM; they only appear if actually called.
    """
    stack, name = stack.lower(), external_name.lower()
    if stack == "aspit":
        if name == "xasetupcomplete":
            return [_transition("aspit", rmap=0xE2E, source="riven_stacks/aspit.cpp#L142-L148")]
        if name in {"xaatrusbookback", "xacathbookback", "xtrapbookback"}:
            return [{"kind": "dynamic-return", "target_stack": None,
                     "condition": "returnstackid / returncardid saved when the inventory item was opened",
                     "source": SOURCE_BASE + "riven_inventory.cpp#L156-L167"}]
        if name == "xaresumegame":
            return [{"kind": "dynamic-return", "target_stack": None,
                     "condition": "menu saved stack and card; resumes the same caller",
                     "source": SOURCE_BASE + "riven.cpp#L338-L347"}]
        if name == "xanewgame":
            return [
                _transition("aspit", card=2, kind="new-game", sequence=0,
                            condition="new game confirmed", source="riven_stacks/aspit.cpp#L369-L389"),
                _transition("tspit", rmap=0x6E9A, kind="new-game", sequence=1,
                            condition="after aspit:2; new game confirmed", source="riven_stacks/aspit.cpp#L369-L389"),
            ]
    if stack == "jspit":
        if name == "xvga1300_carriage":
            return [
                _transition("jspit", rmap=rmap, sequence=i,
                            condition="jgallows != 1 and player clicks during the carriage window",
                            source="riven_stacks/jspit.cpp#L238-L333")
                for i, rmap in enumerate((0x18D4D, 0x18AB5, 0x17167))
            ]
        if name in {"xhandlecontrolup", "xhandlecontroldown"}:
            condition = "handle dragged down" if name == "xhandlecontrolup" else "handle dragged up"
            return [_transition("jspit", rmap=0x1E374, condition=condition,
                                source="riven_stacks/jspit.cpp#L375-L420")]
        if name == "xhandlecontrolmid":
            return [
                _transition("jspit", rmap=0x1E597, condition="handle dragged up", source="riven_stacks/jspit.cpp#L422-L460"),
                _transition("jspit", rmap=0x1E29C, condition="handle dragged down", source="riven_stacks/jspit.cpp#L422-L460"),
            ]
    if stack == "ospit":
        if name == "xbookclick":
            return [
                _transition("ospit", rmap=0x2885, condition="click trap book in the native video window", source="riven_stacks/ospit.cpp#L69-L129"),
                _transition("aspit", card=1, kind="ending", condition="no click on third request; ending credits then game reset", source="riven_stacks/ospit.cpp#L138-L146"),
            ]
        if name == "xorollcredittime":
            return [
                _transition("rspit", rmap=0x3338, condition="returnstackid is rspit (ScummVM workaround)", source="riven_stacks/ospit.cpp#L47-L55"),
                _transition("aspit", card=1, kind="ending", condition="returnstackid is not rspit; ending credits then game reset", source="riven_stack.cpp#L208-L269"),
            ]
    if stack == "rspit" and name == "xrcredittime":
        return [_transition("aspit", card=1, kind="ending", condition="ending credits then game reset", source="riven_stack.cpp#L208-L269")]
    if stack == "tspit" and name == "xtexterior300_telescopedown":
        return [_transition("aspit", card=1, kind="ending", condition="telescope at bottom with cover open and pin released; ending credits then game reset", source="riven_stacks/tspit.cpp#L59-L161")]
    return []


def timer_transitions(stack: str, global_card_id: int) -> list[dict]:
    """Hardcoded Sunner timers can change cards while videos play."""
    if stack.lower() != "jspit":
        return []
    timers = {
        0x77D6: (0x79BD, "jsunners == 0"),
        0x79BD: (0x7BEB, "jsunners == 0"),
        0x7BEB: (0xB6CA, "jsunners == 0"),
    }
    if global_card_id not in timers:
        return []
    target, condition = timers[global_card_id]
    return [_transition("jspit", rmap=target, kind="native-timer",
                        condition=condition + "; click or move forward while timer video plays",
                        source="riven_stacks/jspit.cpp#L542-L668")]


def inventory_model() -> dict:
    """Describe the caller-sensitive overlay without expanding all-card edges."""
    return {
        "source": SOURCE_BASE + "riven_inventory.cpp#L91-L167",
        "available_from": "non-aspit cards when inventory is visible",
        "items": [
            {"name": "Atrus journal", "target_stack": "aspit", "target_card_id": 5, "condition": "always owned"},
            {"name": "Catherine journal", "target_stack": "aspit", "target_card_id": 6, "condition": "rrebel in {5,6}"},
            {"name": "Trap book", "target_stack": "aspit", "target_card_id": 7, "condition": "rrebel in {5,6} and atrapbook == 1"},
        ],
        "return": "external back command returns to saved returnstackid / returncardid (RMAP)",
        "graph_policy": "metadata only; inventory entry/return is caller-sensitive and excluded from ordinary navigation distances",
    }
