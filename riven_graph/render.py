"""Dependency-free browser explorer and portable graph exports."""
from __future__ import annotations

import csv
import html
import json
import subprocess
import xml.etree.ElementTree as ET
from pathlib import Path

COLORS = {"aspit": "#94a3b8", "bspit": "#eb8f54", "gspit": "#72bb8c",
          "jspit": "#d6bb58", "ospit": "#ba8ee2", "pspit": "#72a9db",
          "rspit": "#e88eab", "tspit": "#55c9cc"}


def exports(out: Path, graph: dict) -> None:
    nodes, edges = graph["nodes"], graph["edges"]
    lines = ['digraph Riven {', 'graph [overlap=false, splines=true, bgcolor="#101823", outputorder=edgesfirst];',
             'node [shape=box, style="rounded,filled", fontname="Helvetica", fontsize=8, fontcolor="#111827"];',
             'edge [color="#7b879577", arrowsize=.4, penwidth=.5];']
    for n in nodes:
        label = n["label"] + ("\n" + n["name"] if n["name"] else "")
        lines.append(f'{json.dumps(n["id"])} [label={json.dumps(label)}, fillcolor="{COLORS.get(n["stack"], "#ffffff")}", style="{"dashed,filled" if n.get("virtual") else "rounded,filled"}"];')
    for e in edges:
        if e["kind"] == "zip":
            continue
        attrs = ' [color="#f2c17d", penwidth=1.2]' if e["cross_stack"] else ''
        lines.append(f'{json.dumps(e["source"])} -> {json.dumps(e["target"])}{attrs};')
    lines.append('}')
    (out / 'riven.dot').write_text('\n'.join(lines) + '\n')
    ns = 'http://graphml.graphdrawing.org/xmlns'
    ET.register_namespace('', ns)
    root = ET.Element(f'{{{ns}}}graphml')
    for key, scope in [('label', 'node'), ('stack', 'node'), ('name', 'node'), ('virtual', 'node'), ('kind', 'edge'), ('evidence', 'edge')]:
        ET.SubElement(root, 'key', id=key, **{'for': scope, 'attr.name': key, 'attr.type': 'string'})
    g = ET.SubElement(root, 'graph', id='Riven', edgedefault='directed')
    for n in nodes:
        el = ET.SubElement(g, 'node', id=n['id'])
        for key in ['label', 'stack', 'name', 'virtual']:
            ET.SubElement(el, 'data', key=key).text = str(n[key])
    for i, e in enumerate(edges):
        el = ET.SubElement(g, 'edge', id=f'e{i}', source=e['source'], target=e['target'])
        for key in ['kind', 'evidence']:
            ET.SubElement(el, 'data', key=key).text = json.dumps(e[key]) if key == 'evidence' else e[key]
    ET.ElementTree(root).write(out / 'riven.graphml', encoding='utf-8', xml_declaration=True)
    with (out / 'edges.csv').open('w', newline='') as f:
        w = csv.writer(f)
        w.writerow(['source', 'target', 'kind', 'cross_stack', 'evidence_count', 'evidence_json'])
        for e in edges:
            w.writerow([e['source'], e['target'], e['kind'], e['cross_stack'], len(e['evidence']), json.dumps(e['evidence'])])


def layout(out: Path, graph: dict) -> None:
    """Use Graphviz for deterministic precomputed positions when installed."""
    try:
        result = subprocess.run(['sfdp', '-Gstart=42', '-Grepulsiveforce=1.5', '-Gsplines=false', '-Tplain', str(out / 'riven.dot')],
                                capture_output=True, text=True, timeout=120, check=True)
        import shlex
        positions = {}
        for line in result.stdout.splitlines():
            fields = shlex.split(line)
            if fields and fields[0] == 'node':
                positions[fields[1]] = [float(fields[2]) * 90, -float(fields[3]) * 90]
        for n in graph['nodes']:
            if n['id'] in positions:
                n['x'], n['y'] = positions[n['id']]
    except (FileNotFoundError, subprocess.SubprocessError):
        import math
        for i, n in enumerate(graph['nodes']):
            a = i * math.pi * (3 - math.sqrt(5))
            n['x'], n['y'] = math.cos(a) * math.sqrt(i) * 15, math.sin(a) * math.sqrt(i) * 15


def explorer(out: Path, graph: dict, details: dict, ast: dict) -> None:
    from .web_export import write
    write(out, graph, details, ast)
