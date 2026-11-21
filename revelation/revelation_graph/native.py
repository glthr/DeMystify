"""Node changes written in the game's compiled AI libraries.

Two calls change node from compiled code: ``ai::ContextChange(world, zone, node)`` and the constructor of
``ai::Transition`` (four resources, then world, zone, node, a speed and a flag). Both are imported from
``m4_aisdk_rd.dll``. Where the destination is pushed as three constants, it is read from the disassembly
(``objdump -d``, part of the system's developer tools); nothing is run. Destinations that are computed are
counted and left alone.

A call is attributed to the class whose method contains it: the libraries keep the names of their classes
(run-time type information), and each class has a table of its methods. The nodes that use the class are
those whose archive holds a parameter file for it (``ai/<class>/``).
"""
from __future__ import annotations

import re
import shutil
import struct
import subprocess
import tempfile
from pathlib import Path


class Library:
    def __init__(self, name, data):
        self.name, self.data = name, data
        pe = struct.unpack_from('<I', data, 0x3c)[0]
        if data[pe:pe + 4] != b'PE\0\0':
            raise ValueError(f'{name}: not a PE file')
        sections = struct.unpack_from('<H', data, pe + 6)[0]
        optional = pe + 24
        size = struct.unpack_from('<H', data, pe + 20)[0]
        self.base = struct.unpack_from('<I', data, optional + 28)[0]
        directories = optional + 96
        self.export_rva = struct.unpack_from('<I', data, directories)[0]
        self.import_rva = struct.unpack_from('<I', data, directories + 8)[0]
        self.sections = []
        for i in range(sections):
            entry = optional + size + 40 * i
            title = data[entry:entry + 8].rstrip(b'\0').decode('latin-1')
            vsize, rva, rsize, raw = struct.unpack_from('<IIII', data, entry + 8)
            self.sections.append((title, rva, max(vsize, rsize), raw, rsize))

    def offset(self, rva):
        for _, start, size, raw, rsize in self.sections:
            if start <= rva < start + size and rva - start < rsize:
                return raw + rva - start
        raise ValueError(f'{self.name}: address {rva:#x} outside the stored sections')

    def text_at(self, address):
        """The zero-terminated string at a virtual address, or None."""
        try:
            start = self.offset(address - self.base)
        except ValueError:
            return None
        end = self.data.find(b'\0', start, start + 300)
        raw = self.data[start:end] if end > start else b''
        return raw.decode('latin-1') if raw and all(32 <= c < 127 for c in raw) else None

    def in_code(self, address):
        for title, start, size, _, _ in self.sections:
            if title == '.text' and start <= address - self.base < start + size:
                return True
        return False

    def imports(self):
        """{name: address of its import slot} for every function imported by name."""
        out, entry = {}, self.offset(self.import_rva)
        while True:
            lookup, _, _, name, thunk = struct.unpack_from('<IIIII', self.data, entry)
            if not name:
                return out
            table = self.offset(lookup or thunk)
            index = 0
            while True:
                value = struct.unpack_from('<I', self.data, table + 4 * index)[0]
                if not value:
                    break
                if not value & 0x80000000:
                    start = self.offset(value + 2)
                    out[self.data[start:self.data.index(b'\0', start)].decode('latin-1')] = self.base + thunk + 4 * index
                index += 1
            entry += 20

    def classes(self):
        """{method address: class name} from the run-time type information and the method tables."""
        methods = {}
        for title, rva, size, raw, rsize in self.sections:
            if title not in ('.rdata', '.data'):
                continue
        blob = self.data
        descriptors = {}
        for match in re.finditer(rb'\.\?AV([\w$]+)@((?:[\w$]+@)*)@\0', blob):
            for title, rva, size, raw, rsize in self.sections:
                if raw <= match.start() < raw + rsize:
                    descriptors[self.base + rva + match.start() - raw - 8] = match.group(1).decode()
        tables = {}
        for title, rva, size, raw, rsize in self.sections:
            if title != '.rdata':
                continue
            words = struct.unpack_from(f'<{rsize // 4}I', blob, raw)
            locators = {}
            for i in range(3, len(words)):
                if words[i] in descriptors and words[i - 3] == 0:
                    locators[self.base + rva + 4 * (i - 3)] = descriptors[words[i]]
            for i, word in enumerate(words):
                if word in locators:
                    j = i + 1
                    while j < len(words) and self.in_code(words[j]):
                        methods.setdefault(words[j], locators[word])
                        j += 1
        return methods


