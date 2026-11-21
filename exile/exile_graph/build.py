"""Build the node graph of Myst III: Exile, keeping the evidence of every edge."""
from __future__ import annotations

import hashlib
from collections import Counter, defaultdict, deque

from . import archive as m3a
from . import database, disc, executable, native
from . import scripts as fmt
from .walker import Walker

# The game groups its rooms into ages. Tomahna appears twice: the opening
# and the return.
GROUPS = {1: 'System', 2: 'System', 3: 'Tomahna', 4: 'Tomahna', 5: "J'nanin", 6: 'Edanna', 7: 'Voltaic',
          8: 'Narayan', 9: 'Menu and books', 10: 'Amateria', 11: 'System'}
VIEW_LABEL = {'cube': 'panorama', 'frame': 'frame', 'menu': 'menu screen', 'native': 'engine-drawn view',
              'transit': 'script only', 'absent': 'absent node'}


class Archives:
    """The node archives by room, and the archives shared by several rooms."""

    def __init__(self):
        self.rooms = {}
        self.shared = []

    def entries(self, room, index):
        """Yield (archive, entry) for every archive holding this room and index."""
        opened = self.rooms.get(room)
        if opened and opened.get(room, index):
            yield opened, opened.get(room, index)
        for opened in self.shared:
            if opened.get(room, index):
                yield opened, opened.get(room, index)

    def view(self, room, index, kind):
        """The archive and resources of the picture of a view, or None."""
        for opened, entry in self.entries(room, index):
            if kind == 'cube':
                faces = [entry.find(m3a.CUBE_FACE, face) for face in range(1, 7)]
                if all(faces):
                    return opened, [f[0] for f in faces]
            else:
                frames = entry.find(m3a.FRAME) or entry.find(70)
                if frames:
                    return opened, frames[:1]
        return None


def node_key(room, node):
    return f'{room}:{node}'


def load(inputs):
    source = disc.Source(inputs)
    candidates = source.find('M3.exe')
    if not candidates:
        raise ValueError('M3.exe was not found in the supplied folder or disc images')
    exe_ref = candidates[0]
    raw = exe_ref.read()
    exe = executable.load(raw)
    db = database.load(exe)
    return source, exe_ref, raw, exe, db


def _evidence(reference):
    origin = reference['origin']
    item = {'script': origin['kind'], 'index': origin['index'], 'when': origin['condition'],
            'instruction': reference['text'], 'offset': reference['offset']}
    if 'cursor' in origin:
        item['cursor'] = origin['cursor']
    if reference['via']:
        item['via'] = reference['via']
    if reference['guards']:
        item['guards'] = reference['guards']
    if 'movie' in reference:
        item['movie'] = reference['movie']
    if 'age' in reference:
        item['nextAge'] = reference['age']
    return item


def _kind(reference):
    if reference['zip']:
        return 'zip'
    return {'hotspot': 'move', 'init': 'auto', 'background': 'auto', 'native': 'native'}[reference['origin']['kind']]


def _conditional(reference):
    return bool(reference['origin']['conditional'] or reference['guards'])


