"""Extract one picture per stored view: panoramas for cubes, the frame otherwise.

A panoramic node stores six 640x640 JPEG faces. They are assembled into one
equirectangular picture with FFmpeg, so that the whole place can be seen at
once and its zones drawn over it. Flat frames are copied as they are stored.

Face numbers, from the cube geometry of the engine: 1 looks at heading 180,
2 is the floor, 3 looks at heading 0, 4 at heading 270, 5 at heading 90 and
6 is the sky.
"""
from __future__ import annotations

import concurrent.futures
import hashlib
import json
import shutil
import struct
import subprocess
import tempfile
from pathlib import Path

from . import archive as m3a

PANORAMA_WIDTH, THUMB_WIDTH = 2048, 400
# FFmpeg cube order: right, left, up, down, front, back.
FACE_ORDER = (5, 4, 6, 2, 3, 1)


def jpeg_size(data):
    """Width and height from the frame header of a JPEG stream."""
    pos = 2
    while pos + 9 < len(data):
        if data[pos] != 0xFF:
            pos += 1
            continue
        marker = data[pos + 1]
        if marker in (0xC0, 0xC1, 0xC2):
            height, width = struct.unpack_from('>HH', data, pos + 5)
            return width, height
        if marker in (0xD8, 0x01) or 0xD0 <= marker <= 0xD7:
            pos += 2
            continue
        pos += 2 + struct.unpack_from('>H', data, pos + 2)[0]
    return 0, 0


def _run(arguments):
    subprocess.run(['ffmpeg', '-hide_banner', '-loglevel', 'error', '-y', *arguments], check=True,
                   capture_output=True, timeout=120)


def _panorama(faces, image_path, thumb_path, work):
    inputs = []
    for position, face in enumerate(FACE_ORDER):
        path = work / f'face{position}.jpg'
        path.write_bytes(faces[face])
        inputs += ['-i', str(path)]
    height = PANORAMA_WIDTH // 2
    graph = (f'[0][1][2][3][4][5]hstack=inputs=6,v360=input=c6x1:output=e:in_forder=rludfb:'
             f'w={PANORAMA_WIDTH}:h={height}:interp=lanczos,split[full][small];'
             f'[small]scale={THUMB_WIDTH}:-2:flags=lanczos[thumb]')
    _run([*inputs, '-filter_complex', graph, '-map', '[full]', '-q:v', '3', '-frames:v', '1', '-update', '1',
          str(image_path), '-map', '[thumb]', '-q:v', '4', '-frames:v', '1', '-update', '1', str(thumb_path)])
    return PANORAMA_WIDTH, height


def _frame(data, image_path, thumb_path):
    image_path.write_bytes(data)
    _run(['-i', str(image_path), '-vf', f'scale={THUMB_WIDTH}:-2:flags=lanczos', '-q:v', '4', '-frames:v', '1',
          '-update', '1', str(thumb_path)])
    return jpeg_size(data)


def views(archives):
    """Every cube and frame stored in the archives, as (room, index, kind, archive, resources)."""
    seen = set()
    opened_archives = [(None, a) for a in archives.shared] + list(archives.rooms.items())
    for _, opened in sorted(opened_archives, key=lambda item: item[1].ref.name):
        for entry in opened.entries:
            faces = [entry.find(m3a.CUBE_FACE, face) for face in range(1, 7)]
            if all(faces) and (entry.room, entry.index, 'cube') not in seen:
                seen.add((entry.room, entry.index, 'cube'))
                yield entry.room, entry.index, 'cube', opened, [f[0] for f in faces]
            frames = entry.find(m3a.FRAME) or entry.find(70)
            if frames and (entry.room, entry.index, 'frame') not in seen:
                seen.add((entry.room, entry.index, 'frame'))
                yield entry.room, entry.index, 'frame', opened, frames[:1]


def extract(archives, out, wanted_rooms=None, workers=8):
    """Write the pictures under ``out/images`` and return the manifest."""
    if shutil.which('ffmpeg') is None:
        raise RuntimeError('FFmpeg is needed to assemble the panoramas')
    out = Path(out)
    jobs = [job for job in views(archives) if wanted_rooms is None or job[0] in wanted_rooms]
    manifest = {}

    def process(job):
        room, index, kind, opened, resources = job
        safe_room = ''.join(c if c.isalnum() else '_' for c in room)
        folder, thumbs = out / 'images' / safe_room, out / 'images' / 'thumbs' / safe_room
        folder.mkdir(parents=True, exist_ok=True)
        thumbs.mkdir(parents=True, exist_ok=True)
        name = f'{index}-{kind}.jpg'
        image_path, thumb_path = folder / name, thumbs / name
        blobs = [opened.read(resource) for resource in resources]
        digest = hashlib.sha256(b''.join(blobs)).hexdigest()
        record = {'room': room, 'index': index, 'kind': kind, 'archive': opened.ref.name,
                  'image': f'images/{safe_room}/{name}', 'thumbnail': f'images/thumbs/{safe_room}/{name}',
                  'sourceSha256': digest,
                  'resources': [{'face': r.face, 'type': r.type_name, 'offset': r.offset, 'size': r.size}
                                for r in resources]}
        try:
            if kind == 'cube':
                record['faceSize'] = list(jpeg_size(blobs[0]))
                if image_path.exists() and thumb_path.exists():
                    record['width'], record['height'] = PANORAMA_WIDTH, PANORAMA_WIDTH // 2
                else:
                    with tempfile.TemporaryDirectory(prefix='exile-faces-') as work:
                        faces = {resource.face: blob for resource, blob in zip(resources, blobs)}
                        record['width'], record['height'] = _panorama(faces, image_path, thumb_path, Path(work))
            else:
                if image_path.exists() and thumb_path.exists():
                    record['width'], record['height'] = jpeg_size(blobs[0])
                else:
                    record['width'], record['height'] = _frame(blobs[0], image_path, thumb_path)
        except (subprocess.SubprocessError, OSError) as error:
            record['error'] = str(error)[:300]
        return record

    with concurrent.futures.ThreadPoolExecutor(max_workers=workers) as pool:
        for record in pool.map(process, jobs):
            manifest[f'{record["room"]}:{record["index"]}:{record["kind"]}'] = record
    (out / 'images-manifest.json').write_text(json.dumps(
        {'panoramaWidth': PANORAMA_WIDTH, 'projection': 'equirectangular, heading 0 at the centre, '
                                                        'heading increasing to the right',
         'views': manifest}, indent=1) + '\n')
    return manifest
