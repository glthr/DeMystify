"""Checks against the original 1.0 discs.

Set DEMYSTIFY_EXILE_DISCS to the folder that holds the four CD images of the original English release
(raw .img images of the user's own discs); the checks are skipped when it is unset. No game data is in the repository.
"""
import os
import unittest
from pathlib import Path

from exile_graph import build

DISCS = Path(os.environ.get('DEMYSTIFY_EXILE_DISCS', '')).expanduser()


@unittest.skipUnless(os.environ.get('DEMYSTIFY_EXILE_DISCS') and any(DISCS.glob('*.img')),
                     'set DEMYSTIFY_EXILE_DISCS to the folder of the four 1.0 disc images')
class OriginalReleaseTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.graph, cls.context = build.build([str(DISCS)])

    @classmethod
    def tearDownClass(cls):
        cls.context['source'].close()

    def test_executable_is_the_first_release(self):
        edition = self.graph['edition']
        self.assertEqual((edition['version'], edition['safedisc']), ('1.0', 'v2.10.30'))
        self.assertEqual(edition['detectionMd5'], 'f8ab52da2815c1342eeb42b1bcad5441')

    def test_safedisc_decoding_yields_every_room(self):
        self.assertEqual(len(self.graph['rooms']), 33)
        self.assertEqual(self.graph['stats']['scriptedNodes'], 1169)
        self.assertEqual(self.graph['stats']['missingCalls'], 0)

    def test_every_instruction_is_a_known_opcode_or_a_listed_unknown(self):
        from exile_graph.names import OPCODES
        unknown = set()
        for room in self.context['db'].rooms.values():
            for block in room.blocks:
                for script in list(block.scripts) + list(block.hotspots):
                    unknown.update(o.op for o in script.ops if o.op not in OPCODES)
        self.assertLessEqual(unknown, {5, 128, 142, 143, 245, 246, 247})

    def test_graph_counts(self):
        stats = self.graph['stats']
        self.assertEqual(stats['placeTypes']['cube'], 692)
        self.assertEqual(stats['placeTypes']['frame'], 113)
        self.assertEqual(stats['nodes'], 821)
        self.assertEqual(stats['edgeKinds'], {'move': 1742, 'auto': 91, 'zip': 183, 'continuation': 6, 'native': 11})
        self.assertEqual(stats['dynamicReferences'], 6)
        self.assertEqual(self.graph['start'], 'TOHO:101')

    def test_every_edge_joins_two_nodes_and_keeps_evidence(self):
        ids = {node['id'] for node in self.graph['nodes']}
        for edge in self.graph['edges']:
            self.assertIn(edge['source'], ids)
            self.assertIn(edge['target'], ids)
            self.assertTrue(edge['evidence'])

    def test_absent_target(self):
        self.assertEqual([n['id'] for n in self.graph['nodes'] if n['virtual']], ['MAIS:21'])

    def test_panoramas_without_stored_faces(self):
        # The DEMO and ATIX rooms are scripted but have no archive on the
        # discs; MAIS 20 is the one node of a shipped room without faces.
        missing = [n['id'] for n in self.graph['nodes'] if n['type'] == 'cube' and n['viewIndexes']
                   and not n['storedViews']]
        self.assertEqual(missing, ['ATIX:1', 'ATIX:2', 'DEMO:1', 'DEMO:2', 'DEMO:3', 'DEMO:4', 'MAIS:20'])


if __name__ == '__main__':
    unittest.main()
