"""A disassembler for the Python 2.3 code objects of Myst V's `python.pak` (as loaded by `pak.py`).

Opcode numbers and names are those of CPython 2.3's `Lib/opcode.py` (opcodes from 90 up take a two-byte argument).
It prints what a function does, instruction by instruction, so that a research note can state it; it does not
decompile and runs nothing.
"""
import struct

OPS = {0: 'STOP_CODE', 1: 'POP_TOP', 2: 'ROT_TWO', 3: 'ROT_THREE', 4: 'DUP_TOP', 5: 'ROT_FOUR', 10: 'UNARY_POSITIVE',
       11: 'UNARY_NEGATIVE', 12: 'UNARY_NOT', 13: 'UNARY_CONVERT', 15: 'UNARY_INVERT', 19: 'BINARY_POWER',
       20: 'BINARY_MULTIPLY', 21: 'BINARY_DIVIDE', 22: 'BINARY_MODULO', 23: 'BINARY_ADD', 24: 'BINARY_SUBTRACT',
       25: 'BINARY_SUBSCR', 26: 'BINARY_FLOOR_DIVIDE', 27: 'BINARY_TRUE_DIVIDE', 28: 'INPLACE_FLOOR_DIVIDE',
       29: 'INPLACE_TRUE_DIVIDE', 30: 'SLICE+0', 31: 'SLICE+1', 32: 'SLICE+2', 33: 'SLICE+3', 40: 'STORE_SLICE+0',
       41: 'STORE_SLICE+1', 42: 'STORE_SLICE+2', 43: 'STORE_SLICE+3', 50: 'DELETE_SLICE+0', 51: 'DELETE_SLICE+1',
       52: 'DELETE_SLICE+2', 53: 'DELETE_SLICE+3', 55: 'INPLACE_ADD', 56: 'INPLACE_SUBTRACT', 57: 'INPLACE_MULTIPLY',
       58: 'INPLACE_DIVIDE', 59: 'INPLACE_MODULO', 60: 'STORE_SUBSCR', 61: 'DELETE_SUBSCR', 62: 'BINARY_LSHIFT',
       63: 'BINARY_RSHIFT', 64: 'BINARY_AND', 65: 'BINARY_XOR', 66: 'BINARY_OR', 67: 'INPLACE_POWER', 68: 'GET_ITER',
       70: 'PRINT_EXPR', 71: 'PRINT_ITEM', 72: 'PRINT_NEWLINE', 73: 'PRINT_ITEM_TO', 74: 'PRINT_NEWLINE_TO',
       75: 'INPLACE_LSHIFT', 76: 'INPLACE_RSHIFT', 77: 'INPLACE_AND', 78: 'INPLACE_XOR', 79: 'INPLACE_OR',
       80: 'BREAK_LOOP', 82: 'LOAD_LOCALS', 83: 'RETURN_VALUE', 84: 'IMPORT_STAR', 85: 'EXEC_STMT', 86: 'YIELD_VALUE',
       87: 'POP_BLOCK', 88: 'END_FINALLY', 89: 'BUILD_CLASS', 90: 'STORE_NAME', 91: 'DELETE_NAME',
       92: 'UNPACK_SEQUENCE', 93: 'FOR_ITER', 95: 'STORE_ATTR', 96: 'DELETE_ATTR', 97: 'STORE_GLOBAL',
       98: 'DELETE_GLOBAL', 99: 'DUP_TOPX', 100: 'LOAD_CONST', 101: 'LOAD_NAME', 102: 'BUILD_TUPLE', 103: 'BUILD_LIST',
       104: 'BUILD_MAP', 105: 'LOAD_ATTR', 106: 'COMPARE_OP', 107: 'IMPORT_NAME', 108: 'IMPORT_FROM',
       110: 'JUMP_FORWARD', 111: 'JUMP_IF_FALSE', 112: 'JUMP_IF_TRUE', 113: 'JUMP_ABSOLUTE', 116: 'LOAD_GLOBAL',
       119: 'CONTINUE_LOOP', 120: 'SETUP_LOOP', 121: 'SETUP_EXCEPT', 122: 'SETUP_FINALLY', 124: 'LOAD_FAST',
       125: 'STORE_FAST', 126: 'DELETE_FAST', 130: 'RAISE_VARARGS', 131: 'CALL_FUNCTION', 132: 'MAKE_FUNCTION',
       133: 'BUILD_SLICE', 134: 'MAKE_CLOSURE', 135: 'LOAD_CLOSURE', 136: 'LOAD_DEREF', 137: 'STORE_DEREF',
       140: 'CALL_FUNCTION_VAR', 141: 'CALL_FUNCTION_KW', 142: 'CALL_FUNCTION_VAR_KW', 143: 'EXTENDED_ARG'}
CMP = ('<', '<=', '==', '!=', '>', '>=', 'in', 'not in', 'is', 'is not', 'exception match', 'BAD')
NAME_OPS = {90, 91, 95, 96, 97, 98, 101, 105, 107, 108, 116}
JREL = {93, 110, 111, 112, 120, 121, 122}


def _t(x):
    return x.decode('latin-1') if isinstance(x, bytes) else x


def disassemble(co):
    """[(offset, opname, argument, shown argument)] for one code object."""
    code = co['code']
    names, consts = co['names'], co['consts']
    varnames = co['varnames']
    cells = tuple(co['cellvars']) + tuple(co['freevars'])
    out = []
    i = 0
    while i < len(code):
        op = code[i]
        name = OPS.get(op, f'<{op}>')
        if op >= 90:
            arg = struct.unpack_from('<H', code, i + 1)[0]
            if op in NAME_OPS:
                shown = _t(names[arg])
            elif op == 100:
                c = consts[arg]
                shown = f'<code {_t(c["name"])}>' if isinstance(c, dict) else repr(_t(c))
            elif op in (124, 125, 126):
                shown = _t(varnames[arg])
            elif op in (135, 136, 137):
                shown = _t(cells[arg])
            elif op == 106:
                shown = CMP[arg] if arg < len(CMP) else arg
            elif op in JREL:
                shown = f'to {i + 3 + arg}'
            elif op in (113, 119):
                shown = f'to {arg}'
            else:
                shown = arg
            out.append((i, name, arg, shown))
            i += 3
        else:
            out.append((i, name, None, ''))
            i += 1
    return out


def listing(co):
    return '\n'.join(f'{o:5d} {n:<20s} {s}' for o, n, a, s in disassemble(co))
