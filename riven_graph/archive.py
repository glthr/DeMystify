"""Bounded Mohawk RSRC 1.0 archive reader.

Format semantics are cross-checked against ScummVM engines/mohawk/resource.cpp.
This independent Python implementation reads metadata plus requested resources,
and does not execute code from the game.
"""

from __future__ import annotations

from dataclasses import asdict, dataclass
from pathlib import Path
import struct

from .iso import FileSlice, FormatError


@dataclass(frozen=True)
class Resource:
    tag: str
    id: int
    index: int
    offset: int
    size: int
    name: str
    stored_size: int
    flags: int


class MohawkArchive:
    def __init__(self, source: FileSlice | str | Path):
        self.source = source if isinstance(source, FileSlice) else FileSlice.from_path(source)
        self.resources: dict[str, dict[int, Resource]] = {}
        header = self.source.read(0, 28)
        if header[:4] != b"MHWK" or header[8:12] != b"RSRC":
            raise FormatError(f"Not a Mohawk resource archive: {self.source.name}")
        version, self.compaction, self.rsrc_size, directory, file_table_rel, self.file_table_size = struct.unpack_from(">HHIIHH", header, 12)
        if version != 0x100:
            raise FormatError(f"Unsupported Mohawk resource version: {version:#x}")
        self.directory_offset = directory
        file_table_start = directory + file_table_rel
        file_count = self._u32(file_table_start)
        entries = self.source.read(file_table_start + 4, file_count * 10)
        file_table = []
        for i in range(file_count):
            offset, low, high, flags, _ = struct.unpack_from(">IHBBH", entries, i * 10)
            size = low | (high << 16) | ((flags & 7) << 24)
            if offset > self.source.size:
                raise FormatError(f"Resource file offset outside archive: {self.source.name}:{i}")
            file_table.append((offset, size, flags))

        string_rel, type_count = struct.unpack(">HH", self.source.read(directory, 4))
        type_table = self.source.read(directory + 4, type_count * 8)
        for i in range(type_count):
            tag_bytes, resource_rel, name_rel = struct.unpack_from(">4sHH", type_table, i * 8)
            tag = tag_bytes.decode("latin-1")
            if tag in self.resources:
                raise FormatError(f"Duplicate Mohawk resource type {tag!r}")
            name_start = directory + name_rel
            name_count = self._u16(name_start)
            names_data = self.source.read(name_start + 2, name_count * 4)
            names = {}
            for j in range(name_count):
                string_offset, index = struct.unpack_from(">HH", names_data, j * 4)
                names[index] = self._cstring(directory + string_rel + string_offset)
            resource_start = directory + resource_rel
            resource_count = self._u16(resource_start)
            resource_data = self.source.read(resource_start + 2, resource_count * 4)
            resources = self.resources[tag] = {}
            for j in range(resource_count):
                resource_id, index = struct.unpack_from(">HH", resource_data, j * 4)
                if not 1 <= index <= file_count:
                    raise FormatError(f"Invalid Mohawk file index: {index}")
                if resource_id in resources:
                    raise FormatError(f"Duplicate resource id {tag}:{resource_id}")
                offset, stored_size, flags = file_table[index - 1]
                size = stored_size
                if tag == "tMOV":
                    # QuickTime movie resources ignore their stored size.  Find
                    # the next nonidentical file offset, as the engine does.
                    following = index
                    while following < file_count and file_table[following][0] == offset:
                        following += 1
                    end = file_table[following][0] if following < file_count else self.source.size
                    size = end - offset
                if size < 0 or offset + size > self.source.size:
                    raise FormatError(f"Resource outside archive: {tag}:{resource_id} ({offset}+{size})")
                resources[resource_id] = Resource(tag, resource_id, index, offset, size,
                                                   names.get(index, ""), stored_size, flags)

    def _u16(self, offset: int) -> int:
        return struct.unpack(">H", self.source.read(offset, 2))[0]

    def _u32(self, offset: int) -> int:
        return struct.unpack(">I", self.source.read(offset, 4))[0]

    def _cstring(self, offset: int) -> str:
        # Resource name offsets are 16-bit.  Bound even a malformed unterminated
        # string so invalid metadata cannot trigger an enormous allocation.
        data = self.source.read(offset, min(65536, self.source.size - offset))
        end = data.find(b"\0")
        if end < 0:
            raise FormatError("Unterminated Mohawk resource name")
        return data[:end].decode("mac_roman")

    def ids(self, tag: str) -> list[int]:
        return sorted(self.resources.get(tag, {}))

    def read(self, tag: str, resource_id: int) -> bytes:
        resource = self.resources[tag][resource_id]
        return self.source.read(resource.offset, resource.size)

    def metadata(self, include_resources: bool = False) -> dict:
        result = {**self.source.origin, "directory_offset": self.directory_offset,
                  "types": {tag: len(values) for tag, values in sorted(self.resources.items())}}
        if include_resources:
            result["resources"] = [asdict(resource)
                                   for tag in sorted(self.resources)
                                   for resource in self.resources[tag].values()]
        return result
