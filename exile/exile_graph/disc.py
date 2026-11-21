"""Locate the game's files in a folder or inside CD images, without unpacking.

Two layouts are accepted and can be mixed:

* a folder holding ``M3.exe`` and the ``*nodes.m3a`` archives (an installed
  game, or the DVD edition);
* raw or cooked CD images (``.img``, ``.bin``, ``.iso``) of the four-CD
  release, read through their ISO 9660 directory.

Every file is exposed as a :class:`FileRef` giving random access to its bytes
and the place it came from, so that offsets can be reported as provenance.
"""
from __future__ import annotations

import os
import struct
from dataclasses import dataclass
from pathlib import Path

SYNC = b'\x00' + b'\xff' * 10 + b'\x00'
IMAGE_SUFFIXES = {'.img', '.bin', '.iso'}


class DiscError(ValueError):
    pass


class DiscImage:
    """A CD image with 2048-byte logical sectors, stored cooked or raw."""

    def __init__(self, path):
        self.path = Path(path)
        self.stream = self.path.open('rb')
        head = self.stream.read(16)
        if head[:12] == SYNC:
            # Raw 2352-byte sectors. Mode 1 keeps its data after a 16-byte
            # header, Mode 2 Form 1 after a further 8-byte subheader.
            self.sector_size, self.data_offset = 2352, 16 if head[15] == 1 else 24
        else:
            self.sector_size, self.data_offset = 2048, 0
        self.volume, self.created, self.root = self._descriptor()

    def close(self):
        self.stream.close()

    def sector_position(self, lba):
        return lba * self.sector_size + self.data_offset

    def read(self, lba, offset, size):
        """Read ``size`` bytes starting ``offset`` bytes into sector ``lba``."""
        out = bytearray()
        lba += offset // 2048
        offset %= 2048
        while size > 0:
            take = min(size, 2048 - offset)
            # Positioned reads: several threads extract pictures at once.
            block = os.pread(self.stream.fileno(), take, self.sector_position(lba) + offset)
            if len(block) != take:
                raise DiscError(f'{self.path.name}: read past the end of the image at sector {lba}')
            out += block
            size -= take
            lba += 1
            offset = 0
        return bytes(out)

    def _descriptor(self):
        for lba in range(16, 32):
            block = self.read(lba, 0, 2048)
            if block[1:6] != b'CD001':
                break
            if block[0] == 1:
                return (block[40:72].decode('ascii', 'replace').strip(),
                        block[813:829].decode('ascii', 'replace'),
                        self._record(block[156:156 + block[156]]))
            if block[0] == 255:
                break
        raise DiscError(f'{self.path.name}: no ISO 9660 primary volume descriptor')

    @staticmethod
    def _record(data):
        return {'lba': struct.unpack_from('<I', data, 2)[0], 'size': struct.unpack_from('<I', data, 10)[0],
                'directory': bool(data[25] & 2), 'name': data[33:33 + data[32]].decode('latin1').split(';')[0]}

    def files(self):
        """Yield every file of the ISO 9660 tree as (path, lba, size)."""
        pending = [('', self.root)]
        while pending:
            prefix, directory = pending.pop()
            data, pos = self.read(directory['lba'], 0, directory['size']), 0
            while pos < len(data):
                length = data[pos]
                if not length:
                    pos = (pos // 2048 + 1) * 2048
                    continue
                item = self._record(data[pos:pos + length])
                pos += length
                if item['name'] in ('\0', '\1'):
                    continue
                path = prefix + item['name']
                if item['directory']:
                    pending.append((path + '/', item))
                else:
                    yield path, item['lba'], item['size']


@dataclass
class FileRef:
    """One game file, in a folder or inside a disc image."""
    name: str
    size: int
    path: Path | None = None
    image: DiscImage | None = None
    lba: int = 0
    inner_path: str = ''

    def read(self, offset=0, size=None):
        if size is None:
            size = self.size - offset
        if offset < 0 or size < 0 or offset + size > self.size:
            raise DiscError(f'{self.name}: read of {size} bytes at {offset} is outside the file')
        if self.image is not None:
            return self.image.read(self.lba, offset, size)
        with self.path.open('rb') as stream:
            stream.seek(offset)
            return stream.read(size)

    @property
    def origin(self):
        """Where the file is, for provenance records."""
        if self.image is not None:
            return {'container': self.image.path.name, 'volume': self.image.volume, 'path': self.inner_path,
                    'sector': self.lba, 'containerOffset': self.image.sector_position(self.lba)}
        return {'path': str(self.path)}


class Source:
    """The set of game files found under one or several input paths."""

    def __init__(self, paths):
        self.images = []
        self.files = []
        for path in map(Path, paths):
            if not path.exists():
                raise DiscError(f'{path}: no such file or directory')
            candidates = sorted(p for p in path.rglob('*') if p.is_file()) if path.is_dir() else [path]
            for candidate in candidates:
                if candidate.name.startswith('.'):
                    continue
                if candidate.suffix.lower() in IMAGE_SUFFIXES and candidate.stat().st_size > 1 << 20:
                    try:
                        image = DiscImage(candidate)
                    except DiscError:
                        continue
                    self.images.append(image)
                    for inner, lba, size in image.files():
                        self.files.append(FileRef(inner.rsplit('/', 1)[-1], size, image=image, lba=lba,
                                                  inner_path=inner))
                else:
                    self.files.append(FileRef(candidate.name, candidate.stat().st_size, path=candidate))

    def close(self):
        for image in self.images:
            image.close()

    def find(self, name):
        """Every file with this name, whatever its case."""
        wanted = name.lower()
        return [f for f in self.files if f.name.lower() == wanted]

    def node_archives(self):
        """Map a room name to its node archive.

        The archives are called ``<ROOM>nodes.m3a``. ISO 9660 shortens the
        name to eight characters (``LEISNODE.M3A``), so only the first four
        letters are relied upon.
        """
        archives = {}
        for ref in self.files:
            lowered = ref.name.lower()
            if lowered.endswith('.m3a') and lowered[4:8] == 'node':
                archives.setdefault(ref.name[:4].upper(), []).append(ref)
        return archives
