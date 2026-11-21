"""Open the Windows executable of Myst III: Exile and find its script tables.

The game keeps its ages, rooms, node scripts and hotspots in the data section
of ``M3.exe``, not in the archives on the discs. This module maps the PE
sections, undoes the SafeDisc 2 encryption of the retail executables and
locates the tables by their structure, so that an executable that is not in
the list of known versions can still be read.

The layout of the tables and the SafeDisc decoding follow the reader that the
ResidualVM project used before ScummVM moved the scripts to ``myst3.dat``
(engines/myst3/database.cpp at commit 9cc0513f6908508ca76141ba9e6181aa87c6b606).
"""
from __future__ import annotations

import hashlib
import struct
from dataclasses import dataclass, field

MASK = 0xFFFFFFFF

# MD5 of the first 5,000 bytes, as ScummVM's detection computes it.
KNOWN_VERSIONS = {
    'f8ab52da2815c1342eeb42b1bcad5441': ('1.0', 'English', 'four CDs'),
    '0f2b2703fc79be043007ab4e603d54b8': ('1.1', 'English', 'four CDs'),
    '314f4d2a3c9d22787719219419dcd480': ('1.2', 'English', 'four CDs'),
    '7a4eaea5f19f621e05e732d657c15fe3': ('1.2', 'English', 'four CDs'),
    '6dbe2eed529684c7fb64f0801462b89c': ('1.21', 'English', 'four CDs'),
    'c8ca22d347f947cd52d95229d0f6a3f3': ('1.22', 'English', 'four CDs'),
    '9d76f2f836ef9382e45f354a532f042d': ('1.22', 'English', 'four CDs'),
    '708da0c48c8972025f165df59d823a6b': ('1.27', 'English', 'DVD'),
    'b6c35e49d5a416cf4aa34f6439e8d544': ('1.27', 'French', 'DVD'),
    '819cdb78736fbdef9c9372c1050585d7': ('1.27', 'Dutch', 'DVD'),
    'ec01ddbd45b4f7879571be7ac4f4a54d': ('1.27', 'German', 'DVD'),
    'e5e0ca7a6703c3aee5438f9602214ac9': ('1.27', 'Italian', 'DVD'),
    '1c30c1d2e06bab8dcd483f5c49ef1a65': ('1.27', 'Spanish', 'DVD'),
}

# TEA keys of the SafeDisc-wrapped executables, by game version.
SAFEDISC_KEYS = {
    '1.0': (0xE3A2EB02, 0x190158D2, 0xD5E10A26, 0x6DF705D2),
    '1.1': (0x8D792101, 0x42AF4572, 0x5581B0CE, 0x984525B4),
    '1.22': (0x9D15D040, 0x10A5F8C7, 0x3A37C5B2, 0x95516FC0),
}
SAFEDISC_VERSIONS = {(2, 10, 30), (2, 30, 31), (2, 40, 10)}
ROOM_SIGNATURE = struct.pack('<I', 101) + b'XXXX\0\0\0\0'


class ExecutableError(ValueError):
    pass


def _rol(value, bits):
    bits %= 32
    return ((value << bits) | (value >> (32 - bits))) & MASK if bits else value


def _ror(value, bits):
    bits %= 32
    return ((value >> bits) | (value << (32 - bits))) & MASK if bits else value


def _decode1(d):
    d = (d + 0x15770916) & MASK
    d ^= 0x13932106
    d = _rol(d, 0xE2)
    d = (d - 0x407A1EF5) & MASK
    d ^= 0x33784C64
    d = (d - 0x7BA359F3) & MASK
    d = _ror(d, 0xB1)
    d = (d + 0x0F5123F) & MASK
    d = (d + 0x50C52C7) & MASK
    d = (d - 0x29256E14) & MASK
    d ^= 0x64B95579
    d = _rol(d, 0xC7)
    d = (d - 0x4CF5171E) & MASK
    d ^= 0x5A4D4CF1
    d = _ror(d, 0x73)
    d = _rol(d, 0xE3)
    d = (d + 0x2D415B9B) & MASK
    d = _rol(d, 0x04)
    d = _rol(d, 0xC6)
    d = _rol(d, 0xD6)
    d = (d + 0x56A97DBD) & MASK
    d = (d + 0x714738B4) & MASK
    return d


