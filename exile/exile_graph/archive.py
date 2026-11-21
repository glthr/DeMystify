"""Read the directory of a Myst III resource archive (.m3a, .m3r, .m3t, .m3u).

An archive starts with a directory of 32-bit words, lightly encrypted in the
retail files. Each entry is a node number (and, in archives shared by several
rooms, a room name) followed by its resources: cube faces, frames, movies,
spot items and so on. Resource payloads are stored as they are; pictures are
ordinary JPEG files.
"""
from __future__ import annotations

import struct
from dataclasses import dataclass, field

ADD_KEY, MULT_KEY, MASK = 0x3C6EF35F, 0x0019660D, 0xFFFFFFFF

TYPES = {0: 'cubeFace', 1: 'waterEffectMask', 2: 'lavaEffectMask', 3: 'magneticEffectMask',
         4: 'shieldEffectMask', 5: 'spotItem', 6: 'frame', 7: 'rawData', 8: 'movie', 10: 'stillMovie',
         11: 'text', 12: 'textMetadata', 13: 'numMetadata', 69: 'localizedSpotItem',
         70: 'localizedFrame', 72: 'multitrackMovie', 74: 'dialogMovie'}
CUBE_FACE, SPOT_ITEM, FRAME, MOVIE = 0, 5, 6, 8


class ArchiveError(ValueError):
    pass


@dataclass
class Resource:
    offset: int
    size: int
    face: int
    type: int
    metadata: list

    @property
    def type_name(self):
        return TYPES.get(self.type, f'type{self.type}')


@dataclass
class Entry:
    room: str
    index: int
    resources: list = field(default_factory=list)

    def find(self, kind, face=None):
        return [r for r in self.resources if r.type == kind and (face is None or r.face == face)]


class Archive:
    def __init__(self, ref, room=None):
        """``room`` names a single-room archive; leave it out for shared ones."""
        self.ref = ref
        self.room = room
        self.encrypted = False
        self.entries = self._directory()
        self.by_key = {(e.room, e.index): e for e in self.entries}

    def _directory(self):
        first = struct.unpack('<I', self.ref.read(0, 4))[0]
        self.encrypted = first > 1_000_000
        count = first ^ ADD_KEY if self.encrypted else first
        if count * 4 > self.ref.size or count == 0:
            raise ArchiveError(f'{self.ref.name}: implausible directory of {count} words')
        words = list(struct.unpack(f'<{count}I', self.ref.read(0, count * 4)))
        if self.encrypted:
            key = 0
            for i in range(count):
                key = (key + ADD_KEY) & MASK
                words[i] ^= key
                key = (key * MULT_KEY) & MASK
        data = struct.pack(f'<{count}I', *words)
        self.directory_size = count * 4
        entries, pos = [], 4
        while pos + 4 < len(data):
            if self.room is None:
                room = data[pos:pos + 4].decode('latin1')
                pos += 4
            else:
                room = self.room
            index = data[pos] | data[pos + 1] << 8 | data[pos + 2] << 16
            entry = Entry(room, index)
            sub_count = data[pos + 3]
            pos += 4
            for _ in range(sub_count):
                offset, size, meta_count, face, kind = struct.unpack_from('<IIHBB', data, pos)
                pos += 12
                metadata = list(struct.unpack_from(f'<{meta_count}I', data, pos))
                pos += meta_count * 4
                entry.resources.append(Resource(offset, size, face, kind, metadata))
            entries.append(entry)
        return entries

    def get(self, room, index):
        return self.by_key.get((room, index))

    def read(self, resource):
        return self.ref.read(resource.offset, resource.size)
