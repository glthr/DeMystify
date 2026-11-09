#!/usr/bin/env python3
"""Decode named Myst PICT/SMC pictures into an assets.Prepare image directory.

Optional media tool: Python's standard library plus FFmpeg on PATH. Graph
import does not depend on this script or FFmpeg. Inputs can be native Macintosh
resource-bank files (resource forks preserved), raw .rsrc files, MacBinary .bin
files, or AppleDouble sidecars. Source files are never modified.

Only the original single embedded QuickTime SMC image is decoded. QuickDraw
drawing/compositing and other PICT codecs are not emulated. Unsupported pictures
are listed in extraction-report.json rather than replaced by fabricated pixels.
"""
from __future__ import annotations

import argparse
from concurrent.futures import ThreadPoolExecutor, as_completed
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import struct
import subprocess
import tempfile
import threading

PREFIXES = {'MY': 'Myst', 'CH': 'Channelwood Age', 'DU': 'Dunny Age',
            'ME': 'Mechanical Age', 'SE': 'Selenitic Age', 'ST': 'Stoneship Age'}
MAX_FORK = 256 << 20
WRITE_LOCK = threading.Lock()


def digest(data):
    return hashlib.sha256(data).hexdigest()


def span(data, start, count, label):
    if start < 0 or count < 0 or start > len(data) or count > len(data) - start:
        raise ValueError(f'{label} exceeds resource bounds')
    return data[start:start+count]


def resources(data):
    """Parse resource-map references and Pascal names with bounded offsets."""
    do, mo, dl, ml = struct.unpack('>4I', span(data, 0, 16, 'resource header'))
    payloads = span(data, do, dl, 'resource data')
    resource_map = span(data, mo, ml, 'resource map')
    type_offset, name_offset = struct.unpack('>HH', span(resource_map, 24, 4, 'map offsets'))
    types = span(resource_map, type_offset, ml-type_offset, 'type list')
    count_minus_one = struct.unpack('>H', span(types, 0, 2, 'type count'))[0]
    type_count = 0 if count_minus_one == 65535 else count_minus_one + 1
    span(types, 2, type_count*8, 'type entries')
    seen = set()
    for index in range(type_count):
        kind, count, ref_offset = struct.unpack('>4sHH', types[2+index*8:10+index*8])
        count += 1
        span(types, ref_offset, count*12, 'resource references')
        for i in range(count):
            ref = types[ref_offset+i*12:ref_offset+(i+1)*12]
            identifier, name_index = struct.unpack('>hH', ref[:4])
            key = kind, identifier
            if key in seen:
                raise ValueError(f'duplicate resource {kind!r} #{identifier}')
            seen.add(key)
            offset = int.from_bytes(ref[5:8], 'big')
            length = struct.unpack('>I', span(payloads, offset, 4, 'resource length'))[0]
            content = span(payloads, offset+4, length, 'resource payload')
            name = None
            if name_index != 65535:
                start = name_offset + name_index
                length = span(resource_map, start, 1, 'name length')[0]
                name = span(resource_map, start+1, length, 'resource name').decode('mac_roman')
            yield {'type': kind.decode('mac_roman'), 'id': identifier, 'name': name,
                   'payload': content, 'offset': do+offset+4}


def read_limited(path):
    if path.is_symlink() or not path.is_file() or path.stat().st_size > MAX_FORK:
        raise ValueError(f'expected regular resource bank no larger than {MAX_FORK} bytes: {path}')
    with path.open('rb') as stream:
        data = stream.read(MAX_FORK+1)
    if len(data) > MAX_FORK:
        raise ValueError(f'resource bank exceeds size limit: {path}')
    return data


