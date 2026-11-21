"""Build the node graph of Myst IV: Revelation, keeping the evidence of every edge.

The game keeps one archive per node inside ``data.m4b``. A node archive holds its picture (a cube of tiles), its
hotspots, its command blocks (lists of readable commands) and the parameters of the compiled "AI" classes that
run in it. Four things name another node:

* a ``ContextChange(world, zone, node)`` command;
* the parameters of a transition (``aitransition``: a hotspot, a movie and a destination);
* the parameters of a close-up (``aisubnode``: the node to go back to);
* a hotspot named for its destination (``h_to_w1_z02_n010``) that no command serves: a compiled class of the
  node does, and the edge is attributed to it.
"""
from __future__ import annotations

import json
import re
from collections import Counter, defaultdict, deque
from pathlib import Path

from . import bigfile, disc, native, resources

BY_HAND = Path(__file__).with_name('compiled_by_hand.json')
COMPARES_LAST_NODE = {'w5_z04_incub'}   # classes whose m_subnodeName is only compared with the last node

WORLDS = {1: 'Tomahna (night)', 2: 'Haven', 3: 'Spire', 4: 'Serenia', 5: 'Tomahna (day)', 6: 'Menu'}
NODE_PATH = re.compile(r'global/w(\d)/z(\d\d)/n(\d\d\d)/w\dz\d\dn\d\d\d\.m4b$')
NODE_NAME = re.compile(r'w(\d)_z(\d\d)_n(\d\d\d)')
CONTEXT = re.compile(r'^\s*(ContextChange\w*)\s*\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)')
CALL = re.compile(r'^\s*([A-Za-z_]\w*)\s*\((.*)\)\s*;?\s*$')
# Commands that start another block, with the position of the block names among their arguments.
STARTERS = {'CommandBlockStart': (0,), 'CommandBlockIf': (1,), 'CommandBlockIfNot': (1,), 'CommandBlockIfElse': (1, 2),
            'CommandBlockSleep': (0,), 'CommandBlockStartSynchronous': (0,)}
JUMPS = ('Jump', 'JumpIf', 'JumpIfNot', 'JumpIfElse', 'JumpIfElseLastNode', 'JumpIfElseU8U8', 'JumpIfElseS8S8')
EVENT_CLASSES = {'aigeometricevent', 'aiwidgetbuttonevent', 'aiwidgetevent'}
SMALL = 400_000


def node_id(world, zone, node):
    return f'w{world}_z{zone:02d}_n{node:03d}'


def parse_id(text):
    match = NODE_NAME.fullmatch(text)
    return tuple(map(int, match.groups())) if match else None


class Node:
    def __init__(self, key, path, size, offset):
        self.key, self.path, self.size, self.offset = key, path, size, offset
        self.id = node_id(*key)
        self.files, self.ai, self.blocks, self.info, self.bins = [], {}, {}, None, {}
        self.hotspots, self.layers, self.errors = [], [], []


def read_node(ref, node):
    head = ref.read(node.offset, min(node.size, 1 << 18))
    try:
        node.files, _ = bigfile.parse(head)
    except Exception:
        node.files, _ = bigfile.parse(ref.read(node.offset, node.size))
    for path, size, offset in node.files:
        if path.startswith('cube/') and path.endswith('.m4b'):
            node.layers.append({'name': path[5:-4], 'size': size, 'offset': node.offset + offset})
        if path == 'graphic/hotspots.m4b':
            data = ref.read(node.offset + offset, size)
            for inner, inner_size, inner_offset in bigfile.parse(data)[0]:
                if '/hierarchy/' in inner:
                    node.hotspots.append(hotspot(inner.rsplit('/', 1)[-1][:-4], data[inner_offset:inner_offset + inner_size]))
            continue
        if path.endswith(('.m4b', '.log')) or size > SMALL:
            continue
        data = ref.read(node.offset + offset, size)
        try:
            if path.endswith('.ai'):
                node.ai[path] = data.decode('latin-1')
            elif path.startswith(('command_block/', 'commandblock/', 'command_blocks/')):
                # Four nodes store blocks in a folder spelled otherwise (commandblock/ in w2_z12_n192, w3_z05_n111
                # and w4_z05_n072; command_blocks/ in w6_z01_n021); their loader logs name them, so the game loads them.
                for name, commands in resources.command_block(data):
                    node.blocks[name] = {'file': path, 'offset': node.offset + offset, 'commands': commands}
            elif path == 'node.bin':
                node.info = resources.node(data)
            elif path.endswith('.bin'):
                node.bins[path] = [text for _, text in resources.strings(data)][:200]
        except Exception as error:   # kept and reported: nothing is skipped silently
            node.errors.append(f'{path}: {error}')


