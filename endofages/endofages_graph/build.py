"""The End of Ages Graph: Myst V's Classic points and every way the player moves between them.

    python3 -m endofages_graph.build "<game folder>" [--out graph.json]

<game folder> is the folder holding `dat/`, `Python/`, `SDL/` and `avi/` of the Windows game (GOG 2.0.0.9; inside the
GOG Mac application: `Contents/SharedSupport/myst_v/support/myst_v/drive_c/Myst V`).

Nodes are the Classic points of the eight playable Ages, read from `<Age>.node` (see `nodes.py`): id `<Age>:<index>`
(the Age's file name and the stored, zero-based index), label `<Abbr>:<index>` with the four-letter prefixes the game
itself gives its Ages' scripts (ABBR), position in the Age's coordinates (Plasma units, feet, Z up). A few nodes are
not Classic points: a target named by the data that does not exist (`kind: absent`) and Releeshan, a place with no
Classic point (`kind: place`).

Edges (`kind`):

- `classic`: a link stored in the `.node` file (its type byte 0 to 4 kept as `classicType`);
- `link`: a link to an Age and a named spawn point, found as a link message (plLinkToAgeMsg) in a responder of a page
  or built by a script (`ptLinkToAgeInfo` in the compiled Python). The source is the node nearest to the object the
  player clicks (the clickable the script names, else the responder's own object); the target is the node nearest to
  the spawn point. A link inside one Age is still a link: the screen fades and the player arrives at a spawn point;
- `script-link`: a link a script makes without a click of its own (K'veer to Descent after Yeesha's intro; any Keep
  to K'veer once the four slates are on their Keeps);
- `cutscene`: a scene (a recorded character: Yeesha, Esher, Atrus) that moves the player to a point: from the region
  that starts it to the player's warp point, when the script says "do Player warp?" (xNPCSpawner), and the
  conclusions of K'veer and Myst. The scene's movie (`avi/*.bik`) is given when one has the scene's name;
- `warp`: a script moves the player to a named point of the same Age without a scene: the elevators of Descent
  (dsntElevator), the crack of Tahgira's ice cave (thgrCave);
- `ladder`: a plLadderModifier, climbed up or down (its top and bottom exits);
- `ride`: a moving platform that carries the player between pieces of an Age, inferred from positions (an elevator's
  axis, the levels of a cage, the stops of Todelmer's tram): `inferred: true` and its rule in the evidence.

Every edge placed by a nearest-node rule records the distance from the object to that node (`placement`). Edges
that depend on the state of the game (a book unlocked, a pedestal activated, a scene not yet seen, a platform
moved) have `conditional: true` and say why in `evidence.condition`.
"""
import argparse
import collections
import json
import math
import os
import re
import sys

from . import crypt, nodes as N, prp

AGES = ['Kveer', 'Descent', 'Direbo', 'Laki', 'Siralehn', 'Tahgira', 'Todelmer', 'Myst']
ABBR = {'Kveer': 'Kver', 'Descent': 'Dsnt', 'Direbo': 'Drbo', 'Laki': 'Laki', 'Siralehn': 'Srln', 'Tahgira': 'Thgr',
        'Todelmer': 'Tdlm', 'Myst': 'Myst'}
DISPLAY = {'Kveer': "K'veer", 'Descent': 'Descent', 'Direbo': 'Direbo', 'Laki': "Laki'ahn", 'Siralehn': 'Noloben',
           'Tahgira': 'Tahgira', 'Todelmer': 'Todelmer', 'Myst': 'Myst'}
START = ('Kveer', 'LinkInPointDefault')     # a new game begins in K'veer (Kveer.py: "first time in Kveer - will play intro")
SLATE_AGES = ['Laki', 'Siralehn', 'Tahgira', 'Todelmer']    # the four Ages of the Bahro slates (xBahroHandler)
KEEP_AGES = ['Siralehn', 'Laki', 'Tahgira', 'Todelmer', 'Kveer']   # Ages whose Keep offers the four "take" links
ALL_KEEPS = ('the four slates solid on their Keeps (global.bahroSlate.{laki,srln,tgra,tldm}Data.solidOnKeep, '
             'xBahroHandler)')


