"""Extract script references, retaining the evidence behind every connection."""
from __future__ import annotations

import hashlib
import json
import re
import sys
from collections import Counter, defaultdict, deque
from pathlib import Path

from .archive import MohawkArchive
from .iso import discover_mhk
from . import native, scripts

STACKS = {'aspit': 'Menu', 'bspit': 'Boiler', 'gspit': 'Garden', 'jspit': 'Jungle',
          'ospit': 'Gehn', 'pspit': 'Prison', 'rspit': 'Rebel', 'tspit': 'Temple'}
TAGS = ('CARD', 'HSPT', 'NAME', 'RMAP', 'PLST', 'BLST')


def key(stack, card):
    return f'{stack}:{card}'


def load(inputs):
    archives = [MohawkArchive(source) for source in discover_mhk(inputs)]
    selected = defaultdict(lambda: defaultdict(dict))
    overlays = []
    data_archives = []
    for archive in archives:
        match = re.fullmatch(r'([abgjoprt])_data(\d*)\.mhk', Path(archive.source.name).name.lower())
        if match:
            data_archives.append((int(match[2] or 0), archive.source.name, match[1] + 'spit', archive))
    for _, _, stack, archive in sorted(data_archives):
        for tag in TAGS:
            for rid in archive.ids(tag):
                if rid in selected[stack][tag]:
                    overlays.append({'stack': stack, 'tag': tag, 'id': rid,
                                     'overridden': selected[stack][tag][rid].source.name,
                                     'selected': archive.source.name})
                selected[stack][tag][rid] = archive
    if not selected:
        raise ValueError('No Riven *_Data*.mhk archives found in the supplied ISO files or directory')
    missing = set(STACKS) - selected.keys()
    if missing:
        raise ValueError(f'Incomplete game: missing stacks {sorted(missing)}; provide all five discs')
    return archives, selected, overlays


def provenance(archive, tag, rid):
    resource = archive.resources[tag][rid]
    return {**archive.source.origin, 'resource': f'{tag}:{rid}',
            'resource_offset': resource.offset, 'resource_size': resource.size,
            'absolute_offset': archive.source.offset + resource.offset,
            'sha256': hashlib.sha256(archive.read(tag, rid)).hexdigest()}


