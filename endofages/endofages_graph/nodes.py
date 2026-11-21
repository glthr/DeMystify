"""Myst V's Classic navigation: the `<Age>.node` files read by the engine's `pfNodeMgr`.

Format (version 5), established on 2026-11-18 by reading every byte of the eight files and checked against the
reader in `eoa.exe` (the routine at 0x7c7ed0 opens `<dat>/<Age>.node` as an enciphered stream, reads a version and a
count, then per node a position and a count of links, per link a target index, from version 3 a byte, from version 5
three floats): u32 version, u32 node count; per node: three floats (x, y, z in the Age's coordinates, Z up), u32 link
count; per link: u32 target index, u8 type, three floats (zero on most links; else a vector, mostly 15 units long).
"""
import os
import struct

from . import crypt


def read_nodes(path):
    b = crypt.read_file(path)
    ver, n = struct.unpack_from('<II', b, 0)
    o = 8
    nodes = []
    for i in range(n):
        x, y, z, c = struct.unpack_from('<fffI', b, o)
        o += 16
        links = []
        for _ in range(c):
            t = struct.unpack_from('<I', b, o)[0]
            typ = b[o + 4]
            v = struct.unpack_from('<fff', b, o + 5)
            o += 17
            links.append({'target': t, 'type': typ, 'vector': list(v)})
        nodes.append({'index': i, 'pos': [x, y, z], 'links': links})
    if o != len(b):
        raise ValueError(f'{path}: read {o} of {len(b)} bytes')
    for nd in nodes:
        for l in nd['links']:
            if not 0 <= l['target'] < n:
                raise ValueError(f'{path}: link to {l["target"]} out of range')
    return {'version': ver, 'nodes': nodes}


def components(nodes):
    """Connected components of the links taken both ways: a list of sets of node indexes."""
    adj = {nd['index']: set() for nd in nodes}
    for nd in nodes:
        for l in nd['links']:
            adj[nd['index']].add(l['target'])
            adj[l['target']].add(nd['index'])
    seen, comps = set(), []
    for s in adj:
        if s in seen:
            continue
        stack, comp = [s], set()
        while stack:
            u = stack.pop()
            if u in comp:
                continue
            comp.add(u)
            stack.extend(adj[u] - comp)
        seen |= comp
        comps.append(comp)
    return sorted(comps, key=len, reverse=True)


def closest(nodes, pos, among=None):
    best = None
    for nd in nodes:
        if among is not None and nd['index'] not in among:
            continue
        d = sum((a - b) ** 2 for a, b in zip(nd['pos'], pos)) ** 0.5
        if best is None or d < best[1]:
            best = (nd['index'], d)
    return best
