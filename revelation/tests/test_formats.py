"""Unit tests on built data: no disc is needed."""
import struct
import unittest

from revelation_graph import bigfile, build, native, resources


def swizzled(text):
    data = resources.swizzle(text.encode())
    return struct.pack('<I', len(data)) + data


class Formats(unittest.TestCase):
    def test_swizzle_is_its_own_inverse(self):
        self.assertEqual(resources.swizzle(bytes([0x6E, 0x61, 0x76])), bytes([0x9D, 0x92, 0xB9]))
        self.assertEqual(resources.swizzle(resources.swizzle(b'ContextChange(1,01,021)')), b'ContextChange(1,01,021)')

    def test_command_block(self):
        data = resources.MAGIC + struct.pack('<II', 6, 1) + swizzled('nav_click') + struct.pack('<I', 2) \
            + swizzled('ContextChange(1,01,021)') + swizzled(';SoundPlay(x)')
        self.assertEqual(resources.command_block(data), [('nav_click', ['ContextChange(1,01,021)', ';SoundPlay(x)'])])

    def test_node_record(self):
        data = resources.MAGIC + struct.pack('<II', 8, 1) + swizzled('node') + struct.pack('<II', 1, 2) \
            + struct.pack('<6f', 1, 2, 3, 0, 0, 90) + struct.pack('<I', 1) + swizzled('zip') + struct.pack('<2f', 10, 20)
        node = resources.node(data)
        self.assertEqual((node['viewport'], node['values'][:3], node['arrivals'][0]['entry']), ('fixed', [1.0, 2.0, 3.0], 'zip'))

    def test_parameter_lines(self):
        text = 'm_geo   ( string , "w1_z01_n020.h_to_w1_z01_n021")\nm_dest_node ( u32 , 010 )\n; a comment\n'
        self.assertEqual(resources.ai(text), [('m_geo', 'string', 'w1_z01_n020.h_to_w1_z01_n021'), ('m_dest_node', 'u32', '010')])

    def test_archive_tree(self):
        def name(text):
            return struct.pack('<I', len(text) + 1) + text.encode() + b'\0'
        data = name('UBI_BF_SIG') + struct.pack('<II', 1, 0) + b'\x01' + name('global') + b'\x00' + struct.pack('<I', 1) \
            + name('a.bin') + struct.pack('<II', 7, 99) + struct.pack('<I', 1) + name('nodedb.ndb') + struct.pack('<II', 3, 5)
        files, end = bigfile.parse(data)
        self.assertEqual(files, [('global/a.bin', 7, 99), ('nodedb.ndb', 3, 5)])
        self.assertEqual(end, len(data))

    def test_names(self):
        self.assertEqual(build.node_id(5, 1, 20), 'w5_z01_n020')
        self.assertEqual(build.parse_id('w5_z01_n020'), (5, 1, 20))
        self.assertIsNone(build.parse_id('w5_z1_n20'))

    def test_tests_before_an_instruction(self):
        commands = ['JumpIfElse(global.state.x,yes,no)', 'Label(yes)', ';Jump(end)', 'ContextChange(1,01,021)']
        self.assertEqual(build.tests_before(commands, 3), ['JumpIfElse(global.state.x,yes,no)'])

    def test_pushed_constants_follow_jumps(self):
        # push 0x52; jmp L; push 0x53; L: push 5; push 2; call
        code = [(0, 'pushl', '$0x52'), (2, 'jmp', '0x6 <x>'), (4, 'pushl', '$0x53'), (6, 'pushl', '$0x5'), (8, 'pushl', '$0x2'),
                (10, 'calll', '*0x1000')]
        found, _ = native.paths_back(code, {a: i for i, (a, _, _) in enumerate(code)}, {6: [1]}, 5, 3)
        self.assertEqual(found, {(2, 5, 0x52), (2, 5, 0x53)})

    def test_movement_leaves_out_caller_owned_returns(self):
        graph = {'edges': [{'source': 'a', 'target': 'b', 'kind': 'move', 'self': False},
                           {'source': 'b', 'target': 'a', 'kind': 'return', 'self': False, 'callerOwned': True},
                           {'source': 'b', 'target': 'c', 'kind': 'unreferenced', 'self': False}]}
        self.assertEqual(build.movement(graph), {('a', 'b')})


if __name__ == '__main__':
    unittest.main()
