"""Run with python3 -m riven_graph INPUT [INPUT ...] --output generated."""
import argparse
from .build import build, write_outputs


def main():
    parser = argparse.ArgumentParser(description='Disassemble original Riven Mohawk scripts and generate their card graph.')
    parser.add_argument('inputs', nargs='+', help='ISO images, MHK files, or directories containing them')
    parser.add_argument('--output', default='generated/riven', help='Output directory (default: generated/riven)')
    args = parser.parse_args()
    graph, details, manifest, ast = build(args.inputs)
    write_outputs(args.output, graph, details, manifest, ast)
    print(f'{graph["stats"]["card_count"]} cards, {graph["stats"]["edge_count"]} typed edges; {args.output}/index.html')


if __name__ == '__main__':
    main()