def build(inputs):
    source, exe_ref, raw, exe, db = load(inputs)
    walker = Walker(db)
    walks = {}
    for room in db.rooms.values():
        for node in room.nodes.values():
            walks[(room.name, node.id)] = walker.walk_node(node)

    # Navigation that the engine performs itself, attributed to ScummVM.
    native_references = native.references(db, walker)

    archives, archive_info = Archives(), []
    for room_name, refs in sorted(source.node_archives().items()):
        ref = refs[0]
        opened = m3a.Archive(ref, room_name)
        archives.rooms[room_name] = opened
        archive_info.append({'room': room_name, 'file': ref.name, 'size': ref.size, 'encrypted': opened.encrypted,
                             'entries': len(opened.entries), **ref.origin,
                             'duplicates': [r.origin for r in refs[1:]]})
    for ref in source.files:
        # Archives shared by several rooms: common resources and the texts
        # and pictures of one language (menus, journals).
        if ref.name.lower().endswith(('.m3r', '.m3t', '.m3u')):
            try:
                opened = m3a.Archive(ref)
            except (m3a.ArchiveError, ValueError):
                continue
            archives.shared.append(opened)
            archive_info.append({'room': None, 'file': ref.name, 'size': ref.size, 'encrypted': opened.encrypted,
                                 'entries': len(opened.entries), **ref.origin, 'duplicates': []})

    # -- which nodes are places -------------------------------------------
    targeted = Counter()
    for key, walk in walks.items():
        for reference in walk.references:
            if reference['target'] and not reference['unreachable']:
                targeted[reference['target']] += 1
    for reference in native_references:
        if reference['target']:
            targeted[reference['target']] += 1

    room_age = {room.name: room.age_id for room in db.rooms.values()}
    nodes, graph_keys = [], set()
    helper_count = 0
    for (room_name, node_id), walk in sorted(walks.items()):
        node = db.rooms[room_name].nodes[node_id]
        views = walk.views
        if views:
            kind = views[0]['type']
        elif (room_name, node_id) in targeted or (room_name, node_id) == native.BOOT:
            kind = 'native' if any(p['origin']['kind'] == 'init' for p in walk.puzzles) and node.hotspots else 'transit'
        else:
            helper_count += 1
            continue
        graph_keys.add((room_name, node_id))
        indexes = sorted({i for view in views for i in view['indexes']})
        stored = [i for i in indexes if archives.view(room_name, i, kind)]
        nodes.append({
            'id': node_key(room_name, node_id), 'room': room_name, 'node': node_id, 'age': room_age[room_name],
            'group': GROUPS.get(room_age[room_name], 'System'), 'label': node_key(room_name, node_id),
            'type': kind, 'typeLabel': VIEW_LABEL[kind], 'virtual': False,
            'viewIndexes': indexes, 'storedViews': stored,
            'viewSelectors': sorted({v['selector'] for v in views if v['selector']}),
            'dynamicViews': sorted({d for v in views for d in v['dynamic']}),
            'initScripts': len(node.scripts),
            'hotspots': sum(1 for h in node.hotspots if not h.background),
            'backgroundScripts': sum(1 for h in node.hotspots if h.background),
            'zipHotspots': sum(1 for h in node.hotspots if h.cursor == 7 and not h.background),
            'zipBit': node.zip_bit, 'sharedWith': sorted({i for b in node.blocks if b.kind == 'node'
                                                          for i in b.node_ids if i != node_id}),
            'puzzles': sorted({p['puzzle'] for p in walk.puzzles if p['puzzle'] is not None}),
        })
    absent = sorted(t for t in targeted if t not in walks)
    for room_name, node_id in absent:
        graph_keys.add((room_name, node_id))
        age = room_age.get(room_name, 0)
        nodes.append({'id': node_key(room_name, node_id), 'room': room_name, 'node': node_id, 'age': age,
                      'group': GROUPS.get(age, 'System'), 'label': node_key(room_name, node_id),
                      'type': 'absent', 'typeLabel': VIEW_LABEL['absent'], 'virtual': True,
                      'viewIndexes': [], 'storedViews': [], 'viewSelectors': [], 'dynamicViews': [],
                      'initScripts': 0, 'hotspots': 0, 'backgroundScripts': 0, 'zipHotspots': 0, 'zipBit': -1,
                      'sharedWith': [], 'puzzles': []})

    # -- edges ------------------------------------------------------------
    merged, dynamic, reload_count, unreachable = {}, [], 0, []
    sources = [(key, reference) for key, walk in walks.items() if key in graph_keys
               for reference in walk.references]
    sources += [(reference['source'], reference) for reference in native_references]
    for key, reference in sources:
        if reference['target'] is None:
            if (key[0] == native.TESLA_ROOM and key[1] in native.TESLA_NODES
                    and reference['dynamic']['node'] == native.TESLA_VARIABLE):
                continue  # resolved by the engine's puzzle routine: see native.tesla_returns
            dynamic.append({'source': node_key(*key), 'kind': _kind(reference), 'destination': reference['dynamic'],
                            **_evidence(reference)})
            continue
        if reference['unreachable']:
            unreachable.append({'source': node_key(*key), 'target': node_key(*reference['target']),
                                **_evidence(reference)})
            continue
        kind, conditional = _kind(reference), _conditional(reference)
        edge_key = (key, reference['target'], kind, conditional)
        edge = merged.get(edge_key)
        if edge is None:
            edge = merged[edge_key] = {
                'source': node_key(*key), 'target': node_key(*reference['target']), 'kind': kind,
                'conditional': conditional, 'crossRoom': key[0] != reference['target'][0],
                'crossAge': GROUPS.get(room_age.get(key[0])) != GROUPS.get(room_age.get(reference['target'][0])),
                'self': key == reference['target'], 'movie': False, 'viaScript': True, 'evidence': []}
        edge['evidence'].append(_evidence(reference))
        edge['movie'] = edge['movie'] or 'movie' in reference
        edge['viaScript'] = edge['viaScript'] and bool(reference['via'])
        if reference['origin']['kind'] == 'native':
            edge['evidence'][-1]['engine'] = reference['engine']
    # One script that requests two places in a row, on the same branch,
    # passes through the first. That step belongs to the script that owns it:
    # it is kept as a continuation and never followed by the path search.
    for key, walk in walks.items():
        if key not in graph_keys:
            continue
        reload_count += len(walk.reloads)
        previous = None
        for reference in sorted(walk.references, key=lambda r: r['order']):
            if reference['target'] is None or reference['unreachable']:
                previous = None
                continue
            same_script = previous is not None and all(
                previous['origin'][field] == reference['origin'][field] for field in ('kind', 'index'))
            if (same_script and previous['guards'] == reference['guards'] and previous['via'] == reference['via']
                    and previous['target'] not in (key, reference['target'])
                    and previous['target'] in graph_keys):
                edge_key = (previous['target'], reference['target'], 'continuation', False)
                edge = merged.get(edge_key)
                if edge is None:
                    edge = merged[edge_key] = {
                        'source': node_key(*previous['target']), 'target': node_key(*reference['target']),
                        'kind': 'continuation', 'conditional': False,
                        'crossRoom': previous['target'][0] != reference['target'][0],
                        'crossAge': GROUPS.get(room_age.get(previous['target'][0]))
                        != GROUPS.get(room_age.get(reference['target'][0])),
                        'self': False, 'movie': False, 'viaScript': False, 'evidence': []}
                item = _evidence(reference)
                item['owner'] = node_key(*key)
                item['after'] = previous['text']
                edge['evidence'].append(item)
            previous = reference
    edges = sorted(merged.values(), key=lambda e: (e['source'], e['target'], e['kind'], e['conditional']))

    graph = {
        'schemaVersion': 1,
        'title': 'Myst III: Exile node graph',
        'semantics': 'A node is one scripted place. An edge is a structural reference found in the scripts of '
                     'its source, or an attributed engine transition. Conditions are kept, never evaluated.',
        'edition': {
            'version': exe.version, 'language': exe.language, 'edition': exe.edition,
            'executable': exe_ref.name, 'executableSize': len(raw),
            'executableSha256': hashlib.sha256(raw).hexdigest(), 'detectionMd5': exe.fingerprint,
            'safedisc': exe.safedisc, 'notes': exe.notes, 'origin': exe_ref.origin,
            'discs': [{'file': image.path.name, 'volume': image.volume, 'created': image.created,
                       'sectorSize': image.sector_size} for image in source.images],
        },
        'start': node_key(*native.START), 'boot': node_key(*native.BOOT),
        'ages': [{'id': age.id, 'disc': age.disc, 'labelId': age.label_id, 'group': GROUPS.get(age.id, 'System'),
                  'rooms': age.rooms} for age in db.ages],
        'rooms': [{'name': room.name, 'id': room.id, 'age': room.age_id, 'group': GROUPS.get(room.age_id, 'System'),
                   'scriptedNodes': len(room.nodes),
                   'places': sum(1 for n in nodes if n['room'] == room.name and not n['virtual']),
                   'archive': next((a['file'] for a in archive_info if a['room'] == room.name), None)}
                  for room in db.rooms.values()],
        'nodes': nodes, 'edges': edges,
        'dynamicNavigation': dynamic, 'unreachableReferences': unreachable,
        'engine': native.metadata(),
    }
    graph['analysis'] = analyse(graph)
    graph['stats'] = {
        'scriptedNodes': len(walks), 'places': sum(1 for n in nodes if not n['virtual']),
        'placeTypes': dict(Counter(n['type'] for n in nodes)), 'helperScripts': helper_count,
        'absentTargets': len(absent), 'nodes': len(nodes), 'edges': len(edges),
        'edgeKinds': dict(Counter(e['kind'] for e in edges)),
        'conditionalEdges': sum(e['conditional'] for e in edges),
        'references': sum(len(e['evidence']) for e in edges), 'reloads': reload_count,
        'dynamicReferences': len(dynamic), 'unreachableReferences': len(unreachable),
        'hotspots': sum(sum(1 for h in n.hotspots if not h.background) for r in db.rooms.values()
                        for n in r.nodes.values()),
        'instructions': sum(len(s.ops) for r in db.rooms.values() for b in r.blocks for s in b.scripts)
                        + sum(len(h.ops) for r in db.rooms.values() for b in r.blocks for h in b.hotspots),
        'scriptBlocks': sum(len(r.blocks) for r in db.rooms.values()),
        'soundNames': len(db.sound_names),
        'deadCode': sum(len(w.dead_code) for w in walks.values()),
        'missingCalls': sum(len(w.missing_calls) for w in walks.values()),
        'dynamicCalls': sum(len(w.dynamic_calls) for w in walks.values()),
    }
    context = {'source': source, 'exe': exe, 'db': db, 'walks': walks, 'archives': archives,
               'archive_info': archive_info, 'exe_ref': exe_ref}
    return graph, context


