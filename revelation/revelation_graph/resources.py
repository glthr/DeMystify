"""The binarized resources of Myst IV ("ubi/b0-l" files) and its text AI files.

Strings are stored with the two bits of every pair exchanged (0x55 and 0xAA masks): the transform is
its own inverse. Command blocks are named lists of such strings, each a readable call such as
``ContextChange(1,01,021)``.
"""
import re
import struct

MAGIC = b'ubi/b0-l'


def swizzle(data):
    return bytes(((b & 0xAA) >> 1) | ((b & 0x55) << 1) for b in data)


class Reader:
    def __init__(self, data, pos=0):
        self.data, self.pos = data, pos

    def u32(self):
        self.pos += 4
        return struct.unpack_from('<I', self.data, self.pos - 4)[0]

    def f32(self):
        self.pos += 4
        return struct.unpack_from('<f', self.data, self.pos - 4)[0]

    def text(self):
        length = self.u32()
        if length > 65536 or self.pos + length > len(self.data):
            raise ValueError('string runs past the end')
        self.pos += length
        return swizzle(self.data[self.pos - length:self.pos]).decode('latin-1')


def header(data):
    if data[:8] != MAGIC:
        raise ValueError('not a binarized resource')
    reader = Reader(data, 8)
    return reader, reader.u32(), reader.u32()


def command_block(data):
    """(name, [command, ...]) of a command_block/*.bin file (resource type 6)."""
    reader, kind, count = header(data)
    if kind != 6:
        raise ValueError(f'resource type {kind}, expected 6')
    blocks = []
    for _ in range(count):
        name = reader.text()
        blocks.append((name, [reader.text() for _ in range(reader.u32())]))
    if reader.pos != len(data):
        raise ValueError('bytes left after the last command')
    return blocks


def node(data):
    """node.bin (resource type 8): the kind of viewport and the camera direction on arrival, by entry point."""
    reader, kind, sub = header(data)
    if kind != 8:
        raise ValueError(f'resource type {kind}, expected 8')
    name = reader.text()
    unknown, viewport = reader.u32(), reader.u32()
    values = [reader.f32() for _ in range(6)]
    setters = []
    for _ in range(reader.u32()):
        entry = reader.text()
        setters.append({'entry': entry, 'pitch': reader.f32(), 'yaw': reader.f32()})
    return {'name': name, 'viewport': {1: 'free', 2: 'fixed'}.get(viewport, viewport), 'values': values,
            'arrivals': setters, 'trailing': len(data) - reader.pos}


AI_LINE = re.compile(r'^\s*([\w\[\].]+)\s*\(\s*(\w+)\s*,\s*(.*?)\s*\)\s*$')


def ai(text):
    """The ``key (type, value)`` lines of an .ai file, in order."""
    out = []
    for line in text.splitlines():
        match = AI_LINE.match(line)
        if match:
            value = match.group(3)
            if value.startswith('"') and value.endswith('"'):
                value = value[1:-1]
            out.append((match.group(1), match.group(2), value))
    return out


def strings(data, minimum=4):
    """Length-prefixed swizzled strings found anywhere in a binarized resource (for resources not decoded field by field)."""
    out, i = [], 8
    while i + 4 <= len(data):
        length = struct.unpack_from('<I', data, i)[0]
        if minimum <= length <= 512 and i + 4 + length <= len(data):
            candidate = swizzle(data[i + 4:i + 4 + length])
            if all(32 <= c < 127 for c in candidate):
                out.append((i, candidate.decode('ascii')))
                i += 4 + length
                continue
        i += 1
    return out
