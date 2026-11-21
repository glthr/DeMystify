"""Checks that need no game data: the readers on small synthetic inputs."""
import os
import struct
import subprocess
import tempfile
import unittest

from endofages_graph import analyze, crypt, freeroam, nodes, pak, physics, prp, pydis
from endofages_graph.stream import EOA_STR_KEY, Reader


def safe(s):
    raw = s.encode('latin-1')
    return struct.pack('<H', len(raw)) + bytes(b ^ EOA_STR_KEY[i % 8] for i, b in enumerate(raw))


class Formats(unittest.TestCase):
    def test_safe_string(self):
        r = Reader(safe('LinkInPointDefault') + b'\x07')
        self.assertEqual(r.safe_str(), 'LinkInPointDefault')
        self.assertEqual(r.u8(), 7)

    def test_decrypt_round_trip(self):
        plain = b'Page=GreatRm,2\nPage=Prison,1\n' * 3
        padded = plain + b'\0' * (-len(plain) % 16)
        body = subprocess.run(['openssl', 'enc', '-e', '-aes-128-ecb', '-nopad', '-K', crypt.EOA_KEY.hex()],
                              input=padded, capture_output=True, check=True).stdout
        data = struct.pack('<II', crypt.EOA_MAGIC, len(plain)) + body
        self.assertTrue(crypt.is_encrypted(data))
        self.assertEqual(crypt.decrypt(data), plain)
        with self.assertRaises(ValueError):
            crypt.decrypt(b'not enciphered')

    def test_nodes(self):
        def node(x, links):
            out = struct.pack('<3fI', x, 0.0, 3.0, len(links))
            for t, typ in links:
                out += struct.pack('<IB3f', t, typ, 0.0, 0.0, 0.0)
            return out
        data = struct.pack('<II', 5, 3) + node(0, [(1, 0)]) + node(10, [(0, 0)]) + node(50, [])
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, 'Test.node')
            with open(path, 'wb') as f:
                f.write(data)
            g = nodes.read_nodes(path)
        self.assertEqual(g['version'], 5)
        self.assertEqual([len(n['links']) for n in g['nodes']], [1, 1, 0])
        self.assertEqual([sorted(c) for c in nodes.components(g["nodes"])], [[0, 1], [2]])
        self.assertEqual(nodes.closest(g['nodes'], (9, 0, 3))[0], 1)

    def test_marshal(self):
        data = b'(' + struct.pack('<i', 3) + b'i' + struct.pack('<i', 5) + b's' + struct.pack('<i', 3) + b'abc' + b'N'
        self.assertEqual(pak.Marshal(data).load(), (5, b'abc', None))

    def test_disassembler(self):
        co = {'code': bytes([100, 0, 0, 83]), 'consts': (b'Kveer',), 'names': (), 'varnames': (), 'cellvars': (),
              'freevars': ()}
        self.assertEqual([(op, shown) for _, op, _, shown in pydis.disassemble(co)],
                         [('LOAD_CONST', "'Kveer'"), ('RETURN_VALUE', '')])

    def test_uoid(self):
        data = bytes([0]) + struct.pack('<IBHI', 0x10021, 0, 0x01, 7) + safe('ClickTakeDireboLink')
        u = prp.read_uoid(Reader(data))
        self.assertEqual((u.location, u.cls, u.oid, u.name), (0x10021, 1, 7, 'ClickTakeDireboLink'))

    def test_floor_and_wall(self):
        verts = [(0, 0, 0), (10, 0, 0), (0, 10, 0)]
        (area, centre, nz), = physics.floor_triangles(verts, [(0, 1, 2)])
        self.assertEqual((area, nz), (50.0, 1.0))
        wall = ((5, -5, 0), (5, 5, 0), (5, 0, 10))
        self.assertTrue(freeroam._seg_tri((0, 0, 2), (10, 0, 2), wall))
        self.assertFalse(freeroam._seg_tri((0, 0, 2), (4, 0, 2), wall))

    def test_analysis(self):
        def n(i, age='A', comp=0):
            return {'id': f'{age}:{i}', 'kind': 'classic', 'age': age, 'index': i, 'label': f'{age}:{i}',
                    'pos': [float(i), 0.0, 0.0], 'component': comp}
        g = {'ages': ['A'], 'start': 'A:0', 'nodes': [n(0), n(1), n(2, comp=1)],
             'edges': [{'source': 'A:0', 'target': 'A:1', 'kind': 'classic', 'classicType': 0, 'conditional': False,
                        'evidence': {}},
                       {'source': 'A:1', 'target': 'A:0', 'kind': 'classic', 'classicType': 0, 'conditional': False,
                        'evidence': {}},
                       {'source': 'A:1', 'target': 'A:2', 'kind': 'link', 'conditional': True,
                        'evidence': {'never': 'test'}}]}
        a = analyze.analyze(g)
        self.assertEqual(a['readings']['all']['reached'], 3)
        self.assertEqual(a['readings']['possible']['reached'], 2)
        self.assertEqual(a['readings']['possible']['diameter']['hops'], 1)


if __name__ == '__main__':
    unittest.main()