def _decode2(d):
    d = _ror(d, 0xD2)
    d = _rol(d, 0xF3)
    d = _ror(d, 0x25)
    d = _rol(d, 0xDE)
    d = (d + 1) & MASK
    d = _rol(d, 0xFF)
    d = _ror(d, 0x6B)
    d = (d - 1) & MASK
    d = _ror(d, 0x03)
    d = (d + 1) & MASK
    d = (d + 0x3824412A) & MASK
    d = (d - 0x5C8137F7) & MASK
    d = (d + 1) & MASK
    d = (d - 0x13932106) & MASK
    d = (d - 2) & MASK
    d = (d + 0x4C645056) & MASK
    d = _rol(d, 0xCC)
    d = _rol(d, 0xB2)
    d = (d - 2) & MASK
    d = _ror(d, 0x14)
    d = (-d) & MASK
    d = _rol(d, 0xC7)
    d = (d - 0x4CF5171E) & MASK
    d = (d - 1) & MASK
    d = _rol(d, 0x73)
    d = (d + 3) & MASK
    d = (d + 0x42C270B1) & MASK
    d = (d + 0x49A602E5) & MASK
    d = _rol(d, 0xA9)
    d = (d - 0x714738B4) & MASK
    return d


def _decrypt_block(words, key):
    """Decode one 4,096-byte SafeDisc block held as 1,024 little-endian words."""
    for step in (_decode1, _decode2):
        for i in range(len(words)):
            # The index is not reduced to a byte: above 255 its copies overlap.
            words[i] = step(words[i] ^ ((i << 24 | i << 16 | i << 8 | i) & MASK))
    magic = 0x9E3779B9
    k0, k1, k2, k3 = key
    for j in range(0, len(words), 2):
        d1, d2 = words[j], words[j + 1]
        total = (32 * magic) & MASK
        for _ in range(32):
            d2 = (d2 - (((k3 + (d1 >> 5)) & MASK) ^ ((total + d1) & MASK) ^ ((k2 + (d1 << 4)) & MASK))) & MASK
            d1 = (d1 - (((k1 + (d2 >> 5)) & MASK) ^ ((total + d2) & MASK) ^ ((k0 + (d2 << 4)) & MASK))) & MASK
            total = (total - magic) & MASK
        words[j], words[j + 1] = d1, d2


@dataclass
class Section:
    name: str
    virtual_address: int
    virtual_size: int
    raw_offset: int
    raw_size: int


@dataclass
class Executable:
    """A decoded executable and the addresses of its script tables."""
    data: bytes
    image_base: int
    sections: list
    fingerprint: str
    version: str
    language: str
    edition: str
    safedisc: str = ''
    notes: list = field(default_factory=list)
    age_table: int = 0
    age_count: int = 0
    node_init_script: int = 0
    sound_names: int = 0

    def offset(self, address):
        """File offset of a virtual address."""
        relative = address - self.image_base
        for section in self.sections:
            if section.virtual_address <= relative < section.virtual_address + max(section.raw_size, section.virtual_size):
                position = relative - section.virtual_address + section.raw_offset
                if position < len(self.data):
                    return position
        raise ExecutableError(f'address {address:#x} is outside the executable')

    def address(self, offset):
        """Virtual address of a file offset."""
        for section in self.sections:
            if section.raw_offset <= offset < section.raw_offset + section.raw_size:
                return offset - section.raw_offset + section.virtual_address + self.image_base
        raise ExecutableError(f'offset {offset:#x} is in no section')

    def u16(self, offset):
        return struct.unpack_from('<H', self.data, offset)[0]

    def s16(self, offset):
        return struct.unpack_from('<h', self.data, offset)[0]

    def u32(self, offset):
        return struct.unpack_from('<I', self.data, offset)[0]


