"""Decode Riven's stored Mohawk bitmaps into local PNG view assets.

This is a bounded, independent implementation of the Riven byte-oriented
pixel codec. Format facts were verified against ScummVM's Mohawk bitmap
reader at commit aed0208e772b128d3f953207a297fc521c32833a. The extractor
does not simulate scripts, movie compositing, or game state.
"""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import re
import struct
import zlib


class BitmapError(ValueError):
    """Invalid or unsupported stored image."""


def unpack_riven(data: bytes, expected: int) -> bytes:
    """Decode pixel duplets, arithmetic deltas and overlapping history runs."""
    if not 0 < expected <= 64 * 1024 * 1024:
        raise BitmapError('Invalid unpacked size')
    output = bytearray()
    pos = 0

    def byte():
        nonlocal pos
        if pos >= len(data):
            raise BitmapError('Truncated Riven pixel stream')
        result = data[pos]
        pos += 1
        return result

    def literal(count=1):
        nonlocal pos
        if pos + count > len(data):
            raise BitmapError('Truncated literal pixels')
        if len(output) + count > expected:
            raise BitmapError('Pixel output exceeds image bounds')
        output.extend(data[pos:pos + count])
        pos += count

    def previous(distance=2, delta=0):
        if not 0 < distance <= len(output):
            raise BitmapError('Pixel reference precedes output')
        if len(output) >= expected:
            raise BitmapError('Pixel output exceeds image bounds')
        output.append((output[-distance] + delta) & 255)

    def history(distance, count):
        if not 0 < distance <= len(output):
            raise BitmapError('Pixel history reference precedes output')
        if len(output) + count > expected:
            raise BitmapError('Pixel output exceeds image bounds')
        for _ in range(count):
            output.append(output[-distance])

    runs = {0xa4: (3, True), 0xa8: (4, False), 0xac: (5, True),
            0xb4: (6, False), 0xb8: (7, True), 0xbc: (8, False),
            0xe4: (9, True), 0xe8: (10, False), 0xec: (11, True),
            0xf4: (12, False), 0xf8: (13, True)}
    while len(output) < expected:
        command = byte()
        if command == 0:
            break
        if command < 0x40:
            literal(command * 2)
        elif command < 0x80:
            count = command - 0x40
            if count:
                if len(output) < 2 or len(output) + count * 2 > expected:
                    raise BitmapError('Invalid repeated duplet')
                output.extend(output[-2:] * count)
        elif command < 0xc0:
            count = command - 0x80
            if count:
                if len(output) < 4 or len(output) + count * 4 > expected:
                    raise BitmapError('Invalid repeated four-pixel group')
                output.extend(output[-4:] * count)
        else:
            for _ in range(command - 0xc0):
                sub = byte()
                low = sub & 15
                if 1 <= sub <= 15:
                    previous(low * 2); previous(low * 2)
                elif sub == 0x10:
                    previous(); literal()
                elif 0x11 <= sub <= 0x1f:
                    previous(); previous(low)
                elif 0x20 <= sub <= 0x2f:
                    previous(); previous(delta=low)
                elif 0x30 <= sub <= 0x3f:
                    previous(); previous(delta=-low)
                elif sub == 0x40:
                    literal(); previous()
                elif 0x41 <= sub <= 0x4f:
                    previous(low); previous()
                elif sub == 0x50:
                    literal(2)
                elif 0x51 <= sub <= 0x57:
                    previous(sub & 7); literal()
                elif 0x59 <= sub <= 0x5f:
                    literal(); previous(sub & 7)
                elif 0x60 <= sub <= 0x6f:
                    literal(); previous(delta=low)
                elif 0x70 <= sub <= 0x7f:
                    literal(); previous(delta=-low)
                elif 0x80 <= sub <= 0x8f:
                    previous(delta=low); previous()
                elif 0x90 <= sub <= 0x9f:
                    previous(delta=low); literal()
                elif sub in (0xa0, 0xb0, 0xe0, 0xf0, 0xff):
                    pattern = byte()
                    first = pattern >> 4
                    second = pattern & 15
                    previous(delta=first if sub in (0xa0, 0xb0) else -first)
                    previous(delta=second if sub in (0xa0, 0xe0) else -second)
                elif 0xc0 <= sub <= 0xcf:
                    previous(delta=-low); previous()
                elif 0xd0 <= sub <= 0xdf:
                    previous(delta=-low); literal()
                elif sub & 0xfc in runs:
                    count, tail = runs[sub & 0xfc]
                    history(((sub & 3) << 8) | byte(), count)
                    if tail:
                        literal()
                elif sub == 0xfc:
                    control, low_distance = byte(), byte()
                    distance = ((control & 3) << 8) | low_distance
                    history(distance, ((control >> 3) + 1) * 2 + 1)
                    if control & 4:
                        history(distance, 1)
                    else:
                        literal()
                else:
                    raise BitmapError(f'Unsupported Riven pixel subcommand 0x{sub:02x}')
    if len(output) != expected:
        raise BitmapError(f'Truncated pixel output: {len(output)} of {expected}')
    return bytes(output)


