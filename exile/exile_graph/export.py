"""Write the graph and its evidence in portable formats."""
from __future__ import annotations

import csv
import hashlib
import json
import shlex
import subprocess
import xml.etree.ElementTree as ET
from pathlib import Path

from . import scripts as fmt

COLORS = {'Tomahna': '#d0af76', "J'nanin": '#b5b583', 'Edanna': '#8daf91', 'Voltaic': '#bd8e79',
          'Amateria': '#85a9b3', 'Narayan': '#b29abc', 'Menu and books': '#a4ad87', 'System': '#9aa5ad'}
GROUP_ORDER = ['Tomahna', "J'nanin", 'Voltaic', 'Amateria', 'Edanna', 'Narayan', 'Menu and books', 'System']
DRAWN_KINDS = ('move', 'auto', 'native', 'continuation')


def dot(graph, positioned=False):
    lines = ['digraph Exile {', 'graph [overlap=false, splines=true, bgcolor="white", outputorder=edgesfirst];',
             'node [shape=box, style="rounded,filled", fontname="Helvetica", fontsize=8, fontcolor="#17221b", '
             'margin=".06,.03"];',
             'edge [color="#697665", arrowsize=.45, penwidth=.6];']
    for node in graph['nodes']:
        style = 'dashed,filled' if node['virtual'] else 'rounded,filled'
        fill = '#ffffff' if node['virtual'] else COLORS[node['group']]
        position = f', pos="{node["x"]},{-node["y"]}!"' if positioned and 'x' in node else ''
        lines.append(f'{json.dumps(node["id"])} [label={json.dumps(node["label"] + chr(10) + node["typeLabel"])}, '
                     f'fillcolor="{fill}", style="{style}"{position}];')
    for index, edge in enumerate(graph['edges']):
        if edge['kind'] not in DRAWN_KINDS or edge['self']:
            continue
        attributes = [f'exile_index="{index}"']
        if edge['crossAge']:
            attributes.append('color="#9d7d3d", penwidth=1.1')
        if edge['kind'] == 'continuation':
            attributes.append('arrowhead=empty')
        elif edge['kind'] == 'native':
            attributes.append('color="#a089ae", style=dashed')
        elif edge['kind'] == 'auto':
            attributes.append('style=dashed')
        lines.append(f'{json.dumps(edge["source"])} -> {json.dumps(edge["target"])} [{", ".join(attributes)}];')
    lines.append('}')
    return '\n'.join(lines) + '\n'


def _layout_dot(graph):
    """A reduced graph for placement: unique pairs, with longer links between ages."""
    lines = ['digraph Exile {', 'graph [overlap=prism, sep="+4", splines=false, mode=major, model=shortpath, start=42];',
             'node [shape=box, fontsize=8, width=.6, height=.3, margin=".04,.02"];']
    for node in graph['nodes']:
        lines.append(f'{json.dumps(node["id"])} [label={json.dumps(node["label"])}];')
    seen = set()
    for edge in graph['edges']:
        pair = (edge['source'], edge['target'])
        if edge['kind'] in ('zip', 'continuation') or edge['self'] or pair in seen:
            continue
        seen.add(pair)
        length = ' [len=5, weight=0.3]' if edge['crossAge'] else ' [len=1.6]' if edge['crossRoom'] else ''
        lines.append(f'{json.dumps(edge["source"])} -> {json.dumps(edge["target"])}{length};')
    lines.append('}')
    return '\n'.join(lines) + '\n'


def layout(graph):
    """Place the nodes with Graphviz when it is installed, on a spiral otherwise.

    Neato keeps rooms together and sets the ages apart, as the Myst Graph
    does; sfdp is the second choice.
    """
    for name, command in (('Graphviz neato', ['neato']), ('Graphviz sfdp', ['sfdp', '-GK=0.6', '-Grepulsiveforce=1.6'])):
        try:
            result = subprocess.run([*command, '-Tplain'], input=_layout_dot(graph), capture_output=True, text=True,
                                    timeout=600, check=True)
        except (FileNotFoundError, subprocess.SubprocessError):
            continue
        positions = {}
        for line in result.stdout.splitlines():
            fields = shlex.split(line)
            if fields and fields[0] == 'node':
                positions[fields[1]] = (round(float(fields[2]) * 90, 2), round(-float(fields[3]) * 90, 2))
        for node in graph['nodes']:
            node['x'], node['y'] = positions.get(node['id'], (0.0, 0.0))
        return name
    import math
    for index, node in enumerate(graph['nodes']):
        angle = index * math.pi * (3 - math.sqrt(5))
        node['x'], node['y'] = math.cos(angle) * math.sqrt(index) * 60, math.sin(angle) * math.sqrt(index) * 60
    return 'spiral fallback'


