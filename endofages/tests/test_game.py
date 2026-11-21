"""Checks on the game: Myst V: End of Ages, GOG 2.0.0.9 (the Windows files).

Set DEMYSTIFY_ENDOFAGES_DIR to the folder that holds `dat/`, `Python/`, `SDL/` and `avi/` (in the GOG Mac application:
`Contents/SharedSupport/myst_v/support/myst_v/drive_c/Myst V`); the checks are skipped when it is unset. The game is
read in place and never written to. No game data is in the repository.
"""
import collections
import os
import unittest

from endofages_graph import analyze, build, freeroam, nodes, pak, physics, prp

GAME = os.environ.get('DEMYSTIFY_ENDOFAGES_DIR', '')
COUNTS = {'Kveer': 89, 'Descent': 480, 'Direbo': 100, 'Laki': 711, 'Siralehn': 430, 'Tahgira': 442, 'Todelmer': 363,
          'Myst': 114}


@unittest.skipUnless(GAME and os.path.isdir(os.path.join(GAME, 'dat')),
                     'set DEMYSTIFY_ENDOFAGES_DIR to the Myst V folder (dat, Python, SDL, avi)')
class Game(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.dat = os.path.join(GAME, 'dat')
        cls.graph = build.build(GAME)
        cls.analysis = analyze.analyze(cls.graph)

    def test_classic_points(self):
        total_links = 0
        for age, count in COUNTS.items():
            g = nodes.read_nodes(os.path.join(self.dat, age + '.node'))
            self.assertEqual((g['version'], len(g['nodes'])), (5, count))
            pairs = {(n['index'], l['target']) for n in g['nodes'] for l in n['links']}
            self.assertTrue(all((b, a) in pairs for a, b in pairs), age)      # every link has its way back
            total_links += sum(len(n['links']) for n in g['nodes'])
        self.assertEqual(total_links, 6878)

    def test_pages(self):
        pfms = links = phys = 0
        for age in build.AGES:
            d = build.AgeData(self.dat, age)
            pfms += len(d.pfms)
            links += sum(len(x[2]) for x in d.link_msgs)
            for name, path in build.age_pages(self.dat, age):
                p = prp.Page(path)
                for k in p.of_class('plGenericPhysical'):
                    physics.read_physical(p, k)
                    phys += 1
        self.assertEqual((pfms, links, phys), (333, 50, 1598))
        g = build.AgeData(self.dat, 'Global')
        self.assertEqual(sorted(r for _, r, _ in g.link_msgs),
                         ['RespLakiDireboLink', 'RespSrlnDireboLink', 'RespTdlmDireboLink', 'RespTgraDireboLink'])

    def test_scripts(self):
        mods = pak.read_pak(os.path.join(GAME, 'Python', 'python.pak'))
        self.assertEqual(len(mods), 180)
        names = {s for _, kind, s in pak.strings(mods['Kveer.py']) if kind == 'name'}
        self.assertIn('ILinkToVolcano', names)

    def test_graph(self):
        g = self.graph
        self.assertEqual(g['start'], 'Kveer:4')
        self.assertEqual(g['unresolved'], [])
        self.assertEqual(collections.Counter(n['kind'] for n in g['nodes']), {'classic': 2729, 'absent': 2, 'place': 1})
        self.assertEqual(collections.Counter(e['kind'] for e in g['edges']),
                         {'classic': 6876, 'link': 85, 'script-link': 5, 'cutscene': 8, 'warp': 6, 'ladder': 30,
                          'ride': 14})
        self.assertEqual(self.analysis['betweenAges']['edges'], 52)

    def test_reach(self):
        r = self.analysis['readings']
        self.assertEqual((r['all']['reached'], r['possible']['reached'], r['unconditional']['reached']),
                         (2724, 2722, 1516))
        self.assertEqual(r['possible']['diameter']['hops'], 88)
        one_way = r['possible']['oneWayCount']
        self.assertEqual(one_way, COUNTS['Myst'] + 1)        # Myst and Releeshan: no way back to the start

    def test_points_stand_on_the_floor(self):
        out = freeroam.compare(GAME, dict(self.graph, ages=['Direbo', 'Myst']))
        for age, a in out['ages'].items():
            self.assertAlmostEqual(a['eyeHeight']['median'], 3.0, delta=0.05)


if __name__ == '__main__':
    unittest.main()
