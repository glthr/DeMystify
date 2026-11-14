"""Handcrafted fixtures for the Riven image history and palette codec."""
import struct
import unittest
import zlib

from riven_graph.bitmap import BitmapError, decode_bitmap, png_bytes, unpack_riven


class BitmapTests(unittest.TestCase):
    def test_literal_and_repeated_pixel_groups(self):
        stream = bytes([2, 3, 4, 5, 6, 0x42, 0x81])
        self.assertEqual(unpack_riven(stream, 12), bytes([3, 4, 5, 6, 5, 6, 5, 6, 5, 6, 5, 6]))

    def test_duplet_delta_wraps_unsigned_bytes(self):
        self.assertEqual(unpack_riven(bytes([1, 254, 1, 0xc2, 0xa0, 0x32, 0xf0, 0x54]), 6),
                         bytes([254, 1, 1, 3, 252, 255]))

    def test_relative_history_can_overlap_new_pixels(self):
        # Subcommand a8 copies four pixels from two pixels behind, then fc
        # copies a variable-length run from that same moving history window.
        stream = bytes([1, 5, 9, 0xc2, 0xa8, 2, 0xfc, 4, 2])
        self.assertEqual(unpack_riven(stream, 10), bytes([5, 9] * 5))

    def test_pixel_relative_operations_use_updated_output_position(self):
        stream = bytes([2, 10, 20, 30, 40, 0xc2, 0x11, 0x41])
        self.assertEqual(unpack_riven(stream, 8), bytes([10, 20, 30, 40, 30, 30, 30, 30]))

    def test_every_arithmetic_instruction_family(self):
        # Each expected pair is evaluated against the moving output position,
        # which catches the easy error of sampling both values before writing.
        cases = [
            ([0x02], [10, 20]), ([0x10, 99], [30, 99]), ([0x13], [30, 30]),
            ([0x23], [30, 43]), ([0x33], [30, 37]), ([0x40, 99], [99, 40]),
            ([0x43], [20, 40]), ([0x50, 99, 77], [99, 77]), ([0x53, 99], [20, 99]),
            ([0x5b, 99], [99, 30]), ([0x63, 99], [99, 43]), ([0x73, 99], [99, 37]),
            ([0x83], [33, 40]), ([0x93, 99], [33, 99]), ([0xa0, 0x23], [32, 43]),
            ([0xb0, 0x23], [32, 37]), ([0xc3], [27, 40]), ([0xd3, 99], [27, 99]),
            ([0xe0, 0x23], [28, 43]), ([0xf0, 0x23], [28, 37]), ([0xff, 0x23], [28, 37]),
        ]
        for subcommand, pair in cases:
            with self.subTest(subcommand=subcommand):
                seed = [10, 20, 30, 40]
                self.assertEqual(unpack_riven(bytes([2, *seed, 0xc1, *subcommand]), 6), bytes(seed + pair))

    def test_rejects_truncation_output_overrun_and_invalid_history(self):
        for stream, size in [(b'\x01\x01', 2), (b'\x42', 4), (b'\x01\x01\x02\x42', 4),
                             (b'\x00', 2), (b'\xc1\x58', 2)]:
            with self.subTest(stream=stream):
                with self.assertRaises(BitmapError):
                    unpack_riven(stream, size)

    def test_palette_is_bgr_and_png_rows_drop_stride_padding(self):
        palette = bytes([11, 22, 33]) + bytes(765)
        raw = struct.pack('>4H', 1, 2, 2, 0xa) + bytes(4) + palette + bytes([0, 99, 0, 88])
        bitmap = decode_bitmap(raw)
        self.assertEqual(bitmap['pixels'], bytes([0, 0]))
        self.assertEqual(bitmap['palette'][:3], bytes([33, 22, 11]))
        png = png_bytes(bitmap)
        self.assertTrue(png.startswith(b'\x89PNG\r\n\x1a\n'))
        pos, compressed = 8, b''
        while pos < len(png):
            size = struct.unpack_from('>I', png, pos)[0]
            tag, body = png[pos + 4:pos + 8], png[pos + 8:pos + 8 + size]
            crc = struct.unpack_from('>I', png, pos + 8 + size)[0]
            self.assertEqual(crc, zlib.crc32(tag + body) & 0xffffffff)
            if tag == b'IDAT':
                compressed += body
            pos += size + 12
        self.assertEqual(zlib.decompress(compressed), b'\0\0\0\0')

    def test_raw_rgb_and_thumbnail_keep_expected_sample_positions(self):
        raw = struct.pack('>4H', 2, 1, 6, 4) + bytes([3, 2, 1, 6, 5, 4])
        bitmap = decode_bitmap(raw)
        self.assertEqual(bitmap['pixels'], bytes([1, 2, 3, 4, 5, 6]))
        png = png_bytes(bitmap, 1)
        self.assertEqual(struct.unpack_from('>II', png, 16), (1, 1))
        pos, packed = 8, b''
        while pos < len(png):
            size = struct.unpack_from('>I', png, pos)[0]
            if png[pos + 4:pos + 8] == b'IDAT':
                packed += png[pos + 8:pos + 8 + size]
            pos += size + 12
        self.assertEqual(zlib.decompress(packed), b'\0\x01\x02\x03')


if __name__ == '__main__':
    unittest.main()
