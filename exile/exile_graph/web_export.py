"""Adapt the Exile graph to DeMystify's offline viewer."""
from __future__ import annotations

import hashlib
import json
import math
import subprocess
from pathlib import Path

from . import export
from . import scripts as fmt

WEB = Path(__file__).with_name('web')
KIND = {'move': 'script', 'auto': 'script', 'continuation': 'script', 'zip': 'zip', 'native': 'native'}
TYPE_FLAG = {'cube': 'panorama', 'frame': 'frame', 'menu': 'menu', 'transit': 'transit', 'native': 'engineDrawn',
             'absent': 'virtual'}
THUMB = 180


def polar_box(rect, width, height):
    """Bounding boxes, in panorama pixels, of a hotspot of a panoramic node.

    The zone is a rectangle on the plane tangent to the view sphere at
    (pitch, heading); its half sizes are given in ninetieths of the distance
    to that plane. Its outline is projected point by point. A zone that
    crosses the seam of the picture is returned in two parts.
    """
    pitch, heading, half_width, half_height = rect
    p, h = math.radians(pitch), math.radians(heading)
    direction = (math.cos(p) * math.sin(h), math.sin(p), math.cos(p) * math.cos(h))
    u = (math.cos(h), 0.0, -math.sin(h))
    v = (direction[1] * u[2] - direction[2] * u[1], direction[2] * u[0] - direction[0] * u[2],
         direction[0] * u[1] - direction[1] * u[0])
    length = math.sqrt(sum(c * c for c in v)) or 1.0
    v = tuple(c / length for c in v)
    su, sv = half_width / 90.0, half_height / 90.0
    steps = 12
    outline = [(a / steps * 2 - 1, -1) for a in range(steps + 1)] + [(1, b / steps * 2 - 1) for b in range(steps + 1)] \
        + [(a / steps * 2 - 1, 1) for a in range(steps + 1)] + [(-1, b / steps * 2 - 1) for b in range(steps + 1)]
    offsets, pitches = [], []
    for a, b in outline:
        point = tuple(direction[i] + a * su * u[i] + b * sv * v[i] for i in range(3))
        point_heading = math.degrees(math.atan2(point[0], point[2]))
        offsets.append((point_heading - heading + 180) % 360 - 180)
        pitches.append(math.degrees(math.atan2(point[1], math.hypot(point[0], point[2]))))
    left, right = heading + min(offsets), heading + max(offsets)
    top, bottom = (90 - max(pitches)) / 180 * height, (90 - min(pitches)) / 180 * height
    x0, x1 = (left + 180) / 360 * width, (right + 180) / 360 * width
    while x0 < 0:
        x0, x1 = x0 + width, x1 + width
    while x0 >= width:
        x0, x1 = x0 - width, x1 - width
    if x1 - x0 >= width:
        return [(0, top, width, bottom - top)]
    if x1 <= width:
        return [(x0, top, x1 - x0, bottom - top)]
    return [(x0, top, width - x0, bottom - top), (0, top, x1 - width, bottom - top)]


def zones(node, kind, view_width, view_height, targets):
    out = []
    for index, hotspot in enumerate(node.hotspots):
        if hotspot.background:
            continue
        text = '\n'.join([f'when {fmt.condition(hotspot.condition)}, cursor {hotspot.cursor}']
                         + [f'zone {fmt.rect(r, kind != "cube")}' for r in hotspot.rects] + fmt.lines(hotspot.ops, ''))
        name = f'Hotspot {index}'
        if targets.get(index):
            name += ' → ' + ', '.join(targets[index])
        if hotspot.cursor == 7:
            name += ' · Zip'
        for rect in hotspot.rects:
            if kind == 'cube':
                boxes = polar_box(rect, view_width, view_height)
            elif rect[1] < 0:
                continue  # position read from variables at run time
            else:
                boxes = [(rect[0], rect[1], rect[2], rect[3])]
            for x, y, width, height in boxes:
                if width <= 0 or height <= 0:
                    continue
                out.append({'id': index, 'type': 'hotspot', 'name': name, 'layer': 'card',
                            'x': round(x, 1), 'y': round(y, 1), 'width': round(width, 1), 'height': round(height, 1),
                            'enabled': True, 'hidden': False, 'script': text})
    return out


def atlas_layout(nodes):
    """Pack node rectangles by age, keeping the local order of the layout."""
    groups, area = [], 0
    for stack in export.GROUP_ORDER:
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


