"""Command line: python3 -m exile_graph <folder or disc images> --output generated"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from . import build, export, images, web_export


def main(argv=None):
    parser = argparse.ArgumentParser(prog='exile_graph', description='Extract the node graph of Myst III: Exile.')
    parser.add_argument('inputs', nargs='+', help='folder holding M3.exe and the node archives, or CD images')
    parser.add_argument('--output', default='generated', help='folder to write (default: generated)')
    parser.add_argument('--no-images', action='store_true', help='skip the extraction of the pictures')
    parser.add_argument('--no-web', action='store_true', help='skip the browser explorer')
    args = parser.parse_args(argv)
    out = Path(args.output)
    try:
        graph, context = build.build(args.inputs)
    except (ValueError, OSError) as error:
        print(f'exile_graph: {error}', file=sys.stderr)
        return 1
    export.write(out, graph, context)
    stats, analysis, edition = graph['stats'], graph['analysis'], graph['edition']
    print(f'Myst III: Exile {edition["version"]} ({edition["language"]}, {edition["edition"]})')
    print(f'{stats["nodes"]:,} nodes, {stats["edges"]:,} typed edges, {stats["hotspots"]:,} hotspots, '
          f'{stats["instructions"]:,} instructions')
    print(f'sources {len(analysis["sources"])}, sinks {len(analysis["sinks"])}, isolated {len(analysis["isolated"])}; '
          f'largest shortest distance {analysis["largestShortestDistance"]} '
          f'({" to ".join(analysis["largestShortestDistancePair"])})')
    manifest = {}
    manifest_path = out / 'images-manifest.json'
    if not args.no_images:
        try:
            manifest = images.extract(context['archives'], out)
            failed = sum(1 for record in manifest.values() if record.get('error'))
            print(f'{len(manifest) - failed:,} pictures written, {failed} failed')
        except RuntimeError as error:
            print(f'exile_graph: pictures skipped: {error}', file=sys.stderr)
    elif manifest_path.exists():
        manifest = json.loads(manifest_path.read_text())['views']
    if not args.no_web:
        data = web_export.write(out, graph, context, manifest)
        print(f'Explorer: {out / "index.html"} ({data["meta"]["images"]:,} nodes with a picture, '
              f'{data["meta"]["zones"]:,} zones)')
    context['source'].close()
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
