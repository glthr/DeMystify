"""Regression tests for data preservation in the local browser presentation."""
import copy
import json
import os
from pathlib import Path
import re
import tempfile
import unittest
from unittest.mock import patch

from riven_graph import scripts
from riven_graph.web_export import adapt, write

ROOT = Path(__file__).resolve().parents[1]


class WebDataTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        out = Path(os.environ.get('RIVEN_WEB_TEST_DATA', ROOT / 'generated' / 'riven'))
        if not all((out / name).is_file() for name in ('graph.json', 'cards.json')):
            raise unittest.SkipTest('Optional generated Riven metadata is not available; generate it locally to run dataset regressions')
        cls.graph = json.loads((out / 'graph.json').read_text())
        cls.ast = json.loads((out / 'cards.json').read_text())
        cls.cards = {card['id']: card for card in cls.ast['cards']}
        cls.details = {
            node['id']: {'text': f'Read-only script for {node["id"]}',
                         'provenance': cls.cards.get(node['id'], {}).get('provenance', {}),
                         'unresolved': [u for u in cls.graph['unresolved_navigation'] if u['source'] == node['id']]}
            for node in cls.graph['nodes']
        }
        cls.data = adapt(cls.graph, cls.details, cls.ast)
        cls.by_source = {node['sourceId']: node for node in cls.data['nodes']}

    def test_numeric_browser_ids_are_lossless_and_keep_all_typed_edge_evidence(self):
        self.assertEqual(len(self.data['nodes']), 2312)
        self.assertEqual(len(self.data['edges']), 6686)
        self.assertEqual(set(self.by_source), {node['id'] for node in self.graph['nodes']})
        self.assertEqual(len({n['id'] for n in self.data['nodes']}), 2312)
        reverse = {n['id']: n['sourceId'] for n in self.data['nodes']}
        for original, browser in zip(self.graph['edges'], self.data['edges']):
            self.assertEqual((reverse[browser['source']], reverse[browser['target']], browser['kind']),
                             (original['source'], original['target'], original['kind']))
            self.assertEqual(browser['evidence'], original['evidence'])
        self.assertEqual(reverse[self.data['meta']['startNodeID']], 'tspit:155')

    def test_hotspot_pixel_geometry_and_disassembly_are_preserved_without_execution(self):
        total = 0
        for source, card in self.cards.items():
            original = [h for h in card['hotspots'] if h['rect'][2] > h['rect'][0] and h['rect'][3] > h['rect'][1]]
            browser = self.by_source[source]['zones']
            self.assertEqual(len(original), len(browser))
            for hotspot, zone in zip(original, browser):
                left, top, right, bottom = hotspot['rect']
                self.assertEqual((zone['x'], zone['y'], zone['width'], zone['height']),
                                 (left, top, right-left, bottom-top))
                self.assertEqual(zone['id'], hotspot['blst_id'])
                self.assertEqual(zone['rawFlags'], hotspot['flags'])
                self.assertEqual(zone['script'], scripts.format_scripts(hotspot['scripts'], self.ast['names'][card['stack']]))
                total += 1
        self.assertEqual(self.data['meta']['zones'], total)

    def test_virtual_targets_do_not_gain_fabricated_card_or_picture_data(self):
        virtual = [n for n in self.data['nodes'] if 'virtual' in n['attributes']]
        self.assertEqual(len(virtual), 5)
        for node in virtual:
            self.assertNotIn(node['sourceId'], self.cards)
            self.assertEqual(node['zones'], [])
            self.assertFalse(node.get('image'))
            self.assertEqual(node['pictureRecords'], [])
            self.assertFalse(any(e['source'] == node['id'] for e in self.data['edges']))

    def test_saved_returns_stay_metadata_and_do_not_create_journal_shortcuts(self):
        self.assertEqual(len(self.graph['unresolved_navigation']), 3)
        for source in ('aspit:5', 'aspit:6', 'aspit:7'):
            self.assertTrue(self.by_source[source]['dynamicNavigation'])
        for source in ('aspit:5', 'aspit:6'):
            self.assertFalse(any(e['source'] == self.by_source[source]['id'] for e in self.data['edges']))

    def test_picture_manifest_enriches_only_the_matching_card_and_atlas_boxes_do_not_overlap(self):
        image = {'image': 'images/temple.png', 'thumbnail': 'images/temple-thumb.png',
                 'imageWidth': 608, 'imageHeight': 392, 'assetName': 'tBMP:42'}
        data = adapt(self.graph, self.details, self.ast, {'cards': {'tspit:155': image}})
        by_source = {n['sourceId']: n for n in data['nodes']}
        self.assertEqual(by_source['tspit:155']['image'], image['image'])
        self.assertEqual(data['meta']['images'], 1)
        groups = {}
        for node in data['nodes']:
            groups.setdefault(node['stack'], []).append(node)
        # Packed columns have unique cells and positive gutter widths; this
        # also catches a tall picture card being packed into a 56px row.
        for group in groups.values():
            for i, a in enumerate(group):
                for b in group[i+1:]:
                    self.assertTrue(abs(a['x']-b['x']) >= (a['width']+b['width'])/2 or
                                    abs(a['y']-b['y']) >= (a['height']+b['height'])/2)

    def test_offline_html_safely_embeds_script_like_game_text(self):
        graph = copy.deepcopy(self.graph)
        graph['nodes'][0]['name'] = '</script><script>window.BAD=true</script>'
        details = copy.deepcopy(self.details)
        details[graph['nodes'][0]['id']]['text'] = '</script><img src=x onerror="alert(1)">'
        with tempfile.TemporaryDirectory() as temp, patch('riven_graph.web_export.pdf_drawing'):
            write(temp, graph, details, self.ast)
            text = Path(temp, 'index.html').read_text()
            match = re.search(r'<script>window\.DEMYSTIFY_GRAPH=(.*?);</script>', text, re.S)
            self.assertIsNotNone(match)
            self.assertNotIn('</script>', match[1].lower())
            raw = json.loads(match[1])
            self.assertEqual(raw['nodes'][0]['originalName'], graph['nodes'][0]['name'])
            self.assertEqual(raw['nodes'][0]['scriptText'], details[graph['nodes'][0]['id']]['text'])
            self.assertNotRegex(text, r'<(?:script|link)[^>]+(?:src|href)="https?://')
            self.assertIn('Copyright (c) 2025 Guillaume Lethuillier', text)
            self.assertNotIn('{{STYLE}}', text)
            self.assertNotIn('{{SCRIPT}}', text)


if __name__ == '__main__':
    unittest.main()