def hotspot(name, data):
    """A hotspot of the node: its name and, when the record has the expected shape, where it lies around the viewer.

    The record starts with the class name ``atlas::Node``, the hotspot's name, then a 4 x 4 matrix whose last
    column is the hotspot's position relative to the node, in the axes of the world.
    """
    import struct
    out = {'name': name}
    try:
        pos = 12
        length = struct.unpack_from('<I', data, pos)[0]
        pos += 4 + length
        length = struct.unpack_from('<I', data, pos)[0]
        pos += 4 + length
        matrix = struct.unpack_from('<16f', data, pos)
        if abs(matrix[15] - 1) < 1e-4 and all(abs(v) < 1e-4 for v in matrix[12:15]):
            out['offset'] = [round(matrix[3], 4), round(matrix[7], 4), round(matrix[11], 4)]
    except Exception:
        pass
    return out


def arguments(text):
    return [part.strip() for part in text.split(',')]


def tests_before(commands, index):
    """The tests that decide whether the instruction at ``index`` of a block runs: the jumps met before it.

    A block is a list with labels and jumps. The jumps are kept as text and not followed: an instruction that
    comes after any jump or label is reported as conditional, with the jumps as written.
    """
    found = []
    for command in commands[:index]:
        stripped = command.strip()
        if stripped.startswith(';'):
            continue
        match = CALL.match(stripped)
        if match and match.group(1) in JUMPS:
            found.append(stripped)
    return found


def load(inputs):
    source = disc.Source(inputs)
    candidates = [f for f in source.find('data.m4b') if 'common' in (f.inner_path or str(f.path)).lower()
                  and not re.search(r'(english|french|german|dutch|italian|spanish|japanese)', (f.inner_path or str(f.path)).lower())]
    if not candidates:
        candidates = sorted(source.find('data.m4b'), key=lambda f: -f.size)
    if not candidates:
        raise ValueError('data.m4b was not found in the supplied folder or disc images')
    ref = candidates[0]
    size = 1 << 20
    while True:
        try:
            toc, header = bigfile.parse(ref.read(0, min(size, ref.size)))
            break
        except Exception:
            if size >= ref.size:
                raise
            size *= 4
    nodes, others = {}, {}
    for path, length, offset in toc:
        match = NODE_PATH.search(path)
        if match:
            key = tuple(map(int, match.groups()))
            nodes[key] = Node(key, path, length, offset)
        else:
            others[path] = (length, offset)
    for node in nodes.values():
        read_node(ref, node)
    shared = {}
    for name in ('common/common.m4b', 'shared/shared.m4b'):
        if name in others:
            holder = Node((0, 0, 0), name, *others[name])
            read_node(ref, holder)
            shared[name.split('/')[0]] = holder
    library_pairs = defaultdict(set)
    pattern = re.compile(rb'(w\d_z\d\d_n\d\d\d)\.h_to_(w\d_z\d\d_n\d\d\d)', re.I)
    libraries = []
    for item in source.files:
        if re.fullmatch(r'm4_?ai\w*\.dll', item.name.lower()):
            data = item.read()
            libraries.append({'name': item.name, 'size': item.size, 'origin': item.origin})
            for match in pattern.finditer(data):
                library_pairs[(match.group(1).decode().lower(), match.group(2).decode().lower())].add((item.name, match.start()))
    compiled, computed, notice = [], 0, None
    for item in source.files:
        if re.fullmatch(r'm4_?ai\w*\.dll', item.name.lower()):
            try:
                result = native.context_changes(item.name, item.read())
            except RuntimeError as error:
                notice = str(error)
                break
            compiled.extend(result['changes'])
            computed += result['computed']
    database = None
    if 'nodedb.ndb' in others:
        import struct
        blob = ref.read(others['nodedb.ndb'][1], others['nodedb.ndb'][0])
        position, entries = 12, []
        for _ in range(struct.unpack_from('<I', blob, 8)[0]):
            count = struct.unpack_from('<I', blob, position)[0]
            ids = [struct.unpack_from('<BBH', blob, position + 4 + 4 * i) for i in range(count + 1)]
            position += 4 * (count + 2)
            entries.append((ids[0], ids[1:]))
        database = {'entries': len(entries),
                    'absentKeys': sorted(node_id(*k) for k, _ in entries if k not in nodes),
                    'absentNeighbours': sorted({node_id(*n) for _, ns in entries for n in ns if n not in nodes}),
                    'storedNodesWithoutEntry': sum(1 for k in nodes if k not in {e[0] for e in entries}),
                    'pairs': sorted({(node_id(*k), node_id(*n)) for k, ns in entries for n in ns})}
    boot = None
    if 'data.dsc' in others:
        match = re.search(r'set_initial_context\((w\d_z\d\d_n\d\d\d)\)', ref.read(others['data.dsc'][1], others['data.dsc'][0]).decode('latin-1'))
        boot = match.group(1) if match else None
    return {'source': source, 'ref': ref, 'toc': toc, 'header': header, 'nodes': nodes, 'others': others, 'shared': shared,
            'library_pairs': library_pairs, 'libraries': libraries, 'boot': boot,
            'compiled': compiled, 'computed': computed, 'notice': notice, 'database': database}


