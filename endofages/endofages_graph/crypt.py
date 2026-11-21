"""Myst V: End of Ages (2005) file encryption.

Every `.age`, `.fni`, `.sdl`, `.node`, `.loc`, `.sub` and the `python.pak` of the game begins with the magic
0x0D874288 (bytes 88 42 87 0D), the size of the plain data (u32, little-endian), then the data enciphered with AES-128
in ECB mode under a fixed key. The method and the key were published by the open-source library libhsplasma
(H-uru/libhsplasma, `core/Stream/plEncryptedStream.cpp`, `eoaKey`, `eoaMagic`), which this module follows. The
`.prp` pages are not enciphered.

AES runs through the system's `openssl` command (LibreSSL on macOS), so that nothing has to be installed.
"""
import functools
import struct
import subprocess

EOA_MAGIC = 0x0D874288
EOA_KEY = bytes([240, 77, 37, 51, 172, 93, 39, 90, 158, 24, 120, 62, 101, 44, 72, 8])


def is_encrypted(data):
    return len(data) >= 8 and struct.unpack_from('<I', data, 0)[0] == EOA_MAGIC


def decrypt(data):
    """The plain bytes of an enciphered Myst V file (bytes in, bytes out)."""
    if not is_encrypted(data):
        raise ValueError('not a Myst V enciphered file (magic 0x0D874288 missing)')
    size = struct.unpack_from('<I', data, 4)[0]
    body = data[8:]
    if len(body) % 16:
        raise ValueError('enciphered body is not a whole number of 16-byte blocks')
    out = subprocess.run(['openssl', 'enc', '-d', '-aes-128-ecb', '-nopad', '-K', EOA_KEY.hex()],
                         input=body, capture_output=True, check=True).stdout
    if len(out) < size:
        raise ValueError('decrypted data shorter than the size in the header')
    return out[:size]


@functools.lru_cache(maxsize=64)
def read_file(path):
    """The plain bytes of a file of the game: deciphered when it is enciphered, as it is otherwise."""
    with open(path, 'rb') as f:
        data = f.read()
    return decrypt(data) if is_encrypted(data) else data


def read_text(path, encoding='latin-1'):
    return read_file(path).decode(encoding)
