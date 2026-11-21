"""Myst V's `Python/python.pak`: the compiled scripts of the game.

The file is enciphered like the others (see `crypt.py`). Inside: a u32 count, then for each script a name (u16
length, bytes XORed with "mystnerd", as Myst V's safe strings) and a u32 offset; at each offset, a u32 size and a
module code object in the `marshal` format of Python 2.3 (CPython's `Python/marshal.c` of that version: type codes
c, s, t, R, u, (, [, {, i, l, f, x, N, F, T, S, .). This reader loads the code objects as nested dictionaries so that
their constants (strings, numbers) and names can be searched; it does not run or decompile anything.
"""
import struct

from . import crypt
from .stream import EOA_STR_KEY


class Marshal:
    def __init__(self, data):
        self.d = data
        self.p = 0
        self.interned = []

    def i32(self):
        v = struct.unpack_from('<i', self.d, self.p)[0]
        self.p += 4
        return v

    def raw(self, n):
        b = self.d[self.p:self.p + n]
        self.p += n
        return b

    def load(self):
        t = chr(self.d[self.p])
        self.p += 1
        if t == 'c':
            co = {'argcount': self.i32(), 'nlocals': self.i32(), 'stacksize': self.i32(), 'flags': self.i32()}
            co['code'] = self.load()
            co['consts'] = self.load()
            co['names'] = self.load()
            co['varnames'] = self.load()
            co['freevars'] = self.load()
            co['cellvars'] = self.load()
            co['filename'] = self.load()
            co['name'] = self.load()
            co['firstlineno'] = self.i32()
            co['lnotab'] = self.load()
            return co
        if t in 'st':
            n = self.i32()
            s = self.raw(n)
            if t == 't':
                self.interned.append(s)
            return s
        if t == 'R':
            return self.interned[self.i32()]
        if t == 'u':
            return self.raw(self.i32()).decode('utf-8', 'replace')
        if t in '([':
            return tuple(self.load() for _ in range(self.i32()))
        if t == '{':
            d = {}
            while chr(self.d[self.p]) != '0':
                k = self.load()
                d[k] = self.load()
            self.p += 1
            return d
        if t == 'i':
            return self.i32()
        if t == 'I':
            v = struct.unpack_from('<q', self.d, self.p)[0]
            self.p += 8
            return v
        if t == 'l':
            n = self.i32()
            digits = [struct.unpack_from('<H', self.d, self.p + 2 * k)[0] for k in range(abs(n))]
            self.p += 2 * abs(n)
            v = sum(dg << (15 * k) for k, dg in enumerate(digits))
            return -v if n < 0 else v
        if t == 'f':
            n = self.d[self.p]
            self.p += 1
            return float(self.raw(n))
        if t == 'x':
            n = self.d[self.p]
            self.p += 1
            re_ = float(self.raw(n))
            n = self.d[self.p]
            self.p += 1
            return complex(re_, float(self.raw(n)))
        if t == 'N':
            return None
        if t == 'F':
            return False
        if t == 'T':
            return True
        if t == 'S':
            return StopIteration
        if t == '.':
            return Ellipsis
        raise ValueError(f'unknown marshal type {t!r} at {self.p - 1}')


def read_pak(path):
    """{script name: module code object} for every script of the archive."""
    d = crypt.read_file(path)
    n = struct.unpack_from('<I', d, 0)[0]
    p = 4
    entries = []
    for _ in range(n):
        ln = struct.unpack_from('<H', d, p)[0]
        p += 2
        name = bytes(b ^ EOA_STR_KEY[i % 8] for i, b in enumerate(d[p:p + ln])).decode('latin-1')
        p += ln
        entries.append((name, struct.unpack_from('<I', d, p)[0]))
        p += 4
    out = {}
    for name, off in entries:
        size = struct.unpack_from('<I', d, off)[0]
        out[name] = Marshal(d[off + 4:off + 4 + size]).load()
    return out


def walk(co, path=''):
    """Every code object of a module, with its dotted path."""
    here = f'{path}.{co["name"].decode("latin-1")}' if path else co['name'].decode('latin-1')
    yield here, co
    for c in co['consts']:
        if isinstance(c, dict) and 'code' in c:
            yield from walk(c, here)


def strings(co):
    """(function path, kind, text): the string constants and the names of every code object of a module."""
    for fpath, c in walk(co):
        for k in c['consts']:
            if isinstance(k, bytes):
                yield fpath, 'const', k.decode('latin-1')
            elif isinstance(k, str):
                yield fpath, 'const', k
            elif isinstance(k, tuple):
                for x in k:
                    if isinstance(x, bytes):
                        yield fpath, 'const', x.decode('latin-1')
        for k in c['names']:
            yield fpath, 'name', k.decode('latin-1') if isinstance(k, bytes) else str(k)