def fork_bytes(path):
    """Return a resource fork plus its carrier type; do not alter native forks."""
    data = read_limited(path)
    if len(data) >= 26 and data[:4] in (b'\x00\x05\x16\x07', b'\x00\x05\x16\x00'):
        count = struct.unpack('>H', data[24:26])[0]
        span(data, 26, count*12, 'AppleDouble entry list')
        for i in range(count):
            kind, offset, length = struct.unpack('>III', data[26+i*12:38+i*12])
            if kind == 2:
                return span(data, offset, length, 'AppleDouble resource fork'), 'AppleDouble'
        raise ValueError('AppleDouble contains no resource fork')
    if path.suffix.lower() == '.rsrc':
        return data, 'raw resource fork'
    if path.suffix.lower() == '.bin' and len(data) >= 128 and data[0] == 0 and 1 <= data[1] <= 63:
        data_length, resource_length = struct.unpack('>II', data[83:91])
        offset = 128 + ((data_length+127)//128)*128
        return span(data, offset, resource_length, 'MacBinary resource fork'), 'MacBinary'
    native = Path(str(path) + '/..namedfork/rsrc')
    try:
        with native.open('rb') as stream:
            fork = stream.read(MAX_FORK+1)
        if len(fork) > MAX_FORK:
            raise ValueError('native resource fork exceeds size limit')
        if fork:
            return fork, 'native resource fork'
    except (FileNotFoundError, NotADirectoryError):
        pass
    sidecars = [path.parent/'.rsrc'/path.name, path.with_name('._'+path.name)]
    for sidecar in sidecars:
        if sidecar.exists():
            if sidecar.parent.name == '.rsrc':
                return read_limited(sidecar), 'separate .rsrc directory'
            return fork_bytes(sidecar)
    # A raw resource fork does not require a .rsrc extension.
    list(resources(data))
    return data, 'raw resource fork'


def smc_image(payload):
    candidates = []
    for marker in re.finditer(b'smc ', payload):
        start = marker.start() - 4
        if start < 0 or start+86 > len(payload):
            continue
        size = struct.unpack('>I', payload[start:start+4])[0]
        if size < 86 or size > len(payload)-start:
            continue
        description = payload[start:start+size]
        width, height = struct.unpack('>HH', description[32:36])
        length = struct.unpack('>I', description[44:48])[0]
        if width == 0 or height == 0 or width*height > 32 << 20 or length > len(payload)-start-size:
            continue
        packet = payload[start+size:start+size+length]
        candidates.append((description, packet, width, height))
    if len(candidates) != 1:
        raise ValueError('PICT does not contain exactly one supported SMC image')
    return candidates[0]


def atom(kind, data):
    return struct.pack('>I4s', len(data)+8, kind) + data


def full(kind, data, flags=0):
    return atom(kind, struct.pack('>I', flags)+data)


def smc_movie(description, packet, width, height):
    """Wrap original compressed bytes/palette in a one-frame QuickTime file."""
    description = bytearray(description)
    description[14:16] = b'\0\1'
    matrix = struct.pack('>9I', 0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000)
    table = full(b'stsd', struct.pack('>I', 1)+description)
    table += full(b'stts', struct.pack('>III', 1, 1, 600))
    table += full(b'stsc', struct.pack('>IIII', 1, 1, 1, 1))
    table += full(b'stsz', struct.pack('>II', len(packet), 1))
    table += full(b'stco', struct.pack('>II', 1, 8))
    dref = full(b'dref', struct.pack('>I', 1)+full(b'url ', b'', 1))
    minf = atom(b'minf', full(b'vmhd', b'\0'*8, 1)+atom(b'dinf', dref)+atom(b'stbl', table))
    mdhd = full(b'mdhd', struct.pack('>IIIIHH', 0, 0, 600, 600, 0, 0))
    hdlr = full(b'hdlr', b'\0'*4+b'vide'+b'\0'*12+b'Original SMC image\0')
    tkhd = full(b'tkhd', struct.pack('>IIIII', 0, 0, 1, 0, 600)+b'\0'*8+struct.pack('>hhhh', 0, 0, 0, 0)+matrix+struct.pack('>II', width<<16, height<<16), 7)
    mvhd = full(b'mvhd', struct.pack('>IIII', 0, 0, 600, 600)+struct.pack('>Ih', 0x10000, 0x100)+b'\0'*10+matrix+b'\0'*24+struct.pack('>I', 2))
    return atom(b'mdat', packet)+atom(b'moov', mvhd+atom(b'trak', tkhd+atom(b'mdia', mdhd+hdlr+minf)))


def decode(entry, output, ffmpeg):
    payload = entry.pop('_payload')
    description, packet, width, height = smc_image(payload)
    name = f'{entry["bank"]}-{entry["resource_id"]}-{entry["resource_sha256"][:16]}.png'
    target = output/name
    with tempfile.TemporaryDirectory(prefix='.decode-', dir=output) as temporary:
        movie = Path(temporary)/'image.mov'
        decoded = Path(temporary)/'image.png'
        movie.write_bytes(smc_movie(description, packet, width, height))
        run = subprocess.run([ffmpeg, '-hide_banner', '-loglevel', 'error', '-xerror', '-threads', '1',
                              '-i', str(movie), '-frames:v', '1', '-threads', '1', str(decoded)],
                             capture_output=True, text=True, timeout=60)
        if run.returncode != 0 or not decoded.exists():
            raise ValueError('FFmpeg could not decode SMC: '+run.stderr[:500])
        data = decoded.read_bytes()
        if len(data) < 24 or data[:8] != b'\x89PNG\r\n\x1a\n' or struct.unpack('>II', data[16:24]) != (width, height):
            raise ValueError('decoder output does not match original image dimensions')
        with WRITE_LOCK:
            if target.exists():
                if target.is_symlink() or target.read_bytes() != data:
                    raise ValueError('existing output differs from decoded source: '+name)
            else:
                with target.open('xb') as stream:
                    stream.write(data)
    return {**entry, 'path': name, 'width': width, 'height': height,
            'decoded_sha256': digest(data), 'codec': 'smc ',
            'method': 'original embedded SMC image; QuickDraw compositing not replayed'}


def bank_sources(source):
    for path in sorted(source.rglob('*')):
        if path.is_symlink() or not path.is_file():
            continue
        bank = path.name.removeprefix('._')
        for extension in ('.rsrc', '.bin'):
            if bank.lower().endswith(extension):
                bank = bank[:-len(extension)]
        if re.fullmatch(r'(?:ALL|MY|CH|DU|ME|SE|ST|IN)Res\d*', bank, re.I):
            yield path, bank


def prepare_output(output, source):
    marker = output/'.demystify-images.json'
    identity = {'tool': 'DeMystify image extractor', 'schema_version': 1, 'source': str(source)}
    if output.exists() and any(output.iterdir()):
        if marker.is_symlink() or not marker.is_file():
            raise ValueError('refusing a nonempty output directory not owned by this extractor')
        if json.loads(marker.read_text()) != identity:
            raise ValueError('output directory belongs to a different extractor source')
    output.mkdir(parents=True, exist_ok=True)
    if not marker.exists():
        with marker.open('x') as stream:
            json.dump(identity, stream); stream.write('\n')
    return identity


def atomic_json(output, name, value):
    destination = output/name
    if destination.is_symlink():
        raise ValueError('refusing a symlink metadata destination: '+name)
    with tempfile.NamedTemporaryFile(mode='w', encoding='utf-8', prefix='.'+name+'.',
                                     suffix='.tmp', dir=output, delete=False) as stream:
        temporary = Path(stream.name)
        try:
            json.dump(value, stream, indent=2, ensure_ascii=False)
            stream.write('\n'); stream.flush(); os.fsync(stream.fileno())
        except BaseException:
            temporary.unlink(missing_ok=True)
            raise
    try:
        temporary.replace(destination)
    finally:
        temporary.unlink(missing_ok=True)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('banks', type=Path, help='original Myst Graphics banks or extracted resource-fork directory')
    parser.add_argument('--output', type=Path, required=True, help='decoded image directory (keep game images outside Git)')
    parser.add_argument('--ffmpeg', default='ffmpeg')
    parser.add_argument('--workers', type=int, default=4)
    args = parser.parse_args(argv)
    if args.output.is_symlink():
        parser.error('output directory must not be a symlink')
    source, output = args.banks.resolve(), args.output.resolve()
    if not source.is_dir() or source == output or source in output.parents or output in source.parents:
        parser.error('source/output must be separate non-overlapping directories')
    ffmpeg = shutil.which(args.ffmpeg)
    if ffmpeg is None:
        parser.error('FFmpeg is required only for this optional media extraction step')
    if not 1 <= args.workers <= 16:
        parser.error('--workers must be between 1 and 16')
    try:
        prepare_output(output, source)
    except (OSError, ValueError) as error:
        parser.error(str(error))
    entries, failures, bank_records, seen_forks = [], [], [], set()
    for path, bank in bank_sources(source):
        try:
            fork, carrier = fork_bytes(path)
            key = bank.casefold(), digest(fork)
            if key in seen_forks:
                continue
            seen_forks.add(key)
            bank_records.append({'bank': bank, 'source': str(path), 'resource_fork_sha256': key[1], 'carrier': carrier})
            for resource in resources(fork):
                if resource['type'] != 'PICT' or not resource['name']:
                    continue
                entry = {'bank': bank, 'resource_id': resource['id'], 'picture': resource['name'],
                         'resource_sha256': digest(resource['payload']), 'source_fork_sha256': key[1],
                         'source_offset': resource['offset'], '_payload': resource['payload']}
                stack = PREFIXES.get(bank[:2].upper())
                if stack:
                    entry['stack'] = stack
                entries.append(entry)
        except (OSError, ValueError, struct.error) as error:
            failures.append({'bank': bank, 'error': str(error)})
    images = []
    with ThreadPoolExecutor(max_workers=args.workers) as pool:
        jobs = {pool.submit(decode, dict(entry), output, ffmpeg): entry for entry in entries}
        for job in as_completed(jobs):
            entry = jobs[job]
            try:
                images.append(job.result())
            except (OSError, ValueError, struct.error, subprocess.TimeoutExpired) as error:
                failures.append({k: v for k, v in entry.items() if k != '_payload'} | {'error': str(error)})
    images.sort(key=lambda x: (x.get('stack', ''), x['bank'], x['resource_id']))
    report = {'banks': bank_records, 'named_pictures': len(entries), 'decoded': len(images),
              'unsupported_or_failed': len(failures), 'failures': failures,
              'decoder': subprocess.run([ffmpeg, '-version'], capture_output=True, text=True).stdout.splitlines()[0],
              'scope': 'Named original PICT resources containing one SMC image; no gameplay state or QuickDraw compositing reconstructed.'}
    for name, value in (('images.json', {'images': images}), ('extraction-report.json', report)):
        atomic_json(output, name, value)
    print(json.dumps({k: v for k, v in report.items() if k not in ('banks', 'failures')}, indent=2))
    return 0 if images else 1


if __name__ == '__main__':
    raise SystemExit(main())
