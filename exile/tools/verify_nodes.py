"""Check that every node of the game is read, against independent evidence.

1. ScummVM ships its own copy of the game's scripts (myst3.dat), produced by
   its authors from the executables. Each script list read here is compared
   with it byte for byte, and node by node where the bytes differ.
2. Every byte between the first and the last script list must belong to a
   list that the room table reaches: a gap would be a list nobody reads.
3. The executable is searched for room records that the age table omits.
4. Every cube and frame of the archives must belong to a scripted node or be
   reported.

Usage: python3 tools/verify_nodes.py DISCS [--dat /path/to/myst3.dat]
"""
import argparse
import json
import re
import struct
import sys
from collections import Counter
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from exile_graph import build, database, images  # noqa: E402

KINDS = {0: 'node', 1: 'ambient', 2: 'background-sound', 3: 'node-init', 4: 'ambient-cue'}


class Blob:
    """A byte string with the accessors the script reader expects."""

    def __init__(self, data, version):
        self.data, self.version = data, version

    def u16(self, offset):
        return struct.unpack_from('<H', self.data, offset)[0]

    def s16(self, offset):
        return struct.unpack_from('<h', self.data, offset)[0]


def read_dat(path):
    data = Path(path).read_bytes()
    if data[:4] != b'TSYM' and data[:4] != b'MYST':
        raise SystemExit('not a myst3.dat file')
    version = struct.unpack_from('<I', data, 4)[0]
    pos, indexes = 8, []
    for _ in range(7):
        count = struct.unpack_from('<I', data, pos)[0]
        pos += 4
        entries = []
        for _ in range(count):
            room = data[pos:pos + 4].decode('ascii')
            kind, offset, size = struct.unpack_from('<III', data, pos + 5)
            entries.append((room, kind, offset, size))
            pos += 17
        indexes.append(entries)
    for _ in range(2):
        count = struct.unpack_from('<I', data, pos)[0]
        pos += 4 + count * 36
    return version, indexes, data, pos


def _is_text(head):
    return len(head) >= 8 and all(32 <= c < 127 for c in head[:8])


def nodes_of(blob, version, kind):
    reader = database.Reader(Blob(blob + b'\0' * 64, version))
    room = database.Room(0, 'ROOM', 0, 0, 0, 0, 0, 0, 0)
    end = database._read_blocks(reader, room, kind, 0)
    return room, end


