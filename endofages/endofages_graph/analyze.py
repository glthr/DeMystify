"""Measures of The End of Ages Graph (`graph.json` from `build.py`).

    python3 -m endofages_graph.analyze graph.json [--freeroam freeroam.json] [--out analysis.json]

Reach and distances count moves (hops) along directed edges, self links left out. Three readings of the graph:
`all` (every edge), `possible` (every edge but those whose condition the data shows can never be met: `never`), and
`unconditional` (only the edges that need no state of the game, plus the intro link every new game takes). The walk measures use the moves a walker makes
inside an Age (classic, ladder, ride) weighted by their straight length.
"""
import argparse
import collections
import heapq
import json
import math
import statistics as st


def bfs(adj, s):
    dist = {s: 0}
    prev = {}
    q = collections.deque([s])
    while q:
        u = q.popleft()
        for v in sorted(adj[u]):
            if v not in dist:
                dist[v] = dist[u] + 1
                prev[v] = u
                q.append(v)
    return dist, prev


def path(prev, s, t):
    out = [t]
    while out[-1] != s:
        out.append(prev[out[-1]])
    return out[::-1]


def adjacency(edges, keep):
    adj = collections.defaultdict(set)
    radj = collections.defaultdict(set)
    for e in edges:
        if e['source'] != e['target'] and keep(e):
            adj[e['source']].add(e['target'])
            radj[e['target']].add(e['source'])
    return adj, radj


READINGS = {'all': lambda e: True,
            'possible': lambda e: not e['evidence'].get('never'),
            'unconditional': lambda e: not e['conditional'] or bool(e['evidence'].get('guaranteed'))}