def build(inputs):
    archives, selected, overlays = load(inputs)
    rmaps, names, cards, manifest_resources = {}, {}, {}, []
    stats = Counter()
    opcode_counts = Counter()
    warnings = []
    for stack in sorted(selected):
        resources = selected[stack]
        rmaps[stack] = scripts.parse_rmap(resources['RMAP'][1].read('RMAP', 1))
        names[stack] = {i: scripts.parse_names(a.read('NAME', i)) for i, a in resources['NAME'].items()}
        for tag in TAGS:
            for rid, archive in sorted(resources[tag].items()):
                manifest_resources.append({'stack': stack, **provenance(archive, tag, rid)})
        for rid, archive in sorted(resources['CARD'].items()):
            card = scripts.parse_card(archive.read('CARD', rid))
            hs_archive = resources['HSPT'][rid]
            hotspots = scripts.parse_hotspots(hs_archive.read('HSPT', rid))
            pictures = scripts.parse_plst(resources['PLST'][rid].read('PLST', rid)) if rid in resources['PLST'] else []
            if rid >= len(rmaps[stack]):
                raise ValueError(f'{stack}:{rid}: no RMAP entry')
            card.update(id=key(stack, rid), stack=stack, card_id=rid,
                        label=f'{STACKS[stack]}:{rid}',
                        name=scripts.name_at(names[stack], 1, card['name_id']) if card['name_id'] >= 0 else '',
                        global_id=rmaps[stack][rid], global_id_hex=f'0x{rmaps[stack][rid]:08x}',
                        hotspots=hotspots, pictures=pictures,
                        provenance={'CARD': provenance(archive, 'CARD', rid),
                                    'HSPT': provenance(hs_archive, 'HSPT', rid)})
            for hotspot in hotspots:
                hotspot['name'] = scripts.name_at(names[stack], 2, hotspot['name_id']) if hotspot['name_id'] >= 0 else ''
            cards[card['id']] = card
            stats['hotspot_count'] += len(hotspots)
    edge_map = {}
    unresolved = []
    virtual = {}

    def add(source, target_stack, target_card, kind, evidence):
        resolved = native.resolve_destination(target_stack, target_card, rmaps)
        target = key(resolved['stack'], resolved['card_id'])
        evidence = dict(evidence)
        evidence['literal_target'] = key(target_stack, target_card)
        if resolved['redirected']:
            evidence['native_redirects'] = resolved['redirect_chain']
        if target not in cards:
            evidence['missing_target'] = True
            stats['missing_reference_count'] += 1
            address = rmaps[resolved['stack']][resolved['card_id']] if resolved['card_id'] < len(rmaps[resolved['stack']]) else None
            virtual[target] = {'id': target, 'stack': resolved['stack'], 'card_id': resolved['card_id'],
                               'label': f'{STACKS[resolved["stack"]]}:{resolved["card_id"]}',
                               'name': '[Missing CARD resource]', 'global_id': address,
                               'global_id_hex': f'0x{address:08x}' if address is not None else 'absent',
                               'zip_mode_place': 0, 'hotspot_count': 0, 'image_count': 0, 'virtual': True}
        edge_key = source, target, kind
        if edge_key not in edge_map:
            edge_map[edge_key] = {'source': source, 'target': target, 'kind': kind,
                                  'cross_stack': cards[source]['stack'] != resolved['stack'], 'evidence': []}
        edge_map[edge_key]['evidence'].append(evidence)

    def native_edge(source, transition, evidence):
        if transition.get('target_stack') is None:
            native_evidence = dict(transition)
            native_evidence['engine_source'] = native_evidence.pop('source')
            unresolved.append({'source': source, 'reason': 'caller-sensitive saved return', **evidence, **native_evidence})
            return
        dest_stack = transition['target_stack']
        dest_card = transition.get('target_card_id')
        if dest_card is None:
            dest_card = native.local_card_id(dest_stack, transition['target_rmap'], rmaps)
        kind = transition['kind']
        if not kind.startswith('native'):
            kind = 'native-' + kind
        # `source` in the native record is the engine URL, not the graph node.
        native_evidence = {**transition}
        native_evidence['engine_source'] = native_evidence.pop('source')
        add(source, dest_stack, dest_card, kind, {**evidence, **native_evidence})

    for card in cards.values():
        stack, source = card['stack'], card['id']
        contexts = [(None, card['scripts'])] + [(h, h['scripts']) for h in card['hotspots']]
        for hotspot, typed in contexts:
            for script in typed:
                for cmd, guards in scripts.walk_commands([script]):
                    stats['command_count'] += 1
                    opcode_counts[cmd['opcode']] += 1
                    ev = {'resource': f'{"HSPT" if hotspot is not None else "CARD"}:{card["card_id"]}',
                          'event': script['event'], 'byte_offset': cmd['offset'],
                          'opcode': cmd['opcode'],
                          'guards': [{**g, 'variable_name': scripts.name_at(names[stack], 4, g['variable'])} for g in guards]}
                    if hotspot is not None:
                        ev.update(hotspot_ordinal=hotspot['ordinal'], hotspot_blst_id=hotspot['blst_id'],
                                  hotspot_name=hotspot['name'], hotspot_rect=hotspot['rect'],
                                  hotspot_valid_rect=hotspot['valid_rect'])
                    for warning in cmd.get('warnings', []):
                        warnings.append({'card': source, **ev, 'warning': warning})
                    op = cmd['opcode']
                    if op == 2:
                        add(source, stack, cmd['args'][0], 'script', ev)
                    elif op == 27:
                        target_stack = scripts.name_at(names[stack], 5, cmd['stack_name_id']).lower()
                        target_card = native.local_card_id(target_stack, cmd['global_card_id'], rmaps)
                        add(source, target_stack, target_card, 'script', ev)
                    elif op == 17:
                        external = scripts.name_at(names[stack], 3, cmd['args'][0])
                        for transition in native.external_transitions(stack, external):
                            native_edge(source, transition, {**ev, 'external': external, 'external_args': cmd['args'][2:]})
                    elif op == 45:
                        name = hotspot['name'] if hotspot else ''
                        targets = [c for c in cards.values() if c['stack'] == stack and c['zip_mode_place'] and c['name'] == name] if name else []
                        if not targets:
                            unresolved.append({'source': source, 'reason': 'zip target not identified', **ev})
                        for target in targets:
                            add(source, stack, target['card_id'], 'zip', {**ev, 'condition': 'Zip enabled; destination previously visited', 'zip_name': name})
                    elif op == 38 and cmd.get('stored_opcode') in (2, 27):
                        if cmd['stored_opcode'] == 2:
                            add(source, stack, cmd['stored_args'][0], 'script-deferred', {**ev, 'movie_slot': cmd['movie_slot'], 'movie_time': cmd['movie_time']})
                        else:
                            unresolved.append({'source': source, 'reason': 'deferred stack change requires review', **ev})
        for transition in native.timer_transitions(stack, card['global_id']):
            native_edge(source, transition, {'event': 'native_timer'})
    nodes = []
    details = {}
    for source, card in cards.items():
        nodes.append({k: card[k] for k in ['id', 'stack', 'card_id', 'label', 'name', 'global_id', 'global_id_hex', 'zip_mode_place']} |
                     {'hotspot_count': len(card['hotspots']), 'image_count': len(card['pictures']), 'virtual': False})
        text = f'// {card["label"]} ({card["name"]}) RMAP {card["global_id_hex"]}\n\n// CARD:{card["card_id"]}\n'
        text += scripts.format_scripts(card['scripts'], names[card['stack']]) + '\n'
        for hotspot in card['hotspots']:
            text += f'\n// HSPT:{card["card_id"]} hotspot {hotspot["blst_id"]}: {hotspot["name"]} rect={hotspot["rect"]} flags={hotspot["flags"]}\n'
            text += scripts.format_scripts(hotspot['scripts'], names[card['stack']]) + '\n'
        details[source] = {'text': text, 'provenance': card['provenance'],
                           'unresolved': [u for u in unresolved if u['source'] == source]}
    for source, node in sorted(virtual.items()):
        nodes.append(node)
        details[source] = {'text': '// Virtual target: scripts refer to this local CARD id, but no CARD resource exists in the selected archives.\n// This preserves the reference and does not imply the card is playable.',
                           'provenance': {'status': 'missing CARD resource'}, 'unresolved': []}
    edges = sorted(edge_map.values(), key=lambda e: (e['source'], e['target'], e['kind']))
    stats.update(card_count=len(cards), node_count=len(nodes), virtual_node_count=len(virtual),
                 parse_errors=0, unresolved_navigation_count=len(unresolved))
    graph = {'schema_version': 1, 'title': 'Riven (1997) card reference graph',
             'semantics': 'Static union of script references and separately attributed native transitions. No global game-state execution. Paths do not prove playable routes.',
             'start_card': key('tspit', native.local_card_id('tspit', 0x6E9A, rmaps)),
             'stacks': [{'id': s, 'name': STACKS[s], 'card_count': sum(n['stack'] == s and not n['virtual'] for n in nodes)} for s in sorted(STACKS)],
             'nodes': nodes, 'edges': edges, 'unresolved_navigation': unresolved,
             'inventory': native.inventory_model(),
             'stats': {**stats, 'edge_count': len(edges), 'edge_kinds': dict(Counter(e['kind'] for e in edges)),
                       'opcode_counts': dict(sorted(opcode_counts.items())),
                       'unknown_opcodes': sorted(op for op in opcode_counts if op not in scripts.OPCODES),
                       'semantic_warning_count': len(warnings)}}
    graph['analysis'] = analyze(graph)
    manifest = {'scummvm_commit': native.SCUMMVM_COMMIT, 'sources': [a.metadata() for a in archives],
                'overrides': overlays, 'selected_resources': manifest_resources,
                'warnings': warnings, 'resource_hash_policy': 'SHA-256 of every selected decoded resource, not full ISO/media hashing'}
    ast = {'names': names, 'rmaps': rmaps, 'cards': list(cards.values())}
    return graph, details, manifest, ast


