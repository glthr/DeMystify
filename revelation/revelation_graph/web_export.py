"""Adapt the Revelation graph to DeMystify's offline viewer."""
from __future__ import annotations

import hashlib
import json
import math
import shlex
import subprocess
from pathlib import Path

from . import build, export

WEB = Path(__file__).with_name('web')
COLORS = {'Tomahna (day)': '#e6cf9a', 'Tomahna (night)': '#a9b4d6', 'Haven': '#9cc59a', 'Spire': '#b9a6d3',
          'Serenia': '#e7b1bd', 'Menu': '#b9bec2'}
GROUP_ORDER = ['Tomahna (day)', 'Tomahna (night)', 'Haven', 'Spire', 'Serenia', 'Menu']
KIND = {'move': 'script', 'auto': 'script', 'return': 'script', 'amulet': 'script', 'unreferenced': 'script', 'engine': 'native'}
THUMB = 180


def layout(graph):
    """Place the nodes with Graphviz when it is installed, on a spiral otherwise (as for the Exile graph)."""
    lines = ['digraph Revelation {', 'graph [overlap=prism, sep="+4", splines=false, mode=major, model=shortpath, start=42];',
             'node [shape=box, fontsize=8, width=.9, height=.3, margin=".04,.02"];']
    for node in graph['nodes']:
        lines.append(f'{json.dumps(node["id"])} [label={json.dumps(node["label"])}];')
    seen = set()
    for edge in graph['edges']:
        pair = (edge['source'], edge['target'])
        if edge['self'] or pair in seen:
            continue
        seen.add(pair)
        length = ' [len=5, weight=0.3]' if edge['crossAge'] else ' [len=1.6]' if edge['crossRoom'] else ''
        lines.append(f'{json.dumps(edge["source"])} -> {json.dumps(edge["target"])}{length};')
    lines.append('}')
    try:
        result = subprocess.run(['neato', '-Tplain'], input='\n'.join(lines), capture_output=True, text=True, timeout=600, check=True)
        positions = {}
        for line in result.stdout.splitlines():
            fields = shlex.split(line)
            if fields and fields[0] == 'node':
                positions[fields[1]] = (round(float(fields[2]) * 90, 2), round(-float(fields[3]) * 90, 2))
        for node in graph['nodes']:
            node['x'], node['y'] = positions.get(node['id'], (0.0, 0.0))
        return 'Graphviz neato'
    except (FileNotFoundError, subprocess.SubprocessError):
        for index, node in enumerate(graph['nodes']):
            angle = index * math.pi * (3 - math.sqrt(5))
            node['x'], node['y'] = math.cos(angle) * math.sqrt(index) * 60, math.sin(angle) * math.sqrt(index) * 60
        return 'spiral fallback'


def direction(offset):
    """Heading and pitch, in degrees, of a point given in the axes of the world (z up, the front face towards +y)."""
    x, y, z = offset
    return math.degrees(math.atan2(x, y)), math.degrees(math.atan2(z, math.hypot(x, y)))


def zones(node, width, height, flat, targets):
    out = []
    for index, spot in enumerate(node['hotspots']):
        if 'offset' not in spot or flat:
            continue
        heading, pitch = direction(spot['offset'])
        x = (heading + 180) % 360 / 360 * width
        y = (90 - pitch) / 180 * height
        size = max(60.0, min(220.0, 900 / max(0.5, math.sqrt(sum(v * v for v in spot['offset'])))))
        name = spot['name'] + (' → ' + ', '.join(targets[spot['name']]) if targets.get(spot['name']) else '')
        out.append({'id': index, 'type': 'hotspot', 'name': name, 'layer': 'card',
                    'x': round(max(0, min(width - size, x - size / 2)), 1), 'y': round(max(0, min(height - size, y - size / 2)), 1),
                    'width': round(size, 1), 'height': round(size, 1), 'enabled': True, 'hidden': False,
                    'script': f'{spot["name"]}\nposition relative to the node: {spot["offset"]}\n'
                              'The square marks the direction of the hotspot; its shape is not decoded.'})
    return out