def triggers(node):
    """What starts each command block of a node: an event on a hotspot, a class parameter, a video marker, another block.

    Returns ``{block: [{'by': ..., ...}]}``. Blocks whose name ends in ``_init`` are started when the node loads (a
    naming rule of the game's data: every node has ``<node>_init``, ``snd_init``... and nothing refers to them).
    """
    found = defaultdict(list)
    prefix = node.id + '.'
    for path, text in node.ai.items():
        cls = path.split('/')[1]
        entries = resources.ai(text)
        values = {name: value for name, _, value in entries}
        if cls in EVENT_CLASSES:
            for name, _, value in entries:
                if name.startswith('m_cmd_') and value.startswith(prefix):
                    found[value[len(prefix):]].append({'by': 'event', 'event': name[6:], 'hotspot': values.get('m_geo', ''),
                                                       'class': cls, 'file': path})
        elif cls == 'aivideomarkers':
            for name, _, value in entries:
                if name.startswith('command[') and value.startswith(prefix):
                    index = name[8:-1]
                    found[value[len(prefix):]].append({'by': 'video marker', 'video': values.get('m_video', ''),
                                                       'frame': values.get(f'frame[{index}]', ''), 'file': path})
        else:
            for name, _, value in entries:
                if value.startswith(prefix) and value[len(prefix):] in node.blocks:
                    found[value[len(prefix):]].append({'by': 'class', 'class': cls, 'parameter': name, 'file': path})
    for name, block in node.blocks.items():
        for index, command in enumerate(block['commands']):
            stripped = command.strip()
            if stripped.startswith(';'):
                continue
            match = CALL.match(stripped)
            if not match or match.group(1) not in STARTERS:
                continue
            parts = arguments(match.group(2))
            for position in STARTERS[match.group(1)]:
                if position < len(parts) and parts[position].startswith(prefix):
                    found[parts[position][len(prefix):]].append({'by': 'block', 'block': name, 'instruction': stripped})
    for name in node.blocks:
        if name.endswith('_init') and not found.get(name):
            found[name].append({'by': 'arrival'})
    return found


def roots(node, table, block, seen=None):
    """Follow "started by another block" up to what first starts it; returns the list of first causes with the chain."""
    seen = seen or (block,)
    out = []
    for trigger in table.get(block, []):
        if trigger['by'] == 'block':
            if trigger['block'] in seen:
                continue
            for cause, chain in roots(node, table, trigger['block'], seen + (trigger['block'],)):
                out.append((cause, [trigger['instruction']] + chain))
        else:
            out.append((trigger, []))
    return out


