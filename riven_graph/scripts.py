"""Decode original Riven CARD/HSPT bytecode without executing game state.

All offsets are byte offsets relative to the containing resource. Binary layouts
were checked against ScummVM's engines/mohawk/riven_scripts.cpp, riven_card.cpp
and riven_stack.cpp. No engine fixes or runtime patches are applied.
"""

from __future__ import annotations

import json
import struct
from collections.abc import Iterator, Mapping, Sequence


class ParseError(ValueError):
    """Malformed or truncated Riven resource."""


EVENTS = {
    0: "mouse_down", 1: "mouse_drag", 2: "mouse_up", 3: "mouse_enter",
    4: "mouse_inside", 5: "mouse_leave", 6: "card_load", 7: "card_leave",
    8: "card_frame", 9: "card_enter", 10: "card_update",
}

OPCODES = {
    0: "empty", 1: "draw_bitmap", 2: "change_card", 3: "play_script_slst",
    4: "play_sound", 5: "empty_5", 6: "unused_complex_animation",
    7: "set_variable", 8: "switch", 9: "enable_hotspot", 10: "disable_hotspot",
    11: "empty_11", 12: "stop_sound", 13: "change_cursor", 14: "delay",
    15: "empty_15", 16: "empty_16", 17: "run_external", 18: "transition",
    19: "refresh_card", 20: "begin_screen_update", 21: "apply_screen_update",
    22: "empty_22", 23: "empty_23", 24: "increment_variable", 25: "empty_25",
    26: "empty_26", 27: "change_stack", 28: "disable_movie",
    29: "disable_all_movies", 30: "unused_set_movie_rate", 31: "enable_movie",
    32: "play_movie_blocking", 33: "play_movie", 34: "stop_movie",
    35: "unused_water_effect", 36: "unknown_36", 37: "fade_ambient_sounds",
    38: "store_movie_opcode", 39: "activate_plst", 40: "activate_slst",
    41: "activate_mlst_and_play", 42: "empty_42", 43: "activate_blst",
    44: "activate_flst", 45: "zip_mode", 46: "activate_mlst",
    47: "unused_slst_with_volume",
}


class Reader:
    def __init__(self, data: bytes, label: str = "resource"):
        self.data = data
        self.pos = 0
        self.label = label

    def need(self, size: int) -> None:
        if size < 0 or self.pos + size > len(self.data):
            raise ParseError(f"{self.label}+0x{self.pos:x}: need {size} bytes, "
                             f"only {len(self.data) - self.pos} remain")

    def u16(self) -> int:
        self.need(2)
        value = struct.unpack_from(">H", self.data, self.pos)[0]
        self.pos += 2
        return value

    def i16(self) -> int:
        value = self.u16()
        return value if value < 0x8000 else value - 0x10000

    def u32(self) -> int:
        return (self.u16() << 16) | self.u16()

    def finish(self) -> None:
        if self.pos != len(self.data):
            raise ParseError(f"{self.label}+0x{self.pos:x}: "
                             f"{len(self.data) - self.pos} trailing bytes")


