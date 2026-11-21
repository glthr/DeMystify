"""A little-endian byte reader with the string forms of Myst V's Plasma engine.

Safe strings (`readSafeStr` in libhsplasma's `core/Stream/hsStream.cpp`): for the "new Plasma" versions (Myst V and
Crowthistle), a u16 length then that many bytes, each XORed with the repeating key "mystnerd".
"""
import struct

EOA_STR_KEY = b'mystnerd'


class Reader:
    def __init__(self, data, pos=0):
        self.d = data
        self.p = pos

    def tell(self):
        return self.p

    def seek(self, pos):
        self.p = pos

    def skip(self, n):
        self.p += n

    def read(self, n):
        if self.p + n > len(self.d):
            raise EOFError(f'read of {n} bytes at {self.p} past the end ({len(self.d)})')
        b = self.d[self.p:self.p + n]
        self.p += n
        return b

    def _unpack(self, fmt, n):
        v = struct.unpack_from(fmt, self.d, self.p)
        self.p += n
        return v

    def u8(self):
        return self._unpack('<B', 1)[0]

    def u16(self):
        return self._unpack('<H', 2)[0]

    def i16(self):
        return self._unpack('<h', 2)[0]

    def u32(self):
        return self._unpack('<I', 4)[0]

    def i32(self):
        return self._unpack('<i', 4)[0]

    def f32(self):
        return self._unpack('<f', 4)[0]

    def floats(self, n):
        return list(self._unpack('<%df' % n, 4 * n))

    def safe_str(self):
        n = self.u16()
        if n == 0:
            return ''
        raw = self.read(n)
        return bytes(b ^ EOA_STR_KEY[i % 8] for i, b in enumerate(raw)).decode('latin-1')
