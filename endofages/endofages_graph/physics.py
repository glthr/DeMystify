"""Myst V collision objects: plGenericPhysical in its ODE form (Myst V and Crowthistle).

Format as read in libhsplasma (H-uru/libhsplasma, GPL): `core/PRP/Physics/plGenericPhysical.cpp`
(`IReadODEPhysical`) and the enumerations of `core/PRP/Physics/plODEPhysical.h`. After the synched-object header: the
bounds type (i32: 1 box, 2 sphere, 3 hull, 4 proxy, 5 explicit, 6 cylinder); for explicit bounds, the vertices (i32
count, three floats each), the triangles (i32 count, three i32 indices each) and an opaque buffer (i32 size, bytes);
for a sphere its radius, for a box its three half-dimensions, for a cylinder its length and radius; then the mass
(f32), the member, collide and report groups (i32 each; members: 0 static, 1 avatar blocker, 2 dynamic blocker,
3 avatar, 4 dynamic, 5 detector, 6 line of sight only), a property bit vector, the line-of-sight databases (u16), the
key of the scene object and of the scene node.
"""
import math

from . import prp

BOUNDS = {1: 'box', 2: 'sphere', 3: 'hull', 4: 'proxy', 5: 'explicit', 6: 'cylinder'}
GROUPS = {0: 'static', 1: 'avatar blocker', 2: 'dynamic blocker', 3: 'avatar', 4: 'dynamic', 5: 'detector',
          6: 'line of sight'}


def read_physical(page, key):
    r, end = page.object_reader(key)
    prp._synched(r)
    ph = {'bounds': BOUNDS.get(r.i32(), 'unknown')}
    if ph['bounds'] == 'explicit':
        ph['verts'] = [tuple(r.floats(3)) for _ in range(r.i32())]
        ph['tris'] = [tuple(r.i32() for _ in range(3)) for _ in range(r.i32())]
        r.read(r.i32())                     # the opaque buffer (ODE's own data)
    elif ph['bounds'] == 'sphere':
        ph['radius'] = r.f32()
    elif ph['bounds'] == 'box':
        ph['dims'] = r.floats(3)
    elif ph['bounds'] == 'cylinder':
        ph['length'] = r.f32()
        ph['radius'] = r.f32()
    ph['mass'] = r.f32()
    ph['member'] = r.i32()
    ph['collide'] = r.i32()
    ph['report'] = r.i32()
    for _ in range(r.u32()):
        r.u32()
    ph['los'] = r.u16()
    obj = prp.read_key(r)
    prp.read_key(r)
    ph['object'] = obj.name if obj else None
    if r.tell() != end:
        raise ValueError(f'{page.path}: physical {key[0].name!r} read {r.tell() - key[1]} of {key[2]} bytes')
    return ph


def floor_triangles(verts, tris, min_up=0.7):
    """(area, centroid, normal z) of the triangles facing up enough to stand on (normal z >= min_up)."""
    out = []
    for a, b, c in tris:
        pa, pb, pc = verts[a], verts[b], verts[c]
        u = [pb[i] - pa[i] for i in range(3)]
        v = [pc[i] - pa[i] for i in range(3)]
        n = (u[1] * v[2] - u[2] * v[1], u[2] * v[0] - u[0] * v[2], u[0] * v[1] - u[1] * v[0])
        ln = math.sqrt(n[0] ** 2 + n[1] ** 2 + n[2] ** 2)
        if ln == 0:
            continue
        nz = n[2] / ln
        if abs(nz) >= min_up:
            out.append((ln / 2, tuple((pa[i] + pb[i] + pc[i]) / 3 for i in range(3)), nz))
    return out
