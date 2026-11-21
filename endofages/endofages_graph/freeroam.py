"""Free Roam against Classic: what the collision floors of each Age allow beyond the Classic points.

    python3 -m endofages_graph.freeroam "<game folder>" graph.json [--out freeroam.json]

Myst V's Free Roam walks the avatar on the collision geometry (plGenericPhysical, see `physics.py`); Classic moves
it between the points of `<Age>.node`. This module compares the two from the data alone, as an approximation that a
player in the game can check:

- floors: the triangles of the static collision meshes (explicit bounds, member group "static", avatar blockers left
  out) that face up enough to stand on (normal z >= 0.7, about 45 degrees);
- where Classic stands: under each Classic point, the highest floor triangle within `EYE_MAX` units below it (the
  points are at eye height; the offsets are reported);
- reachable floor: the floor triangles joined to those by walking: two floor triangles are joined when a corner of
  one lies on the other (in plan, within 0.5 unit) at a height within a step (`STEP` units) and the line between
  their centres, `BODY` units above the floor, crosses no wall (a static triangle steeper than the floors) and no
  avatar blocker. It ignores jumps and falls, and the moving platforms;
- places only in Free Roam: reachable floor farther than `R` units (in plan) from every Classic standing point at a
  similar height, grouped into connected patches, the largest listed with their area and nearest Classic point;
- standing: a point stands on that floor when its height above it is within a step of the Age's median (3.0); the
  others (56) stand on something the floor model leaves out, or over a surface that is not theirs, and take no part
  below (review of 2026-11-19);
- shortcuts: two Classic points close in a straight line (<= 30 units) whose Classic walk (classic, ladder and ride
  edges, by length) is at least three times longer, when the straight line between their standing points follows
  reachable floor without a gap or a step higher than `STEP` and crosses no wall or blocker triangle at a body
  height (1 and 4 units above the floor).
"""
import argparse
import collections
import heapq
import json
import math
import os
import sys

from . import build, physics, prp

EYE_MAX = 12.0      # a Classic point stands at most this far above its floor
STEP = 1.6          # a step the avatar climbs without a ladder (Plasma units are feet)
R = 15.0            # a floor patch farther than this (in plan) from every Classic standing point is "Free Roam only"
CELL = 8.0
BODY = 2.5          # height above the floor at which walls and blockers stop a walker


class Floors:
    def __init__(self, tris, walls):
        self.tris = tris            # [(verts, centroid, area, plane)]
        self.walls = walls          # [verts]
        self.grid = collections.defaultdict(list)
        for i, t in enumerate(tris):
            for c in self._cells(t[0]):
                self.grid[c].append(i)
        self._nb = {}
        self.wgrid = collections.defaultdict(list)
        for i, w in enumerate(walls):
            for c in self._cells(w):
                self.wgrid[c].append(i)

    @staticmethod
    def _cells(vs, pad=0.0):
        x0 = math.floor((min(v[0] for v in vs) - pad) / CELL)
        x1 = math.floor((max(v[0] for v in vs) + pad) / CELL)
        y0 = math.floor((min(v[1] for v in vs) - pad) / CELL)
        y1 = math.floor((max(v[1] for v in vs) + pad) / CELL)
        return [(x, y) for x in range(x0, x1 + 1) for y in range(y0, y1 + 1)]

    def z_at(self, i, x, y):
        a, b, c, d = self.tris[i][3]
        return -(a * x + b * y + d) / c

    def plan_distance(self, i, x, y):
        (ax, ay, _), (bx, by, _), (cx, cy, _) = self.tris[i][0]
        det = (by - cy) * (ax - cx) + (cx - bx) * (ay - cy)
        if det:
            l1 = ((by - cy) * (x - cx) + (cx - bx) * (y - cy)) / det
            l2 = ((cy - ay) * (x - cx) + (ax - cx) * (y - cy)) / det
            if l1 >= 0 and l2 >= 0 and l1 + l2 <= 1:
                return 0.0
        return min(_seg_dist(x, y, p, q) for p, q in (((ax, ay), (bx, by)), ((bx, by), (cx, cy)), ((cx, cy), (ax, ay))))

    def under(self, x, y, tol=0.0):
        cell = (math.floor(x / CELL), math.floor(y / CELL))
        return [i for i in self.grid.get(cell, []) if self.plan_distance(i, x, y) <= tol]

    def standing(self, pos):
        """The floor triangle under a Classic point: the highest one at most EYE_MAX below it."""
        best = None
        for i in self.under(pos[0], pos[1], 0.3):
            z = self.z_at(i, pos[0], pos[1])
            if pos[2] - EYE_MAX <= z <= pos[2] + 0.5 and (best is None or z > best[1]):
                best = (i, z)
        return best

    def neighbours(self, i):
        """The floor triangles a walker goes to from triangle i: a corner of i on them within a step, and no wall or
        blocker on the way from centre to centre at a body height (BODY units above the floor)."""
        if i in self._nb:
            return self._nb[i]
        out = set()
        ci = self.tris[i][1]
        for v in self.tris[i][0]:
            for j in self.under(v[0], v[1], 0.5):
                if j != i and j not in out and abs(self.z_at(j, v[0], v[1]) - v[2]) <= STEP:
                    cj = self.tris[j][1]
                    if not self.crosses_wall((ci[0], ci[1], ci[2] + BODY), (cj[0], cj[1], cj[2] + BODY)):
                        out.add(j)
        self._nb[i] = out
        return out

    def crosses_wall(self, p, q):
        cells = set(self._cells([p, q], 1.0))
        seen = set()
        for c in cells:
            for w in self.wgrid.get(c, []):
                if w not in seen:
                    seen.add(w)
                    if _seg_tri(p, q, self.walls[w]):
                        return True
        return False


