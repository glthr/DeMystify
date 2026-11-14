"""Small, read-only ISO9660 reader for the original Riven CD-ROMs.

Only directory metadata and requested resource ranges are read.  The several
gigabytes of media on the discs do not need to be unpacked or mounted.
"""

from __future__ import annotations

from dataclasses import dataclass
import hashlib
from pathlib import Path
import struct
from typing import Iterable, Iterator


class FormatError(ValueError):
    """An input container is malformed or unsupported."""


@dataclass(frozen=True)
class FileSlice:
    """A bounded file (possibly embedded inside an ISO image)."""

    path: Path
    offset: int
    size: int
    name: str

    def __post_init__(self) -> None:
        object.__setattr__(self, "path", Path(self.path))
        if self.offset < 0 or self.size < 0:
            raise FormatError("Negative file slice extent")
        if self.offset + self.size > self.path.stat().st_size:
            raise FormatError(f"File slice exceeds input: {self.path}:{self.name}")

    @classmethod
    def from_path(cls, path: str | Path) -> "FileSlice":
        path = Path(path)
        return cls(path, 0, path.stat().st_size, path.name)

    def read(self, offset: int = 0, size: int | None = None) -> bytes:
        if size is None:
            size = self.size - offset
        if offset < 0 or size < 0 or offset + size > self.size:
            raise FormatError(f"Read outside {self.name}: {offset}+{size} > {self.size}")
        with self.path.open("rb") as stream:
            stream.seek(self.offset + offset)
            data = stream.read(size)
        if len(data) != size:
            raise FormatError(f"Truncated input: {self.path}:{self.name}")
        return data

    def sha256(self) -> str:
        digest = hashlib.sha256()
        with self.path.open("rb") as stream:
            stream.seek(self.offset)
            left = self.size
            while left:
                data = stream.read(min(left, 1024 * 1024))
                if not data:
                    raise FormatError(f"Truncated input: {self.path}:{self.name}")
                digest.update(data)
                left -= len(data)
        return digest.hexdigest()

    @property
    def origin(self) -> dict:
        return {"container": str(self.path), "path": self.name,
                "offset": self.offset, "size": self.size}


class ISO9660:
    """Read the primary ISO9660 directory tree (2048-byte logical sectors)."""

    sector_size = 2048

    def __init__(self, path: str | Path):
        self.source = FileSlice.from_path(path)
        self.volume_id = ""
        primary = None
        for sector in range(16, min(self.source.size // self.sector_size, 256)):
            block = self.source.read(sector * self.sector_size, self.sector_size)
            if block[1:6] != b"CD001" or block[6] != 1:
                raise FormatError(f"Invalid ISO9660 volume descriptor: {path}")
            if block[0] == 1:
                primary = block
            if block[0] == 255:
                break
        if primary is None:
            raise FormatError(f"No ISO9660 primary volume descriptor: {path}")
        logical_block = self._both16(primary, 128)
        if logical_block != self.sector_size:
            raise FormatError(f"Unsupported ISO block size {logical_block}")
        self.volume_id = primary[40:72].decode("ascii", errors="replace").strip()
        self._root = self._record(primary[156:156 + primary[156]])

    @staticmethod
    def _both16(data: bytes, offset: int) -> int:
        le = struct.unpack_from("<H", data, offset)[0]
        be = struct.unpack_from(">H", data, offset + 2)[0]
        if le != be:
            raise FormatError("ISO9660 dual-endian 16-bit field mismatch")
        return le

    @staticmethod
    def _both32(data: bytes, offset: int) -> int:
        le = struct.unpack_from("<I", data, offset)[0]
        be = struct.unpack_from(">I", data, offset + 4)[0]
        if le != be:
            raise FormatError("ISO9660 dual-endian 32-bit field mismatch")
        return le

    def _record(self, data: bytes) -> tuple[int, int, int, bytes]:
        if len(data) < 34 or data[0] != len(data) or 33 + data[32] > len(data):
            raise FormatError("Invalid ISO9660 directory record")
        extent = self._both32(data, 2)
        size = self._both32(data, 10)
        flags = data[25]
        if data[1] or data[26] or data[27] or flags & 0x80:
            raise FormatError("Extended/interleaved/multi-extent ISO files unsupported")
        if extent * self.sector_size + size > self.source.size:
            raise FormatError("ISO9660 directory extent exceeds image")
        return extent, size, flags, data[33:33 + data[32]]

    def files(self) -> Iterator[FileSlice]:
        visited: set[tuple[int, int]] = set()

        def walk(record: tuple, prefix: str) -> Iterator[FileSlice]:
            extent, size, flags, _ = record
            key = (extent, size)
            if key in visited:
                raise FormatError("Cyclic ISO9660 directory tree")
            visited.add(key)
            data = self.source.read(extent * self.sector_size, size)
            pos = 0
            while pos < len(data):
                length = data[pos]
                if length == 0:
                    pos = (pos // self.sector_size + 1) * self.sector_size
                    continue
                if pos + length > len(data) or pos % self.sector_size + length > self.sector_size:
                    raise FormatError("ISO9660 directory record crosses block boundary")
                child = self._record(data[pos:pos + length])
                pos += length
                child_extent, child_size, child_flags, raw_name = child
                if raw_name in (b"\x00", b"\x01"):
                    continue
                # Apple's hybrid CDs store a resource fork as an ISO9660
                # associated file with the same name as the data fork.  Only
                # the ordinary data fork contains the Mohawk archive.
                if child_flags & 4:
                    continue
                name = raw_name.decode("ascii", errors="replace").split(";", 1)[0].rstrip(".")
                if not name or "/" in name or name in (".", ".."):
                    raise FormatError("Invalid ISO9660 file identifier")
                full_name = prefix + name
                if child_flags & 2:
                    yield from walk(child, full_name + "/")
                else:
                    yield FileSlice(self.source.path, child_extent * self.sector_size,
                                    child_size, full_name)

        yield from walk(self._root, "")


def discover_mhk(inputs: Iterable[str | Path]) -> list[FileSlice]:
    """Find MHK files in directories, loose files, or ISO9660 images.

    Discovery is deterministic and preserves repeated archives on separate
    discs.  The caller can deduplicate by content hash or resource precedence.
    """
    paths: set[Path] = set()
    for value in inputs:
        path = Path(value)
        if path.is_dir():
            paths.update(p for p in path.rglob("*")
                         if p.is_file() and p.suffix.lower() in (".iso", ".mhk"))
        elif path.is_file():
            paths.add(path)
        else:
            raise FileNotFoundError(path)
    found = []
    for path in sorted(paths, key=lambda p: str(p).casefold()):
        if path.suffix.lower() == ".iso":
            found.extend(f for f in ISO9660(path).files() if f.name.lower().endswith(".mhk"))
        elif path.suffix.lower() == ".mhk":
            found.append(FileSlice.from_path(path))
        else:
            raise FormatError(f"Expected directory, ISO, or MHK: {path}")
    return found