def movement(graph):
    """Unique ordered pairs of the movement projection.

    Zip shortcuts are left out, as are continuations and self-references: a
    Zip needs its destination to have been visited, a continuation belongs to
    the script of another place, and a place does not lead to itself.
    """
    pairs = set()
    for edge in graph['edges']:
        if edge['kind'] not in ('zip', 'continuation') and not edge['self']:
            pairs.add((edge['source'], edge['target']))
    return pairs


def analyse(graph):
    ids = [n['id'] for n in graph['nodes']]
    pairs = movement(graph)
    out_edges, in_edges = defaultdict(set), defaultdict(set)
    for a, b in pairs:
        out_edges[a].add(b)
        in_edges[b].add(a)
    sources = sorted(i for i in ids if not in_edges[i] and out_edges[i])
    sinks = sorted(i for i in ids if in_edges[i] and not out_edges[i])
    isolated = sorted(i for i in ids if not in_edges[i] and not out_edges[i])

    def distances(start, forward=True):
        table = out_edges if forward else in_edges
        seen = {start: 0}
        queue = deque([start])
        while queue:
            current = queue.popleft()
            for following in table[current]:
                if following not in seen:
                    seen[following] = seen[current] + 1
                    queue.append(following)
        return seen

    start = graph['start']
    from_start = distances(start)
    farthest = max(from_start.values())
    longest, longest_pair = 0, None
    for origin in ids:
        reached = distances(origin)
        best = max(reached, key=lambda k: (reached[k], k))
        if reached[best] > longest:
            longest, longest_pair = reached[best], (origin, best)

    # Weak components.
    parent = {i: i for i in ids}

    def find(x):
        while parent[x] != x:
            parent[x] = parent[parent[x]]
            x = parent[x]
        return x
    for a, b in pairs:
        parent[find(a)] = find(b)
    weak = Counter(find(i) for i in ids)

    # Strong components (iterative Tarjan).
    index, low, on_stack, stack, components = {}, {}, set(), [], []
    counter = 0
    for root in ids:
        if root in index:
            continue
        work = [(root, iter(sorted(out_edges[root])))]
        index[root] = low[root] = counter
        counter += 1
        stack.append(root)
        on_stack.add(root)
        while work:
            current, followers = work[-1]
            advanced = False
            for following in followers:
                if following not in index:
                    index[following] = low[following] = counter
                    counter += 1
                    stack.append(following)
                    on_stack.add(following)
                    work.append((following, iter(sorted(out_edges[following]))))
                    advanced = True
                    break
                if following in on_stack:
                    low[current] = min(low[current], index[following])
            if advanced:
                continue
            work.pop()
            if work:
                low[work[-1][0]] = min(low[work[-1][0]], low[current])
            if low[current] == index[current]:
                component = []
                while True:
                    member = stack.pop()
                    on_stack.discard(member)
                    component.append(member)
                    if member == current:
                        break
                components.append(sorted(component))
    components.sort(key=len, reverse=True)
    room_of = {n['id']: n['room'] for n in graph['nodes']}
    group_of = {n['id']: n['group'] for n in graph['nodes']}
    reciprocal = sum(1 for a, b in pairs if (b, a) in pairs)
    return {
        'projection': 'unique ordered pairs of movement, automatic and engine edges; Zip shortcuts, '
                      'continuations and self-references excluded',
        'uniquePairs': len(pairs), 'reciprocalPairs': reciprocal,
        'reciprocalShare': round(reciprocal / len(pairs), 4) if pairs else 0,
        'crossRoomPairs': sum(1 for a, b in pairs if room_of[a] != room_of[b]),
        'crossAgePairs': sum(1 for a, b in pairs if group_of[a] != group_of[b]),
        'sources': sources, 'sinks': sinks, 'isolated': isolated,
        'weakComponents': len(weak), 'largestWeakComponent': max(weak.values()),
        'strongComponents': len(components), 'largestStrongComponent': len(components[0]),
        'strongComponentSizes': [len(c) for c in components if len(c) > 1],
        'reachableFromStart': len(from_start),
        'notReachedFromStart': sorted(i for i in ids if i not in from_start),
        'maximumDistanceFromStart': farthest,
        'mostDistantFromStart': sorted(k for k, v in from_start.items() if v == farthest),
        'largestShortestDistance': longest, 'largestShortestDistancePair': list(longest_pair or ()),
        'distanceFromStart': from_start,
        'caveat': 'Structural results only. A path is a chain of script references; it says nothing about '
                  'the puzzle state those references test. Not reached does not mean unused.',
    }