def _seg_dist(x, y, p, q):
    dx, dy = q[0] - p[0], q[1] - p[1]
    ln = dx * dx + dy * dy
    t = 0.0 if ln == 0 else max(0.0, min(1.0, ((x - p[0]) * dx + (y - p[1]) * dy) / ln))
    return math.hypot(x - (p[0] + t * dx), y - (p[1] + t * dy))


def _seg_tri(p, q, tri, eps=1e-9):
    """Does the segment p-q cross the triangle (Moller-Trumbore)?"""
    v0, v1, v2 = tri
    d = [q[k] - p[k] for k in range(3)]
    e1 = [v1[k] - v0[k] for k in range(3)]
    e2 = [v2[k] - v0[k] for k in range(3)]
    h = (d[1] * e2[2] - d[2] * e2[1], d[2] * e2[0] - d[0] * e2[2], d[0] * e2[1] - d[1] * e2[0])
    a = e1[0] * h[0] + e1[1] * h[1] + e1[2] * h[2]
    if abs(a) < eps:
        return False
    f = 1.0 / a
    s = [p[k] - v0[k] for k in range(3)]
    u = f * (s[0] * h[0] + s[1] * h[1] + s[2] * h[2])
    if u < 0 or u > 1:
        return False
    qv = (s[1] * e1[2] - s[2] * e1[1], s[2] * e1[0] - s[0] * e1[2], s[0] * e1[1] - s[1] * e1[0])
    v = f * (d[0] * qv[0] + d[1] * qv[1] + d[2] * qv[2])
    if v < 0 or u + v > 1:
        return False
    t = f * (e2[0] * qv[0] + e2[1] * qv[1] + e2[2] * qv[2])
    return 0 <= t <= 1


def load_floors(dat, age):
    tris, walls = [], []
    objects = collections.Counter()
    for name, path in build.age_pages(dat, age):
        p = prp.Page(path)
        for k in p.of_class('plGenericPhysical'):
            ph = physics.read_physical(p, k)
            if ph['bounds'] != 'explicit':
                continue
            blocker = bool(ph['member'] & 0x800000)
            static = bool(ph['member'] & 0x2000000)
            if not static:
                continue
            vs = ph['verts']
            for a, b, c in ph['tris']:
                pa, pb, pc = vs[a], vs[b], vs[c]
                u = [pb[i] - pa[i] for i in range(3)]
                v = [pc[i] - pa[i] for i in range(3)]
                n = (u[1] * v[2] - u[2] * v[1], u[2] * v[0] - u[0] * v[2], u[0] * v[1] - u[1] * v[0])
                ln = math.sqrt(n[0] ** 2 + n[1] ** 2 + n[2] ** 2)
                if ln == 0:
                    continue
                nz = n[2] / ln
                if nz >= 0.7 and not blocker:
                    d = -(n[0] * pa[0] + n[1] * pa[1] + n[2] * pa[2])
                    cen = tuple((pa[i] + pb[i] + pc[i]) / 3 for i in range(3))
                    tris.append(((pa, pb, pc), cen, ln / 2, (n[0], n[1], n[2], d), ph['object']))
                    objects[ph['object']] += 1
                elif abs(nz) < 0.7 or blocker:
                    walls.append((pa, pb, pc))
    return Floors(tris, walls), objects