def graphml(graph, path):
    namespace = 'http://graphml.graphdrawing.org/xmlns'
    ET.register_namespace('', namespace)
    root = ET.Element(f'{{{namespace}}}graphml')
    node_keys = ['label', 'room', 'group', 'type', 'virtual']
    edge_keys = ['kind', 'conditional', 'crossRoom', 'crossAge', 'evidence']
    for key in node_keys:
        ET.SubElement(root, 'key', id=key, **{'for': 'node', 'attr.name': key, 'attr.type': 'string'})
    for key in edge_keys:
        ET.SubElement(root, 'key', id=key, **{'for': 'edge', 'attr.name': key, 'attr.type': 'string'})
    element = ET.SubElement(root, 'graph', id='Exile', edgedefault='directed')
    for node in graph['nodes']:
        item = ET.SubElement(element, 'node', id=node['id'])
        for key in node_keys:
            ET.SubElement(item, 'data', key=key).text = str(node[key])
    for index, edge in enumerate(graph['edges']):
        item = ET.SubElement(element, 'edge', id=f'e{index}', source=edge['source'], target=edge['target'])
        for key in edge_keys:
            ET.SubElement(item, 'data', key=key).text = (json.dumps(edge[key]) if key == 'evidence'
                                                         else str(edge[key]))
    ET.ElementTree(root).write(path, encoding='utf-8', xml_declaration=True)


def node_listing(db, node_id, kind):
    room, number = node_id.split(':')
    node = db.rooms[room].nodes.get(int(number)) if room in db.rooms else None
    if node is None:
        return (f'{room} {number}\n  No node with this number is stored in room {room}.\n'
                '  It is kept in the graph because a script refers to it.\n')
    return fmt.node_text(node, kind)


def _ast(db):
    def ops(items):
        return [{'op': o.op, 'storedOp': o.raw_op, 'name': fmt.op_name(o.op), 'args': o.args, 'offset': o.offset}
                for o in items]
    rooms = []
    for room in db.rooms.values():
        blocks = []
        for block in room.blocks:
            blocks.append({
                'kind': block.kind, 'index': block.index, 'nodes': block.node_ids, 'offset': block.offset,
                'end': block.end,
                'scripts': [{'condition': s.condition, 'when': fmt.condition(s.condition), 'offset': s.offset,
                             'repaired': s.repaired, 'ops': ops(s.ops)} for s in block.scripts],
                'hotspots': [{'condition': h.condition, 'when': fmt.condition(h.condition), 'offset': h.offset,
                              'cursor': h.cursor, 'rects': [list(r) for r in h.rects], 'ops': ops(h.ops)}
                             for h in block.hotspots]})
        rooms.append({'name': room.name, 'id': room.id, 'age': room.age_id, 'offset': room.offset,
                      'storedZipBit': room.stored_zip_bit, 'unknown': room.unknown, 'blocks': blocks})
    return rooms


def write(out, graph, context):
    """Write every export of the graph into ``out``."""
    out = Path(out)
    out.mkdir(parents=True, exist_ok=True)
    db, exe = context['db'], context['exe']
    graph['layoutAlgorithm'] = layout(graph)
    (out / 'graph.json').write_text(json.dumps(graph, separators=(',', ':')) + '\n')
    (out / 'exile.dot').write_text(dot(graph))
    graphml(graph, out / 'exile.graphml')
    with (out / 'edges.csv').open('w', newline='') as stream:
        writer = csv.writer(stream)
        writer.writerow(['source', 'target', 'kind', 'conditional', 'cross_room', 'cross_age', 'evidence_count',
                         'evidence_json'])
        for edge in graph['edges']:
            writer.writerow([edge['source'], edge['target'], edge['kind'], edge['conditional'], edge['crossRoom'],
                             edge['crossAge'], len(edge['evidence']), json.dumps(edge['evidence'])])

    kinds = {node['id']: node['type'] for node in graph['nodes']}
    for room in db.rooms.values():
        folder = out / 'scripts' / room.name
        folder.mkdir(parents=True, exist_ok=True)
        for node in room.nodes.values():
            (folder / f'{node.id}.txt').write_text(fmt.node_text(node, kinds.get(f'{room.name}:{node.id}', 'cube')))
    (out / 'scripts' / 'node-init.txt').write_text(
        'Run before every node change\n' + '\n'.join(fmt.lines(db.node_init_script)) + '\n')
    (out / 'nodes.json').write_text(json.dumps(
        {'nodeInitScript': [{'op': o.op, 'name': fmt.op_name(o.op), 'args': o.args, 'offset': o.offset}
                            for o in db.node_init_script],
         'soundNames': db.sound_names, 'rooms': _ast(db)}, separators=(',', ':')) + '\n')

    manifest = {
        'edition': graph['edition'],
        'executable': {
            'imageBase': exe.image_base, 'ageTableOffset': exe.age_table, 'ageCount': exe.age_count,
            'nodeInitScriptOffset': exe.node_init_script, 'soundNamesOffset': exe.sound_names,
            'sections': [{'name': s.name, 'virtualAddress': s.virtual_address, 'rawOffset': s.raw_offset,
                          'rawSize': s.raw_size} for s in exe.sections],
            'decodedSha256': hashlib.sha256(exe.data).hexdigest(),
            'note': 'Offsets of scripts in the exports are file offsets into the decoded executable.'},
        'archives': context['archive_info'],
        'warnings': db.warnings,
        'repairedScripts': [{'room': room.name, 'nodes': block.node_ids, 'offset': script.offset}
                            for room in db.rooms.values() for block in room.blocks
                            for script in block.scripts if script.repaired],
    }
    (out / 'manifest.json').write_text(json.dumps(manifest, indent=1) + '\n')
    (out / 'analysis.md').write_text(analysis_text(graph, manifest))


