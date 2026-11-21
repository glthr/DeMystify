"""Render Myst III scripts as readable text."""
from __future__ import annotations

from .names import OPCODES, VARIABLES

CURSORS = {7: 'zip'}


def var_name(number):
    name = VARIABLES.get(number)
    return f'v{name}' if name else f'v{number}'


def condition(value):
    """Describe a 16-bit condition: a variable, an optional value, a sign.

    The low eleven bits name the variable. The next bits hold a value plus
    one, or zero for a plain truth test. A negative condition is negated.
    """
    if value == 1:
        return 'always'
    if value == -1:
        return 'every frame'
    magnitude = abs(value)
    variable, target = magnitude & 2047, (magnitude >> 11) - 1
    if target >= 0:
        return f'{var_name(variable)} {"==" if value >= 0 else "!="} {target}'
    return f'{var_name(variable)} {"!=" if value >= 0 else "=="} 0'


def condition_variable(value):
    return abs(value) & 2047


def argument(kind, value):
    if kind == 'v':
        return var_name(value)
    if kind == 'e':
        return str(value) if value >= 0 else var_name(-value)
    if kind == 'c':
        return '[' + condition(value) + ']'
    return str(value)


def op_name(op):
    entry = OPCODES.get(op)
    return entry[0] if entry else f'op{op}'


def opcode(op):
    name, signature = OPCODES.get(op.op, (f'op{op.op}', ''))
    parts = [argument(signature[i] if i < len(signature) else 'i', value) for i, value in enumerate(op.args)]
    return f'{name}({", ".join(parts)})'


def lines(ops, indent='    '):
    """Indent a script by its ``if`` guards.

    An ``if`` that fails jumps past the next ``ifElse``; reaching an
    ``ifElse`` ends the script. A script is therefore a chain of guarded
    segments, each one the alternative of the segment before it.
    """
    out, depth = [], 0
    for op in ops:
        name = op_name(op.op)
        if name == 'ifElse':
            out.append(f'{indent}else')
            depth = 0
            continue
        out.append(f'{indent}{"  " * depth}{opcode(op)}')
        if name.startswith('if'):
            depth += 1
    return out


def rect(values, frame):
    a, b, width, height = values
    if frame:
        if b < 0:
            return f'frame x={var_name(a)} y={var_name(-b)} {width}x{-height}'
        return f'frame x={a} y={b} {width}x{height}'
    return f'pitch {a} heading {b} ±{width}x{height}'


def node_text(node, view_type):
    """The full listing of one node."""
    out = [f'{node.room} {node.id}']
    frame = view_type != 'cube'
    for i, script in enumerate(node.scripts):
        note = '  ; repaired: the stored script lacks its first two bytes' if script.repaired else ''
        out.append(f'  init {i} when {condition(script.condition)}  @{script.offset:#x}{note}')
        out.extend(lines(script.ops))
    for i, hotspot in enumerate(node.hotspots):
        if hotspot.background:
            out.append(f'  background {i} (every frame)  @{hotspot.offset:#x}')
        else:
            cursor = CURSORS.get(hotspot.cursor, str(hotspot.cursor))
            out.append(f'  hotspot {i} when {condition(hotspot.condition)}, cursor {cursor}  @{hotspot.offset:#x}')
            for values in hotspot.rects:
                out.append(f'    zone {rect(values, frame)}')
        out.extend(lines(hotspot.ops))
    for label, group in (('ambient', node.ambient_scripts), ('background sound', node.background_sound_scripts)):
        for i, script in enumerate(group):
            out.append(f'  {label} {i} when {condition(script.condition)}  @{script.offset:#x}')
            out.extend(lines(script.ops))
    return '\n'.join(out) + '\n'