def adapt(graph, context, images):
    db = context['db']
    ids = {node['id']: index + 1 for index, node in enumerate(graph['nodes'])}
    analysis = graph['analysis']
    sets = {name: set(analysis[key]) for name, key in (('source', 'sources'), ('sink', 'sinks'),
                                                        ('isolated', 'isolated'), ('unreached', 'notReachedFromStart'))}
    zip_targets = {edge['target'] for edge in graph['edges'] if edge['kind'] == 'zip'}
    room_ids = {room['name']: room['id'] for room in graph['rooms']}
    dynamic = {}
    for item in graph['dynamicNavigation']:
        dynamic.setdefault(item['source'], []).append(item)
    hotspot_targets = {}
    for edge in graph['edges']:
        for evidence in edge['evidence']:
            if evidence['script'] == 'hotspot' and edge['kind'] != 'continuation':
                per_node = hotspot_targets.setdefault(edge['source'], {})
                names = per_node.setdefault(evidence['index'], [])
                if edge['target'] not in names:
                    names.append(edge['target'])
    returns = {entry['entry']: entry for entry in graph['engine']['savedReturns'] if entry.get('entry')}

    nodes = []
    for node in graph['nodes']:
        source = node['id']
        kind = node['type']
        stored = db.rooms[node['room']].nodes.get(node['node']) if node['room'] in db.rooms else None
        flags = ['card', TYPE_FLAG[kind]] + [flag for flag, values in sets.items() if source in values]
        if source in zip_targets:
            flags.append('zipDestination')
        if node['sharedWith']:
            flags.append('sharedScript')
        image = None
        picture_kind = 'cube' if kind == 'cube' else 'frame'
        for index in node['storedViews']:
            image = images.get(f'{node["room"]}:{index}:{picture_kind}')
            if image and not image.get('error'):
                break
            image = None
        if kind in ('cube', 'frame', 'menu') and image is None:
            flags.append('noPicture')
        if image:
            view_width, view_height = image['width'], image['height']
        else:
            view_width, view_height = (2048, 1024) if kind == 'cube' else (640, 480 if kind == 'menu' else 360)
        node_zones = zones(stored, kind, view_width, view_height, hotspot_targets.get(source, {})) if stored else []
        views = ', '.join(str(i) for i in node['viewIndexes'])
        if node['viewSelectors']:
            views += ' · chosen by ' + ', '.join(node['viewSelectors'])
        if node['dynamicViews']:
            views += (' · ' if views else '') + 'from ' + ', '.join(node['dynamicViews'])
        provenance = {'room': node['room'], 'roomNumber': room_ids.get(node['room']), 'node': node['node'],
                      'executable': graph['edition']['executable'], 'version': graph['edition']['version']}
        if stored:
            provenance['scriptBlocks'] = [{'kind': b.kind, 'fileOffset': b.offset, 'end': b.end,
                                           'sharedBy': b.node_ids} for b in stored.blocks]
        else:
            provenance['status'] = 'no node with this number is stored in the room'
        if image:
            provenance['picture'] = {key: image[key] for key in ('archive', 'index', 'kind', 'sourceSha256',
                                                                 'resources')}
        if source in returns:
            provenance['engineEntry'] = returns[source]
        item = {
            'id': ids[source], 'sourceId': source, 'name': node['label'], 'stack': node['group'],
            'originalName': node['typeLabel'] if not node['virtual'] else '', 'assetName': '',
            'room': node['room'], 'roomId': room_ids.get(node['room'], ''),
            'attributes': flags, 'x': node['x'], 'y': node['y'], 'originalX': node['x'], 'originalY': node['y'],
            'width': THUMB, 'height': round(THUMB * view_height / view_width) + 30 if image else 56,
            'viewWidth': view_width, 'viewHeight': view_height, 'zones': node_zones,
            'hotspotCount': node['hotspots'], 'imageCount': len(node['viewIndexes']), 'viewList': views,
            'sharedWith': ', '.join(f'{node["room"]}:{i}' for i in node['sharedWith']),
            'puzzles': ', '.join(str(p) for p in node['puzzles']), 'zipBit': node['zipBit'],
            'scriptText': export.node_listing(db, source, kind), 'provenance': provenance,
            'dynamicNavigation': dynamic.get(source, []),
        }
        if image:
            item.update({'image': image['image'], 'thumbnail': image['thumbnail'], 'imageWidth': image['width'],
                         'imageHeight': image['height'], 'assetName': f'{image["archive"]} · {image["index"]}'})
        nodes.append(item)

    virtual = {n['id'] for n in graph['nodes'] if n['virtual']}
    edges = []
    for edge in graph['edges']:
        flags = ['crossAge' if edge['crossAge'] else 'intraAge']
        if edge['self']:
            flags.append('selfReference')
        if edge['target'] in virtual:
            flags.append('notImplemented')
        if edge['kind'] == 'zip':
            flags.append('zip')
        elif edge['conditional']:
            flags.append('conditional')
        if edge['kind'] == 'native':
            flags.append('native')
        if edge['kind'] == 'auto':
            flags.append('automatic')
        if edge['kind'] == 'continuation':
            flags.append('sequenceContinuation')
        if edge['movie']:
            flags.append('movie')
        if edge['viaScript'] and edge['kind'] != 'continuation':
            flags.append('viaScript')
        edges.append({'source': ids[edge['source']], 'target': ids[edge['target']], 'kind': KIND[edge['kind']],
                      'exileKind': edge['kind'], 'cross_stack': edge['crossAge'], 'attributes': flags,
                      'evidence': edge['evidence'], 'transitivityId': 0})
    groups = atlas_layout(nodes)
    stats = graph['stats']
    return {'nodes': nodes, 'edges': edges,
            'meta': {'game': 'Myst III: Exile', 'version': graph['edition']['version'],
                     'cards': stats['places'], 'nodes': len(nodes), 'stacks': len(groups), 'edges': len(edges),
                     'images': sum(bool(n.get('image')) for n in nodes),
                     'missingImages': sum('noPicture' in n['attributes'] for n in nodes),
                     'zones': sum(len(n['zones']) for n in nodes), 'groups': groups,
                     'startNodeID': ids[graph['start']], 'defaultLayout': 'pdf',
                     'layoutAlgorithm': 'Topology-ordered nonoverlapping node atlas',
                     'layout': {'minX': 0, 'minY': 0, 'maxX': max(g['maxX'] for g in groups) + 80,
                                'maxY': max(g['maxY'] for g in groups) + 80}}}


