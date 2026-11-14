"""Adapt the Riven graph to DeMystify's MIT-licensed local card-atlas viewer."""
from __future__ import annotations

import argparse
import hashlib
import json
import math
import subprocess
from pathlib import Path

from . import scripts

WEB = Path(__file__).with_name('web')
PALETTE = ['#b5b583', '#8daf91', '#b29abc', '#d0af76', '#85a9b3', '#bd8e79', '#a4ad87', '#8ba7a1']


def atlas_layout(nodes):
    """Pack card rectangles by stack, retaining the topology's local ordering."""
    groups = []
    area = 0
    for stack in sorted({n['stack'] for n in nodes}):
        group = [n for n in nodes if n['stack'] == stack]
        step_x, step_y = 236, max(n['height'] for n in group) + 56
        columns = max(1, math.ceil(math.sqrt(len(group) * step_y / step_x)))
        order = sorted(group, key=lambda n: (n['originalY'], n['originalX'], n['id']))
        for start in range(0, len(order), columns):
            order[start:start + columns] = sorted(order[start:start + columns], key=lambda n: (n['originalX'], n['id']))
        for rank, node in enumerate(order):
            node['x'], node['y'] = (rank % columns) * step_x + step_x / 2, (rank // columns) * step_y + step_y / 2 + 36
        width, height = columns * step_x, math.ceil(len(group) / columns) * step_y + 36
        groups.append((stack, group, width, height))
        area += (width + 160) * (height + 160)
    target_width = max(600, math.sqrt(area) * 1.4)
    x = y = 80
    row_height = 0
    boxes = []
    for stack, group, width, height in groups:
        if x > 80 and x + width > target_width:
            x, y, row_height = 80, y + row_height + 160, 0
        for node in group:
            node['x'] += x
            node['y'] += y
        boxes.append({'name': stack, 'minX': x, 'minY': y, 'maxX': x + width, 'maxY': y + height})
        x += width + 160
        row_height = max(row_height, height)
    return boxes


def adapt(graph, details, ast, images=None):
    images = images or {}
    if 'cards' in images:
        images = images['cards']
    ids = {node['id']: i + 1 for i, node in enumerate(graph['nodes'])}
    cards = {card['id']: card for card in ast['cards']}
    stack_names = {s['id']: s['name'] for s in graph['stacks']}
    sets = {k: set(graph['analysis'][k]) for k in ('sources', 'sinks', 'isolated')}
    nodes = []
    for node in graph['nodes']:
        source, stack = node['id'], node['stack']
        card = cards.get(source)
        flags = ['virtual'] if node.get('virtual') else ['card']
        flags += [flag.rstrip('s') if flag != 'isolated' else flag for flag, values in sets.items() if source in values]
        image = images.get(source, {})
        zones = []
        if card:
            for hotspot in card['hotspots']:
                left, top, right, bottom = hotspot['rect']
                if right <= left or bottom <= top:
                    continue
                zones.append({'id': hotspot['blst_id'], 'type': 'hotspot', 'name': hotspot['name'],
                              'layer': 'card', 'x': left, 'y': top, 'width': right - left, 'height': bottom - top,
                              'enabled': True, 'hidden': False, 'rawFlags': hotspot['flags'],
                              'script': scripts.format_scripts(hotspot['scripts'], ast['names'][stack])})
        item = {'id': ids[source], 'sourceId': source, 'name': node['label'], 'stack': stack_names[stack],
                'originalName': node['name'] if not node.get('virtual') else '',
                'assetName': image.get('assetName', ''), 'attributes': flags,
                'x': node['x'], 'y': node['y'], 'originalX': node['x'], 'originalY': node['y'],
                'width': 180, 'height': 132 if image.get('image') else 56,
                'viewWidth': 608, 'viewHeight': 392, 'zones': zones,
                'rmap': node['global_id_hex'], 'hotspotCount': node['hotspot_count'],
                'imageCount': node['image_count'], 'pictureRecords': card['pictures'] if card else [],
                'scriptText': details[source]['text'], 'provenance': details[source]['provenance'],
                'dynamicNavigation': details[source].get('unresolved', [])}
        item.update({k: image[k] for k in ('image', 'thumbnail', 'imageWidth', 'imageHeight', 'assetName') if k in image})
        nodes.append(item)
    edges = []
    virtual = {n['id'] for n in graph['nodes'] if n.get('virtual')}
    for edge in graph['edges']:
        flags = ['crossAge' if edge['cross_stack'] else 'intraAge']
        if edge['source'] == edge['target']:
            flags.append('selfReference')
        if edge['target'] in virtual:
            flags.append('notImplemented')
        if edge['kind'] == 'zip':
            flags.append('zip')
        if edge['kind'].startswith('native'):
            flags.append({'native-timer': 'nativeTimer', 'native-ending': 'nativeEnding',
                          'native-new-game': 'nativeEnding'}.get(edge['kind'], 'native'))
        edges.append({**edge, 'source': ids[edge['source']], 'target': ids[edge['target']],
                      'attributes': flags, 'transitivityId': 0})
    groups = atlas_layout(nodes)
    return {'nodes': nodes, 'edges': edges,
            'meta': {'game': 'Riven', 'cards': graph['stats']['card_count'], 'nodes': len(nodes),
                     'stacks': len(graph['stacks']), 'edges': len(edges),
                     'images': sum(bool(n.get('image')) for n in nodes),
                     'missingImages': sum('card' in n['attributes'] and not n.get('image') for n in nodes),
                     'zones': sum(len(n['zones']) for n in nodes), 'groups': groups,
                     'startNodeID': ids[graph['start_card']], 'defaultLayout': 'pdf',
                     'layoutAlgorithm': 'Topology-ordered nonoverlapping card atlas',
                     'layout': {'minX': 0, 'minY': 0, 'maxX': max(g['maxX'] for g in groups) + 80,
                                'maxY': max(g['maxY'] for g in groups) + 80}}}


def pdf_drawing(data, out):
    """Embed Graphviz drawing operations so the PDF-style map works offline."""
    colors = {stack: PALETTE[i % len(PALETTE)] for i, stack in enumerate(sorted({n['stack'] for n in data['nodes']}))}
    lines = ['digraph Riven {', 'graph [bgcolor="white", overlap=true, splines=false, outputorder=edgesfirst];',
             'node [shape=box, style="rounded,filled", fontname="Helvetica", fontsize=8, fontcolor="#17221b", margin=".06,.03"];',
             'edge [color="#697665", arrowsize=.45, penwidth=.6];']
    for node in data['nodes']:
        label = node['name'] + ('\n' + node['originalName'] if node['originalName'] else '')
        fill, style, stroke = colors[node['stack']], 'rounded,filled', '#55634c'
        if 'virtual' in node['attributes']:
            fill, style, stroke = '#ffffff', 'dashed,filled', '#8a754a'
        lines.append(f'{node["id"]} [label={json.dumps(label)}, pos="{node["originalX"]},{-node["originalY"]}!", fillcolor="{fill}", color="{stroke}", style="{style}"];')
    for i, edge in enumerate(data['edges']):
        color = '#9d7d3d' if edge['cross_stack'] else '#697665'
        style = 'solid'
        if edge['kind'] == 'zip':
            color, style = '#75a89c', 'dashed'
        elif edge['kind'].startswith('native'):
            color, style = '#a089ae', 'dashed'
        lines.append(f'{edge["source"]} -> {edge["target"]} [riven_index="{i}", color="{color}", style="{style}"];')
    lines.append('}')
    dot = '\n'.join(lines)
    cache = out / 'web-layout.json'
    cache_key = out / 'web-layout.sha256'
    digest = hashlib.sha256(dot.encode()).hexdigest()
    if cache.exists() and cache_key.exists() and cache_key.read_text().strip() == digest:
        data['pdfLayout'] = json.loads(cache.read_text())
        return
    # Fixed positions retain the extracted graph's organic arrangement. Only
    # the presentation is rebuilt; graph.json remains the authoritative data.
    try:
        result = subprocess.run(['neato', '-n2', '-Tjson'], input=dot, capture_output=True,
                                text=True, timeout=90, check=True)
    except (FileNotFoundError, subprocess.SubprocessError):
        data['meta']['defaultLayout'] = 'atlas'
        return
    raw = json.loads(result.stdout)
    x0, y0, x1, y1 = map(float, raw['bb'].split(','))
    def point(p):
        return [p[0] - x0, y1 - p[1]]
    def operations(ops):
        values = []
        for op in ops or []:
            item = dict(op)
            if 'points' in item:
                item['points'] = [point(p) for p in item['points']]
            if 'pt' in item:
                item['pt'] = point(item['pt'])
            if 'rect' in item:
                item['rect'] = point(item['rect'][:2]) + item['rect'][2:]
            for field in ('p0', 'p1'):
                if field in item:
                    item[field] = point(item[field][:2]) + item[field][2:]
            values.append(item)
        return values
    drawing = {'bounds': {'minX': 0, 'minY': 0, 'maxX': x1 - x0, 'maxY': y1 - y0},
               'nodes': [], 'edges': [], 'draw': [], 'labels': []}
    ids = {}
    for node in raw['objects']:
        node_id = int(node['name'])
        ids[node['_gvid']] = node_id
        x, y = point(list(map(float, node['pos'].split(',')[:2])))
        drawing['nodes'].append({'id': node_id, 'x': x, 'y': y,
                                 'width': float(node['width']) * 72, 'height': float(node['height']) * 72,
                                 'fill': node.get('fillcolor', '#ffffff'), 'stroke': node.get('color', '#000000'),
                                 'fontColor': '#17221b', 'penWidth': 1, 'shape': 'box', 'style': node.get('style', ''),
                                 'draw': operations(node.get('_draw_')), 'labels': operations(node.get('_ldraw_'))})
    for edge in raw.get('edges', []):
        original = data['edges'][int(edge['riven_index'])]
        drawing['edges'].append({'source': ids[edge['tail']], 'target': ids[edge['head']],
                                 'kind': original['kind'], 'graphIndex': int(edge['riven_index']),
                                 'attributes': original['attributes'], 'color': edge.get('color', '#697665'),
                                 'style': edge.get('style', ''), 'penWidth': float(edge.get('penwidth', .6)),
                                 'draw': operations(edge.get('_draw_')), 'head': operations(edge.get('_hdraw_')),
                                 'tail': operations(edge.get('_tdraw_')), 'labels': operations(edge.get('_ldraw_'))})
    data['pdfLayout'] = drawing
    cache.write_text(json.dumps(drawing, separators=(',', ':')) + '\n')
    cache_key.write_text(digest + '\n')


def write(out, graph, details, ast):
    out = Path(out)
    manifest = out / 'images-manifest.json'
    images = json.loads(manifest.read_text()) if manifest.exists() else {}
    data = adapt(graph, details, ast, images)
    pdf_drawing(data, out)
    text = (WEB / 'viewer.html').read_text()
    license_text = (WEB / 'LICENSE.demystify').read_text()
    payload = json.dumps(data, separators=(',', ':')).replace('<', '\\u003c')
    text = text.replace('{{STYLE}}', (WEB / 'viewer.css').read_text())
    text = text.replace('{{SCRIPT}}', (WEB / 'viewer.js').read_text())
    text = text.replace('{{GRAPH_JSON}}', payload)
    (out / 'index.html').write_text(text)
    (out / 'LICENSE.demystify.txt').write_text(license_text)
    print(f'Local Riven atlas: {len(data["nodes"])} nodes, {data["meta"]["images"]} pictures, {out / "index.html"}')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', default='generated/riven')
    args = parser.parse_args()
    out = Path(args.output)
    graph = json.loads((out / 'graph.json').read_text())
    ast = json.loads((out / 'cards.json').read_text())
    cards = {card['id']: card for card in ast['cards']}
    details = {}
    for node in graph['nodes']:
        stack, rid = node['id'].split(':')
        details[node['id']] = {'text': (out / 'scripts' / stack / f'{rid}.txt').read_text(),
                               'provenance': cards.get(node['id'], {}).get('provenance', {'status': 'missing CARD resource'}),
                               'unresolved': [u for u in graph['unresolved_navigation'] if u['source'] == node['id']]}
    write(out, graph, details, ast)


if __name__ == '__main__':
    main()