def signature(room):
    """What a list says, without the offsets it was read at."""
    out = []
    for block in room.blocks:
        out.append((tuple(block.node_ids),
                    tuple((s.condition, tuple((o.op, tuple(o.args)) for o in s.ops)) for s in block.scripts),
                    tuple((h.condition, tuple(h.rects), h.cursor, tuple((o.op, tuple(o.args)) for o in h.ops))
                          for h in block.hotspots)))
    return out


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('inputs', nargs='+')
    parser.add_argument('--dat', default='/opt/homebrew/share/scummvm/myst3.dat')
    parser.add_argument('--json', type=Path)
    args = parser.parse_args()
    graph, context = build.build(args.inputs)
    db, exe = context['db'], context['exe']
    report = {'edition': graph['edition']['version'], 'checks': {}}
    failures = 0

    # 1. ScummVM's copy of the scripts.
    if Path(args.dat).exists():
        dat_version, indexes, dat, base = read_dat(args.dat)
        main_index = {(room, KINDS[kind]): dat[base + offset:base + offset + size]
                      for room, kind, offset, size in indexes[0]}
        menu_rooms = {room for index in indexes[1:4] for room, *_ in index}
        same = different = absent = 0
        details = []
        for room in db.rooms.values():
            for kind, (start, end) in room.lists.items():
                mine = exe.data[start:end]
                theirs = main_index.get((room.name, kind))
                if theirs is None:
                    # Menu scripts exist once per localisation: six languages,
                    # two languages, English only.
                    variants = [dat[base + o:base + o + n] for index in indexes[1:4]
                                for r, k, o, n in index if r == room.name and KINDS[k] == kind]
                    if any(mine == v[:len(mine)] and not any(v[len(mine):]) for v in variants):
                        same += 1
                        details.append({'room': room.name, 'list': kind,
                                        'result': 'identical to one of the localised menu scripts'})
                        continue
                    if variants:
                        theirs = max(variants, key=len)
                if theirs is None:
                    absent += 1
                    details.append({'room': room.name, 'list': kind, 'result': 'not in the main index of myst3.dat'
                                    + (' (menu scripts are stored per language)' if room.name in menu_rooms else '')})
                    continue
                if mine == theirs[:len(mine)] and not any(theirs[len(mine):]):
                    same += 1
                    continue
                different += 1
                reference, _ = nodes_of(theirs, '1.27', kind)
                local, _ = nodes_of(mine, exe.version, kind)
                a, b = signature(local), signature(reference)
                ids_a = sorted({i for block in local.blocks for i in block.node_ids})
                ids_b = sorted({i for block in reference.blocks for i in block.node_ids})
                details.append({'room': room.name, 'list': kind, 'result': 'bytes differ',
                                'sameContent': a == b,
                                'nodesOnlyHere': sorted(set(ids_a) - set(ids_b)),
                                'nodesOnlyInDat': sorted(set(ids_b) - set(ids_a)),
                                'blocksHere': len(a), 'blocksInDat': len(b),
                                'changedBlocks': sum(1 for x, y in zip(a, b) if x != y) + abs(len(a) - len(b))})
        report['checks']['scummvm'] = {'datVersion': dat_version, 'identicalLists': same, 'differentLists': different,
                                       'listsNotInMainIndex': absent, 'details': details}
        dat_rooms = {room for room, *_ in indexes[0]} | menu_rooms
        report['checks']['scummvm']['roomsOnlyInDat'] = sorted(dat_rooms - set(db.rooms) - {'INIT'})
        report['checks']['scummvm']['roomsOnlyHere'] = sorted(set(db.rooms) - dat_rooms)
    else:
        report['checks']['scummvm'] = {'skipped': f'{args.dat} not found'}

    # 2. No unread bytes between the script lists.
    ranges = sorted((start, end, room.name, kind) for room in db.rooms.values()
                    for kind, (start, end) in room.lists.items())
    # The 32-byte room records sit between the lists: they are read too.
    records = [(room.offset, room.offset + 32) for room in db.rooms.values()]
    merged = []
    for start, end, *_ in sorted([r[:2] for r in ranges] + records):
        if merged and start <= merged[-1][1]:
            merged[-1][1] = max(merged[-1][1], end)
        else:
            merged.append([start, end])
    gaps = []
    for (_, previous_end), (next_start, _) in zip(merged, merged[1:]):
        chunk = exe.data[previous_end:next_start]
        gaps.append({'offset': previous_end, 'size': len(chunk), 'allZero': not any(chunk),
                     'head': chunk[:24].hex()})
    shared = Counter((start, end) for start, end, *_ in ranges)
    report['checks']['coverage'] = {
        'lists': len(ranges), 'bytes': sum(end - start for start, end, *_ in ranges),
        'first': merged[0][0], 'last': merged[-1][1],
        'listsReadByTwoRooms': [{'offset': start, 'rooms': [r for s, e, r, k in ranges if (s, e) == (start, end)]}
                                for (start, end), count in shared.items() if count > 1],
        # Lists are aligned on eight bytes; other data of the program (names of
        # shader parameters, tables) can sit between two rooms.
        'unexplainedGaps': [g for g in gaps if not g['allZero'] and not _is_text(bytes.fromhex(g['head']))],
        'otherDataBetweenLists': [g for g in gaps if not g['allZero'] and _is_text(bytes.fromhex(g['head']))],
        'paddingGaps': sum(1 for g in gaps if g['allZero'] and g['size'] <= 8),
        'largeZeroGaps': [g for g in gaps if g['allZero'] and g['size'] > 8]}
    failures += len(report['checks']['coverage']['unexplainedGaps']) + len(report['checks']['coverage']['largeZeroGaps'])

    # 3. Room records that the age table does not reach.
    known = {room.offset for room in db.rooms.values()}
    stray = []
    for match in re.finditer(rb'[A-Z]{4}\x00\x00\x00\x00', exe.data):
        offset = match.start() - 4
        if offset < 0 or offset in known:
            continue
        room_id = exe.u32(offset)
        pointers = struct.unpack_from('<3I', exe.data, offset + 12)
        if 100 < room_id < 2000 and all(p == 0 or exe.image_base < p < exe.image_base + len(exe.data) for p in pointers):
            stray.append({'offset': offset, 'id': room_id, 'name': match.group()[:4].decode()})
    report['checks']['strayRooms'] = stray
    failures += len(stray)

    # 4. Archives against nodes.
    walks = context['walks']
    selected = {}
    for (room, _), walk in walks.items():
        for view in walk.views:
            kind = 'cube' if view['type'] == 'cube' else 'frame'
            for index in view['indexes']:
                selected.setdefault((room, kind), set()).add(index)
    unselected = {}
    total = Counter()
    for room, index, kind, _, _ in images.views(context['archives']):
        total[kind] += 1
        if index not in selected.get((room, kind), ()):
            unselected.setdefault(f'{room} {kind}', []).append(index)
    missing = sorted(f'{n["id"]}' for n in graph['nodes'] if n['type'] in ('cube', 'frame', 'menu')
                     and n['viewIndexes'] and not n['storedViews'])
    place_ids = {n['id'] for n in graph['nodes']}
    helpers_with_zones = sorted(f'{room}:{node_id}' for (room, node_id), walk in walks.items()
                                if f'{room}:{node_id}' not in place_ids
                                and any(not h.background for h in db.rooms[room].nodes[node_id].hotspots))
    report['checks']['archives'] = {'storedViews': dict(total), 'storedButNeverSelected': unselected,
                                    'nodesWithoutStoredView': missing,
                                    'helperScriptsWithZones': helpers_with_zones}
    failures += len(helpers_with_zones)
    report['summary'] = {'scriptedNodes': graph['stats']['scriptedNodes'], 'graphNodes': graph['stats']['nodes'],
                         'helperScripts': graph['stats']['helperScripts'], 'failures': failures}
    text = json.dumps(report, indent=1)
    if args.json:
        args.json.write_text(text + '\n')
    print(text)
    context['source'].close()
    return 1 if failures else 0


if __name__ == '__main__':
    raise SystemExit(main())