def decode_bitmap(data: bytes) -> dict:
    """Return width, height, tight pixel rows, color type and RGB palette."""
    if len(data) < 8:
        raise BitmapError('Truncated bitmap header')
    width, height, stride, format_word = struct.unpack_from('>4H', data)
    width &= 0x3fff
    height &= 0x3fff
    stride &= 0x3ffe
    bits = {2: 8, 4: 24}.get(format_word & 7)
    if bits is None:
        raise BitmapError(f'Unsupported bitmap depth {format_word & 7}')
    if not 0 < width <= 8192 or not 0 < height <= 8192 or stride < width * (bits // 8) or stride * height > 64 * 1024 * 1024:
        raise BitmapError('Invalid bitmap dimensions or stride')
    pack, draw = format_word & 0x0f00, format_word & 0x00f0
    if draw != 0:
        raise BitmapError(f'Unsupported bitmap draw format 0x{draw:02x}')
    pos, palette = 8, None
    if format_word & 8 or pack == 0x400 and bits == 8:
        if len(data) < pos + 772:
            raise BitmapError('Truncated bitmap palette')
        pos += 4
        bgr = data[pos:pos + 768]
        palette = bytes(channel for i in range(0, 768, 3) for channel in (bgr[i + 2], bgr[i + 1], bgr[i]))
        pos += 768
    if bits == 8 and palette is None:
        raise BitmapError('Indexed bitmap has no local palette')
    if pack == 0x400:
        if len(data) < pos + 4:
            raise BitmapError('Truncated pixel packing header')
        pixels = unpack_riven(data[pos + 4:], stride * height)
    elif pack == 0:
        pixels = data[pos:pos + stride * height]
        if len(pixels) != stride * height:
            raise BitmapError('Truncated raw pixel data')
    else:
        raise BitmapError(f'Unsupported bitmap packing 0x{pack:03x}')
    row_bytes = width * (bits // 8)
    tight = b''.join(pixels[y * stride:y * stride + row_bytes] for y in range(height))
    if bits == 24:
        tight = bytes(channel for i in range(0, len(tight), 3) for channel in (tight[i + 2], tight[i + 1], tight[i]))
    return {'width': width, 'height': height, 'pixels': tight, 'palette': palette,
            'channels': bits // 8, 'format': format_word}


def png_bytes(bitmap: dict, max_width: int | None = None) -> bytes:
    """Write a lossless PNG, optionally making a small nearest-neighbor thumb."""
    width, height, channels = bitmap['width'], bitmap['height'], bitmap['channels']
    pixels = bitmap['pixels']
    if max_width and width > max_width:
        new_width = max_width
        new_height = max(1, round(height * new_width / width))
        pixels = b''.join(pixels[(min(height - 1, y * height // new_height) * width + min(width - 1, x * width // new_width)) * channels: (min(height - 1, y * height // new_height) * width + min(width - 1, x * width // new_width)) * channels + channels]
                          for y in range(new_height) for x in range(new_width))
        width, height = new_width, new_height
    rows = b''.join(b'\0' + pixels[y * width * channels:(y + 1) * width * channels] for y in range(height))
    def chunk(tag, body):
        return struct.pack('>I', len(body)) + tag + body + struct.pack('>I', zlib.crc32(tag + body) & 0xffffffff)
    result = b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', width, height, 8, 3 if channels == 1 else 2, 0, 0, 0))
    if channels == 1:
        result += chunk(b'PLTE', bitmap['palette'])
    return result + chunk(b'IDAT', zlib.compress(rows, 6)) + chunk(b'IEND', b'')


def extract_images(inputs, output='generated', *, progress=False):
    """Extract one stored base view per card and return (card mapping, report).

    Output image paths are relative to ``output``. Missing/unsupported resources
    are recorded explicitly. Existing PNG files are reused only when the
    source resource SHA matches the previous image manifest.
    """
    from .build import load
    from .scripts import parse_plst
    archives, selected, _ = load(inputs)
    resources = {}
    data_archives = []
    for archive in archives:
        match = re.fullmatch(r'([abgjoprt])_data(\d*)\.mhk', Path(archive.source.name).name.lower())
        if match:
            data_archives.append((int(match[2] or 0), archive.source.name, match[1] + 'spit', archive))
    for _, _, stack, archive in sorted(data_archives):
        for rid in archive.ids('tBMP'):
            resources[stack, rid] = archive
    output = Path(output)
    (output / 'images').mkdir(parents=True, exist_ok=True)
    manifest_path = output / 'images-manifest.json'
    old = json.loads(manifest_path.read_text()).get('bitmaps', {}) if manifest_path.exists() else {}
    decoded, mapping, failures = {}, {}, []
    for stack, tags in sorted(selected.items()):
        for card_id in sorted(tags['CARD']):
            source_id = f'{stack}:{card_id}'
            if card_id not in tags['PLST']:
                failures.append({'card': source_id, 'error': 'No PLST resource'})
                continue
            pictures = parse_plst(tags['PLST'][card_id].read('PLST', card_id))
            # Index 1 is the engine's default record. Prefer the largest stored
            # rectangle if a title/overlay card has no index-1 picture.
            pictures.sort(key=lambda p: (p['index'] != 1, -(p['rect'][2] - p['rect'][0]) * (p['rect'][3] - p['rect'][1])))
            if not pictures:
                failures.append({'card': source_id, 'error': 'Empty PLST resource'})
                continue
            picture = pictures[0]
            rid = picture['image_id']
            bitmap_key = f'{stack}:{rid}'
            if bitmap_key not in decoded:
                try:
                    archive = resources[stack, rid]
                    raw = archive.read('tBMP', rid)
                    digest = hashlib.sha256(raw).hexdigest()
                    image = f'images/{stack}-{rid}.png'
                    thumbnail = f'images/{stack}-{rid}-thumb.png'
                    previous = old.get(bitmap_key)
                    if previous and previous.get('sha256') == digest and (output / image).exists() and (output / thumbnail).exists():
                        record = previous
                    else:
                        bitmap = decode_bitmap(raw)
                        (output / image).write_bytes(png_bytes(bitmap))
                        (output / thumbnail).write_bytes(png_bytes(bitmap, 240))
                        record = {'image': image, 'thumbnail': thumbnail,
                                  'imageWidth': bitmap['width'], 'imageHeight': bitmap['height'],
                                  'assetName': archive.resources['tBMP'][rid].name or f'tBMP:{rid}',
                                  'bitmapID': rid, 'sha256': digest,
                                  'archive': archive.source.origin, 'format': bitmap['format']}
                    # A content cache may be reused for a different input
                    # path. Always attribute the current selected archive.
                    resource = archive.resources['tBMP'][rid]
                    record = {**record, 'archive': archive.source.origin,
                              'resourceOffset': resource.offset,
                              'absoluteOffset': archive.source.offset + resource.offset}
                    decoded[bitmap_key] = record
                except (BitmapError, KeyError, ValueError) as error:
                    decoded[bitmap_key] = {'error': str(error)}
            record = decoded[bitmap_key]
            if 'error' in record:
                failures.append({'card': source_id, 'bitmap': bitmap_key, 'error': record['error']})
            else:
                mapping[source_id] = {**record, 'pictureIndex': picture['index'], 'pictureRect': picture['rect'],
                                      'viewWidth': 608, 'viewHeight': 392}
            if progress and (len(mapping) + len(failures)) % 100 == 0:
                print(f'{len(mapping)} card views decoded; {len(failures)} unavailable', flush=True)
    report = {'cards_with_images': len(mapping), 'unique_bitmaps': sum('error' not in r for r in decoded.values()),
              'failures': failures, 'policy': 'One stored default PLST view per card; no runtime state or movie compositing.'}
    temporary = manifest_path.with_suffix('.json.tmp')
    temporary.write_text(json.dumps({'cards': mapping, 'bitmaps': decoded, 'report': report}, indent=2) + '\n')
    temporary.replace(manifest_path)
    return mapping, report


def main():
    parser = argparse.ArgumentParser(description='Extract local PNG card views from Riven Mohawk archives.')
    parser.add_argument('inputs', nargs='+')
    parser.add_argument('--output', default='generated/riven')
    args = parser.parse_args()
    _, report = extract_images(args.inputs, args.output, progress=True)
    print(json.dumps(report, indent=2))


if __name__ == '__main__':
    main()