def disassemble(path):
    """[(address, mnemonic, operands)] of a library, from objdump."""
    tool = shutil.which('objdump') or shutil.which('llvm-objdump')
    if not tool:
        raise RuntimeError('objdump was not found: the node changes of the compiled libraries are left out')
    listing = subprocess.run([tool, '-d', '--no-show-raw-insn', str(path)], check=True, capture_output=True, text=True).stdout
    out = []
    for line in listing.splitlines():
        match = re.match(r'\s*([0-9a-f]+):\s+(\S+)\s*(.*)$', line)
        if match:
            out.append((int(match.group(1), 16), match.group(2), match.group(3).split('#')[0].strip()))
    return out


def constant(operands):
    match = re.fullmatch(r'\$(-?0x[0-9a-f]+|-?\d+)', operands)
    return int(match.group(1), 0) & 0xFFFFFFFF if match else None


def paths_back(code, index_of, jumps_to, start, wanted, limit=60):
    """Every sequence of ``wanted`` constants pushed on the ways that lead to instruction ``start`` (last push first).

    Only pushes of constants and jumps are walked through; a way that meets anything else before the constants are
    complete is dropped (the destination is computed there).
    """
    found, computed = set(), False
    stack = [(start, ())]
    seen = set()
    while stack:
        position, values = stack.pop()
        if len(values) == wanted:
            found.add(values)
            continue
        if (position, values) in seen or len(seen) > limit * 50:
            continue
        seen.add((position, values))
        previous = []
        if position > 0 and code[position - 1][1] not in ('jmp', 'retl', 'ret', 'int3'):
            previous.append(position - 1)
        previous.extend(jumps_to.get(code[position][0], []))
        for candidate in previous:
            _, mnemonic, operands = code[candidate]
            if mnemonic == 'pushl' and constant(operands) is not None:
                stack.append((candidate, values + (constant(operands),)))
            elif mnemonic.startswith('j'):
                stack.append((candidate, values))
            else:
                computed = True
    return found, computed


def context_changes(name, data):
    """Node changes of one library: {'changes': [...], 'computed': n}."""
    library = Library(name, data)
    imports = library.imports()
    change_slots = {slot: imported for imported, slot in imports.items() if imported.startswith('?ContextChange')}
    transition_slots = {slot for imported, slot in imports.items() if imported.startswith('??0Transition@ai@@')}
    with tempfile.TemporaryDirectory(prefix='revelation-') as folder:
        path = Path(folder) / name
        path.write_bytes(data)
        code = disassemble(path)
    index_of = {address: i for i, (address, _, _) in enumerate(code)}
    jumps_to, starts = {}, set(library.classes())
    for i, (address, mnemonic, operands) in enumerate(code):
        match = re.match(r'0x([0-9a-f]+)', operands)
        if match and mnemonic.startswith('j'):
            jumps_to.setdefault(int(match.group(1), 16), []).append(i)
        elif match and mnemonic == 'calll':
            starts.add(int(match.group(1), 16))
    methods = library.classes()
    ordered = sorted(starts)

    def owner(address):
        import bisect
        position = bisect.bisect_right(ordered, address) - 1
        start = ordered[position] if position >= 0 else None
        strings = []
        if start is not None:
            for a, mnemonic, operands in code[index_of.get(start, 0):index_of[address] + 1]:
                value = constant(operands) if mnemonic == 'pushl' else None
                text = library.text_at(value) if value and value > library.base else None
                if text and re.match(r'w\d_z\d\d_n\d\d\d', text, re.I):
                    strings.append(text)
        return methods.get(start, ''), start, strings

    changes, computed = [], 0
    for i, (address, mnemonic, operands) in enumerate(code):
        if mnemonic != 'calll':
            continue
        match = re.fullmatch(r'\*0x([0-9a-f]+)', operands)
        slot = int(match.group(1), 16) if match else None
        destinations, kind = set(), None
        if slot in change_slots:
            kind = 'ContextChange'
            found, partial = paths_back(code, index_of, jumps_to, i, 3)
            destinations = {(v[0], v[1], v[2]) for v in found}
            if not destinations:
                computed += 1
        elif slot in transition_slots:
            kind = 'Transition'
            # The constants come first: flag, speed, node, zone, world, then room for the four resources.
            for j in range(i - 1, max(i - 80, 4), -1):
                if code[j][1] == 'subl' and code[j][2].startswith('$0x40') and all(
                        code[j - k][1] == 'pushl' and constant(code[j - k][2]) is not None for k in range(1, 6)):
                    destinations = {(constant(code[j - 1][2]), constant(code[j - 2][2]), constant(code[j - 3][2]))}
                    break
            if not destinations:
                computed += 1
        for world, zone, node in sorted(destinations):
            if not (0 < world < 10 and zone < 100 and node < 1000):
                computed += 1
                continue
            cls, start, strings = owner(address)
            changes.append({'library': name, 'address': address, 'call': kind, 'world': world, 'zone': zone, 'node': node,
                            'class': cls, 'function': start, 'resources': sorted(set(strings))[:6]})
    return {'changes': changes, 'computed': computed}
