"""Read ages, rooms, nodes, hotspots and scripts from the decoded executable.

A room owns up to three lists of script blocks: node scripts with hotspots,
ambient sound scripts and background sound scripts. Each block starts with the
node numbers it applies to, so that several nodes can share one script.

Every structure keeps the file offset it was read from.
"""
from __future__ import annotations

import struct
from dataclasses import dataclass, field

from .executable import Executable

ROOM_SCRIPT_NODE = 32765  # run before the scripts of every node of its room


class DatabaseError(ValueError):
    pass


@dataclass
class Opcode:
    op: int            # numbering of versions 1.1 and later
    args: list
    offset: int
    raw_op: int        # number stored in this executable


@dataclass
class CondScript:
    condition: int
    ops: list
    offset: int
    repaired: bool = False


@dataclass
class Hotspot:
    condition: int
    rects: list        # (centerPitch, centerHeading, width, height), or frame x, y, w, h
    cursor: int
    ops: list
    offset: int

    @property
    def background(self):
        """A condition of -1 marks a script run on every frame, with no zone."""
        return self.condition == -1


@dataclass
class Block:
    """One entry of a room's script list, shared by ``node_ids``."""
    kind: str          # 'node', 'ambient' or 'background-sound'
    node_ids: list
    scripts: list
    hotspots: list
    offset: int
    end: int
    index: int


@dataclass
class Node:
    room: str
    id: int
    zip_bit: int = -1
    scripts: list = field(default_factory=list)
    hotspots: list = field(default_factory=list)
    ambient_scripts: list = field(default_factory=list)
    background_sound_scripts: list = field(default_factory=list)
    blocks: list = field(default_factory=list)


@dataclass
class Room:
    id: int
    name: str
    age_id: int
    offset: int
    scripts_offset: int
    ambient_offset: int
    background_offset: int
    stored_zip_bit: int
    unknown: int
    nodes: dict = field(default_factory=dict)
    blocks: list = field(default_factory=list)
    lists: dict = field(default_factory=dict)   # kind -> (start, end) file offsets of its script list


@dataclass
class Age:
    id: int
    disc: int
    label_id: int
    rooms: list


class Reader:
    def __init__(self, exe: Executable):
        self.exe = exe
        self.pos = 0
        # Version 1.0 has no opcode 122: every later number is one lower.
        self.shift_from = 122 if exe.version == '1.0' else None
        self.warnings = []

    def u16(self):
        value = self.exe.u16(self.pos)
        self.pos += 2
        return value

    def s16(self):
        value = self.exe.s16(self.pos)
        self.pos += 2
        return value

    def opcodes(self):
        ops = []
        while True:
            offset = self.pos
            code = self.u16()
            op, count = code & 0xFF, code >> 8
            if code == 0:
                return ops
            args = [self.s16() for _ in range(count)]
            raw = op
            if self.shift_from is not None and op >= self.shift_from:
                op += 1
            ops.append(Opcode(op, args, offset, raw))

    def cond_script(self):
        offset = self.pos
        condition = self.s16()
        if not condition:
            return None
        repaired = False
        if condition == 565:
            # Data bug of the game: one script of MATO 32765 lost its first
            # two bytes. Reading it as stored yields opcodes with too few
            # arguments. As ScummVM does, the condition is taken to be
            # "always" and the two bytes are read as the first opcode.
            condition, repaired = 1, True
            self.pos -= 2
        return CondScript(condition, self.opcodes(), offset, repaired)

    def cond_scripts(self):
        scripts = []
        while True:
            script = self.cond_script()
            if script is None:
                return scripts
            scripts.append(script)

    def rects(self):
        rects = []
        while True:
            a, b, width, height = self.s16(), self.s16(), self.s16(), self.s16()
            last = width >= 0
            rects.append((a, b, abs(width), height))
            if last:
                return rects

    def hotspots(self):
        hotspots = []
        while True:
            offset = self.pos
            condition = self.s16()
            if not condition:
                return hotspots
            rects, cursor = [], 0
            if condition != -1:
                rects = self.rects()
                cursor = self.u16()
            hotspots.append(Hotspot(condition, rects, cursor, self.opcodes(), offset))

    def node_ids(self):
        """Read the header of a block; ``None`` ends the list."""
        first = self.s16()
        if first == 0:
            return None
        if first > 0:
            return [first]
        if first < -10:
            raise DatabaseError(f'unknown node list command {first} at {self.pos - 2:#x}')
        ids = []
        if first == -10:
            # A list of nodes and of inclusive ranges, closed by a zero.
            while True:
                value = self.s16()
                if value == 0:
                    break
                if value < 0:
                    ids.extend(range(-value, self.u16() + 1))
                else:
                    ids.append(value)
        else:
            ids = [self.s16() for _ in range(-first)]
        return ids


