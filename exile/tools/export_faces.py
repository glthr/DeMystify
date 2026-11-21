"""Write the six stored faces of chosen panoramas, for figures.

Usage: python3 tools/export_faces.py DISCS generated LEIS:1 LIFO:47 LIFO:147
Files go to <output>/faces/<ROOM>/<index>-<face>.jpg, exactly as stored.
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from exile_graph import archive as m3a, disc  # noqa: E402


def main(discs, out, *views):
    source = disc.Source([discs])
    archives = {room: m3a.Archive(refs[0], room) for room, refs in source.node_archives().items()}
    for view in views:
        room, index = view.split(':')
        entry = archives[room].get(room, int(index))
        folder = Path(out) / 'faces' / room
        folder.mkdir(parents=True, exist_ok=True)
        for resource in entry.find(m3a.CUBE_FACE):
            (folder / f'{index}-{resource.face}.jpg').write_bytes(archives[room].read(resource))
        print(view, 'written')
    source.close()


if __name__ == '__main__':
    main(*sys.argv[1:])
