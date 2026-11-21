"""Write the graph and its evidence: JSON, one listing per node, CSV, DOT and a summary."""
from __future__ import annotations

import csv
import hashlib
import json
from pathlib import Path

from . import build, resources


def listing(node, table):
    """The readable record of one node: what it stores, its hotspots, its class parameters and its command blocks."""
    info = node.info or {}
    out = [node.id, f'  archive {node.path} @{node.offset:#x}, {node.size:,} bytes',
           f'  viewport {info.get("viewport", "?")}; position {", ".join(f"{v:.3f}" for v in info.get("values", [])[:3])}; '
           f'camera {", ".join(f"{v:g}" for v in info.get("values", [])[3:6])}']
    for arrival in info.get('arrivals', []):
        out.append(f'  arrival from {arrival["entry"]}: pitch {arrival["pitch"]:.1f}, yaw {arrival["yaw"]:.1f}')
    if node.layers:
        out.append('  picture layers: ' + ', '.join(layer['name'] for layer in node.layers))
    for spot in node.hotspots:
        place = f' at {spot["offset"]}' if 'offset' in spot else ''
        out.append(f'  hotspot {spot["name"]}{place}')
    for path, text in sorted(node.ai.items()):
        out.append(f'  class {path.split("/")[1]}  ({path})')
        for name, kind, value in resources.ai(text):
            out.append(f'    {name} = {value}')
    for name, block in sorted(node.blocks.items()):
        started = '; '.join(sorted({describe(t) for t in table.get(name, [])})) or 'nothing stored starts it'
        out.append(f'  block {name}  @{block["offset"]:#x}  [{started}]')
        out.extend(f'    {command}' for command in block['commands'])
    return '\n'.join(out) + '\n'


def describe(trigger):
    by = trigger['by']
    if by == 'event':
        return f'{trigger["event"]} on {trigger["hotspot"]}'
    if by == 'video marker':
        return f'frame {trigger["frame"]} of {trigger["video"]}'
    if by == 'class':
        return f'{trigger["class"]}.{trigger["parameter"]}'
    if by == 'block':
        return f'block {trigger["block"]}'
    return 'on arrival'


def dot(graph):
    lines = ['digraph revelation {', '  node [shape=box, style="rounded,filled", fontname="Arial"];']
    for node in graph['nodes']:
        lines.append(f'  "{node["id"]}" [label="{node["id"]}\\n{node["type"]}"];')
    style = {'move': '', 'auto': ' [style=dashed]', 'return': ' [color=gray40]', 'engine': ' [style=dashed, color=purple]',
             'amulet': ' [style=dashed, color=goldenrod]', 'unreferenced': ' [style=dotted]'}
    for edge in graph['edges']:
        lines.append(f'  "{edge["source"]}" -> "{edge["target"]}"{style[edge["kind"]]};')
    lines.append('}')
    return '\n'.join(lines) + '\n'


def analysis_text(graph, manifest):
    stats, analysis = graph['stats'], graph['analysis']
    kinds = ', '.join(f'{count:,} {kind}' for kind, count in stats['edgeKinds'].items())
    rows = [
        '# Myst IV: Revelation, measured results', '',
        f'Source: `{manifest["data"]["container"]}` (SHA-1 `{manifest["data"].get("containerSha1", "not computed")}`), '
        f'`{manifest["data"]["path"]}`.', '',
        f'- {stats["stored"]:,} stored nodes ({stats["panoramas"]:,} panoramas, {stats["fixedViews"]:,} fixed views) and '
        f'{stats["absent"]} absent targets.',
        f'- {stats["edges"]:,} typed edges: {kinds}; {stats["conditional"]} conditional, {stats["inferred"]} inferred, '
        f'{stats["crossAge"]} between worlds, {stats["movies"]} with a transition movie.',
        f'- {stats["commandBlocks"]:,} command blocks, {stats["commands"]:,} commands, {stats["hotspots"]:,} named hotspots, '
        f'{stats["zipPlaces"]} Zip places.',
        f'- Movement projection: {analysis["uniquePairs"]:,} ordered pairs, {analysis["reciprocalShare"]:.1%} with their reverse.',
        f'- From `{analysis["start"]}`: {analysis["reachableFromStart"]:,} nodes reached, the farthest at '
        f'{analysis["maximumDistanceFromStart"]} ({", ".join(analysis["mostDistantFromStart"])}).',
        f'- Largest shortest distance: {analysis["largestShortestDistance"]}.',
        f'- Sources {len(analysis["sources"])}, sinks {len(analysis["sinks"])}, isolated {len(analysis["isolated"])}.',
        f'- Compiled libraries: {graph["compiledDestinations"]["found"]} node changes with a constant destination, '
        f'{graph["compiledDestinations"]["computed"]} computed.', '',
        analysis['caveat'], '']
    return '\n'.join(rows)


def write(out, graph, loaded):
    out = Path(out)
    (out / 'scripts').mkdir(parents=True, exist_ok=True)
    ref = loaded['ref']
    origin = dict(ref.origin)
    manifest = {'title': graph['title'], 'data': origin, 'dataSize': ref.size, 'tableBytes': loaded['header'],
                'archives': len(loaded['toc']), 'libraries': loaded['libraries'], 'boot': loaded['boot']}
    (out / 'graph.json').write_text(json.dumps(graph, indent=1))
    for key, node in sorted(loaded['nodes'].items()):
        folder = out / 'scripts' / f'w{key[0]}'
        folder.mkdir(exist_ok=True)
        (folder / f'{node.id}.txt').write_text(listing(node, build.triggers(node)))
    with (out / 'edges.csv').open('w', newline='') as stream:
        writer = csv.writer(stream)
        writer.writerow(['source', 'target', 'kind', 'conditional', 'inferred', 'crossAge', 'via', 'instruction'])
        for edge in graph['edges']:
            first = edge['evidence'][0]
            writer.writerow([edge['source'], edge['target'], edge['kind'], edge['conditional'],
                             any(e.get('inferred') for e in edge['evidence']), edge['crossAge'], first['via'],
                             first.get('instruction', first.get('hotspot', ''))])
    (out / 'revelation.dot').write_text(dot(graph))
    (out / 'manifest.json').write_text(json.dumps(manifest, indent=1))
    (out / 'analysis.md').write_text(analysis_text(graph, manifest))
    return manifest
