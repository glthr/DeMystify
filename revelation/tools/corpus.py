"""Collect every small file of every node (all but the picture tiles) into one JSON corpus, for study."""
import json, re, sys
sys.path.insert(0, '.')
from revelation_graph import bigfile, disc, resources as r

source = disc.Source(sys.argv[1:-1])
ref = [f for f in source.find_path('common/data/data.m4b')][0]
files, _ = bigfile.parse(ref.read(0, 1 << 20))
out = {'origin': ref.origin, 'files': files, 'nodes': {}, 'other': {}}
for path, size, offset in files:
    if not path.endswith('.m4b'):
        if size < 100_000:
            out['other'][path] = ref.read(offset, size).hex()
        continue
    head = ref.read(offset, min(size, 1 << 18))
    try:
        inner, _ = bigfile.parse(head)
    except Exception:
        inner, _ = bigfile.parse(ref.read(offset, size))
    entry = {'size': size, 'offset': offset, 'files': inner, 'ai': {}, 'commands': {}, 'bin': {}, 'errors': []}
    for p, s, o in inner:
        if p.endswith('.m4b') or p.endswith('.log') or s > 400_000:
            continue
        data = ref.read(offset + o, s)
        try:
            if p.endswith('.ai'):
                entry['ai'][p] = data.decode('latin-1')
            elif p.startswith('command_block/'):
                entry['commands'][p] = r.command_block(data)
            elif p == 'node.bin':
                entry['node'] = r.node(data)
            elif p.endswith('.bin'):
                entry['bin'][p] = {'type': data[8:12].hex(), 'size': s, 'strings': [t for _, t in r.strings(data)][:400]}
            else:
                entry['bin'][p] = {'type': 'raw', 'size': s}
        except Exception as error:
            entry['errors'].append(f'{p}: {error}')
    out['nodes'][path] = entry
json.dump(out, open(sys.argv[-1], 'w'))
print(len(out['nodes']), 'archives', sum(len(e['errors']) for e in out['nodes'].values()), 'errors')