def _commands(reader: Reader, depth: int = 0) -> list[dict]:
    if depth > 128:
        raise ParseError(f"{reader.label}+0x{reader.pos:x}: excessive switch nesting")
    count = reader.u16()
    # Every command needs at least opcode + argument count.
    reader.need(count * 4)
    result = []
    for _ in range(count):
        offset = reader.pos
        opcode = reader.u16()
        arg_count = reader.u16()
        command = {"opcode": opcode, "name": OPCODES.get(opcode, f"unknown_{opcode}"),
                   "offset": offset, "argument_count": arg_count}
        if opcode == 8:
            variable = reader.u16()
            branch_count = reader.u16()
            reader.need(branch_count * 4)
            command.update(args=[variable], variable=variable, branches=[])
            if arg_count != 2:
                command["warnings"] = [f"switch header word is {arg_count}, expected 2"]
            for _ in range(branch_count):
                branch_offset = reader.pos
                value = reader.u16()
                commands = _commands(reader, depth + 1)
                command["branches"].append({"value": value, "offset": branch_offset,
                                             "commands": commands,
                                             "end_offset": reader.pos})
        elif opcode == 27:
            # The engine reads exactly 3 words, independent of argument_count.
            # Reject a conflicting count instead of guessing a new format.
            if arg_count != 3:
                raise ParseError(f"{reader.label}+0x{offset:x}: change_stack "
                                 f"argument count {arg_count}, expected 3")
            stack_name_id = reader.u16()
            high, low = reader.u16(), reader.u16()
            command.update(args=[stack_name_id, high, low], stack_name_id=stack_name_id,
                           global_card_id=(high << 16) | low)
        else:
            reader.need(arg_count * 2)
            command["args"] = [reader.u16() for _ in range(arg_count)]
            if opcode not in OPCODES or opcode == 36:
                command["warnings"] = ["opcode semantics unknown; raw arguments retained"]
            if opcode == 17:
                args = command["args"]
                if len(args) < 2 or args[1] != len(args) - 2:
                    command.setdefault("warnings", []).append("external argument count mismatch")
            if opcode == 38:
                args = command["args"]
                if len(args) >= 5:
                    command.update(movie_slot=args[0], movie_time=(args[1] << 16) | args[2],
                                   stored_opcode=args[3], stored_args=args[4:])
                else:
                    command.setdefault("warnings", []).append("short stored movie opcode")
        command["end_offset"] = reader.pos
        result.append(command)
    return result


def _typed_scripts(reader: Reader) -> list[dict]:
    count = reader.u16()
    reader.need(count * 4)
    scripts = []
    for _ in range(count):
        offset = reader.pos
        event_type = reader.u16()
        commands = _commands(reader)
        scripts.append({"type": event_type, "event": EVENTS.get(event_type, f"unknown_{event_type}"),
                        "offset": offset, "end_offset": reader.pos, "commands": commands})
    return scripts


def parse_script(data: bytes, offset: int = 0, *, require_end: bool = True) -> dict:
    """Read one untyped script (uint16 command count, then commands)."""
    reader = Reader(data, "script")
    reader.pos = offset
    commands = _commands(reader)
    if require_end:
        reader.finish()
    return {"offset": offset, "end_offset": reader.pos, "commands": commands}


def parse_card(data: bytes) -> dict:
    reader = Reader(data, "CARD")
    name_id, zip_mode_place = reader.i16(), reader.u16()
    scripts = _typed_scripts(reader)
    reader.finish()
    return {"name_id": name_id, "zip_mode_place": zip_mode_place, "scripts": scripts,
            "consumed_bytes": reader.pos}


def parse_hotspots(data: bytes) -> list[dict]:
    reader = Reader(data, "HSPT")
    count = reader.u16()
    # 22 bytes fixed data + 2-byte script list count.
    reader.need(count * 24)
    hotspots = []
    for ordinal in range(count):
        offset = reader.pos
        blst_id, name_id = reader.u16(), reader.i16()
        rect = [reader.i16() for _ in range(4)]
        unknown, cursor, index = reader.u16(), reader.u16(), reader.u16()
        transition_offset, flags = reader.i16(), reader.u16()
        scripts = _typed_scripts(reader)
        hotspots.append({"ordinal": ordinal, "offset": offset, "end_offset": reader.pos,
                         "blst_id": blst_id, "name_id": name_id, "rect": rect,
                         "unknown": unknown, "cursor": cursor, "index": index,
                         "transition_offset": transition_offset, "flags": flags,
                         "valid_rect": rect[0] < rect[2] and rect[1] < rect[3],
                         "scripts": scripts})
    reader.finish()
    return hotspots


def parse_names(data: bytes) -> list[str]:
    """Read NAME strings in numeric name-ID order, not its sorted index order."""
    reader = Reader(data, "NAME")
    count = reader.u16()
    reader.need(count * 4)
    offsets = [reader.u16() for _ in range(count)]
    # Sorted lookup index is deliberately separate from name IDs.
    indices = [reader.u16() for _ in range(count)]
    if any(i >= count for i in indices):
        raise ParseError("NAME: sorted index outside name table")
    base = reader.pos
    names = []
    for offset in offsets:
        start = base + offset
        if start >= len(data):
            raise ParseError(f"NAME: string offset 0x{start:x} outside resource")
        end = data.find(b"\0", start)
        if end < 0:
            raise ParseError(f"NAME+0x{start:x}: unterminated string")
        names.append(data[start:end].decode("mac_roman"))
    return names