def analyze(g, freeroam=None):
    N = {n['id']: n for n in g['nodes']}
    classic = [n for n in g['nodes'] if n['kind'] == 'classic']
    kinds = collections.Counter(e['kind'] for e in g['edges'])
    a = {'nodes': len(N), 'classicNodes': len(classic),
         'otherNodes': {n['id']: n['kind'] for n in g['nodes'] if n['kind'] != 'classic'},
         'edges': len(g['edges']), 'edgeKinds': dict(sorted(kinds.items())),
         'conditionalByKind': dict(sorted(collections.Counter(e['kind'] for e in g['edges'] if e['conditional']).items())),
         'never': [{'source': e['source'], 'target': e['target'], 'why': e['evidence']['never']}
                   for e in g['edges'] if e['evidence'].get('never')],
         'selfLinks': dict(collections.Counter(e['kind'] for e in g['edges'] if e['source'] == e['target'])),
         'droppedDuplicates': g.get('droppedDuplicates')}
    per_age = {}
    for age in g['ages']:
        ids = [n['id'] for n in classic if n['age'] == age]
        cl = [e for e in g['edges'] if e['kind'] == 'classic' and N[e['source']]['age'] == age]
        pairs = {tuple(sorted((e['source'], e['target']))) for e in cl if e['source'] != e['target']}
        lengths = sorted(math.dist(N[e['source']]['pos'], N[e['target']]['pos']) for e in cl if e['source'] != e['target'])
        comps = collections.Counter(N[i]['component'] for i in ids)
        per_age[age] = {'points': len(ids), 'classicLinks': len(cl), 'pairs': len(pairs),
                        'components': len(comps), 'componentSizes': [comps[c] for c in sorted(comps)],
                        'types': dict(sorted(collections.Counter(e['classicType'] for e in cl).items())),
                        'linkLength': {'median': round(st.median(lengths), 2), 'p90': round(lengths[9 * len(lengths) // 10], 2),
                                       'max': round(lengths[-1], 2)}}
    a['ages'] = per_age
    cross = [e for e in g['edges'] if N[e['source']]['age'] != N[e['target']]['age']]
    a['betweenAges'] = {'edges': len(cross), 'byKind': dict(collections.Counter(e['kind'] for e in cross)),
                        'pairs': dict(sorted(collections.Counter(f"{N[e['source']]['age']}>{N[e['target']]['age']}"
                                                                for e in cross).items()))}
    a['readings'] = {}
    for name, keep in READINGS.items():
        adj, radj = adjacency(g['edges'], keep)
        dist, prev = bfs(adj, g['start'])
        unreached = collections.defaultdict(list)
        for n in g['nodes']:
            if n['id'] not in dist:
                unreached[f"{n['age']} piece {n['component']}" if n['kind'] == 'classic' else n['id']].append(n['id'])
        far = max(dist, key=lambda n: (dist[n], n))
        r = {'reached': len(dist), 'of': len(N),
             'reachedByAge': {age: sum(1 for i in dist if N[i]['age'] == age) for age in g['ages']},
             'unreached': {k: (len(v), v[:6]) for k, v in sorted(unreached.items())},
             'farthest': {'node': far, 'hops': dist[far], 'path': path(prev, g['start'], far)},
             'averageHops': round(sum(dist.values()) / len(dist), 2),
             'agesInOrderOfFirstReach': _age_order(dist, N)}
        if name == 'possible':
            best = (0, None, None)
            for s in sorted(dist):
                d, _ = bfs(adj, s)
                t = max(d, key=lambda n: (d[n], n))
                if d[t] > best[0]:
                    best = (d[t], s, t)
            d, pv = bfs(adj, best[1])
            r['diameter'] = {'hops': best[0], 'from': best[1], 'to': best[2], 'path': path(pv, best[1], best[2])}
            deg = sorted(((len(adj[n]), len(radj[n]), n) for n in N), key=lambda x: (-x[0], -x[1], x[2]))
            r['hubs'] = [{'node': n, 'label': N[n]['label'], 'out': o, 'in': i, 'near': N[n].get('near')} for o, i, n in deg[:12]]
            r['sources'] = sorted(n for n in N if not radj[n] and n != g['start'] and N[n]['kind'] != 'absent')
            r['sinks'] = sorted(n for n in N if not adj[n] and N[n]['kind'] != 'absent')
            # strongly connected pieces: from where can one come back to the start?
            back, _ = bfs(radj, g['start'])
            r['canReturnToStart'] = len(back)
            r['oneWayFromStart'] = sorted(n for n in dist if n not in back)[:40]
            r['oneWayCount'] = sum(1 for n in dist if n not in back)
        a['readings'][name] = r
    a['walk'] = walk_measures(g, N)
    if freeroam:
        a['freeRoam'] = {age: {k: v for k, v in f.items() if k not in ('largestPatches', 'shortcutExamples')}
                         for age, f in freeroam['ages'].items()}
        a['freeRoamParameters'] = freeroam['parameters']
    return a


def _age_order(dist, N):
    first = {}
    for n, d in dist.items():
        age = N[n]['age']
        first[age] = min(first.get(age, d), d)
    return [f'{age} ({d})' for age, d in sorted(first.items(), key=lambda x: x[1])]


def walk_measures(g, N):
    """Walking inside an Age (classic, ladder, ride by straight length) against the straight line."""
    walk = collections.defaultdict(list)
    for e in g['edges']:
        if e['kind'] in ('classic', 'ladder', 'ride') and e['source'] != e['target']:
            walk[e['source']].append((e['target'], math.dist(N[e['source']]['pos'], N[e['target']]['pos'])))
    out = {}
    for age in g['ages']:
        ids = [n['id'] for n in g['nodes'] if n['age'] == age and n['kind'] == 'classic']
        ratios, near_far = [], 0
        for s in ids:
            dist = {s: 0.0}
            h = [(0.0, s)]
            while h:
                d, u = heapq.heappop(h)
                if d > dist[u]:
                    continue
                for v, w in walk[u]:
                    if d + w < dist.get(v, math.inf):
                        dist[v] = d + w
                        heapq.heappush(h, (d + w, v))
            for t in ids:
                if t <= s:
                    continue
                straight = math.dist(N[s]['pos'], N[t]['pos'])
                if straight < 1:
                    continue
                gd = dist.get(t)
                if gd is not None:
                    ratios.append(gd / straight)
                if straight <= 15 and (gd is None or gd >= 10 * straight):
                    near_far += 1
        ratios.sort()
        out[age] = {'detourMedian': round(ratios[len(ratios) // 2], 3), 'detourP90': round(ratios[9 * len(ratios) // 10], 3),
                    'nearButFarPairs': near_far}
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('graph')
    ap.add_argument('--freeroam')
    ap.add_argument('--out', default='analysis.json')
    x = ap.parse_args()
    g = json.load(open(x.graph))
    fr = json.load(open(x.freeroam)) if x.freeroam else None
    a = analyze(g, fr)
    json.dump(a, open(x.out, 'w'), indent=1)
    print('edges', a['edgeKinds'], 'conditional', a['conditionalByKind'])
    for name, r in a['readings'].items():
        print(f"{name:13s} reached {r['reached']}/{r['of']} farthest {r['farthest']['node']} {r['farthest']['hops']} "
              f"average {r['averageHops']} order {r['agesInOrderOfFirstReach']}")
        print('   unreached', {k: v[0] for k, v in r['unreached'].items()})
    p = a['readings']['possible']
    print('diameter', p['diameter']['hops'], p['diameter']['from'], '->', p['diameter']['to'], '| sources', p['sources'],
          '| sinks', p['sinks'], '| can return to start', p['canReturnToStart'], 'one-way', p['oneWayCount'])
    print('hubs', [(h['node'], h['out']) for h in p['hubs']])
    print('between Ages', a['betweenAges'])


if __name__ == '__main__':
    main()
