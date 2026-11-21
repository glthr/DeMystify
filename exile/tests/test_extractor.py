"""Tests on invented data: no game file is needed."""
import struct
import tempfile
import unittest
from pathlib import Path

from exile_graph import archive, database, disc, executable, scripts, web_export
from exile_graph.names import OPCODES
from exile_graph.walker import Walker

OP = {name: number for number, (name, _) in OPCODES.items()}


def op(name, *args):
    return struct.pack('<H', OP[name] | len(args) << 8) + b''.join(struct.pack('<h', a) for a in args)


def words(*values):
    return b''.join(struct.pack('<h', v) for v in values)


END = words(0)


class FakeExecutable:
    """Just enough of an executable for the script reader."""
    version = '1.2'

    def __init__(self, data):
        self.data = data

    def u16(self, offset):
        return struct.unpack_from('<H', self.data, offset)[0]

    def s16(self, offset):
        return struct.unpack_from('<h', self.data, offset)[0]


def room_from(blob, name='ROOM'):
    reader = database.Reader(FakeExecutable(blob))
    room = database.Room(1, name, 1, 0, 0, 0, 0, 0, 0)
    database._read_blocks(reader, room, 'node', 0)
    return room


def hotspot(condition, rect, cursor, body):
    return words(condition) + words(*rect) + words(cursor) + body + END


class ConditionTest(unittest.TestCase):
    def test_truth_and_value_tests(self):
        self.assertEqual(scripts.condition(1), 'always')
        self.assertEqual(scripts.condition(400), 'v400 != 0')
        self.assertEqual(scripts.condition(-400), 'v400 == 0')
        # A value is stored plus one above the eleven bits of the variable.
        self.assertEqual(scripts.condition((3 + 1) << 11 | 400), 'v400 == 3')
        self.assertEqual(scripts.condition(-((0 + 1) << 11 | 400)), 'v400 != 0')


class DatabaseTest(unittest.TestCase):
    def test_nodes_sharing_one_block(self):
        blob = words(-2, 5, 6) + words(1) + op('nodeCubeInit', 5) + END + END + END + END
        room = room_from(blob)
        self.assertEqual(sorted(room.nodes), [5, 6])
        self.assertIs(room.nodes[5].scripts[0], room.nodes[6].scripts[0])

    def test_ranges_are_inclusive(self):
        blob = words(-10, -3, 5, 9, 0) + END + END + END
        self.assertEqual(sorted(room_from(blob).nodes), [3, 4, 5, 9])

    def test_hotspot_with_two_zones(self):
        body = op('goToNodeTrans1', 2)
        spot = words(1) + words(10, 20, -30, 40) + words(11, 21, 31, 41) + words(7) + body + END
        room = room_from(words(1) + END + spot + END + END)
        stored = room.nodes[1].hotspots[0]
        self.assertEqual(stored.rects, [(10, 20, 30, 40), (11, 21, 31, 41)])
        self.assertEqual(stored.cursor, 7)

    def test_version_1_0_opcode_numbers_are_shifted(self):
        blob = words(1) + words(1) + struct.pack('<H', 136 | 1 << 8) + words(4) + END + END + END + END
        reader_exe = FakeExecutable(blob)
        reader_exe.version = '1.0'
        reader = database.Reader(reader_exe)
        room = database.Room(1, 'ROOM', 1, 0, 0, 0, 0, 0, 0)
        database._read_blocks(reader, room, 'node', 0)
        stored = room.nodes[1].scripts[0].ops[0]
        self.assertEqual((stored.raw_op, scripts.op_name(stored.op)), (136, 'goToNodeTrans2'))


