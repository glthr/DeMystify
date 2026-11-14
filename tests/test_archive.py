"""Container boundaries, Mac resource forks and the supplied CD edition."""

from pathlib import Path
import os
import struct
import tempfile
import unittest

from riven_graph.archive import MohawkArchive
from riven_graph.iso import FileSlice, FormatError, ISO9660, discover_mhk


def both16(value):
    return struct.pack("<H", value) + struct.pack(">H", value)


def both32(value):
    return struct.pack("<I", value) + struct.pack(">I", value)


def record(name, extent, size, flags=0):
    length = 33 + len(name) + (len(name) % 2 == 0)
    data = bytearray(length)
    data[0] = length
    data[2:10] = both32(extent)
    data[10:18] = both32(size)
    data[25] = flags
    data[28:32] = both16(1)
    data[32] = len(name)
    data[33:33 + len(name)] = name
    return bytes(data)


def synthetic_iso():
    data = bytearray(24 * 2048)
    primary = memoryview(data)[16 * 2048:17 * 2048]
    primary[:7] = b"\x01CD001\x01"
    primary[40:72] = b"RIVEN_TEST".ljust(32)
    primary[128:132] = both16(2048)
    primary[156:190] = record(b"\0", 20, 2048, 2)
    data[17 * 2048:17 * 2048 + 7] = b"\xffCD001\x01"
    directory = b"".join((record(b"\0", 20, 2048, 2),
                          record(b"\1", 20, 2048, 2),
                          record(b"A_DATA.MHK;1", 21, 4, 4),
                          record(b"A_DATA.MHK;1", 22, 4)))
    data[20 * 2048:20 * 2048 + len(directory)] = directory
    data[21 * 2048:21 * 2048 + 4] = b"FORK"
    data[22 * 2048:22 * 2048 + 4] = b"MHWK"
    return data


def synthetic_mohawk(tag=b"CARD", stored_size=3):
    data = bytearray(83)
    data[:4] = b"MHWK"
    struct.pack_into(">I", data, 4, len(data))
    data[8:12] = b"RSRC"
    struct.pack_into(">HHIIHH", data, 12, 0x100, 0, 55, 28, 32, 14)
    # Directory, one type, one resource, one name.
    struct.pack_into(">HH4sHH", data, 28, 24, 1, tag, 12, 18)
    struct.pack_into(">HHH", data, 40, 1, 9, 1)
    struct.pack_into(">HHH", data, 46, 1, 0, 1)
    data[52:58] = b"entry\0"
    struct.pack_into(">I", data, 60, 1)
    struct.pack_into(">IHBBH", data, 64, 80, stored_size, 0, 0, 0)
    data[80:] = b"abc"
    return data


class ContainerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def write(self, name, content):
        path = self.root / name
        path.write_bytes(content)
        return path

    def test_slice_preserves_absolute_extent_and_enforces_boundary(self):
        path = self.write("container.bin", b"prefixPAYLOADsuffix")
        source = FileSlice(path, 6, 7, "payload")
        self.assertEqual(source.read(), b"PAYLOAD")
        self.assertEqual(source.read(2, 3), b"YLO")
        self.assertEqual(source.origin["offset"], 6)
        for offset, size in ((-1, 1), (0, 8), (7, 1), (1, -1)):
            with self.subTest(offset=offset, size=size), self.assertRaises(FormatError):
                source.read(offset, size)
        with self.assertRaises(FormatError):
            FileSlice(path, path.stat().st_size, 1, "outside")

    def test_iso_uses_data_fork_and_returns_exact_extent(self):
        path = self.write("disc.iso", synthetic_iso())
        image = ISO9660(path)
        self.assertEqual(image.volume_id, "RIVEN_TEST")
        sources = list(image.files())
        self.assertEqual(len(sources), 1)
        self.assertEqual(sources[0].name, "A_DATA.MHK")
        self.assertEqual(sources[0].offset, 22 * 2048)
        self.assertEqual(sources[0].read(), b"MHWK")
        self.assertEqual(discover_mhk([self.root]), sources)

    def test_iso_rejects_extent_outside_container(self):
        data = synthetic_iso()
        data[16 * 2048 + 156 + 2:16 * 2048 + 156 + 10] = both32(24)
        with self.assertRaisesRegex(FormatError, "extent exceeds"):
            ISO9660(self.write("outside.iso", data))

    def test_iso_rejects_disagreeing_dual_endian_metadata(self):
        data = synthetic_iso()
        data[16 * 2048 + 130:16 * 2048 + 132] = struct.pack(">H", 1024)
        with self.assertRaisesRegex(FormatError, "dual-endian"):
            ISO9660(self.write("endian.iso", data))

    def test_mohawk_resource_and_name_are_bound_to_embedded_archive(self):
        data = synthetic_mohawk()
        path = self.write("embedded.bin", b"PREFIX" + data + b"SUFFIX")
        archive = MohawkArchive(FileSlice(path, 6, len(data), "A_DATA.MHK"))
        self.assertEqual(archive.ids("CARD"), [9])
        self.assertEqual(archive.read("CARD", 9), b"abc")
        resource = archive.resources["CARD"][9]
        self.assertEqual((resource.offset, resource.size, resource.name), (80, 3, "entry"))

    def test_mohawk_rejects_invalid_index(self):
        data = synthetic_mohawk()
        struct.pack_into(">H", data, 44, 2)
        with self.assertRaisesRegex(FormatError, "Invalid Mohawk file index"):
            MohawkArchive(self.write("bad-index.mhk", data))

    def test_mohawk_rejects_resource_crossing_end(self):
        data = synthetic_mohawk(stored_size=4)
        with self.assertRaisesRegex(FormatError, "Resource outside archive"):
            MohawkArchive(self.write("bad-size.mhk", data))

    def test_mohawk_rejects_directory_outside_archive(self):
        data = synthetic_mohawk()
        struct.pack_into(">I", data, 20, 1000)
        with self.assertRaises(FormatError):
            MohawkArchive(self.write("bad-directory.mhk", data))

    def test_mohawk_movie_uses_extent_instead_of_unreliable_stored_size(self):
        archive = MohawkArchive(self.write("movie.mhk", synthetic_mohawk(b"tMOV", 0)))
        self.assertEqual(archive.read("tMOV", 9), b"abc")
        self.assertEqual(archive.resources["tMOV"][9].stored_size, 0)


class SuppliedDiscTests(unittest.TestCase):
    @unittest.skipUnless(Path(os.environ.get("RIVEN_TEST_DATA", Path(__file__).resolve().parents[1] / "riven-the-sequel-to-myst_202302")).is_dir(),
                         "Supplied original CD images are not installed")
    def test_all_graph_metadata_on_five_supplied_discs(self):
        root = Path(os.environ.get("RIVEN_TEST_DATA", Path(__file__).resolve().parents[1] / "riven-the-sequel-to-myst_202302"))
        sources = discover_mhk([root])
        self.assertEqual(len(sources), 28)
        cards = {}
        for source in sources:
            archive = MohawkArchive(source)
            if "DATA" in source.name:
                stack = source.name.rsplit("/", 1)[-1][0]
                cards.setdefault(stack, set()).update(archive.ids("CARD"))
            for tag in ("CARD", "HSPT", "NAME", "RMAP", "PLST"):
                for resource_id in archive.ids(tag):
                    resource = archive.resources[tag][resource_id]
                    self.assertEqual(len(archive.read(tag, resource_id)), resource.size)
        self.assertEqual({key: len(value) for key, value in cards.items()},
                         {"A": 8, "B": 498, "J": 814, "T": 547,
                          "G": 267, "O": 89, "P": 47, "R": 37})


if __name__ == "__main__":
    unittest.main()
