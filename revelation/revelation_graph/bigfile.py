"""The BigFile archive of Myst IV ("UBI_BF_SIG"): a tree of names, sizes and offsets, then the data, uncompressed.

Layout after github.com/coderjo/myst4tools (m4b-format.md); an archive may hold other archives.
"""
import struct


class Reader:
    def __init__(self, data):
        self.data, self.pos = data, 0

    def u8(self):
        self.pos += 1
        return self.data[self.pos - 1]

    def u32(self):
        self.pos += 4
        return struct.unpack_from('<I', self.data, self.pos - 4)[0]

    def name(self):
        length = self.u32()
        if length > 4096 or self.pos + length > len(self.data):
            raise EOFError
        self.pos += length
        return self.data[self.pos - length:self.pos].rstrip(b'\0').decode('latin-1')


def directory(reader, prefix=''):
    """Yield (path, size, offset) for every file under a DIR structure."""
    subdirectories = reader.u8()
    pending = []
    for _ in range(subdirectories):
        name = reader.name()
        pending.extend(directory(reader, prefix + name + '/'))
    count = reader.u32()
    for _ in range(count):
        name = reader.name()
        size, offset = reader.u32(), reader.u32()
        pending.append((prefix + name, size, offset))
    return pending


def parse(data):
    """The file table of an archive whose head is `data`. Raises EOFError (or struct.error) if `data` is too short."""
    reader = Reader(data)
    if reader.name() != 'UBI_BF_SIG':
        raise ValueError('not a BigFile archive')
    version, unknown = reader.u32(), reader.u32()
    files = directory(reader)
    return files, reader.pos