class WalkerTest(unittest.TestCase):
    def walk(self, blob, node=1):
        room = room_from(blob)

        class Db:
            rooms = {'ROOM': room, 'XXXX': database.Room(101, 'XXXX', 1, 0, 0, 0, 0, 0, 0)}
        room.id = 501
        return Walker(Db).walk_node(room.nodes[node])

    def test_hotspot_reference(self):
        blob = words(1) + END + hotspot(1, (0, 0, 10, 10), 0, op('goToNodeTrans1', 2)) + END + END
        references = self.walk(blob).references
        self.assertEqual([r['target'] for r in references], [('ROOM', 2)])
        self.assertFalse(references[0]['guards'])

    def test_helper_script_is_followed(self):
        helper = words(1000) + words(1) + op('goToNodeTrans1', 9) + END + END + END
        blob = words(1) + END + hotspot(1, (0, 0, 10, 10), 0, op('runScript', 1000)) + END + helper + END
        reference = self.walk(blob).references[0]
        self.assertEqual(reference['target'], ('ROOM', 9))
        self.assertEqual(reference['via'], ['ROOM 1000'])

    def test_else_branch_is_guarded(self):
        body = op('ifCondition', 400) + op('goToNodeTrans1', 2) + op('ifElse') + op('goToNodeTrans1', 3)
        blob = words(1) + END + hotspot(1, (0, 0, 10, 10), 0, body) + END + END
        first, second = self.walk(blob).references
        self.assertEqual((first['target'], second['target']), (('ROOM', 2), ('ROOM', 3)))
        self.assertTrue(first['guards'] and second['guards'])
        self.assertFalse(second['unreachable'])

    def test_code_after_an_unconditional_else_is_unreachable(self):
        body = op('goToNodeTrans1', 2) + op('ifElse') + op('goToNodeTrans1', 3)
        blob = words(1) + END + hotspot(1, (0, 0, 10, 10), 0, body) + END + END
        walk = self.walk(blob)
        self.assertEqual([r['unreachable'] for r in walk.references], [False, True])
        self.assertEqual(len(walk.dead_code), 1)

    def test_next_location_variables(self):
        body = op('varSetValue', 190, 501) + op('varSetValue', 189, 7) + op('goToNodeTrans1', 0)
        blob = words(1) + END + hotspot(1, (0, 0, 10, 10), 0, body) + END + END
        self.assertEqual(self.walk(blob).references[0]['target'], ('ROOM', 7))

    def test_saved_return_is_dynamic(self):
        body = op('varCopy', 66, 189) + op('goToNodeTrans1', 0)
        blob = words(1) + END + hotspot(1, (0, 0, 10, 10), 0, body) + END + END
        reference = self.walk(blob).references[0]
        self.assertIsNone(reference['target'])
        self.assertEqual(reference['dynamic']['node'], 'vBookSavedNode')

    def test_negative_node_names_a_variable(self):
        blob = words(1) + END + hotspot(1, (0, 0, 10, 10), 0, op('goToNodeTrans1', -32)) + END + END
        self.assertEqual(self.walk(blob).references[0]['dynamic']['node'], 'v32')

    def test_movie_then_reload_is_not_a_reference(self):
        blob = words(1) + END + hotspot(1, (0, 0, 10, 10), 0, op('moviePlayChangeNode', 0, 10001)) + END + END
        walk = self.walk(blob)
        self.assertEqual((len(walk.references), len(walk.reloads)), (0, 1))

    def test_view_is_recorded(self):
        blob = words(1) + words(1) + op('nodeCubeInit', 1) + END + END + END + END
        self.assertEqual(self.walk(blob).views[0]['indexes'], [1])


