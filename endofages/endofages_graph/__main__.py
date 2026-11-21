"""Command line: python3 -m endofages_graph <game folder> --output generated"""
import argparse
import collections
import json
import os
import sys

from . import analyze, build, freeroam


def main(argv=None):
    parser = argparse.ArgumentParser(prog='endofages_graph',
                                     description='Extract the Classic-mode graph of Myst V: End of Ages.')
    parser.add_argument('game', help="the game's folder, holding dat, Python, SDL and avi")
    parser.add_argument('--output', default='generated', help='folder to write (default: generated)')
    parser.add_argument('--no-freeroam', action='store_true',
                        help='skip the comparison with the collision floors (about thirty seconds)')
    args = parser.parse_args(argv)
    if not os.path.isdir(os.path.join(args.game, 'dat')):
        print(f'endofages_graph: no dat folder in {args.game!r}', file=sys.stderr)
        return 1
    os.makedirs(args.output, exist_ok=True)
    graph = build.build(args.game)
    _write(args.output, 'graph.json', graph)
    floors = None
    if not args.no_freeroam:
        floors = freeroam.compare(args.game, graph)
        _write(args.output, 'freeroam.json', floors)
    result = analyze.analyze(graph, floors)
    _write(args.output, 'analysis.json', result)
    kinds = collections.Counter(e['kind'] for e in graph['edges'])
    possible = result['readings']['possible']
    print(f"Myst V: End of Ages, {result['classicNodes']:,} Classic points, {len(graph['edges']):,} edges "
          f"({', '.join(f'{k} {v:,}' for k, v in sorted(kinds.items()))})")
    print(f"from {graph['start']}: {possible['reached']:,} of {possible['of']:,} reached, farthest at "
          f"{possible['farthest']['hops']}; largest shortest distance {possible['diameter']['hops']}")
    return 0


def _write(folder, name, data):
    with open(os.path.join(folder, name), 'w') as f:
        json.dump(data, f, indent=1)


if __name__ == '__main__':
    sys.exit(main())