def age_pages(dat, age):
    out = []
    for line in crypt.read_text(os.path.join(dat, age + '.age')).splitlines():
        if line.startswith('Page='):
            name = line.split('=', 1)[1].split(',')[0]
            path = os.path.join(dat, f'{age}_{name}.prp')
            if os.path.exists(path):
                out.append((name, path))
    return out


class AgeData:
    """What the pages of one Age say: positions of scene objects, owners of modifiers, spawn points, scripts, links."""

    def __init__(self, dat, age):
        self.age = age
        self.so_pos = {}            # scene object name -> world position (first seen)
        self.mod_owner = {}         # modifier name -> scene object name (first seen)
        self.page_pos = {}          # (page, scene object name) -> world position: names repeat across pages
        self.page_owner = {}        # (page, modifier name) -> scene object name
        self.spawns = collections.OrderedDict()   # spawn point object name -> position (first seen)
        self.pfms = []
        self.link_msgs = []         # (page, responder name, [links in order])
        self.ladders = []
        self.elevators = []         # (page, modifier name): Myst V only, not decoded
        self.node_regions = []      # (page, modifier name): Myst V only, not decoded
        self.physical_pos = []      # positions of scene objects with a simulation (collision) interface
        for page_name, path in age_pages(dat, age):
            p = prp.Page(path)
            cis = {k[0].ref(): k for k in p.of_class('plCoordinateInterface')}
            spawn_mods = {k[0].ref() for k in p.of_class('plSpawnModifier')}
            for k in p.of_class('plSceneObject'):
                so = prp.read_scene_object(p, k)
                pos = None
                if so['coord'] and so['coord'].ref() in cis:
                    ci = prp.read_coordinate_interface(p, cis[so['coord'].ref()])
                    pos = list(prp.translation(ci['local_to_world']))
                    self.so_pos.setdefault(k[0].name, pos)
                    self.page_pos[(page_name, k[0].name)] = pos
                    if so['sim']:
                        self.physical_pos.append(pos)
                for m in so['modifiers']:
                    if m:
                        self.mod_owner.setdefault(m.name, k[0].name)
                        self.page_owner.setdefault((page_name, m.name), k[0].name)
                        if m.ref() in spawn_mods and pos is not None:
                            self.spawns.setdefault(k[0].name, pos)
            for k in p.of_class('plPythonFileMod'):
                m = prp.read_python_file_mod(p, k)
                m['name'] = k[0].name
                m['page'] = page_name
                m['params'] = [(i, t, v.name if isinstance(v, prp.Uoid) else v) for i, t, v in m['params']]
                self.pfms.append(m)
            for k in p.of_class('plLadderModifier'):
                lad = prp.read_ladder_modifier(p, k)
                lad['name'] = k[0].name
                lad['page'] = page_name
                self.ladders.append(lad)
            for k in p.of_class('plElevatorModifier'):
                self.elevators.append((page_name, k[0].name))
            for k in p.of_class('plNodeRegionModifier'):
                self.node_regions.append((page_name, k[0].name))
            for k in p.of_class('plResponderModifier'):
                msgs = prp.link_messages(p, k, AGES)
                if msgs:
                    self.link_msgs.append((page_name, k[0].name, [l for _, l in msgs]))

    def position_of(self, name, page=None):
        """The world position of a scene object, or of the scene object that owns a modifier of that name; looked
        for in `page` first when it is given (the same name can stand for different objects in different pages)."""
        if not name:
            return None, None
        if page is not None:
            if (page, name) in self.page_pos:
                return self.page_pos[(page, name)], name
            owner = self.page_owner.get((page, name))
            if owner and (page, owner) in self.page_pos:
                return self.page_pos[(page, owner)], owner
            for suffix in ('_Enter', '_Exit'):
                if name.endswith(suffix):
                    pos, obj = self.position_of(name[:-len(suffix)], page)
                    if pos is not None:
                        return pos, obj
        if name in self.so_pos:
            return self.so_pos[name], name
        owner = self.mod_owner.get(name)
        if owner and owner in self.so_pos:
            return self.so_pos[owner], owner
        for suffix in ('_Enter', '_Exit'):
            if name.endswith(suffix):
                return self.position_of(name[:-len(suffix)])
        return None, None

    def point_in(self, name, page):
        """A spawn or warp point by name, in `page` first."""
        if page is not None and (page, name) in self.page_pos:
            return self.page_pos[(page, name)], 'spawn point' if name in self.spawns else 'object'
        return self.point(name)

    def point(self, name):
        """A spawn or warp point by name: (position, how)."""
        if name in self.spawns:
            return self.spawns[name], 'spawn point'
        pos, obj = self.position_of(name)
        return pos, 'object' if pos else None