class ArchiveTest(unittest.TestCase):
    class Ref:
        name = 'TESTnodes.m3a'

        def __init__(self, data):
            self.data, self.size = data, len(data)

        def read(self, offset=0, size=None):
            return self.data[offset:offset + (self.size - offset if size is None else size)]

    def directory(self, encrypted):
        payload = b'\xff\xd8JPEG'
        entry = struct.pack('<I', 12 | 1 << 24) + struct.pack('<IIHBB', 64, len(payload), 1, 3, 0) + struct.pack('<I', 99)
        plain = struct.pack('<I', 1 + len(entry) // 4) + entry
        values = list(struct.unpack(f'<{len(plain) // 4}I', plain))
        if encrypted:
            key = 0
            for i in range(len(values)):
                key = (key + archive.ADD_KEY) & archive.MASK
                values[i] ^= key
                key = (key * archive.MULT_KEY) & archive.MASK
        data = struct.pack(f'<{len(values)}I', *values)
        return data.ljust(64, b'\0') + payload

    def test_plain_and_encrypted_directories(self):
        for encrypted in (False, True):
            opened = archive.Archive(self.Ref(self.directory(encrypted)), 'TEST')
            self.assertEqual(opened.encrypted, encrypted)
            entry = opened.get('TEST', 12)
            resource = entry.find(archive.CUBE_FACE, 3)[0]
            self.assertEqual((resource.metadata, opened.read(resource)), ([99], b'\xff\xd8JPEG'))


class DiscTest(unittest.TestCase):
    def image(self, raw):
        def record(name, lba, size, directory=False):
            body = bytearray(33 + len(name) + (1 - len(name) % 2))
            body[0] = len(body)
            struct.pack_into('<I', body, 2, lba)
            struct.pack_into('<I', body, 10, size)
            body[25] = 2 if directory else 0
            body[32] = len(name)
            body[33:33 + len(name)] = name
            return bytes(body)
        content = b'node archive bytes'
        root = record(b'\0', 20, 2048, True) + record(b'\1', 20, 2048, True) + record(b'LEISNODE.M3A;1', 21, len(content))
        sectors = [b''] * 22
        volume = bytearray(2048)
        volume[0:6] = b'\x01CD001'
        volume[40:72] = b'EXILE_TEST'.ljust(32)
        volume[156:156 + 34] = record(b'\0', 20, 2048, True)
        sectors[16], sectors[17] = bytes(volume), b'\xffCD001'.ljust(2048, b'\0')
        sectors[20], sectors[21] = root, content
        out = b''
        for index, data in enumerate(sectors):
            data = data.ljust(2048, b'\0')
            out += (disc.SYNC + bytes([0, 2, index, 1]) + data + bytes(288)) if raw else data
        return out, content

    def test_cooked_and_raw_images(self):
        for raw in (False, True):
            data, content = self.image(raw)
            with tempfile.TemporaryDirectory() as folder:
                path = Path(folder) / ('disc.img' if raw else 'disc.iso')
                path.write_bytes(data + bytes(1 << 20))
                source = disc.Source([folder])
                try:
                    self.assertEqual(source.images[0].sector_size, 2352 if raw else 2048)
                    self.assertEqual(source.images[0].volume, 'EXILE_TEST')
                    ref = source.node_archives()['LEIS'][0]
                    self.assertEqual(ref.read(), content)
                    self.assertEqual(ref.read(5, 7), content[5:12])
                finally:
                    source.close()


class SafeDiscTest(unittest.TestCase):
    def test_block_cipher_is_deterministic_and_stays_in_32_bits(self):
        # The decoding itself is checked against the real executable in
        # test_discs.py; this only guards the arithmetic against overflow.
        block = [0] * 1024
        executable._decrypt_block(block, (0, 0, 0, 0))
        again = [0] * 1024
        executable._decrypt_block(again, (0, 0, 0, 0))
        self.assertEqual(block, again)
        self.assertNotEqual(block[:2], [0, 0])
        self.assertTrue(all(0 <= value <= 0xFFFFFFFF for value in block))

    def test_rotations(self):
        self.assertEqual(executable._rol(0x80000001, 1), 0x00000003)
        self.assertEqual(executable._ror(0x00000003, 1), 0x80000001)
        self.assertEqual(executable._rol(0x12345678, 0xE0), 0x12345678)


class ZoneTest(unittest.TestCase):
    def test_zone_ahead_is_centred(self):
        (x, y, width, height), = web_export.polar_box((0, 0, 45, 45), 2048, 1024)
        self.assertAlmostEqual(x + width / 2, 1024, delta=1)
        self.assertAlmostEqual(y + height / 2, 512, delta=1)
        # Half sizes are tangents: 45/90 is about 26.6 degrees each way.
        self.assertAlmostEqual(width, 2 * 26.565 / 360 * 2048, delta=2)

    def test_zone_behind_is_split_at_the_seam(self):
        boxes = web_export.polar_box((0, 180, 20, 20), 2048, 1024)
        self.assertEqual(len(boxes), 2)
        self.assertAlmostEqual(sum(box[2] for box in boxes), 2 * 12.53 / 360 * 2048, delta=2)

    def test_heading_grows_to_the_right(self):
        (x, _, width, _), = web_export.polar_box((0, 90, 10, 10), 2048, 1024)
        self.assertAlmostEqual(x + width / 2, 1536, delta=1)


if __name__ == '__main__':
    unittest.main()
