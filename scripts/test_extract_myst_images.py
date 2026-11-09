#!/usr/bin/env python3
"""Synthetic tests for the optional resource-picture extractor; no game data."""
import importlib.util
import json
from pathlib import Path
import struct
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('extract_myst_images', Path(__file__).with_name('extract_myst_images.py'))
extract = importlib.util.module_from_spec(spec)
spec.loader.exec_module(extract)


def resource_fork():
    payload = b'original compressed bytes'
    name = 'Dock é'.encode('mac_roman')
    data = struct.pack('>I', len(payload))+payload
    do, mo = 256, 256+len(data)
    types = struct.pack('>H4sHH', 0, b'PICT', 0, 10)
    types += struct.pack('>hH', 1234, 0)+b'\0'*8
    names = bytes([len(name)])+name
    ml = 28+len(types)+len(names)
    header = struct.pack('>4I', do, mo, len(data), ml)
    resource_map = header+b'\0'*8+struct.pack('>HH', 28, 28+len(types))+types+names
    return header+b'\0'*(do-16)+data+resource_map, payload


class ResourceImagesTests(unittest.TestCase):
    def test_resource_identity_names_and_payload_bounds(self):
        fork, payload = resource_fork()
        entries = list(extract.resources(fork))
        self.assertEqual(len(entries), 1)
        self.assertEqual((entries[0]['type'], entries[0]['id'], entries[0]['name']), ('PICT', 1234, 'Dock é'))
        self.assertEqual(entries[0]['payload'], payload)
        self.assertEqual(entries[0]['offset'], 260)
        for length in (0, 8, 15, 16, 255, len(fork)-1):
            with self.subTest(length=length), self.assertRaises((ValueError, struct.error)):
                list(extract.resources(fork[:length]))
        malformed = bytearray(fork)
        malformed[8:12] = struct.pack('>I', 0xffffffff)
        with self.assertRaises(ValueError):
            list(extract.resources(malformed))

    def test_appledouble_only_and_rsrc_root_discovery(self):
        fork, _ = resource_fork()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)/'.rsrc'; root.mkdir()
            raw = root/'CHRes1'; raw.write_bytes(fork)
            apple = root/'._MYRes1'
            apple.write_bytes(struct.pack('>II', 0x00051607, 0x00020000)+b'\0'*16+struct.pack('>HIII', 1, 2, 38, len(fork))+fork)
            sources = {bank: path for path, bank in extract.bank_sources(root)}
            self.assertEqual(set(sources), {'CHRes1', 'MYRes1'})
            self.assertEqual(extract.fork_bytes(sources['CHRes1'])[0], fork)
            self.assertEqual(extract.fork_bytes(sources['MYRes1']), (fork, 'AppleDouble'))

    def test_macbinary_carrier(self):
        fork, _ = resource_fork()
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary)/'MYRes1.bin'
            header = bytearray(128); header[1] = 6; header[2:8] = b'MYRes1'
            header[83:91] = struct.pack('>II', 3, len(fork))
            path.write_bytes(header+b'abc'+b'\0'*125+fork)
            self.assertEqual(extract.fork_bytes(path), (fork, 'MacBinary'))

    def test_smc_wrapper_retains_original_packet_and_palette(self):
        description = bytearray(86); description[:8] = struct.pack('>I4s', 86, b'smc ')
        description[32:36] = struct.pack('>HH', 4, 5)
        packet = b'original packet'; description[44:48] = struct.pack('>I', len(packet))
        decoded = extract.smc_image(b'PICT prefix'+description+packet)
        self.assertEqual(decoded, (description, packet, 4, 5))
        movie = extract.smc_movie(*decoded)
        self.assertEqual(movie[4:8], b'mdat')
        self.assertEqual(movie[8:8+len(packet)], packet)
        with self.assertRaises(ValueError):
            extract.smc_image(description+packet+description+packet)

    def test_metadata_writes_do_not_follow_fixed_temporary_symlinks(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); source = root/'source'; source.mkdir()
            output = root/'output'; extract.prepare_output(output, source)
            original = source/'original'; original.write_bytes(b'unchanged')
            (output/'.images.json.tmp').symlink_to(original)
            extract.atomic_json(output, 'images.json', {'images': []})
            self.assertEqual(original.read_bytes(), b'unchanged')
            self.assertEqual(json.loads((output/'images.json').read_text()), {'images': []})
            (output/'images.json').unlink(); (output/'images.json').symlink_to(original)
            with self.assertRaises(ValueError):
                extract.atomic_json(output, 'images.json', {'images': []})
            self.assertEqual(original.read_bytes(), b'unchanged')

    def test_unowned_output_and_other_source_are_not_overwritten(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary); source = root/'source'; source.mkdir()
            output = root/'existing'; output.mkdir()
            original = output/'images.json'; original.write_bytes(b'user data')
            with self.assertRaises(ValueError):
                extract.prepare_output(output, source)
            self.assertEqual(original.read_bytes(), b'user data')
            managed = root/'managed'; extract.prepare_output(managed, source)
            with self.assertRaises(ValueError):
                extract.prepare_output(managed, root/'other')


if __name__ == '__main__':
    unittest.main()
