"""Myst V `.prp` pages: the header, the key index, and the few object classes the graph needs.

Formats as read in libhsplasma (H-uru/libhsplasma, GPL): `core/PRP/plPageInfo.cpp` (header, version 6 with a class
table = Myst V), `core/ResManager/plResManager.cpp` (`ReadKeyring`, `readKey`, `readUoid`), `core/PRP/KeyedObject/
plUoid.cpp`, `plLocation.cpp`, `plLoadMask.cpp`, `plKey.cpp`, `core/ResManager/pdUnifiedTypeMap.cpp` (the Myst V class
numbers, `fEoaP2MTable`, and the class names, copied into `eoa_types.json`), `core/PRP/Object/plSynchedObject.cpp`,
`plSceneObject.cpp`, `plObjInterface.cpp`, `plCoordinateInterface.cpp`, `core/Math/hsMatrix44.cpp`. This is an own,
partial reader: it reads keys, scene objects and coordinate interfaces, nothing else.
"""
import json
import os
import struct

from .stream import Reader

_TYPES = json.load(open(os.path.join(os.path.dirname(__file__), 'eoa_types.json')))
EOA_P2M = _TYPES['eoaP2M']
NAMES = _TYPES['names']


def class_name(eoa_type):
    if 0 <= eoa_type < len(EOA_P2M):
        m = EOA_P2M[eoa_type]
        if 0 <= m < len(NAMES) and NAMES[m]:
            return NAMES[m]
    return f'unknown_{eoa_type:#06x}'


class Uoid:
    __slots__ = ('location', 'loc_flags', 'cls', 'oid', 'name', 'load_mask')

    def __init__(self, location, loc_flags, cls, oid, name, load_mask):
        self.location, self.loc_flags, self.cls, self.oid, self.name, self.load_mask = \
            location, loc_flags, cls, oid, name, load_mask

    @property
    def cls_name(self):
        return class_name(self.cls)

    def valid(self):
        return self.location not in (0xFFFFFFFF,)

    def ref(self):
        return (self.location, self.cls, self.name)

    def __repr__(self):
        return f'<{self.cls_name} {self.name!r} @{self.location:#x}>'


def read_uoid(r):
    contents = r.u8()
    location = r.u32()
    loc_flags = r.u8()
    cls = r.u16()
    oid = r.u32()
    name = r.safe_str()
    load_mask = r.u8() if contents & 0x6 else None
    return Uoid(location, loc_flags, cls, oid, name, load_mask)


def read_key(r):
    """A key inside an object: in Myst V, a whole uoid (no "exists" flag), invalid when its location is."""
    u = read_uoid(r)
    return u if u.valid() else None


class Page:
    def __init__(self, path):
        self.path = path
        with open(path, 'rb') as f:
            self.data = f.read()
        r = Reader(self.data)
        self.prp_ver = r.i16()
        if self.prp_ver != 6:
            raise ValueError(f'{path}: page version {self.prp_ver}, not 6 (Myst V)')
        ntypes = r.u16()
        self.class_versions = {}
        for _ in range(ntypes):
            t = r.u16()
            self.class_versions[t] = r.u16()
        self.location = r.u32()
        self.loc_flags = r.u8()
        self.age = r.safe_str()
        self.page = r.safe_str()
        self.checksum = r.u32()
        self.data_start = r.u32()
        self.idx_start = r.u32()
        self.keys = []          # (Uoid, file_off, size)
        r.seek(self.idx_start)
        ntypes = r.u32()
        for _ in range(ntypes):
            t = r.u16()
            r.u32()             # bytes to the next list
            r.u8()              # flags
            n = r.u32()
            for _ in range(n):
                u = read_uoid(r)
                off = r.u32()
                size = r.u32()
                if u.cls != t:
                    raise ValueError(f'{path}: key of class {u.cls:#x} in the list of {t:#x}')
                self.keys.append((u, off, size))
        self.by_ref = {u.ref(): (u, off, size) for u, off, size in self.keys}

    def of_class(self, name):
        return [k for k in self.keys if k[0].cls_name == name]

    def object_reader(self, key):
        u, off, size = key
        r = Reader(self.data, off)
        cls = r.u16()
        if cls != u.cls:
            raise ValueError(f'{self.path}: object at {off} has class {cls:#x}, its key says {u.cls:#x}')
        return r, off + size


def _synched(r):
    read_uoid(r)                # the object's own key (hsKeyedObject)
    flags = r.u32()
    if (flags & 0x6) == 0:      # Myst V: an SDL exclude list unless bits 1 or 2 are set
        for _ in range(r.u16()):
            r.read(r.u16())
    return flags


def read_scene_object(page, key):
    r, end = page.object_reader(key)
    _synched(r)
    so = {'draw': read_key(r), 'sim': read_key(r), 'coord': read_key(r), 'audio': read_key(r)}
    so['interfaces'] = [read_key(r) for _ in range(r.u32())]
    so['modifiers'] = [read_key(r) for _ in range(r.u32())]
    so['scene_node'] = read_key(r)
    if r.tell() != end:
        raise ValueError(f'{page.path}: scene object {key[0].name!r} read {r.tell() - key[1]} of {key[2]} bytes')
    return so


def read_coordinate_interface(page, key):
    r, end = page.object_reader(key)
    _synched(r)
    owner = read_key(r)
    for _ in range(r.u32()):    # property bit vector
        r.u32()
    l2p = r.floats(16)
    p2l = r.floats(16)
    l2w = r.floats(16)
    w2l = r.floats(16)
    children = [read_key(r) for _ in range(r.u32())]
    if r.tell() != end:
        raise ValueError(f'{page.path}: coordinate interface {key[0].name!r} read {r.tell() - key[1]} of {key[2]}')
    return {'owner': owner, 'local_to_world': l2w, 'local_to_parent': l2p, 'children': children}