def _room(exe, age_id, offset):
    room_id = exe.u32(offset)
    name = exe.data[offset + 4:offset + 12].split(b'\0')[0].decode('ascii')
    scripts, ambient, background, zip_bit, unknown = struct.unpack_from('<5I', exe.data, offset + 12)
    return Room(room_id, name, age_id, offset, exe.offset(scripts) if scripts else 0,
                exe.offset(ambient) if ambient else 0, exe.offset(background) if background else 0,
                zip_bit, unknown)


def _read_blocks(reader, room, kind, start):
    reader.pos = start
    index = 0
    while True:
        offset = reader.pos
        ids = reader.node_ids()
        if ids is None:
            break
        scripts = reader.cond_scripts()
        hotspots = reader.hotspots() if kind == 'node' else []
        block = Block(kind, ids, scripts, hotspots, offset, reader.pos, index)
        room.blocks.append(block)
        for node_id in ids:
            node = room.nodes.get(node_id)
            if node is None:
                # Sound scripts shared by several nodes only apply to nodes
                # that already exist; everything else creates the node.
                if kind != 'node' and len(ids) > 1:
                    continue
                node = room.nodes[node_id] = Node(room.name, node_id)
            node.blocks.append(block)
            if kind == 'node':
                node.zip_bit = index
                node.scripts.extend(scripts)
                node.hotspots.extend(hotspots)
            elif kind == 'ambient':
                node.ambient_scripts.extend(scripts)
            else:
                node.background_sound_scripts.extend(scripts)
        index += 1
    return reader.pos


@dataclass
class Database:
    exe: Executable
    ages: list
    rooms: dict                 # name -> Room
    rooms_by_id: dict
    node_init_script: list
    sound_names: dict
    warnings: list


def load(exe: Executable) -> Database:
    reader = Reader(exe)
    ages, rooms, by_id = [], {}, {}
    for i in range(exe.age_count):
        age_id, disc, room_count, pointers, label = struct.unpack_from('<5I', exe.data, exe.age_table + i * 20)
        pointers = exe.offset(pointers)
        age = Age(age_id, disc if disc != 0xFFFFFFFF else -1, label, [])
        for j in range(room_count):
            room = _room(exe, age_id, exe.offset(exe.u32(pointers + j * 4)))
            for kind, start in (('node', room.scripts_offset), ('ambient', room.ambient_offset),
                                ('background-sound', room.background_offset)):
                if start:
                    room.lists[kind] = (start, _read_blocks(reader, room, kind, start))
            age.rooms.append(room.name)
            rooms[room.name] = room
            by_id[room.id] = room
        ages.append(age)
    init = []
    if exe.node_init_script:
        reader.pos = exe.node_init_script
        init = reader.opcodes()
    names = {}
    position = exe.sound_names
    while True:
        sound_id = exe.u32(position)
        if not sound_id:
            break
        names[sound_id] = exe.data[position + 4:position + 36].split(b'\0')[0].decode('latin1')
        position += 36
    return Database(exe, ages, rooms, by_id, init, names, reader.warnings)