def _sections(data):
    if data[:2] != b'MZ':
        raise ExecutableError('not a Windows executable')
    pe = struct.unpack_from('<I', data, 0x3C)[0]
    if data[pe:pe + 4] != b'PE\0\0':
        raise ExecutableError('not a PE executable')
    count = struct.unpack_from('<H', data, pe + 6)[0]
    optional = struct.unpack_from('<H', data, pe + 20)[0]
    image_base = struct.unpack_from('<I', data, pe + 24 + 28)[0]
    table = pe + 24 + optional
    sections = []
    for i in range(count):
        entry = table + i * 40
        name = data[entry:entry + 8].rstrip(b'\0').decode('latin1')
        virtual_size, virtual_address, raw_size, raw_offset = struct.unpack_from('<IIII', data, entry + 8)
        sections.append(Section(name, virtual_address, virtual_size, raw_offset, raw_size))
    return image_base, sections


def _safedisc_version(data, sections):
    text = next((s for s in sections if s.name == '.text'), None)
    if text is None or text.raw_offset < 12:
        return None
    version = struct.unpack_from('<III', data, text.raw_offset - 12)
    return version if version in SAFEDISC_VERSIONS else None


def _decrypt(data, sections, key):
    out = bytearray(data)
    for name in ('.text', '.data'):
        section = next((s for s in sections if s.name == name), None)
        if section is None:
            raise ExecutableError(f'SafeDisc executable without a {name} section')
        size = section.raw_size - section.raw_size % 0x1000
        for start in range(section.raw_offset, section.raw_offset + size, 0x1000):
            words = list(struct.unpack_from('<1024I', out, start))
            _decrypt_block(words, key)
            struct.pack_into('<1024I', out, start, *words)
    return bytes(out)


def _find_age_table(exe):
    """Find the age table: 20-byte records whose identifiers count up from 1."""
    data = exe.data
    position = data.find(struct.pack('<III', 1, 0, 1))
    while position >= 0:
        try:
            count = 0
            while exe.u32(position + count * 20) == count + 1 and 0 < exe.u32(position + count * 20 + 8) <= 16:
                exe.offset(exe.u32(position + count * 20 + 12))
                count += 1
            if count >= 10:
                first_room = exe.offset(exe.u32(exe.offset(exe.u32(position + 12))))
                if data[first_room:first_room + 12] == ROOM_SIGNATURE:
                    return position, count
        except (ExecutableError, struct.error):
            pass
        position = data.find(struct.pack('<III', 1, 0, 1), position + 4)
    raise ExecutableError('the age table was not found')


def load(data):
    """Decode an ``M3.exe`` image and locate its tables."""
    fingerprint = hashlib.md5(data[:5000]).hexdigest()
    version, language, edition = KNOWN_VERSIONS.get(fingerprint, ('unknown', 'unknown', 'unknown'))
    image_base, sections = _sections(data)
    safedisc, notes = '', []
    if ROOM_SIGNATURE not in data:
        found = _safedisc_version(data, sections)
        if found is None:
            raise ExecutableError('no room table, and no SafeDisc version this reader knows')
        safedisc = 'v%d.%d.%d' % found
        ordered = sorted(SAFEDISC_KEYS, key=lambda v: v != version)
        for candidate in ordered:
            decoded = _decrypt(data, sections, SAFEDISC_KEYS[candidate])
            if ROOM_SIGNATURE in decoded:
                data = decoded
                notes.append(f'SafeDisc {safedisc}: .text and .data decoded with the {candidate} key')
                break
        else:
            raise ExecutableError(f'SafeDisc {safedisc} executable, but none of the known keys decodes it')
    exe = Executable(data, image_base, sections, fingerprint, version, language, edition, safedisc, notes)
    exe.age_table, exe.age_count = _find_age_table(exe)
    first_pointers = min(exe.offset(exe.u32(exe.age_table + i * 20 + 12)) for i in range(exe.age_count))
    data_section = next(s for s in sections if s.raw_offset <= exe.age_table < s.raw_offset + s.raw_size)
    # In every Windows build examined, the script run before each node change
    # sits 0x40 bytes into the data section, just before the room pointers.
    exe.node_init_script = data_section.raw_offset + 0x40
    if not exe.node_init_script < first_pointers:
        exe.node_init_script = 0
        notes.append('the node initialisation script was not located')
    names = exe.age_table + exe.age_count * 20
    while exe.u32(names) == 0 and names < exe.age_table + exe.age_count * 20 + 16:
        names += 4
    exe.sound_names = names
    return exe
