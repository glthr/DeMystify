"""Checks on the original 1.0 release.

Set DEMYSTIFY_REVELATION_DISC to the image of the first DVD of the original English release (or to the folder that
holds it, or to an installed game's folder); the checks are skipped when it is unset. No game data is in the repository.
"""
import os
import unittest
from collections import Counter
from pathlib import Path

from revelation_graph import build

DISCS = Path(os.environ.get('DEMYSTIFY_REVELATION_DISC', '')).expanduser()


@unittest.skipUnless(os.environ.get('DEMYSTIFY_REVELATION_DISC') and DISCS.exists(),
                     'set DEMYSTIFY_REVELATION_DISC to the first DVD image of the 1.0 release')
class OriginalRelease(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.graph, cls.loaded = build.build([DISCS])
        cls.loaded['source'].close()

    def test_counts(self):
        stats = self.graph['stats']
        self.assertEqual((stats['stored'], stats['absent'], stats['panoramas'], stats['fixedViews']), (618, 11, 382, 236))
        self.assertEqual(stats['edgeKinds'], {'amulet': 9, 'auto': 2, 'engine': 95, 'move': 1031, 'return': 209, 'unreferenced': 9})
        self.assertEqual((stats['commandBlocks'], stats['commands'], stats['conditional']), (6913, 28980, 28))
        self.assertEqual((stats['inferred'], stats['callerOwned'], stats['zipPlaces']), (63, 10, 28))
        self.assertFalse([n['id'] for n in self.graph['nodes'] if n['errors']])

    def test_paths(self):
        analysis = self.graph['analysis']
        self.assertEqual((self.graph['start'], self.graph['boot']), ('w5_z01_n010', 'w6_z01_n021'))
        self.assertEqual((analysis['uniquePairs'], analysis['reachableFromStart'], analysis['maximumDistanceFromStart'],
                          analysis['largestShortestDistance']), (1336, 609, 74, 87))
        self.assertEqual(analysis['distanceFromStart']['w4_z01_n010'], 32)

    def test_numbering_rule(self):
        stored = [n for n in self.graph['nodes'] if not n['virtual'] and n['world'] != 6]
        self.assertTrue(all((n['type'] == 'panorama') == (n['node'] % 10 == 0) for n in stored))

    def test_the_one_passage_into_the_night(self):
        crossing = sorted((e['source'], e['target']) for e in self.graph['edges']
                          if e['source'].startswith('w5') and e['target'].startswith('w1') and not e['callerOwned']
                          and e['target'][-1] == '0')
        self.assertEqual(crossing, [('w5_z01_n110', 'w1_z01_n310')])
        # The name that the day node w5_z02_n160 gives a transition it does not store is kept, never followed.
        self.assertEqual([d['value'] for d in self.graph['danglingTransitions']], ['w5_z02_n160.tr_w1_z02_n170'])

    def test_compiled_changes(self):
        found = Counter(change['library'] for change in self.loaded['compiled'])
        self.assertEqual(sum(found.values()), 55)
        self.assertEqual(self.graph['compiledDestinations']['computed'], 57)


if __name__ == '__main__':
    unittest.main()