LANGS = {'eng', 'fre', 'fra', 'ger', 'deu', 'ita', 'spa', 'esp', 'jpn', 'rus', 'pol'}


def first_params(m):
    """{parameter id: value} of a script, keeping the first value of a parameter given several times."""
    ps = {}
    for i, t, v in m['params']:
        ps.setdefault(i, v)
    return ps


def _norm(s):
    return re.sub(r'[^a-z0-9]', '', s.lower())


class Builder:
    def __init__(self, game):
        self.game = game
        self.dat = os.path.join(game, 'dat')
        avi = os.path.join(game, 'avi')
        self.movies = {_norm(f[:-4]): f for f in sorted(os.listdir(avi))} if os.path.isdir(avi) else {}
        self.nodes, self.edges, self.unresolved = [], [], []
        self.node_sets, self.data, self.by_id = {}, {}, {}
        self.dropped_duplicates = 0

    # ---- nodes ----------------------------------------------------------------------------------------------

    def read_classic(self):
        for age in AGES:
            g = N.read_nodes(os.path.join(self.dat, age + '.node'))
            self.node_sets[age] = g['nodes']
            comps = N.components(g['nodes'])
            comp_of = {i: c for c, comp in enumerate(comps) for i in comp}
            for nd in g['nodes']:
                node = {'id': f'{age}:{nd["index"]}', 'kind': 'classic', 'age': age, 'index': nd['index'],
                        'label': f'{ABBR[age]}:{nd["index"]}', 'name': f'{age}.node, point {nd["index"]}',
                        'pos': [round(v, 3) for v in nd['pos']], 'component': comp_of[nd['index']],
                        'links': len(nd['links'])}
                self.nodes.append(node)
                self.by_id[node['id']] = node
                for l in nd['links']:
                    e = {'source': node['id'], 'target': f'{age}:{l["target"]}', 'kind': 'classic',
                         'classicType': l['type'], 'conditional': False,
                         'evidence': {'file': f'dat/{age}.node', 'node': nd['index']}}
                    if any(l['vector']):
                        e['vector'] = [round(v, 3) for v in l['vector']]
                    self.edges.append(e)
            self.data[age] = AgeData(self.dat, age)
            d = self.data[age]
            print(f'{age}: {len(g["nodes"])} nodes, {len(comps)} components, {len(d.spawns)} spawn points, '
                  f'{len(d.pfms)} scripts, {sum(len(x[2]) for x in d.link_msgs)} link messages', file=sys.stderr)
        self.glob = AgeData(self.dat, 'Global')
        self.symbols = {}           # slate Age -> {'Keep': symbol, 'Ped1': ..., 'Ped2': ..., 'Ped3': ...}
        for m in self.glob.pfms:
            if m['script'] == 'xBahroHandler':
                ps = first_params(m)
                self.symbols[ps[1]] = {'Keep': ps.get(17), 'Ped1': ps.get(18), 'Ped2': ps.get(19), 'Ped3': ps.get(20)}

    def near(self, age, pos):
        i, d = N.closest(self.node_sets[age], pos)
        return f'{age}:{i}', round(d, 2)

    def extra_node(self, age, name, kind, pos=None, note=None):
        nid = f'{age}:{name}'
        if nid not in self.by_id:
            node = {'id': nid, 'kind': kind, 'age': age, 'index': None,
                    'label': f'{ABBR[age]}:{name}', 'name': f'{name} ({kind})',
                    'pos': [round(v, 3) for v in pos] if pos else None, 'component': None, 'links': 0}
            if note:
                node['note'] = note
            self.nodes.append(node)
            self.by_id[nid] = node
        return nid

    def target(self, age, name, far=None, page=None):
        """The node of a spawn or warp point; an `absent` node when the Age has no point of that name; a `place`
        node when the point is farther than `far` from every Classic point."""
        pos, how = self.data[age].point_in(name, page)
        if pos is None:
            return self.extra_node(age, name, 'absent', note='named by the data, absent from the Age'), None, None
        nid, d = self.near(age, pos)
        if far is not None and d > far:
            return self.extra_node(age, name, 'place', pos, note=f'no Classic point within {far:g} units'), 0.0, how
        return nid, d, how

    def movie_of(self, behavior):
        """The movie of a scene: the file of the behavior's name (a language suffix allowed), else candidates."""
        if not behavior:
            return None, []
        b = _norm(behavior)
        if b in self.movies:
            return f'avi/{self.movies[b]}', []
        cands = [f'avi/{f}' for n, f in self.movies.items() if n.startswith(b)]
        exact = [f'avi/{f}' for n, f in self.movies.items() if n.startswith(b) and n[len(b):] in LANGS]
        if len(exact) == 1:
            return exact[0], []
        return None, cands

    # ---- edges ----------------------------------------------------------------------------------------------

    def add(self, src_age, src_obj, dst, kind, conditional, evidence, src_pos=None, how=None, extra=None, page=None):
        """dst: (node id, distance, how). src: an object name of src_age (or a position)."""
        if src_pos is None:
            src_pos, obj = self.data[src_age].position_of(src_obj, page)
        else:
            obj = src_obj
        if src_pos is None:
            self.unresolved.append({'age': src_age, 'from': src_obj, 'kind': kind, 'evidence': evidence,
                                    'why': 'no position for the source object'})
            return None
        src, dsrc = self.near(src_age, src_pos)
        if kind == 'cutscene' and src == dst[0]:
            self.by_id[src].setdefault('scenes', []).append(dict(evidence, distance=dsrc))
            return {'source': src, 'target': src}
        e = {'source': src, 'target': dst[0], 'kind': kind, 'conditional': bool(conditional), 'evidence': evidence,
             'placement': {'source': {'object': obj, 'by': how or 'object', 'distance': dsrc},
                           'target': {'distance': dst[1], 'by': dst[2]}}}
        if extra:
            e.update(extra)
        self.edges.append(e)
        return e

    def responder_links(self):
        for age in AGES:
            triggers = {}
            for m in self.data[age].pfms:
                ps = m['params']
                one = lambda j: next((v for i, t, v in ps if i == j), None)
                if m['script'] == 'xLinkingBookGUIPopup' and one(1) and one(2):
                    triggers[one(2)] = {'kind': 'book', 'clickables': [one(1)], 'script': m['name'], 'page': m['page']}
                if m['script'] == 'drboPedestal' and one(8):
                    triggers[one(8)] = {'kind': 'pedestal', 'clickables': [one(j) for j in (2, 3, 4, 5, 6)],
                                        'script': m['name'], 'active': one(1), 'page': m['page']}
                if m['script'] == 'kverMystBook' and one(4):
                    triggers[one(4)] = {'kind': 'book', 'clickables': [one(3)], 'script': m['name'], 'page': m['page'],
                                        'condition': 'the Myst book unlocked (kverMystBook: cClkMystBookUnlocked)'}
            for page_name, resp, links in self.data[age].link_msgs:
                trig = triggers.get(resp)
                for k, l in enumerate(links):
                    clickable = None
                    if trig and trig['kind'] == 'pedestal':
                        clickable = trig['clickables'][k] if k < len(trig['clickables']) else None
                    elif trig:
                        clickable = trig['clickables'][0]
                    pos, obj = self.data[age].position_of(clickable, trig['page'] if trig else None)
                    how = 'clickable'
                    if pos is None:
                        pos, obj = self.data[age].position_of(resp, page_name)
                        how = 'responder object'
                    condition = trig.get('condition') if trig else None
                    never = None
                    if trig and trig['kind'] == 'pedestal' and l.get('spawn') != 'LinkInTake':
                        condition = f'the place is among the activated pedestals ({trig["active"]}, drboPedestal)'
                        place = re.match(r'LinkIn(Ped\d)$', l.get('spawn') or '')
                        sym = self.symbols.get(l['age'], {}).get(place.group(1)) if place else None
                        if sym in ('DoNotGoHere', 'NotUsed'):
                            never = (f'the symbol of {l["age"]} {place.group(1)} is "{sym}" (xBahroHandler): the '
                                     f'Bahro never take the slate there, so the pedestal never activates')
                    if trig is None and resp == 'RespLinkToDirebo':
                        condition = None
                    self.add(age, obj, self.target(l['age'], l.get('spawn')), 'link', condition,
                             {'page': f'{age}_{page_name}.prp', 'responder': resp, 'state': k, 'to': l['age'],
                              'spawn': l.get('spawn'), 'title': l.get('title'), 'trigger': clickable,
                              'script': trig['script'] if trig else None, 'condition': condition,
                              **({'never': never} if never else {})},
                             src_pos=pos, how=how)

    def bahro_links(self):
        """The links of xBahroHandler (one per slate Age, in the Global pages), read from its compiled code (see the
        research note): in the slate's Age, `ClickTakeDireboLink` runs the handler's Direbo responder and
        `ClickPed1/2/3TakeLink` link to the Age's `LinkInTake`; in Direbo, `ClickTakeDireboLink<Age>` runs the same
        responder; at the Keep of every slate Age and of K'veer, `Click<Age>KeepTakeLink` links to that Age's
        `LinkInTake`; a click on the slate made solid on its Keep links to K'veer's `SpawnPtBubble` once all four are."""
        global_links = {resp: links for page, resp, links in self.glob.link_msgs}
        for m in self.glob.pfms:
            if m['script'] != 'xBahroHandler':
                continue
            ps = first_params(m)
            x = ps[1]
            ev = {'page': f'Global_{m["page"]}.prp', 'script': m['name'] + ' (xBahroHandler)'}
            resp = ps.get(50)
            for l in global_links.get(resp, []):
                tgt = self.target(l['age'], l.get('spawn'))
                for age, obj in ((x, 'ClickTakeDireboLink'), ('Direbo', 'ClickTakeDireboLink' + x)):
                    if self.data[age].position_of(obj)[0] is not None:
                        self.add(age, obj, tgt, 'link', None,
                                 dict(ev, trigger=obj, responder=resp, to=l['age'], spawn=l.get('spawn')))
            take = self.target(x, 'LinkInTake')
            for n in (1, 2, 3):
                obj = f'ClickPed{n}TakeLink'
                if self.data[x].position_of(obj)[0] is not None:
                    self.add(x, obj, take, 'link', None, dict(ev, trigger=obj, to=x, spawn='LinkInTake'))
            for age in KEEP_AGES:
                obj = f'Click{x}KeepTakeLink'
                if self.data[age].position_of(obj)[0] is not None:
                    cond = f'the take symbol of {x} enabled at the Keep (Resp{x}KeepSymbols "takesym_enable")'
                    self.add(age, obj, take, 'link', cond, dict(ev, trigger=obj, to=x, spawn='LinkInTake',
                                                                condition=cond))
            keep_click = ps.get(35)
            bubble = self.target('Kveer', 'SpawnPtBubble')
            if self.data[x].position_of(keep_click)[0] is not None:
                self.add(x, keep_click, bubble, 'script-link', ALL_KEEPS,
                         dict(ev, trigger=f'{ps.get(4)} (the slate, solid on the Keep pedestal {keep_click})',
                              to='Kveer', spawn='SpawnPtBubble', condition=ALL_KEEPS))

    def scripted(self):
        """The K'veer start, the scenes, the warps of scripts."""
        # K'veer: Yeesha's intro, then Kveer.py links to Descent (ILinkToVolcano, called back by the intro region)
        intro = self.target('Kveer', 'WarpPlayerIntro')
        self.add('Kveer', None, self.target('Descent', 'LinkInPointDefault'), 'script-link',
                 "first visit of K'veer, after Yeesha's intro (Kveer.py: global.Kveer.boolYeeshaKveerIntro)",
                 {'script': 'Kveer.py', 'function': 'ILinkToVolcano', 'to': 'Descent', 'spawn': 'LinkInPointDefault',
                  'condition': "first visit of K'veer, after Yeesha's intro", 'guaranteed': 'every new game'},
                 src_pos=self.by_id[intro[0]]['pos'], how='after the intro warp (WarpPlayerIntro)')
        for age in AGES:
            for m in self.data[age].pfms:
                ps = first_params(m)
                ev = {'page': f'{age}_{m["page"]}.prp', 'script': f'{m["name"]} ({m["script"]})'}
                if m['script'] == 'xNPCSpawner':
                    movie, candidates = self.movie_of(ps.get(18))
                    scene = {'npc': ps.get(1), 'behavior': ps.get(18), 'movie': movie, 'sdl': ps.get(12),
                             'region': ps.get(16)}
                    if candidates:
                        scene['movieCandidates'] = candidates
                    if not ps.get(18) and ps.get(21):
                        scene['movieNote'] = ('no behavior named: a scene at the Keep ("or, at Age\'s Keep?"), '
                                              'one of avi/glblEsher - First/Second/Third/FourthAgeKeep.bik')
                    if ps.get(25) is True and ps.get(24):
                        cond = f'the scene not yet seen ({ps.get(12)})'
                        self.add(age, ps.get(16), self.target(age, ps[24], page=m['page']), 'cutscene', cond,
                                 dict(ev, **scene, warp=ps[24], condition=cond), page=m['page'])
                    else:
                        pos, obj = self.data[age].position_of(ps.get(16), m['page'])
                        if pos is not None:
                            nid, d = self.near(age, pos)
                            self.by_id[nid].setdefault('scenes', []).append(dict(scene, warp=None, distance=d))
                if m['script'] == 'dsntElevator' and ps.get(25) and ps.get(26):
                    for a, b, what in ((ps[25], ps[26], 'down'), (ps[26], ps[25], 'up')):
                        self.add(age, a, self.target(age, b, page=m['page']), 'warp', None, dict(ev, what=f'elevator {what}', to=b),
                                 src_pos=self.data[age].point_in(a, m['page'])[0], how='warp point')
                if m['script'] == 'thgrCave' and ps.get(2) and ps.get(5):
                    # the last stage of the crack: "will now break the crack and fall through"; at the bottom
                    # ("rgn: fell to bottom (novice)"), Classic sends the player to the closest node (PtClosestNode)
                    bottom = self.data[age].position_of(ps[5], m['page'])[0]
                    if bottom is not None:
                        cond = f'the crack at its last stage, the player on it with the slate ({ps.get(1)})'
                        nid, d = self.near(age, bottom)
                        self.add(age, ps[2], (nid, d, 'closest node to the bottom region'), 'warp', cond,
                                 dict(ev, what='the crack breaks, the player falls; Classic then goes to the closest '
                                               'node (thgrCave: PtClosestNode)', to=ps[5], condition=cond),
                                 page=m['page'])
                if m['script'] == 'thgrCave' and ps.get(2) and ps.get(6):
                    cond = f'an early stage of the crack ({ps.get(1)})'
                    self.add(age, ps[2], self.target(age, ps[6], page=m['page']), 'warp', cond, dict(ev, to=ps[6], condition=cond),
                             page=m['page'])
                if m['script'] == 'kverYeeshaConc' and ps.get(11) and ps.get(6):
                    cond = "the conclusion in K'veer (global.bahroSlate.finalTablet.*)"
                    e = self.add(age, ps[11], self.target(age, ps[6], page=m['page']), 'cutscene', cond,
                                 dict(ev, npc='Yeesha', movie='avi/kverYeesha_Conc03.bik', warp=ps[6], condition=cond),
                                 page=m['page'])
                    rel = self.target(age, 'WarpReleeshan', far=100)
                    if e:
                        self.add(age, ps[6], rel, 'cutscene', cond,
                                 dict(ev, what='kverYeeshaConc.OnTimer: "will now warp Player to Releeshan"',
                                      warp='WarpReleeshan', condition=cond),
                                 src_pos=self.data[age].point_in(ps[6], m['page'])[0], how='warp point')
                if m['script'] == 'mystConcs' and ps.get(4):
                    for w, state in ((ps.get(6), 'NoTablet'), (ps.get(10), 'GotTablet')):
                        if w:
                            cond = f'Esher in Myst, {state} (mystConcs: global.bahroSlate.finalTablet.*)'
                            movie = 'mystEsher-Conc01.bik' if state == 'NoTablet' else 'mystEsher - Conc02.bik'
                            self.add(age, ps[4], self.target(age, w, page=m['page']), 'cutscene', cond,
                                     dict(ev, npc='Esher', movie=f'avi/{movie}', warp=w, condition=cond), page=m['page'])

    def ladders(self):
        for age in AGES:
            for lad in self.data[age].ladders:
                top_name = lad['exit_top'] or lad['top_pos']
                bot_name = lad['exit_bottom'] or lad['bottom_pos']
                top = self.data[age].position_of(top_name, lad['page'])[0]
                bot = self.data[age].position_of(bot_name, lad['page'])[0]
                cond = self.ladder_condition(age, lad)
                if top is None or bot is None:
                    self.unresolved.append({'age': age, 'ladder': lad['name'], 'why': 'position'})
                    continue
                t, dt = self.near(age, top)
                b, db = self.near(age, bot)
                if t == b:
                    continue
                ev = {'page': f'{age}_{lad["page"]}.prp', 'ladder': lad['name'], 'top': top_name, 'bottom': bot_name}
                if cond:
                    ev['condition'] = cond
                self.add(age, bot_name, (t, dt, 'ladder top'), 'ladder', cond, dict(ev, what='climb up'),
                         src_pos=bot, how='ladder bottom')
                self.add(age, top_name, (b, db, 'ladder bottom'), 'ladder', cond, dict(ev, what='climb down'),
                         src_pos=top, how='ladder top')

    def ladder_condition(self, age, lad):
        """A ladder is conditional when a script names one of its logic modifiers, or names it in a responder
        (srlnCliffLadder: `cClkCliffLadderBtm` and the SDL `boolPlateauLadderDown`; srlnLabLadder: `cRespLowerRoofLadder`)."""
        logic = {lad['top_logic'], lad['bottom_logic'], lad['main_logic']} - {None}
        stem = lad['name'][1:] if lad['name'].startswith('c') else lad['name']
        for m in self.data[age].pfms:
            values = [v for i, t, v in m['params'] if isinstance(v, str)]
            named = any(v in logic for v in values) and m['page'] == lad['page']
            if not named and len(stem) >= 8 and not re.fullmatch(r'L(adde)?r?\d+|Lddr\w*', stem):
                named = any(stem in v for v in values if v.startswith('cResp'))
            if named:
                sdl = [v for v in values if v.startswith('global.')]
                return f'set by {m["name"]} ({m["script"]})' + (f': {", ".join(sdl)}' if sdl else '')
        return None

    def rides(self):
        def ride(age, a, b, ev, axis=None):
            for s, t in ((a, b), (b, a)):
                self.edges.append({'source': s, 'target': t, 'kind': 'ride', 'conditional': True,
                                   'evidence': dict(ev, inferred=True), 'placement': {'axis': axis}})

        # elevators (plElevatorModifier, Myst V only, not decoded): a platform on a vertical axis. For every piece
        # (component) with a node within 1.5 units of the axis, the node nearest to it; consecutive ones by height
        # are joined both ways. Descent's ElevatorA and ElevatorB are left to dsntElevator's warp points.
        for age in AGES:
            ns = [n for n in self.nodes if n['age'] == age and n['kind'] == 'classic']
            for page_name, name in self.data[age].elevators:
                if age == 'Descent' and name in ('ElevatorA', 'ElevatorB'):
                    continue
                pos = self.data[age].position_of(name, page_name)[0]  # per page: Todelmer's pillars share names
                if pos is None:
                    self.unresolved.append({'age': age, 'elevator': name, 'why': 'position'})
                    continue
                best = {}
                for n in ns:
                    h = math.hypot(n['pos'][0] - pos[0], n['pos'][1] - pos[1])
                    if h <= 1.5 and (n['component'] not in best or h < best[n['component']][0]):
                        best[n['component']] = (h, n['id'])
                stops = sorted((self.by_id[i]['pos'][2], i) for h, i in best.values())
                for (za, a), (zb, b) in zip(stops, stops[1:]):
                    ride(age, a, b, {'page': f'{age}_{page_name}.prp', 'elevator': name,
                                     'rule': 'the nodes of different pieces on the axis of the platform',
                                     'condition': 'the platform at that level', 'rise': round(zb - za, 2)},
                         [round(v, 2) for v in pos])
        # node regions on one vertical axis (plNodeRegionModifier, not decoded): the levels of Laki's cage
        for age in AGES:
            groups = collections.defaultdict(list)
            for page_name, name in self.data[age].node_regions:
                pos = self.data[age].position_of(name, page_name)[0]  # per page: Todelmer's pillars share names
                if pos is not None:
                    groups[(round(pos[0]), round(pos[1]))].append((pos[2], name, page_name, pos))
            for key, levels in groups.items():
                if len(levels) < 2 or not all('Cage' in n for z, n, *_ in levels):
                    continue
                levels.sort()
                ids = [self.near(age, p)[0] for z, n, pg, p in levels]
                for (za, na, pga, pa), (zb, nb, pgb, pb), a, b in zip(levels, levels[1:], ids, ids[1:]):
                    linked = any(e['source'] == a and e['target'] == b for e in self.edges)
                    if a != b and not linked:
                        ride(age, a, b, {'page': f'{age}_{pga}.prp', 'nodeRegions': [na, nb],
                                         'rule': 'node regions stacked on one vertical axis (a moving cage)',
                                         'condition': 'the cage at that level', 'rise': round(zb - za, 2)},
                             [round(v, 2) for v in pa])
        # Todelmer's tram (tdlmTram: "car at start", "car at midpoint", "car at end"): its three stops by their spawn
        # points SpawnPointTram1, LinkInPoint-TramStop, SpawnPointTram2
        stops = [self.target('Todelmer', s)[0] for s in ('SpawnPointTram1', 'LinkInPoint-TramStop', 'SpawnPointTram2')]
        for a, b in zip(stops, stops[1:]):
            ride('Todelmer', a, b, {'page': 'Todelmer (tdlmTram)', 'tram': 'tdlmTram',
                                    'rule': 'the tram stops by the spawn points named after them',
                                    'condition': 'the tram powered and its lever set (tdlmTram, tdlmMainPower)'})

    def dedupe(self):
        seen, out = set(), []
        for e in self.edges:
            if e['kind'] == 'classic':
                k = (e['source'], e['target'], 'classic', e['classicType'])
            elif e['kind'] == 'ladder':
                k = (e['source'], e['target'], 'ladder', e['evidence']['ladder'])
            else:
                k = (e['source'], e['target'], e['kind'], json.dumps(e['evidence'], sort_keys=True))
            if k in seen:
                self.dropped_duplicates += 1
                continue
            seen.add(k)
            out.append(e)
        self.edges = out

    def run(self):
        self.read_classic()
        self.responder_links()
        self.bahro_links()
        self.scripted()
        self.ladders()
        self.rides()
        self.dedupe()
        spawn_points = []
        by_node = collections.defaultdict(list)
        for age in AGES:
            for name, pos in self.data[age].spawns.items():
                nid, d = self.near(age, pos)
                spawn_points.append({'age': age, 'name': name, 'pos': [round(v, 3) for v in pos], 'node': nid,
                                     'distance': d})
                if d <= 6:
                    by_node[nid].append((d, name))
        for nd in self.nodes:
            if by_node.get(nd['id']):
                nd['near'] = [n for _, n in sorted(by_node[nd['id']])]
        node_regions = []
        for age in AGES:
            for page_name, name in self.data[age].node_regions:
                pos = self.data[age].position_of(name, page_name)[0]  # per page: Todelmer's pillars share names
                node_regions.append({'age': age, 'page': page_name, 'name': name,
                                     'pos': [round(v, 2) for v in pos] if pos else None,
                                     'node': self.near(age, pos)[0] if pos else None})
        start = self.target(*START)[0]
        return {'game': 'Myst V: End of Ages (GOG 2.0.0.9)', 'ages': AGES, 'abbr': ABBR, 'display': DISPLAY,
                'start': start, 'nodes': self.nodes, 'edges': self.edges, 'unresolved': self.unresolved,
                'droppedDuplicates': self.dropped_duplicates, 'spawnPoints': spawn_points,
                'nodeRegions': node_regions,
                'physicals': {age: [[round(v, 2) for v in p] for p in self.data[age].physical_pos] for age in AGES}}


def build(game):
    return Builder(game).run()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('game')
    ap.add_argument('--out', default='graph.json')
    a = ap.parse_args()
    g = build(a.game)
    with open(a.out, 'w') as f:
        json.dump(g, f, indent=1)
    kinds = collections.Counter(e['kind'] for e in g['edges'])
    extra = collections.Counter(n['kind'] for n in g['nodes'] if n['kind'] != 'classic')
    print(f'{len(g["nodes"])} nodes {dict(extra)}, {len(g["edges"])} edges {dict(kinds)}, start {g["start"]}, '
          f'{len(g["unresolved"])} unresolved, {g["droppedDuplicates"]} duplicates dropped', file=sys.stderr)


if __name__ == '__main__':
    main()