def build(inputs):
    loaded = load(inputs)
    nodes = loaded['nodes']
    edges = {}
    absent = {}
    commented = []
    dangling = []   # class parameters that name a transition that their node does not store
    last_node_checks = []   # class parameters that name a close-up only to compare it with the last node

    def add(source, target_key, kind, conditional, evidence):
        target = node_id(*target_key)
        if target_key not in nodes:
            absent.setdefault(target, []).append(source)
        key = (source, target, kind)
        edge = edges.setdefault(key, {'source': source, 'target': target, 'kind': kind, 'conditional': conditional,
                                      'evidence': []})
        edge['conditional'] = edge['conditional'] and conditional
        edge['evidence'].append(evidence)

    for key, node in sorted(nodes.items()):
        table = triggers(node)
        served = set()   # destinations that a command or a transition of this node already reaches
        for name, block in sorted(node.blocks.items()):
            for index, command in enumerate(block['commands']):
                stripped = command.strip()
                match = CONTEXT.match(stripped.lstrip(';').strip())
                if not match:
                    continue
                target = tuple(map(int, match.groups()[1:]))
                if stripped.startswith(';'):
                    commented.append({'node': node.id, 'block': name, 'instruction': stripped, 'target': node_id(*target)})
                    continue
                tests = tests_before(block['commands'], index)
                causes = roots(node, table, name)
                kinds = {cause['by'] for cause, _ in causes}
                chain_tests = [step for _, chain in causes for step in chain if not step.startswith('CommandBlockStart')]
                if 'event' in kinds and any(cause.get('event') == 'click' for cause, _ in causes if cause['by'] == 'event'):
                    kind = 'move'
                elif kinds:
                    kind = 'auto'
                else:
                    kind = 'unreferenced'
                evidence = {'via': 'command', 'block': name, 'file': block['file'], 'offset': block['offset'],
                            'instruction': stripped, 'tests': tests + chain_tests,
                            'startedBy': [dict(cause, chain=chain) if chain else cause for cause, chain in causes]}
                add(node.id, target, kind, bool(tests or chain_tests), evidence)
                served.add(target)
        for path, text in sorted(node.ai.items()):
            cls = path.split('/')[1]
            values = {name: value for name, _, value in resources.ai(text)}
            if cls == 'aitransition':
                try:
                    target = (int(values['m_dest_world']), int(values['m_dest_zone']), int(values['m_dest_node']))
                except (KeyError, ValueError):
                    node.errors.append(f'{path}: transition without a destination')
                    continue
                add(node.id, target, 'move', False, {'via': 'transition', 'file': path, 'hotspot': values.get('m_hotspot', ''),
                                                     'movie': values.get('m_video', ''),
                                                     'tests': [values['m_can_transit_variable']] if values.get('m_can_transit_variable') else []})
                served.add(target)
            elif cls == 'aisubnode':
                try:
                    target = (int(values['m_worldFrom']), int(values['m_zoneFrom']), int(values['m_nodeFrom']))
                except (KeyError, ValueError):
                    node.errors.append(f'{path}: close-up without a parent')
                    continue
                node.parent = node_id(*target)
                add(node.id, target, 'return', False, {'via': 'close-up', 'file': path,
                                                       'note': 'the node to go back to, as the close-up records it'})
                served.add(target)
        # Any other class whose parameters give a world, a zone and a node (an elevator's m_world_to_go[1],
        # m_zone_to_go[1], m_node_to_go[1]): the destination is data, the move is the class's.
        for path, text in sorted(node.ai.items()):
            cls = path.split('/')[1]
            if cls in ('aitransition', 'aisubnode', 'aiamuletstaticvisualsource', 'aiamuletstaticvisualdestination'):
                continue
            values = {name: value for name, _, value in resources.ai(text)}
            for name, value in values.items():
                lowered = name.lower()
                if 'world' not in lowered:
                    continue
                at = lowered.index('world')
                zone = next((values[n] for n in values if n.lower() == lowered[:at] + 'zone' + lowered[at + 5:]), None)
                number = next((values[n] for n in values if n.lower() == lowered[:at] + 'node' + lowered[at + 5:]), None)
                if zone is None or number is None or not (value.isdigit() and zone.isdigit() and number.isdigit()):
                    continue
                target = (int(value), int(zone), int(number))
                if target in served or target == key or not 0 < target[0] < 10:
                    continue
                add(node.id, target, 'engine', False,
                    {'via': 'class', 'classes': [{'class': cls, 'parameter': name, 'file': path}],
                     'note': 'a destination given as three parameters of a compiled class (world, zone, node)'})
                served.add(target)
        # Hotspots named for a destination that nothing above serves: the compiled class of the node does.
        classes = defaultdict(list)
        for path, text in node.ai.items():
            for name, _, value in resources.ai(text):
                classes[value].append((path.split('/')[1], name, path))
        for spot in node.hotspots:
            match = re.fullmatch(r'h_to_(w\d_z\d\d_n\d\d\d)\w*', spot['name'])
            if not match:
                continue
            target = parse_id(match.group(1))
            if target in served or target == key:
                continue
            users = classes.get(f'{node.id}.{spot["name"]}', [])
            add(node.id, target, 'engine', False,
                {'via': 'hotspot', 'hotspot': spot['name'],
                 'classes': [{'class': c, 'parameter': p, 'file': f} for c, p, f in users],
                 'note': 'a hotspot named for its destination; no command block changes node, a compiled class does'})
            served.add(target)
        # A class parameter that names a hotspot "h_to_<node>" kept outside the node's own hotspot list.
        for path, text in sorted(node.ai.items()):
            cls = path.split('/')[1]
            for name, _, value in resources.ai(text):
                match = re.search(r'\.h_to_(w\d_z\d\d_n\d\d\d)', value)
                if match and value.startswith(node.id + '.') and parse_id(match.group(1)) not in served | {key}:
                    add(node.id, parse_id(match.group(1)), 'engine', False,
                        {'via': 'class', 'hotspot': value.split('.', 1)[1],
                         'classes': [{'class': cls, 'parameter': name, 'file': path}],
                         'note': 'a class parameter names a hotspot called after its destination'})
                    served.add(parse_id(match.group(1)))
        # A class parameter that holds a transition of this node named for its destination
        # (w1_z06_n050.p_tr_w1_z09_n030_default_transit). The transition, its panel or its movie must be stored in
        # the node: a name without a thing is no way out. The day node w5_z02_n160 names w5_z02_n160.tr_w1_z02_n170,
        # which only its twin of the night, w1_z02_n160, stores (corrected 2026-11-18): it is kept in
        # danglingTransitions, never as an edge.
        stored_names = {path.rsplit('/', 1)[-1].rsplit('.', 1)[0] for path, _, _ in node.files}
        for path, text in sorted(node.ai.items()):
            cls = path.split('/')[1]
            for name, _, value in resources.ai(text):
                match = re.match(re.escape(node.id) + r'\.(?:p_)?tr_(w\d_z\d\d_n\d\d\d)', value)
                if not match or parse_id(match.group(1)) in served | {key}:
                    continue
                thing = value.split('.states.')[0].split('.', 1)[1]
                if not any(stem == thing or stem.startswith(thing + '_') for stem in stored_names):
                    dangling.append({'node': node.id, 'value': value, 'target': match.group(1), 'class': cls,
                                     'parameter': name, 'file': path,
                                     'note': 'the node stores no transition, panel or movie of that name'})
                    continue
                add(node.id, parse_id(match.group(1)), 'engine', False,
                    {'via': 'class', 'transition': value.split('.states.')[0],
                     'classes': [{'class': cls, 'parameter': name, 'file': path}],
                     'note': 'a class parameter holds a transition of this node named for its destination'})
                served.add(parse_id(match.group(1)))
        # The same name written as a constant in one of the game's AI libraries.
        for (source_name, target_name), places in sorted(loaded['library_pairs'].items()):
            if source_name == node.id and parse_id(target_name) not in served | {key}:
                add(node.id, parse_id(target_name), 'engine', False,
                    {'via': 'library', 'hotspot': 'h_to_' + target_name,
                     'constants': [{'library': library, 'offset': offset} for library, offset in sorted(places)],
                     'note': 'the hotspot is named by a constant of the compiled library'})
                served.add(parse_id(target_name))
        # A class parameter that names a close-up of this node (a drawer pulled open, for instance): the rail classes
        # (airailopentosubnode and its kin) open it. One class does not: w5_z04_incub (the incubator of the laboratory)
        # only reads global.state.game.last_node and compares it with this name, to know whether the player comes
        # back from the close-up (M4AIW5RD.DLL, activation 0x10003930, compare at 0x10003a3c to 0x10003a65); no
        # routine of the class changes node with it, and in the game nothing opens the close-up from these nodes
        # (corrected 2026-11-18). Its six names are kept as checks of the last node, never as edges.
        for path, text in sorted(node.ai.items()):
            cls = path.split('/')[1]
            for name, _, value in resources.ai(text):
                if name != 'm_subnodeName' or not parse_id(value):
                    continue
                if cls in COMPARES_LAST_NODE:
                    last_node_checks.append({'node': node.id, 'names': value, 'class': cls, 'parameter': name,
                                             'file': path, 'note': 'compared with the last node visited; '
                                             'the class never changes node with it'})
                elif parse_id(value) not in served:
                    add(node.id, parse_id(value), 'engine', False,
                        {'via': 'class', 'classes': [{'class': cls, 'parameter': name, 'file': path}],
                         'note': 'the close-up that this class opens'})
                    served.add(parse_id(value))

    # The vision that the amulet shows of an object: the node names the vision, and the vision names the node.
    for key, node in sorted(nodes.items()):
        for path, text in sorted(node.ai.items()):
            cls = path.split('/')[1]
            if cls in ('aiamuletstaticvisualsource', 'aiamuletstaticvisualdestination'):
                values = {name: value for name, _, value in resources.ai(text)}
                try:
                    target = (int(values['m_world']), int(values['m_zone']), int(values['m_node']))
                except (KeyError, ValueError):
                    continue
                add(node.id, target, 'amulet' if cls.endswith('source') else 'return', False,
                    {'via': 'amulet', 'file': path, 'classes': [{'class': cls, 'parameter': 'm_world, m_zone, m_node', 'file': path}],
                     'note': 'the vision that the amulet opens here' if cls.endswith('source') else 'the node that this vision goes back to'})
    # Node changes of the compiled libraries. A change is given to a node when the method that makes it names
    # resources of that node and of no other; the others are kept with their destination only.
    unattributed = []
    by_hand = json.loads(BY_HAND.read_text()) if BY_HAND.exists() else {'edges': [], 'wrong': []}
    # Calls that the rule below gives to the wrong node (it reads the strings of an earlier routine): their right
    # source is in compiled_by_hand.json.
    wrong = {(item['library'].upper(), item['address']) for item in by_hand.get('wrong', [])}
    read_by_hand = {(r['evidence']['library'].upper(), r['evidence']['address'], r['target']) for r in by_hand['edges']}
    for change in loaded['compiled']:
        target = (change['world'], change['zone'], change['node'])
        if (change['library'].upper(), change['address']) in wrong:
            continue
        owners = sorted({parse_id(text.split('.')[0].lower()) for text in change['resources']} - {None, target})
        evidence = {'via': 'library', 'library': change['library'], 'address': change['address'], 'call': change['call'],
                    'resources': change['resources'],
                    'note': 'a constant destination in compiled code; the method names resources of this node'}
        if len(owners) == 1 and owners[0] in nodes:
            if not any((node_id(*owners[0]), node_id(*target), kind) in edges for kind in ('move', 'auto', 'engine', 'return', 'amulet')):
                add(node_id(*owners[0]), target, 'engine', False, evidence)
        elif (change['library'].upper(), change['address'], node_id(*target)) not in read_by_hand:
            unattributed.append({'target': node_id(*target), 'library': change['library'], 'address': change['address'],
                                 'call': change['call'], 'stored': target in nodes})
    # Constant destinations read by hand in the disassembly, which the scan above does not find or cannot attribute
    # (other instructions stand between the constants and the call, or the routine is a callback handed over by a
    # method of the class), with the addresses that let anyone check them and the tests of the class kept as text,
    # never evaluated (compiled_by_hand.json, beside this file).
    for record in by_hand['edges']:
        target = parse_id(record['target'])
        if parse_id(record['source']) in nodes and target:
            add(record['source'], target, record['kind'], record['conditional'],
                dict(record['evidence'], tests=record.get('tests', [])))
    # The destination keeps the direction to face on arrival from a named node: when nothing else joins the two,
    # that record is the evidence of the edge.
    present = {(e['source'], e['target']) for e in edges.values()}
    for key, node in sorted(nodes.items()):
        for arrival in (node.info or {}).get('arrivals', []):
            match = re.fullmatch(r'w(\d)_?z(\d\d)_?n(\d\d\d)', arrival['entry'])
            if not match:
                continue
            origin = tuple(map(int, match.groups()))
            if origin in nodes and (node_id(*origin), node.id) not in present and origin != key:
                add(node_id(*origin), key, 'engine', False,
                    {'via': 'arrival', 'file': 'node.bin', 'pitch': round(arrival['pitch'], 2), 'yaw': round(arrival['yaw'], 2),
                     'note': 'the destination stores the direction to face when arriving from this node'})
    # A close-up records the node to go back to. When nothing stored opens the close-up, the edge that enters it
    # is inferred from that record and marked as such.
    entered = {(e['target']) for e in edges.values()}
    for key, node in sorted(nodes.items()):
        parent = getattr(node, 'parent', None)
        if parent and node.id not in entered and parse_id(parent) in nodes:
            add(parent, key, 'engine', False,
                {'via': 'close-up', 'inferred': True, 'file': next(p for p in node.ai if '/aisubnode/' in p),
                 'note': 'inferred: the close-up names this node as the one to go back to, and nothing stored opens it'})
    # The menu: the node of a new game, a parameter of the menu manager.
    start = None
    common = loaded['shared'].get('common')
    if common:
        for path, text in common.ai.items():
            values = {name: value for name, _, value in resources.ai(text)}
            if 'm_newGameNode.m_world' in values:
                start = node_id(int(values['m_newGameNode.m_world']), int(values['m_newGameNode.m_zone']),
                                int(values['m_newGameNode.m_node']))
                loaded['menu_nodes'] = sorted({node_id(int(values[f'm_MenuNode[{i}].m_world']), int(values[f'm_MenuNode[{i}].m_zone']),
                                                       int(values[f'm_MenuNode[{i}].m_node']))
                                               for i in range(40) if f'm_MenuNode[{i}].m_world' in values})
                main_menu = loaded['menu_nodes'][0] if loaded['menu_nodes'] else None
                if main_menu and parse_id(main_menu) in nodes:
                    add(main_menu, parse_id(start), 'engine', False,
                        {'via': 'class', 'classes': [{'class': path.split('/')[1], 'parameter': 'm_newGameNode', 'file': 'common/' + path}],
                         'note': 'the node of a new game, a parameter of the menu manager'})
                    for other in loaded['menu_nodes'][1:]:
                        if parse_id(other) in nodes:
                            for a, b in ((main_menu, other), (other, main_menu)):
                                add(a, parse_id(b), 'engine', False,
                                    {'via': 'class', 'classes': [{'class': path.split('/')[1], 'parameter': 'm_MenuNode', 'file': 'common/' + path}],
                                     'note': 'a page of the menu, listed by the menu manager'})
    # The first node of a new game has no stored exit. Its library holds one compiled transition to another node
    # of the Age: the edge is set from that, and marked as inferred.
    if start and not any(e['source'] == start for e in edges.values()):
        library = {5: 'm4aiw5', 1: 'm4aiw1'}.get(parse_id(start)[0], '')
        for change in loaded['compiled']:
            target = (change['world'], change['zone'], change['node'])
            if change['library'].lower().startswith(library) and change['call'] == 'Transition' and node_id(*target) != start and target in nodes:
                add(start, target, 'engine', False,
                    {'via': 'library', 'inferred': True, 'library': change['library'], 'address': change['address'], 'call': change['call'],
                     'note': 'inferred: the first node of a new game stores no exit, and its library holds this one compiled '
                             'transition to another node'})
    # A fixed view that stores no way out goes back where the player came from (the engine keeps the last node).
    outgoing = {e['source'] for e in edges.values()}
    for key, node in sorted(nodes.items()):
        if (node.info or {}).get('viewport') == 'fixed' and node.id not in outgoing:
            origins = sorted({e['source'] for e in edges.values() if e['target'] == node.id})
            for origin in origins:
                evidence = {'via': 'last node', 'inferred': True,
                            'note': 'inferred: a fixed view without a stored exit; the engine goes back to the last node'}
                if len(origins) > 1:
                    # Entered from several nodes, the view goes back to the one the player came from: the return
                    # belongs to that node, and following it from another would be a shortcut that the game does not have.
                    evidence['callerOwned'] = True
                    evidence['note'] += '; several nodes enter this view, and each return belongs to the node it goes back to'
                add(node.id, parse_id(origin), 'return', False, evidence)
    zip_places = sorted({node_id(int(m.group(1)), int(m.group(2)), int(m.group(3)))
                         for path, _, _ in (common.files if common else [])
                         for m in [re.match(r'zip/w(\d)/z(\d\d)/n(\d\d\d)/normal\.png$', path)] if m})
    # Parameters named after a destination whose value names another one: the name kept an older numbering.
    renamed = []
    for key, node in sorted(nodes.items()):
        for path, text in sorted(node.ai.items()):
            for name, _, value in resources.ai(text):
                match = re.match(r'm_(?:h|hs|transition)_?[tT]o_(w\d_z\d\d_n\d\d\d)', name)
                inside = re.search(r'h_to_(w\d_z\d\d_n\d\d\d)', value)
                if match and inside and match.group(1) != inside.group(1):
                    renamed.append({'node': node.id, 'file': path, 'parameter': name, 'value': value,
                                    'nameSays': match.group(1), 'valueSays': inside.group(1),
                                    'nameStored': parse_id(match.group(1)) in nodes})
    # Names of nodes that the data mentions and that are not stored (old names kept in parameters).
    mentions = defaultdict(set)
    for key, node in sorted(nodes.items()):
        for path, text in node.ai.items():
            for line in text.splitlines():
                for match in NODE_NAME.finditer(line.split(';')[0]):
                    if parse_id(match.group(0)) not in nodes and match.group(0)[-3:] != '000':
                        mentions[match.group(0)].add((node.id, path))

    graph_nodes = []
    for key, node in sorted(nodes.items()):
        info = node.info or {}
        values = info.get('values', [0] * 6)
        graph_nodes.append({
            'id': node.id, 'world': key[0], 'zone': key[1], 'node': key[2], 'group': WORLDS.get(key[0], f'World {key[0]}'),
            'room': f'w{key[0]}_z{key[1]:02d}', 'label': node.id,
            'type': 'panorama' if info.get('viewport') == 'free' else 'fixed view', 'virtual': False,
            'parent': getattr(node, 'parent', None),
            'position': [round(v, 4) for v in values[:3]], 'camera': [round(v, 2) for v in values[3:6]],
            'arrivals': info.get('arrivals', []),
            'layers': [layer['name'] for layer in node.layers],
            'hotspots': node.hotspots, 'classes': sorted({path.split('/')[1] for path in node.ai}),
            'blocks': len(node.blocks), 'archive': {'path': node.path, 'offset': node.offset, 'size': node.size},
            'errors': node.errors})
    for target, sources in sorted(absent.items()):
        world, zone, number = parse_id(target)
        graph_nodes.append({'id': target, 'world': world, 'zone': zone, 'node': number, 'group': WORLDS.get(world, ''),
                            'room': f'w{world}_z{zone:02d}', 'label': target, 'type': 'absent', 'virtual': True,
                            'parent': None, 'position': None, 'camera': None, 'arrivals': [], 'layers': [], 'hotspots': [],
                            'classes': [], 'blocks': 0, 'archive': None, 'errors': [], 'namedBy': sorted(set(sources))})
    world_of = {n['id']: n['world'] for n in graph_nodes}
    room_of = {n['id']: n['room'] for n in graph_nodes}
    edge_list = []
    for (source, target, kind), edge in sorted(edges.items()):
        edge.update({'callerOwned': all(e.get('callerOwned') for e in edge['evidence']),
                     'crossRoom': room_of[source] != room_of[target], 'crossAge': world_of[source] != world_of[target],
                     'self': source == target, 'movie': any(e.get('via') == 'transition' for e in edge['evidence'])})
        edge_list.append(edge)

    zip_targets = []
    for key, node in sorted(nodes.items()):
        for name, block in node.blocks.items():
            for command in block['commands']:
                match = re.match(r'\s*ZipEnableNode\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*(\w+)', command)
                if match:
                    zip_targets.append({'node': node.id, 'block': name, 'target': node_id(*map(int, match.groups()[:3])),
                                        'enabled': match.group(4) == 'true'})
    graph = {
        'schemaVersion': 1, 'title': 'Myst IV: Revelation',
        'semantics': 'Nodes are the places the game stores, one archive each. Edges are references to another node: '
                     'a ContextChange command, a transition, the return of a close-up, or a hotspot named for its '
                     'destination that a compiled class serves. Tests are kept as text and never evaluated.',
        'worlds': WORLDS, 'nodes': graph_nodes, 'edges': edge_list, 'commentedOut': commented,
        'zip': {'places': zip_places, 'enabledByCommand': zip_targets},
        'start': start, 'boot': loaded.get('boot'), 'menuNodes': loaded.get('menu_nodes', []),
        'compiledDestinations': {'unattributed': unattributed, 'computed': loaded['computed'], 'notice': loaded['notice'],
                                 'found': len(loaded['compiled'])},
        'renamedParameters': renamed,
        'danglingTransitions': dangling,
        'lastNodeChecks': last_node_checks,
        'nodeDatabase': ({k: v for k, v in loaded['database'].items() if k != 'pairs'} if loaded['database'] else None),
        'mentionsOfAbsentNodes': {name: sorted(f'{a} ({b})' for a, b in users) for name, users in sorted(mentions.items())},
    }
    by_library = defaultdict(list)
    for entry in unattributed:
        by_library[entry['target']].append({k: entry[k] for k in ('library', 'address', 'call')})
    for item in graph_nodes:
        item['zipPlace'] = item['id'] in zip_places
        item['compiledEntries'] = by_library.get(item['id'], [])
    graph['analysis'] = analyse(graph, start)
    kinds = Counter(e['kind'] for e in edge_list)
    graph['stats'] = {'nodes': len(graph_nodes), 'stored': sum(not n['virtual'] for n in graph_nodes),
                      'absent': sum(n['virtual'] for n in graph_nodes),
                      'panoramas': sum(n['type'] == 'panorama' for n in graph_nodes),
                      'fixedViews': sum(n['type'] == 'fixed view' for n in graph_nodes),
                      'edges': len(edge_list), 'edgeKinds': dict(sorted(kinds.items())),
                      'conditional': sum(e['conditional'] for e in edge_list),
                      'inferred': sum(any(ev.get('inferred') for ev in e['evidence']) for e in edge_list),
                      'callerOwned': sum(e['callerOwned'] for e in edge_list),
                      'crossAge': sum(e['crossAge'] for e in edge_list), 'movies': sum(e['movie'] for e in edge_list),
                      'commandBlocks': sum(len(n.blocks) for n in nodes.values()),
                      'commands': sum(len(b['commands']) for n in nodes.values() for b in n.blocks.values()),
                      'hotspots': sum(len(n.hotspots) for n in nodes.values()),
                      'zipPlaces': len(zip_places)}
    return graph, loaded