def atlas_layout(nodes):
    """Pack node rectangles by age, keeping the local order of the layout."""
    groups, area = [], 0
    for stack in GROUP_ORDER:
        group = [n for n in nodes if n['stack'] == stack]
        if not group:
            continue
        step_x, step_y = 236, max(n['height'] for n in group) + 56
        columns = max(1, math.ceil(math.sqrt(len(group) * step_y / step_x)))
        order = sorted(group, key=lambda n: (n['originalY'], n['originalX'], n['id']))
        for start in range(0, len(order), columns):
            order[start:start + columns] = sorted(order[start:start + columns], key=lambda n: (n['originalX'], n['id']))
        for rank, node in enumerate(order):
            node['x'] = (rank % columns) * step_x + step_x / 2
            node['y'] = (rank // columns) * step_y + step_y / 2 + 36
        width, height = columns * step_x, math.ceil(len(group) / columns) * step_y + 36
        groups.append((stack, group, width, height))
        area += (width + 160) * (height + 160)
    target_width = max(600, math.sqrt(area) * 1.4)
    x = y = 80
    row_height, boxes = 0, []
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



def adapt(graph, loaded, images):
    ids = {node['id']: index + 1 for index, node in enumerate(graph['nodes'])}
    analysis = graph['analysis']
    sets = {name: set(analysis[key]) for name, key in (('source', 'sources'), ('sink', 'sinks'),
                                                        ('isolated', 'isolated'), ('unreached', 'notReachedFromStart'))}
    stored = {node.id: node for node in loaded['nodes'].values()}
    hotspot_targets = {}
    for edge in graph['edges']:
        for evidence in edge['evidence']:
            spot = evidence.get('hotspot') or next((c.get('hotspot') for c in evidence.get('startedBy', []) if c.get('hotspot')), '')
            if spot:
                names = hotspot_targets.setdefault(edge['source'], {}).setdefault(spot.split('.')[-1], [])
                if edge['target'] not in names:
                    names.append(edge['target'])
    nodes = []
    for node in graph['nodes']:
        source = node['id']
        flags = ['card', {'panorama': 'panorama', 'fixed view': 'frame', 'absent': 'virtual'}[node['type']]]
        flags += [flag for flag, values in sets.items() if source in values]
        if node.get('zipPlace'):
            flags.append('zipDestination')
        image = images.get(source)
        if image and image.get('error'):
            image = None
        if not node['virtual'] and image is None:
            flags.append('noPicture')
        view_width, view_height = (image['width'], image['height']) if image else (2048, 1024)
        flat = bool(image) and image.get('kind') == 'frame'
        provenance = {'world': node['world'], 'zone': node['zone'], 'node': node['node'], 'archive': node['archive'],
                      'position': node['position'], 'camera': node['camera'], 'arrivals': node['arrivals'],
                      'pictureLayers': node['layers'], 'classes': node['classes'], 'compiledEntries': node.get('compiledEntries', [])}
        if node['virtual']:
            provenance = {'status': 'no archive with this name is stored', 'namedBy': node.get('namedBy', [])}
        if image:
            provenance['picture'] = {key: image[key] for key in ('layer', 'tiles', 'faces', 'sourceSha256') if key in image}
        item = {
            'id': ids[source], 'sourceId': source, 'name': node['label'], 'stack': node['group'],
            'originalName': node['type'] if not node['virtual'] else '', 'assetName': '',
            'room': node['room'], 'roomId': node['zone'],
            'attributes': flags, 'x': node['x'], 'y': node['y'], 'originalX': node['x'], 'originalY': node['y'],
            'width': THUMB, 'height': round(THUMB * view_height / view_width) + 30 if image else 56,
            'viewWidth': view_width, 'viewHeight': view_height,
            'zones': zones(node, view_width, view_height, flat, hotspot_targets.get(source, {})) if image else [],
            'hotspotCount': len(node['hotspots']), 'imageCount': len(node['layers']), 'viewList': ', '.join(node['layers']),
            'sharedWith': '', 'puzzles': '', 'zipBit': 0,
            'scriptText': export.listing(stored[source], build.triggers(stored[source])) if source in stored else '',
            'provenance': provenance, 'dynamicNavigation': [],
        }
        if image:
            item.update({'image': image['image'], 'thumbnail': image['thumbnail'], 'imageWidth': image['width'],
                         'imageHeight': image['height'], 'assetName': node['archive']['path'].rsplit('/', 1)[-1]})
        nodes.append(item)
    virtual = {n['id'] for n in graph['nodes'] if n['virtual']}
    edges = []
    for edge in graph['edges']:
        flags = ['crossAge' if edge['crossAge'] else 'intraAge']
        if edge['target'] in virtual:
            flags.append('notImplemented')
        if edge['conditional']:
            flags.append('conditional')
        if edge['kind'] == 'engine':
            flags.append('native')
        if edge['kind'] in ('auto', 'return'):
            flags.append('automatic')
        if edge['kind'] == 'unreferenced' or edge.get('callerOwned'):
            flags.append('sequenceContinuation')
        if edge['movie']:
            flags.append('movie')
        edges.append({'source': ids[edge['source']], 'target': ids[edge['target']], 'kind': KIND[edge['kind']],
                      'gameKind': edge['kind'], 'cross_stack': edge['crossAge'], 'attributes': flags,
                      'evidence': edge['evidence'], 'transitivityId': 0})
    groups = atlas_layout(nodes)
    stats = graph['stats']
    return {'nodes': nodes, 'edges': edges,
            'meta': {'game': 'Myst IV: Revelation', 'version': '1.0', 'cards': stats['stored'], 'nodes': len(nodes),
                     'stacks': len(groups), 'edges': len(edges), 'images': sum(bool(n.get('image')) for n in nodes),
                     'missingImages': sum('noPicture' in n['attributes'] for n in nodes),
                     'zones': sum(len(n['zones']) for n in nodes), 'groups': groups,
                     'startNodeID': ids[graph['start']], 'defaultLayout': 'pdf',
                     'layoutAlgorithm': 'Topology-ordered nonoverlapping node atlas',
                     'layout': {'minX': 0, 'minY': 0, 'maxX': max(g['maxX'] for g in groups) + 80,
                                'maxY': max(g['maxY'] for g in groups) + 80}}}


def pdf_drawing(data, out):
    """Embed Graphviz drawing operations so that the graph view works offline."""
    lines = ['digraph Revelation {', 'graph [bgcolor="white", overlap=true, splines=false, outputorder=edgesfirst];',
             'node [shape=box, style="rounded,filled", fontname="Helvetica", fontsize=8, fontcolor="#17221b", '
             'margin=".06,.03"];', 'edge [color="#697665", arrowsize=.45, penwidth=.6];']
    for node in data['nodes']:
        label = node['name'] + ('\n' + node['originalName'] if node['originalName'] else '')
        fill, style, stroke = COLORS[node['stack']], 'rounded,filled', '#55634c'
        if 'virtual' in node['attributes']:
            fill, style, stroke = '#ffffff', 'dashed,filled', '#8a754a'
        lines.append(f'{node["id"]} [label={json.dumps(label)}, pos="{node["originalX"]},{-node["originalY"]}!", '
                     f'fillcolor="{fill}", color="{stroke}", style="{style}"];')
    for index, edge in enumerate(data['edges']):
        color, style, head = ('#9d7d3d' if edge['cross_stack'] else '#697665'), 'solid', 'normal'
        if edge['kind'] == 'zip':
            color, style = '#75a89c', 'dashed'
        elif edge['kind'] == 'native':
            color, style = '#a089ae', 'dashed'
        elif edge['gameKind'] == 'unreferenced':
            head = 'empty'
        elif edge['gameKind'] in ('auto', 'return'):
            style = 'dashed'
        lines.append(f'{edge["source"]} -> {edge["target"]} [edge_index="{index}", color="{color}", '
                     f'style="{style}", arrowhead="{head}"];')
    lines.append('}')
    dot = '\n'.join(lines)
    cache, cache_key = out / 'web-layout.json', out / 'web-layout.sha256'
    digest = hashlib.sha256(dot.encode()).hexdigest()
    if cache.exists() and cache_key.exists() and cache_key.read_text().strip() == digest:
        data['pdfLayout'] = json.loads(cache.read_text())
        return
    try:
        result = subprocess.run(['neato', '-n2', '-Tjson'], input=dot, capture_output=True, text=True, timeout=180,
                                check=True)
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
    names = {}
    for node in raw['objects']:
        node_id = int(node['name'])
        names[node['_gvid']] = node_id
        x, y = point(list(map(float, node['pos'].split(',')[:2])))
        drawing['nodes'].append({'id': node_id, 'x': x, 'y': y,
                                 'width': float(node['width']) * 72, 'height': float(node['height']) * 72,
                                 'fill': node.get('fillcolor', '#ffffff'), 'stroke': node.get('color', '#000000'),
                                 'fontColor': '#17221b', 'penWidth': 1, 'shape': 'box', 'style': node.get('style', ''),
                                 'draw': operations(node.get('_draw_')), 'labels': operations(node.get('_ldraw_'))})
    for edge in raw.get('edges', []):
        original = data['edges'][int(edge['edge_index'])]
        drawing['edges'].append({'source': names[edge['tail']], 'target': names[edge['head']],
                                 'kind': original['kind'], 'graphIndex': int(edge['edge_index']),
                                 'attributes': original['attributes'], 'color': edge.get('color', '#697665'),
                                 'style': edge.get('style', ''), 'penWidth': float(edge.get('penwidth', .6)),
                                 'draw': operations(edge.get('_draw_')), 'head': operations(edge.get('_hdraw_')),
                                 'tail': operations(edge.get('_tdraw_')), 'labels': operations(edge.get('_ldraw_'))})
    data['pdfLayout'] = drawing
    cache.write_text(json.dumps(drawing, separators=(',', ':')) + '\n')
    cache_key.write_text(digest + '\n')



def write(out, graph, loaded, images):
    out = Path(out)
    if 'x' not in graph['nodes'][0]:
        graph['layoutAlgorithm'] = layout(graph)
        (out / 'graph.json').write_text(json.dumps(graph, indent=1))
    data = adapt(graph, loaded, images)
    pdf_drawing(data, out)
    text = (WEB / 'viewer.html').read_text()
    payload = json.dumps(data, separators=(',', ':')).replace('<', '\\u003c')
    text = text.replace('{{STYLE}}', (WEB / 'viewer.css').read_text())
    text = text.replace('{{SCRIPT}}', (WEB / 'viewer.js').read_text())
    text = text.replace('{{GRAPH_JSON}}', payload)
    (out / 'index.html').write_text(text)
    (out / 'LICENSE.demystify.txt').write_text((WEB / 'LICENSE.demystify').read_text())
    return data