def analysis_text(graph, manifest):
    stats, analysis, edition = graph['stats'], graph['analysis'], graph['edition']
    kinds = stats['edgeKinds']
    types = stats['placeTypes']
    rows = '\n'.join(f'| {room["group"]} | {room["name"]} | {room["id"]} | {room["scriptedNodes"]} | '
                     f'{room["places"]} |' for room in graph['rooms'])
    kind_rows = '\n'.join(f'| {kind} | {kinds[kind]:,} |' for kind in sorted(kinds))
    absent = ', '.join(n['id'] for n in graph['nodes'] if n['virtual']) or 'none'
    repaired = manifest['repairedScripts']
    return f"""# Myst III: Exile ({edition['version']}, {edition['edition']}): extracted node graph

Generated from the supplied discs. This is a reading of the game's script data, taken from its executable;
it is not recovered source code, and no script is executed.

**{stats['nodes']:,} nodes, {stats['edges']:,} typed directed edges, {stats['hotspots']:,} hotspots,
{stats['instructions']:,} decoded instructions.**

- Executable: `{edition['executable']}`, version {edition['version']} ({edition['language']}),
  SHA-256 `{edition['executableSha256']}`.
- Protection: {edition['safedisc'] or 'none'}. {' '.join(edition['notes'])}
- {stats['scriptedNodes']:,} scripted nodes: {types.get('cube', 0):,} panoramas, {types.get('frame', 0):,} frames,
  {types.get('menu', 0):,} menu screens, {types.get('native', 0) + types.get('transit', 0):,} script-only or
  engine-drawn stops, and {stats['helperScripts']:,} helper scripts that are called and never visited.
- Absent targets kept as virtual nodes: {absent}.

| Age | Room | Room number | Scripted nodes | Places in the graph |
|---|---|---:|---:|---:|
{rows}

## What an edge means

| Kind | Edges |
|---|---:|
{kind_rows}

- **move**: a hotspot script of the source requests the target.
- **auto**: a script that runs on arrival or on every frame requests the target.
- **zip**: a Zip shortcut. The engine only offers it once the target has been visited.
- **continuation**: one script requests two places in a row; the edge joins the first to the second.
- **native**: a transition performed by the engine, attributed to ScummVM.

{stats['conditionalEdges']:,} edges are conditional: their hotspot, their script or a test before the instruction
depends on a variable. The tests are kept as text in the evidence. {stats['reloads']:,} instructions reload the
current node (usually after a movie) and are not edges. {stats['dynamicReferences']:,} instructions take their
destination from a variable: they are listed under `dynamicNavigation` in `graph.json`.

Helper scripts are followed: when a hotspot calls a script node that performs the move, the edge starts at the
place that owns the hotspot, and the evidence names the helper.

## Structure

The movement projection has {analysis['uniquePairs']:,} unique ordered pairs, of which
{analysis['reciprocalShare'] * 100:.1f}% have their reverse. {analysis['crossRoomPairs']:,} pairs join two rooms
and {analysis['crossAgePairs']:,} join two ages. It has {analysis['weakComponents']:,} weakly connected components
(the largest holds {analysis['largestWeakComponent']:,} nodes) and {analysis['strongComponents']:,} strongly
connected components (the largest holds {analysis['largestStrongComponent']:,}).

From {graph['start']}, where a new game begins, {analysis['reachableFromStart']:,} nodes are reached. The most
distant is {', '.join(analysis['mostDistantFromStart'])}, {analysis['maximumDistanceFromStart']} references away.
The largest shortest distance between two nodes is {analysis['largestShortestDistance']}
({' to '.join(analysis['largestShortestDistancePair'])}).

- Sources ({len(analysis['sources'])}): {', '.join(analysis['sources']) or 'none'}
- Sinks ({len(analysis['sinks'])}): {', '.join(analysis['sinks']) or 'none'}
- Isolated ({len(analysis['isolated'])}): {', '.join(analysis['isolated']) or 'none'}
- Not reached from the start ({len(analysis['notReachedFromStart'])}):
  {', '.join(analysis['notReachedFromStart']) or 'none'}

These are properties of the projection. {analysis['caveat']}

## Verification and limits

- Every script list is read to its terminator; {stats['scriptBlocks']:,} script blocks were decoded with
  {stats['missingCalls']} calls to missing script nodes and {stats['deadCode']} unreachable script tails.
- {len(repaired)} stored script needed the repair that ScummVM also applies (a script of MATO 32765 that lacks
  its first two bytes).
- Engine behaviour (menu actions, saved returns, book items) follows the ScummVM implementation at a pinned
  commit, not the original interpreter. Saved returns are metadata, not edges.
- Puzzle helpers written in native code (`runPuzzle`) are listed per node and not interpreted.
- A panorama is one node: turning inside it is not an edge.
"""
