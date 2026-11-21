"""Command line: python3 -m revelation_graph <folder or disc images> --output generated"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from . import build, export


def main(argv=None):
    parser = argparse.ArgumentParser(prog='revelation_graph', description='Extract the node graph of Myst IV: Revelation.')
    parser.add_argument('inputs', nargs='+', help='folder holding data.m4b, or DVD images')
    parser.add_argument('--output', default='generated', help='folder to write (default: generated)')
    parser.add_argument('--no-images', action='store_true', help='skip the extraction of the pictures')
    parser.add_argument('--no-web', action='store_true', help='skip the browser explorer')
    args = parser.parse_args(argv)
    out = Path(args.output)
    try:
        graph, loaded = build.build(args.inputs)
    except (ValueError, OSError) as error:
        print(f'revelation_graph: {error}', file=sys.stderr)
        return 1
    export.write(out, graph, loaded)
    stats, analysis = graph['stats'], graph['analysis']
    print(f'Myst IV: Revelation, {stats["stored"]:,} stored nodes, {stats["absent"]} absent targets, {stats["edges"]:,} typed edges')
    print(f'from {analysis["start"]}: {analysis["reachableFromStart"]:,} reached, farthest at {analysis["maximumDistanceFromStart"]}; '
          f'largest shortest distance {analysis["largestShortestDistance"]}')
    if graph['compiledDestinations']['notice']:
        print('revelation_graph: ' + graph['compiledDestinations']['notice'], file=sys.stderr)
    manifest = {}
    manifest_path = out / 'images-manifest.json'
    if not args.no_images:
        from . import images
        try:
            manifest = images.extract(loaded, out)
            failed = sum(1 for record in manifest.values() if record.get('error'))
            print(f'{len(manifest) - failed:,} pictures written, {failed} failed')
        except RuntimeError as error:
            print(f'revelation_graph: pictures skipped: {error}', file=sys.stderr)
    elif manifest_path.exists():
        manifest = json.loads(manifest_path.read_text())['views']
    if not args.no_web:
        from . import web_export
        data = web_export.write(out, graph, loaded, manifest)
        print(f'Explorer: {out / "index.html"} ({data["meta"]["images"]:,} nodes with a picture)')
    loaded['source'].close()
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