def parse_rmap(data: bytes) -> list[int]:
    if len(data) % 4:
        raise ParseError("RMAP size is not divisible by four")
    return [value[0] for value in struct.iter_unpack(">I", data)]


def parse_plst(data: bytes) -> list[dict]:
    reader = Reader(data, "PLST")
    count = reader.u16()
    reader.need(count * 12)
    pictures = []
    for _ in range(count):
        offset = reader.pos
        index, image_id = reader.u16(), reader.u16()
        rect = [reader.u16() for _ in range(4)]
        pictures.append({"index": index, "image_id": image_id, "rect": rect, "offset": offset})
    reader.finish()
    return pictures


def walk_commands(scripts: Sequence[dict]) -> Iterator[tuple[dict, list[dict]]]:
    """Yield (command, branch guards), recursively, with no state assumptions.

    Each guard is {variable, value, switch_offset, excluded_values}; value 65535
    denotes default (all explicit branch values excluded). Event and hotspot
    context belong to the caller, not to the guard list.
    """
    def visit(commands, guards):
        for command in commands:
            yield command, guards
            if command["opcode"] == 8:
                values = [b["value"] for b in command["branches"] if b["value"] != 0xffff]
                for branch in command["branches"]:
                    guard = {"variable": command["variable"], "value": branch["value"],
                             "switch_offset": command["offset"],
                             "excluded_values": values if branch["value"] == 0xffff else []}
                    yield from visit(branch["commands"], guards + [guard])
    for script in scripts:
        yield from visit(script["commands"], [])


def name_at(names: Mapping, table: int, index: int) -> str:
    values = names.get(table, names.get(str(table), []))
    return values[index] if 0 <= index < len(values) else f"name_{table}[{index}]"


def format_command(command: dict, names: Mapping | None = None) -> str:
    names = names or {}
    op, args = command["opcode"], command["args"]
    if op == 7 and len(args) == 2:
        return f"{name_at(names, 4, args[0])} = {args[1]};"
    if op == 24 and len(args) == 2:
        return f"{name_at(names, 4, args[0])} += {args[1]};"
    if op == 17 and len(args) >= 2:
        return f"{name_at(names, 3, args[0])}({', '.join(map(str, args[2:]))});"
    if op == 27:
        stack = json.dumps(name_at(names, 5, command["stack_name_id"]))
        return f"change_stack({stack}, rmap=0x{command['global_card_id']:x});"
    if op == 38 and "stored_opcode" in command:
        stored_name = OPCODES.get(command["stored_opcode"], f"unknown_{command['stored_opcode']}")
        stored_args = ", ".join(map(str, command["stored_args"]))
        return (f"store_movie_opcode(slot={command['movie_slot']}, time={command['movie_time']}, "
                f"command={stored_name}({stored_args}));")
    return f"{command['name']}({', '.join(map(str, args))});"


def format_scripts(scripts: Sequence[dict], names: Mapping | None = None) -> str:
    names = names or {}
    lines = []
    def emit(commands, indent):
        for command in commands:
            prefix = "  " * indent
            address = f"/* +0x{command['offset']:04x} */ "
            if command["opcode"] == 8:
                variable = name_at(names, 4, command["variable"])
                lines.append(f"{prefix}{address}switch ({variable}) {{")
                for branch in command["branches"]:
                    case = "default" if branch["value"] == 0xffff else f"case {branch['value']}"
                    lines.append(f"{prefix}  {case}:")
                    emit(branch["commands"], indent + 2)
                    lines.append(f"{prefix}    break;")
                lines.append(f"{prefix}}}")
            else:
                lines.append(prefix + address + format_command(command, names))
            for warning in command.get("warnings", []):
                lines.append(f"{prefix}// {warning}")
    for script in scripts:
        event = script.get("event", "script")
        lines.append(f"on {event} {{")
        emit(script["commands"], 1)
        lines.append("}")
    return "\n".join(lines)
