"""Write the six faces of a node's cube as the game stores them (768 x 768), for figures that show a single face.

  uv run --with pillow python tools/export_faces.py DISCS OUT NODE [NODE...]     # OUT/faces/<node>/<face>.jpg
"""
import io
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from PIL import Image  # noqa: E402

from revelation_graph import build, images  # noqa: E402

loaded = build.load([sys.argv[1]])
wanted = {build.parse_id(name): name for name in sys.argv[3:]}
for key, name in wanted.items():
    node = loaded['nodes'][key]
    found = images.tiles(loaded['ref'], node)
    folder = Path(sys.argv[2]) / 'faces' / name
    folder.mkdir(parents=True, exist_ok=True)
    for face in images.FACES:
        picture = Image.new('RGB', (images.GRID * images.TILE, images.GRID * images.TILE))
        for (owner, row, column), (suffix, offset, size) in found.items():
            if owner == face:
                with Image.open(io.BytesIO(images.colours(loaded['ref'].read(offset, size), suffix))) as tile:
                    picture.paste(tile.convert('RGB'), ((column - 1) * images.TILE, (row - 1) * images.TILE))
        picture.save(folder / f'{face}.jpg', quality=93)
    print(name, sum(1 for k in found), 'tiles')