def analyze(graph):
    """Structural analysis of unique pairs, excluding Zip and reset/ending edges."""
    ids = {n['id'] for n in graph['nodes']}
    adj, rev = {n: set() for n in ids}, {n: set() for n in ids}
    for e in graph['edges']:
        if e['kind'] in ('zip', 'native-ending', 'native-new-game'):
            continue
        adj[e['source']].add(e['target'])
        rev[e['target']].add(e['source'])
    seen = {graph['start_card']}
    queue = deque(seen)
    distance = {graph['start_card']: 0}
    while queue:
        source = queue.popleft()
        for target in sorted(adj[source] - seen):
            seen.add(target)
            queue.append(target)
            distance[target] = distance[source] + 1
    # Kosaraju, iterative to handle chains longer than Python's recursion limit.
    visited, order = set(), []
    for n in sorted(ids):
        if n in visited:
            continue
        todo = [(n, False)]
        while todo:
            v, leave = todo.pop()
            if leave:
                order.append(v)
            elif v not in visited:
                visited.add(v)
                todo.append((v, True))
                todo.extend((w, False) for w in sorted(adj[v], reverse=True) if w not in visited)
    visited, components = set(), []
    for n in reversed(order):
        if n in visited:
            continue
        todo, component = [n], []
        visited.add(n)
        while todo:
            v = todo.pop()
            component.append(v)
            for w in rev[v] - visited:
                visited.add(w)
                todo.append(w)
        components.append(sorted(component))
    return {'projection': 'unique pairs; Zip, ending reset and new-game transitions excluded; native movement included',
            'unique_pairs': sum(map(len, adj.values())),
            'self_loops': sorted(n for n in ids if n in adj[n]),
            'sources': sorted(n for n in ids if not rev[n] - {n}),
            'sinks': sorted(n for n in ids if not adj[n] - {n}),
            'isolated': sorted(n for n in ids if not (adj[n] | rev[n]) - {n}),
            'strongly_connected_components': sorted(components, key=lambda c: (-len(c), c)),
            'reachable_from_start': len(seen), 'not_reached_in_projection': sorted(ids - seen),
            'maximum_distance_from_start': max(distance.values()),
            'most_distant_from_start': sorted(n for n in distance if distance[n] == max(distance.values())),
            'caveat': 'Not reached means absent from this static projection, not proven inaccessible during play. Native UI/returns and global state are not expanded.'}