def pdf_drawing(data, out):
    """Embed Graphviz drawing operations so that the graph view works offline."""
    lines = ['digraph Exile {', 'graph [bgcolor="white", overlap=true, splines=false, outputorder=edgesfirst];',
             'node [shape=box, style="rounded,filled", fontname="Helvetica", fontsize=8, fontcolor="#17221b", '
             'margin=".06,.03"];', 'edge [color="#697665", arrowsize=.45, penwidth=.6];']
    for node in data['nodes']:
        label = node['name'] + ('\n' + node['originalName'] if node['originalName'] else '')
        fill, style, stroke = export.COLORS[node['stack']], 'rounded,filled', '#55634c'
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
        elif edge['exileKind'] == 'continuation':
            head = 'empty'
        elif edge['exileKind'] == 'auto':
            style = 'dashed'
        lines.append(f'{edge["source"]} -> {edge["target"]} [exile_index="{index}", color="{color}", '
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
        original = data['edges'][int(edge['exile_index'])]
        drawing['edges'].append({'source': names[edge['tail']], 'target': names[edge['head']],
                                 'kind': original['kind'], 'graphIndex': int(edge['exile_index']),
                                 'attributes': original['attributes'], 'color': edge.get('color', '#697665'),
                                 'style': edge.get('style', ''), 'penWidth': float(edge.get('penwidth', .6)),
                                 'draw': operations(edge.get('_draw_')), 'head': operations(edge.get('_hdraw_')),
                                 'tail': operations(edge.get('_tdraw_')), 'labels': operations(edge.get('_ldraw_'))})
    data['pdfLayout'] = drawing
    cache.write_text(json.dumps(drawing, separators=(',', ':')) + '\n')
    cache_key.write_text(digest + '\n')


def write(out, graph, context, images):
    out = Path(out)
    data = adapt(graph, context, images)
    pdf_drawing(data, out)
    text = (WEB / 'viewer.html').read_text()
    payload = json.dumps(data, separators=(',', ':')).replace('<', '\\u003c')
    text = text.replace('{{STYLE}}', (WEB / 'viewer.css').read_text())
    text = text.replace('{{SCRIPT}}', (WEB / 'viewer.js').read_text())
    text = text.replace('{{GRAPH_JSON}}', payload)
    (out / 'index.html').write_text(text)
    (out / 'LICENSE.demystify.txt').write_text((WEB / 'LICENSE.demystify').read_text())
    return data