def compare(game, graph):
    dat = os.path.join(game, 'dat')
    nodes = [n for n in graph['nodes'] if n['kind'] == 'classic']
    walk = collections.defaultdict(list)
    by_id = {n['id']: n for n in nodes}
    for e in graph['edges']:
        if e['kind'] in ('classic', 'ladder', 'ride') and e['source'] != e['target'] and e['source'] in by_id \
                and e['target'] in by_id:
            walk[e['source']].append((e['target'], math.dist(by_id[e['source']]['pos'], by_id[e['target']]['pos'])))
    out = {'parameters': {'EYE_MAX': EYE_MAX, 'STEP': STEP, 'R': R, 'floorNormalZ': 0.7}, 'ages': {}}
    for age in graph['ages']:
        fl, objects = load_floors(dat, age)
        ns = [n for n in nodes if n['age'] == age]
        stand = {}
        for n in ns:
            s = fl.standing(n['pos'])
            if s:
                stand[n['id']] = s
        eyes = sorted(n['pos'][2] - stand[n['id']][1] for n in ns if n['id'] in stand)
        # A point stands on the floor found under it only at the Age's usual height above it, within a step. The others
        # (56 of 2,537: 32 higher, up to 12 units, 24 lower) stand on something the floor model leaves out (a platform, a
        # collider shaped as a box) or over a surface that is not their floor (review of 2026-11-19: such points made 77
        # of the 171 earlier shortcut candidates). The heights above are still reported for every point with a floor.
        usual = eyes[len(eyes) // 2] if eyes else 3.0
        on = {nid: s for nid, s in stand.items() if abs(by_id[nid]['pos'][2] - s[1] - usual) <= STEP}
        # reachable floor from the Classic standing triangles
        seeds = {s[0] for s in on.values()}
        reach = set(seeds)
        q = list(seeds)
        while q:
            i = q.pop()
            for j in fl.neighbours(i):
                if j not in reach:
                    reach.add(j)
                    q.append(j)
        pts = [(n['pos'][0], n['pos'][1], on[n['id']][1], n['id']) for n in ns if n['id'] in on]
        pgrid = collections.defaultdict(list)
        for p in pts:
            pgrid[(math.floor(p[0] / R), math.floor(p[1] / R))].append(p)

        def nearest(c):
            gx, gy = math.floor(c[0] / R), math.floor(c[1] / R)
            best = (math.inf, None)
            for dx in (-1, 0, 1):
                for dy in (-1, 0, 1):
                    for p in pgrid.get((gx + dx, gy + dy), []):
                        if abs(p[2] - c[2]) <= 8:
                            d = math.hypot(p[0] - c[0], p[1] - c[1])
                            if d < best[0]:
                                best = (d, p[3])
            return best

        far = {}
        for i in reach:
            d, nid = nearest(fl.tris[i][1])
            if d > R:
                far[i] = (d, nid)
        # patches of far floor
        patches, seen = [], set()
        for i in far:
            if i in seen:
                continue
            comp, q = [], [i]
            seen.add(i)
            while q:
                a = q.pop()
                comp.append(a)
                for b in fl.neighbours(a):
                    if b in far and b not in seen:
                        seen.add(b)
                        q.append(b)
            area = sum(fl.tris[a][2] for a in comp)
            cx = sum(fl.tris[a][1][0] * fl.tris[a][2] for a in comp) / area
            cy = sum(fl.tris[a][1][1] * fl.tris[a][2] for a in comp) / area
            cz = sum(fl.tris[a][1][2] * fl.tris[a][2] for a in comp) / area
            near_pt = min((math.dist((cx, cy, cz), n['pos']), n['id']) for n in ns)
            patches.append({'area': round(area, 1), 'triangles': len(comp), 'centroid': [round(cx, 1), round(cy, 1), round(cz, 1)],
                            'box': [[round(min(fl.tris[a][1][k] for a in comp), 1) for k in range(3)],
                                    [round(max(fl.tris[a][1][k] for a in comp), 1) for k in range(3)]],
                            'nearestPoint': near_pt[1], 'nearestDistance': round(near_pt[0], 1),
                            'objects': [o for o, _ in sorted(_areas(fl, comp).items(), key=lambda kv: -kv[1])[:4]]})
        patches.sort(key=lambda x: -x['area'])
        # shortcuts
        shortcuts = []
        for n in ns:
            if n['id'] not in on:
                continue
            dist = {n['id']: 0.0}
            h = [(0.0, n['id'])]
            while h:
                d, u = heapq.heappop(h)
                if d > dist[u]:
                    continue
                for v, w in walk[u]:
                    if d + w < dist.get(v, math.inf):
                        dist[v] = d + w
                        heapq.heappush(h, (d + w, v))
            for m in ns:
                if m['id'] <= n['id'] or m['id'] not in on:
                    continue
                straight = math.dist(n['pos'], m['pos'])
                if straight > 30 or straight < 3:
                    continue
                gd = dist.get(m['id'])
                if gd is not None and gd < 3 * straight:
                    continue
                ok = _walkable(fl, reach, (n['pos'][0], n['pos'][1], on[n['id']][1]),
                               (m['pos'][0], m['pos'][1], on[m['id']][1]))
                if ok:
                    shortcuts.append({'a': n['id'], 'b': m['id'], 'straight': round(straight, 1),
                                      'classicWalk': round(gd, 1) if gd is not None else None})
        shortcuts.sort(key=lambda x: (x['classicWalk'] is not None, -(x['classicWalk'] or 0) / x['straight']))
        total = sum(t[2] for t in fl.tris)
        reach_area = sum(fl.tris[i][2] for i in reach)
        far_area = sum(fl.tris[i][2] for i in far)
        out['ages'][age] = {
            'floorTriangles': len(fl.tris), 'floorArea': round(total), 'wallTriangles': len(fl.walls),
            'pointsWithFloor': len(stand), 'pointsOnFloor': len(on), 'points': len(ns),
            'eyeHeight': {'median': round(eyes[len(eyes) // 2], 2) if eyes else None,
                          'p10': round(eyes[len(eyes) // 10], 2) if eyes else None,
                          'p90': round(eyes[9 * len(eyes) // 10], 2) if eyes else None},
            'reachableArea': round(reach_area), 'reachableTriangles': len(reach),
            'freeRoamOnlyArea': round(far_area), 'freeRoamOnlyShare': round(far_area / reach_area, 3) if reach_area else None,
            'patches': len(patches), 'largestPatches': patches[:12],
            'shortcuts': len(shortcuts), 'shortcutExamples': shortcuts[:12]}
        a = out['ages'][age]
        print(f'{age:9s} floor {a["floorArea"]:7d} reach {a["reachableArea"]:7d} only-FR {a["freeRoamOnlyArea"]:7d} '
              f'({a["freeRoamOnlyShare"]}) patches {a["patches"]:4d} shortcuts {a["shortcuts"]:4d} '
              f'eye {a["eyeHeight"]} points with floor {a["pointsWithFloor"]}/{a["points"]}', file=sys.stderr)
    return out


def _areas(fl, comp):
    out = collections.Counter()
    for a in comp:
        out[fl.tris[a][4]] += fl.tris[a][2]
    return out


def _walkable(fl, reach, p, q):
    n = max(2, int(math.dist(p[:2], q[:2]) / 1.5))
    z = p[2]
    prev = p
    for k in range(1, n + 1):
        t = k / n
        x, y = p[0] + t * (q[0] - p[0]), p[1] + t * (q[1] - p[1])
        cands = [(abs(fl.z_at(i, x, y) - z), fl.z_at(i, x, y)) for i in fl.under(x, y, 0.2) if i in reach]
        cands = [c for c in cands if c[0] <= STEP]
        if not cands:
            return False
        z = min(cands)[1]
        here = (x, y, z)
        for hgt in (1.0, 4.0):
            if fl.crosses_wall((prev[0], prev[1], prev[2] + hgt), (x, y, z + hgt)):
                return False
        prev = here
    return abs(z - q[2]) <= STEP


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('game')
    ap.add_argument('graph')
    ap.add_argument('--out', default='freeroam.json')
    a = ap.parse_args()
    out = compare(a.game, json.load(open(a.graph)))
    json.dump(out, open(a.out, 'w'), indent=1)


if __name__ == '__main__':
    main()
