"""Assemble one picture per node from its cube of tiles.

A node stores its view as a cube: six faces of 6 x 6 tiles, 128 pixels each, in ``cube/layer_default.m4b``
(``set_default/<face>_<row>_<column>``). A tile is a JPEG, or a ``.zap`` (a JPEG for the colours followed by one
for the transparency); a tile that is entirely transparent is not stored. The faces are put back together with
Pillow and turned into one equirectangular picture with FFmpeg. What the game draws behind the cube (the sky)
stays black. Only the default layer is drawn: movies, masks, water
and the other layers that the game adds are left out.
"""
from __future__ import annotations

import concurrent.futures
import hashlib
import json
import re
import shutil
import struct
import subprocess
import tempfile
from pathlib import Path

from . import bigfile

PANORAMA_WIDTH, THUMB_WIDTH, TILE, GRID = 2048, 400, 128, 6
# FFmpeg cube order: right, left, up, down, front, back.
FACES = ('right', 'left', 'top', 'bottom', 'front', 'back')
TILE_NAME = re.compile(r'set_default/(\w+?)_(\d\d)_(\d\d)\.(jpg|zap)$')


def colours(data, suffix):
    """The JPEG of the colours of a tile."""
    if suffix == 'zap':
        header, _, _, _, rgb = struct.unpack_from('<IIIII', data, 0)
        return data[header:header + rgb]
    return data


def _run(arguments):
    subprocess.run(['ffmpeg', '-hide_banner', '-loglevel', 'error', '-y', *arguments], check=True,
                   capture_output=True, timeout=180)


def tiles(ref, node):
    """{(face, row, column): (suffix, offset, size)} of the default layer of a node, offsets in data.m4b."""
    layer = next((l for l in node.layers if l['name'] == 'layer_default'), None)
    if layer is None:
        return {}
    head = ref.read(layer['offset'], min(layer['size'], 1 << 16))
    try:
        files, _ = bigfile.parse(head)
    except Exception:
        files, _ = bigfile.parse(ref.read(layer['offset'], layer['size']))
    out = {}
    for path, size, offset in files:
        match = TILE_NAME.match(path)
        if match:
            out[(match.group(1), int(match.group(2)), int(match.group(3)))] = (match.group(4), layer['offset'] + offset, size)
    return out


def assemble(ref, found, image_path, thumb_path, work):
    """Paste the tiles into a strip of six faces, then let FFmpeg unroll the cube."""
    import io
    from PIL import Image
    strip = Image.new('RGB', (6 * GRID * TILE, GRID * TILE))
    digest = hashlib.sha256()
    for position, face in enumerate(FACES):
        for (name, row, column), (suffix, offset, size) in sorted(found.items()):
            if name != face:
                continue
            data = colours(ref.read(offset, size), suffix)
            digest.update(data)
            with Image.open(io.BytesIO(data)) as tile:
                strip.paste(tile.convert('RGB'), (position * GRID * TILE + (column - 1) * TILE, (row - 1) * TILE))
    if {name for name, _, _ in found} == {'front'}:
        # A fixed view stores its front face alone: it is kept flat, as the game shows it.
        face = strip.crop((4 * GRID * TILE, 0, 5 * GRID * TILE, GRID * TILE))
        face.save(image_path, quality=92)
        face.resize((THUMB_WIDTH, THUMB_WIDTH), Image.LANCZOS).save(thumb_path, quality=85)
        return digest.hexdigest(), 'frame', face.size
    source = Path(work) / 'strip.png'
    strip.save(source)
    height = PANORAMA_WIDTH // 2
    graph = (f'v360=input=c6x1:output=e:in_forder=rludfb:w={PANORAMA_WIDTH}:h={height}:'
             f'interp=lanczos,split[full][small];[small]scale={THUMB_WIDTH}:-2:flags=lanczos[thumb]')
    _run(['-i', str(source), '-filter_complex', graph,
          '-map', '[full]', '-q:v', '3', '-frames:v', '1', '-update', '1', str(image_path),
          '-map', '[thumb]', '-q:v', '4', '-frames:v', '1', '-update', '1', str(thumb_path)])
    return digest.hexdigest(), 'panorama', (PANORAMA_WIDTH, height)


def extract(loaded, out, wanted=None, workers=6):
    """Write the pictures under ``out/images`` and return the manifest."""
    if shutil.which('ffmpeg') is None:
        raise RuntimeError('FFmpeg is needed to assemble the panoramas')
    try:
        import PIL  # noqa: F401
    except ImportError:
        raise RuntimeError('Pillow is needed to put the tiles together (uv run --with pillow python -m revelation_graph ...)')
    out, ref = Path(out), loaded['ref']
    jobs = [node for key, node in sorted(loaded['nodes'].items()) if wanted is None or node.id in wanted]

    def process(node):
        folder, thumbs = out / 'images' / f'w{node.key[0]}', out / 'images' / 'thumbs' / f'w{node.key[0]}'
        folder.mkdir(parents=True, exist_ok=True)
        thumbs.mkdir(parents=True, exist_ok=True)
        image_path, thumb_path = folder / f'{node.id}.jpg', thumbs / f'{node.id}.jpg'
        record = {'node': node.id, 'image': f'images/w{node.key[0]}/{node.id}.jpg',
                  'thumbnail': f'images/thumbs/w{node.key[0]}/{node.id}.jpg', 'width': PANORAMA_WIDTH,
                  'height': PANORAMA_WIDTH // 2, 'layer': 'layer_default'}
        try:
            found = tiles(ref, node)
            if not found:
                record['error'] = 'no default layer'
                return record
            record['tiles'] = len(found)
            record['faces'] = sorted({face for face, _, _ in found})
            flat = set(record['faces']) == {'front'}
            record['kind'] = 'frame' if flat else 'panorama'
            if flat:
                record['width'], record['height'] = GRID * TILE, GRID * TILE
            if not (image_path.exists() and thumb_path.exists()):
                with tempfile.TemporaryDirectory(prefix='revelation-tiles-') as work:
                    record['sourceSha256'], _, _ = assemble(ref, found, image_path, thumb_path, work)
        except (subprocess.SubprocessError, OSError, struct.error) as error:
            record['error'] = str(getattr(error, 'stderr', b'') or error)[:300]
        return record

    manifest = {}
    with concurrent.futures.ThreadPoolExecutor(max_workers=workers) as pool:
        for record in pool.map(process, jobs):
            manifest[record['node']] = record
    (out / 'images-manifest.json').write_text(json.dumps(
        {'panoramaWidth': PANORAMA_WIDTH, 'projection': 'equirectangular, the front face at the centre',
         'views': manifest}, indent=1) + '\n')
    return manifest