def report(graph, manifest):
    stats, a = graph['stats'], graph['analysis']
    lines = ['# Riven (1997): extracted card graph', '',
             'Generated from the supplied five-CD release. This is a disassembly of the Mohawk data and its script bytecode, not recovered original C++ source.', '',
             f'**{stats["card_count"]:,} shipped cards + {stats["virtual_node_count"]} virtual missing targets, {stats["hotspot_count"]:,} hotspots, {stats["command_count"]:,} decoded commands, {stats["edge_count"]:,} typed directed edges.**', '',
             '| Stack | Internal name | Cards |', '|---|---|---:|']
    lines += [f'| {s["name"]} | {s["id"]} | {s["card_count"]} |' for s in graph['stacks']]
    lines += ['', '## What an edge means', '',
              'A script edge means a CARD or HSPT script contains a change-card or change-stack command. All nested switch branches are retained, with variable names and guards attached to the edge evidence. Repeated references merge only when source, resolved target and kind agree; every original callsite remains in the evidence array.', '',
              'This is a reference graph. Sequential commands, automatic redirects, hotspot activation and state dependencies are not executed. A script that visits several cards contributes several references from its owner; these must not be read as independent player actions. Branch guards describe local script tests, not a complete global precondition.', '',
              'The 13 native cross-stack destination redirects are applied when resolving a requested target. The literal destination and engine redirect remain in the evidence; a redirect is not misrepresented as an outgoing script on the placeholder card.', '',
              '| Kind | Edges |', '|---|---:|']
    lines += [f'| {kind} | {count} |' for kind, count in sorted(stats['edge_kinds'].items())]
    lines += ['', '## Verification and provenance', '',
              f'- {stats["parse_errors"]} parsing errors; CARD, HSPT and PLST parsers require exact end-of-resource consumption.',
              f'- {len(manifest["sources"])} Mohawk archives discovered without mounting or unpacking the ISO files.',
              f'- {len(manifest["overrides"])} resources overridden using CD archive precedence (J_DATA3 > J_DATA2 > J_DATA1; B_DATA1 > B_DATA).',
              '- Selected resource SHA-256 hashes, ISO offsets, archive paths and override records are in `manifest.json`.',
              '- `cards.json` contains the full parsed script AST, names, RMAPs, hotspot rectangles and picture references. `scripts/` contains one readable disassembly per card.',
              f'- {stats["missing_reference_count"]} script callsites refer to {stats["virtual_node_count"]} absent CARD resources. These references remain as edges into explicitly virtual nodes: ' + ', '.join(sorted(n['id'] for n in graph['nodes'] if n['virtual'])) + '.',
              f'- {stats["semantic_warning_count"]} semantic warnings retained in the manifest. Opcode 36 is decoded structurally but its purpose remains unknown in the reference engine; raw arguments are preserved.', '',
              '## Structural findings', '',
              f'The movement projection has {a["unique_pairs"]:,} unique directed pairs, {len(a["strongly_connected_components"]):,} strongly connected components, {len(a["sources"])} source cards, {len(a["sinks"])} sink cards and {len(a["isolated"])} isolated cards. Self-loops do not prevent a card from being counted as a source or sink.', '',
              f'From {graph["start_card"]} (the new-game gameplay entry), this projection reaches {a["reachable_from_start"]:,} cards; its largest start distance is {a["maximum_distance_from_start"]} references. These are topological results, not playability claims. Zip, new-game and ending-reset edges are excluded from this analysis. See graph.json for exact node lists.', '',
              f'{stats["unresolved_navigation_count"]} dynamic navigation callsites are retained separately. Inventory and menu returns depend on saved caller state and are not expanded into all-to-all edges. The inventory overlay is described in graph.json metadata.', '',
              '## Limits', '',
              '- Native behavior is reconstructed from the pinned ScummVM implementation, rather than decompiled from the original executable. Native edges are explicitly attributed. Engine fixes can differ from the original interpreter.',
              '- Original disc scripts are decoded without ScummVM runtime script patches. Raw hotspot flags and rectangles are preserved; no static enable/disable flag is treated as proof of permanent accessibility.',
              '- Journal page changes, picture substitutions, sounds, videos and puzzle-state changes remain within a card and are not extra navigation vertices. Picture IDs and rectangles are exported; raster/media decoding is outside this graph extractor.',
              '- Loading arbitrary savegames, opening the system menu, keyboard navigation and global inventory access are not ordinary card-to-card edges. External calls without navigation are retained in the disassembly.',
              '- No claim that a source, sink or unreached card is unused or invisible follows from this graph alone.', '',
              '## References', '',
              '- [DeMystify](https://github.com/glthr/DeMystify) and [The Myst Graph technical article](https://glthr.com/myst-graph-3): inspiration for a graph whose vertices are cards and whose edges retain structural references.',
              '- [Riven Navigator](https://glthr.com/riven-navigator) and [Riven Diffs](https://glthr.com/riven-diffs): context for cards having multiple image states.',
              f'- [ScummVM Mohawk engine, pinned commit {native.SCUMMVM_COMMIT[:12]}]({native.SOURCE_BASE}): archive layout, opcode semantics, RMAP/name resolution, native redirects and external navigation.', '',
              'This local output contains game-derived script data. The extractor source does not require redistributing the game archives.', '']
    return '\n'.join(lines)


def write_outputs(out, graph, details, manifest, ast):
    from .render import exports, layout, explorer
    out = Path(out)
    out.mkdir(parents=True, exist_ok=True)
    exports(out, graph)
    layout(out, graph)
    for filename, obj in [('graph.json', graph), ('manifest.json', manifest), ('cards.json', ast)]:
        (out / filename).write_text(json.dumps(obj, indent=2) + '\n')
    (out / 'analysis.md').write_text(report(graph, manifest))
    for source, detail in details.items():
        stack, rid = source.split(':')
        directory = out / 'scripts' / stack
        directory.mkdir(parents=True, exist_ok=True)
        (directory / f'{rid}.txt').write_text(detail['text'])
    explorer(out, graph, details, ast)