def movement(graph):
    """Unique ordered pairs of the movement projection.

    Left out: self-references, the commands that nothing stored starts, and the returns that belong to the node the
    player came from (a fixed view entered from several nodes).
    """
    return {(e['source'], e['target']) for e in graph['edges']
            if not e['self'] and e['kind'] != 'unreferenced' and not e.get('callerOwned')}


def analyse(graph, start):
    ids = [n['id'] for n in graph['nodes']]
    pairs = movement(graph)
    out_edges, in_edges = defaultdict(set), defaultdict(set)
    for a, b in pairs:
        out_edges[a].add(b)
        in_edges[b].add(a)

    def distances(origin, forward=True):
        table = out_edges if forward else in_edges
        seen = {origin: 0}
        queue = deque([origin])
        while queue:
            current = queue.popleft()
            for following in sorted(table[current]):
                if following not in seen:
                    seen[following] = seen[current] + 1
                    queue.append(following)
        return seen

    from_start = distances(start)
    farthest = max(from_start.values())
    longest, longest_pairs = 0, []
    for origin in ids:
        reached = distances(origin)
        best = max(reached.values())
        if best > longest:
            longest, longest_pairs = best, []
        if best == longest and best:
            longest_pairs.extend((origin, k) for k, v in sorted(reached.items()) if v == best)
    parent = {i: i for i in ids}

    def find(x):
        while parent[x] != x:
            parent[x] = parent[parent[x]]
            x = parent[x]
        return x
    for a, b in pairs:
        parent[find(a)] = find(b)
    weak = Counter(find(i) for i in ids)
    world_of = {n['id']: n['world'] for n in graph['nodes']}
    room_of = {n['id']: n['room'] for n in graph['nodes']}
    reciprocal = sum(1 for a, b in pairs if (b, a) in pairs)
    return {
        'projection': 'unique ordered pairs of every edge kind but unreferenced commands and caller-owned returns; '
                      'self-references excluded',
        'start': start, 'uniquePairs': len(pairs), 'reciprocalPairs': reciprocal,
        'reciprocalShare': round(reciprocal / len(pairs), 4) if pairs else 0,
        'crossRoomPairs': sum(1 for a, b in pairs if room_of[a] != room_of[b]),
        'crossAgePairs': sum(1 for a, b in pairs if world_of[a] != world_of[b]),
        'sources': sorted(i for i in ids if not in_edges[i] and out_edges[i]),
        'sinks': sorted(i for i in ids if in_edges[i] and not out_edges[i]),
        'isolated': sorted(i for i in ids if not in_edges[i] and not out_edges[i]),
        'weakComponents': len(weak), 'largestWeakComponent': max(weak.values()),
        'reachableFromStart': len(from_start),
        'notReachedFromStart': sorted(i for i in ids if i not in from_start),
        'maximumDistanceFromStart': farthest,
        'mostDistantFromStart': sorted(k for k, v in from_start.items() if v == farthest),
        'largestShortestDistance': longest, 'largestShortestDistancePairs': longest_pairs[:20],
        'distanceFromStart': from_start,
        'caveat': 'Structural results only. A path is a chain of references; it says nothing about the state '
                  'those references test. Not reached does not mean unused.',
    }