def translation(m):
    """The position of a row-major 4x4 matrix (hsMatrix44: the translation is the fourth column)."""
    return (m[3], m[7], m[11])


def heading(m):
    """The direction of the local Y axis in the world plane, in degrees (Plasma: Z up, an object faces +Y)."""
    import math
    return math.degrees(math.atan2(m[4], m[5])) % 360 if (m[4] or m[5]) else None


# plPythonFileMod (libhsplasma core/PRP/Modifier/plPythonFileMod.cpp): a multi-modifier (synched object + flag bit
# vector), the script's name, receiver keys, then parameters (id, type, value). Myst V parameter types: 1 int,
# 2 float, 3 boolean, 4/13/20/23 strings (string, animation name, global SDL variable, subtitle), 26 none, others keys.
STRING_TYPES = {4, 13, 20, 23}


def read_python_file_mod(page, key):
    r, end = page.object_reader(key)
    _synched(r)
    for _ in range(r.u32()):
        r.u32()
    script = r.safe_str()
    receivers = [read_key(r) for _ in range(r.u32())]
    params = []
    for _ in range(r.u32()):
        pid = r.u32()
        t = r.u32()
        if t == 1:
            v = r.i32()
        elif t == 2:
            v = r.f32()
        elif t == 3:
            v = r.u32() != 0
        elif t in STRING_TYPES:
            n = r.u32()
            v = r.read(n - 1).decode('latin-1') if n else ''
            if n:
                r.u8()
        elif t == 26:
            v = None
        else:
            v = read_key(r)
        params.append((pid, t, v))
    if r.tell() != end:
        raise ValueError(f'{page.path}: python file mod {key[0].name!r} read {r.tell() - key[1]} of {key[2]} bytes')
    return {'script': script, 'receivers': receivers, 'params': params}


# plLinkToAgeMsg in Myst V (libhsplasma core/PRP/Message/plLinkToAgeMsg.cpp, plMessage.cpp, core/PRP/Misc/
# plAgeLinkInfo.cpp, plSpawnPointInfo.cpp): the message header (sender key, receivers, broadcast flags), the Age link
# as a field bit vector with plain length-prefixed strings (Age file name, instance name) and a spawn point record
# (bit vector, title, spawn point, camera stack), the link effects (bit vector, link-in animation name, four ints),
# then one byte. Found inside responders by its class number; kept only when it parses whole.

def _bits(r):
    words = [r.u32() for _ in range(r.u32())]
    return lambda i: bool(words[i // 32] >> (i % 32) & 1) if i // 32 < len(words) else False


def _plain(r):
    return r.read(r.u16()).decode('latin-1')


def parse_link_to_age(data, pos):
    from .stream import Reader
    r = Reader(data, pos)
    sender = read_key(r)
    receivers = [read_key(r) for _ in range(r.u32())]
    if len(receivers) > 64:
        raise ValueError('too many receivers')
    bcast = r.u32()
    f = _bits(r)
    link = {}
    if f(0):
        link['age'] = _plain(r)
    if f(1):
        link['instance'] = _plain(r)
    if f(2):
        g = _bits(r)
        if g(0):
            link['title'] = _plain(r)
        if g(1):
            link['spawn'] = _plain(r)
        if g(2):
            link['camera_stack'] = _plain(r)
    e = _bits(r)
    if e(0):
        link['link_in_anim'] = _plain(r)
    for i in range(1, 5):
        if e(i):
            r.u32()
    r.u8()
    link['sender'] = sender.name if sender else None
    link['receivers'] = [x.name if x else None for x in receivers]
    link['bcast'] = bcast
    return link, r.tell()


def link_messages(page, key, ages):
    """The plLinkToAgeMsg commands inside a responder: [(offset, link)]."""
    u, off, size = key
    data = page.data
    cls = EOA_CLASS['plLinkToAgeMsg']
    pat = struct.pack('<H', cls)
    out = []
    p = data.find(pat, off, off + size)
    while p != -1:
        try:
            link, end = parse_link_to_age(data, p + 2)
            if end <= off + size and link.get('age') in ages:
                out.append((p, link))
        except Exception:
            pass
        p = data.find(pat, p + 1, off + size)
    return out


EOA_CLASS = {}
for _e, _m in enumerate(EOA_P2M):
    if 0 <= _m < len(NAMES) and NAMES[_m] and NAMES[_m] not in EOA_CLASS:
        EOA_CLASS[NAMES[_m]] = _e


def read_ladder_modifier(page, key):
    """plLadderModifier (libhsplasma core/PRP/Avatar/plLadderModifier.cpp): a single modifier, then seven keys: the
    top, bottom and main logic, the exits at the top and the bottom, and the top and bottom positions."""
    r, end = page.object_reader(key)
    _synched(r)
    for _ in range(r.u32()):
        r.u32()
    keys = {k: read_key(r) for k in ('top_logic', 'bottom_logic', 'main_logic', 'exit_top', 'exit_bottom',
                                     'top_pos', 'bottom_pos')}
    if r.tell() != end:
        raise ValueError(f'{page.path}: ladder {key[0].name!r} read {r.tell() - key[1]} of {key[2]} bytes')
    return {k: (v.name if v else None) for k, v in keys.items()}
