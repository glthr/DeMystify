"""Follow the scripts of a node and record where they can lead.

Nothing is executed. The walker reads each script in order, follows the calls
to helper scripts, and keeps the three "next location" variables that the game
sets before some moves. Conditions are kept as text beside each reference;
they are never evaluated.
"""
from __future__ import annotations

from dataclasses import dataclass, field

from . import scripts as fmt
from .names import OPCODES

NEXT_NODE, NEXT_ROOM, NEXT_AGE = 189, 190, 191
MOVIE_SCRIPT = 183
SHARED_ROOM = 'XXXX'
ZIP_CURSOR = 7
MAX_DEPTH = 12

OP = {name: number for number, (name, _) in OPCODES.items()}
VIEW_OPS = {4: 'cube', 6: 'cube', 7: 'frame', 8: 'frame', 9: 'frame', 10: 'menu'}
GO_OPS = {136, 137, 138, 140, 142}          # node
GO_ROOM_OPS = {139, 141, 143}               # room, node
ZIP_OPS = {140, 141, 142, 143}
MOVIE_GO_OPS = {151, 152}                   # node, movie
# opcode -> index of the argument naming a script node of the current room
CALLS = {187: 0, 188: 0, 174: 1, 175: 1, 176: 3, 177: 3, 178: 3, 179: 3, 180: 3, 181: 3, 182: 3, 183: 3,
         127: 6, 132: 1, 130: 4, 134: 7}
COMMON_CALLS = {189: 0, 190: 0}
LOOP_CALLS = {174, 175, 176, 177, 178, 179, 180, 181, 182, 183, 127, 132, 130, 134}


@dataclass
class Pending:
    """What a script has stored in one "next location" variable."""
    values: frozenset = frozenset()
    dynamic: str = ''

    def merge(self, other):
        return Pending(self.values | other.values, self.dynamic or other.dynamic)

    @property
    def is_set(self):
        return bool(self.values or self.dynamic)


def _fresh():
    return {NEXT_NODE: Pending(), NEXT_ROOM: Pending(), NEXT_AGE: Pending()}


@dataclass
class Walk:
    """The result of walking the scripts of one owner."""
    owner: tuple
    references: list = field(default_factory=list)
    reloads: list = field(default_factory=list)
    views: list = field(default_factory=list)
    calls: list = field(default_factory=list)
    missing_calls: list = field(default_factory=list)
    dynamic_calls: list = field(default_factory=list)
    dead_code: list = field(default_factory=list)
    puzzles: list = field(default_factory=list)
    truncated: list = field(default_factory=list)


class Walker:
    def __init__(self, db):
        self.db = db
        self.room_names = {room.id: room.name for room in db.rooms.values()}

    def walk_node(self, node):
        """Walk every script that runs while the player is at ``node``."""
        result = Walk((node.room, node.id))
        state = _fresh()
        for index, script in enumerate(node.scripts):
            origin = {'kind': 'init', 'index': index, 'condition': fmt.condition(script.condition),
                      'conditional': script.condition != 1}
            self._script(script.ops, node.room, result, origin, [], [], state, False)
        for index, hotspot in enumerate(node.hotspots):
            origin = {'kind': 'background' if hotspot.background else 'hotspot', 'index': index,
                      'condition': fmt.condition(hotspot.condition),
                      'conditional': hotspot.condition not in (1, -1), 'cursor': hotspot.cursor}
            self._script(hotspot.ops, node.room, result, origin, [], [], _fresh(), False)
        return result

    # -- script traversal -------------------------------------------------

    def _script(self, ops, room, result, origin, stack, guards, state, dead):
        """Walk one script. ``dead`` marks code that can never be reached."""
        segment_guards, snapshots, stopped, segment = [], [], False, 0
        for op in ops:
            name = fmt.op_name(op.op)
            unreachable = dead or stopped
            current = guards + ([f'else branch {segment}'] if segment else []) + segment_guards
            if name == 'ifElse':
                # Reaching an ifElse ends the script; only a failed test of
                # the segment before it jumps past it.
                if not snapshots and not dead:
                    dead = True
                    result.dead_code.append({'offset': op.offset, 'origin': origin, 'via': _chain(stack),
                                             'reason': 'code after an "else" that no test jumps to'})
                elif snapshots:
                    merged = _fresh()
                    for snapshot in snapshots:
                        for key in merged:
                            merged[key] = merged[key].merge(snapshot[key])
                    state.update(merged)
                segment_guards, snapshots, stopped = [], [], False
                segment += 1
                continue
            if name.startswith('if'):
                snapshots.append(dict(state))
                segment_guards.append(fmt.opcode(op))
                continue
            if name == 'stopWholeScript':
                stopped = True
                continue
            if op.op in VIEW_OPS:
                if not unreachable:
                    result.views.append(self._view(op, current, origin, stack))
                continue
            if name == 'runPuzzle1' or name.startswith('runPuzzle'):
                result.puzzles.append({'puzzle': op.args[0] if op.args else None, 'offset': op.offset,
                                       'origin': origin, 'via': _chain(stack)})
                continue
            if self._assignment(op, name, room, result, origin, stack, current, state, unreachable):
                continue
            if op.op in GO_OPS or op.op in GO_ROOM_OPS or op.op in MOVIE_GO_OPS:
                self._go(op, name, room, result, origin, stack, current, state, unreachable)
                continue
            if name in ('changeNode', 'changeNodeRoom', 'changeNodeRoomAge'):
                self._change(op, name, room, result, origin, stack, current, unreachable)
                continue
            if op.op in CALLS or op.op in COMMON_CALLS:
                self._call(op, name, room, result, origin, stack, current, state, unreachable)

    def _assignment(self, op, name, room, result, origin, stack, guards, state, unreachable):
        args = op.args
        if name == 'varSetValue' and len(args) >= 2:
            if args[0] in state:
                state[args[0]] = Pending(frozenset([args[1]])) if args[1] else Pending()
                return True
            if args[0] == MOVIE_SCRIPT and args[1] > 0:
                # A script attached to a movie: it runs when the movie
                # reaches a given frame.
                self._enter(args[1], room, result, origin, stack, guards + ['at a movie frame'], state,
                            unreachable, op, 'movie script')
                return True
            return False
        if name in ('varSetZero', 'varSetOne', 'varSetTwo', 'varSetOneHundred') and args and args[0] in state:
            value = {'varSetZero': 0, 'varSetOne': 1, 'varSetTwo': 2, 'varSetOneHundred': 100}[name]
            state[args[0]] = Pending(frozenset([value])) if value else Pending()
            return True
        if name == 'varCopy' and len(args) >= 2 and args[1] in state:
            state[args[1]] = Pending(dynamic=fmt.var_name(args[0]))
            return True
        if name == 'varZeroRange' and len(args) >= 2:
            for key in state:
                if args[0] <= key <= args[1]:
                    state[key] = Pending()
            return False
        if name == 'chooseNextNode' and len(args) >= 3:
            state[NEXT_NODE] = Pending(frozenset(v for v in args[1:3] if v))
            return True
        signature = OPCODES.get(op.op, ('', ''))[1]
        for index, value in enumerate(args):
            if index < len(signature) and signature[index] == 'v' and value in state and name.startswith('var'):
                state[value] = Pending(dynamic=f'computed by {name}')
        return False

    # -- navigation -------------------------------------------------------

    def _go(self, op, name, room, result, origin, stack, guards, state, unreachable):
        args = op.args
        movie = None
        if op.op in GO_ROOM_OPS:
            rooms, nodes = Pending(frozenset([args[0]])), Pending(frozenset([args[1]]) if args[1] else frozenset())
        else:
            rooms = state[NEXT_ROOM]
            nodes = state[NEXT_NODE]
            if not nodes.is_set:
                # A negative node number names the variable holding it.
                value = args[0] if args else 0
                if value < 0:
                    nodes = Pending(dynamic=fmt.var_name(-value))
                elif value:
                    nodes = Pending(frozenset([value]))
            if op.op in MOVIE_GO_OPS and len(args) > 1:
                movie = args[1] if args[1] >= 0 else fmt.var_name(-args[1])
        age = state[NEXT_AGE]
        self._record(op, name, room, result, origin, stack, guards, rooms, nodes, age, movie,
                     op.op in ZIP_OPS, unreachable)
        for key in state:
            state[key] = Pending()

    def _change(self, op, name, room, result, origin, stack, guards, unreachable):
        args = list(op.args)
        node, room_arg, age = args[-1], (args[-2] if len(args) >= 2 else 0), (args[-3] if len(args) >= 3 else 0)

        def pending(value):
            if value < 0:
                return Pending(dynamic=fmt.var_name(-value))
            return Pending(frozenset([value])) if value else Pending()
        self._record(op, name, room, result, origin, stack, guards, pending(room_arg), pending(node),
                     pending(age), None, False, unreachable)

    def _record(self, op, name, room, result, origin, stack, guards, rooms, nodes, age, movie, zip_op,
                unreachable):
        base = {'op': name, 'text': fmt.opcode(op), 'offset': op.offset, 'origin': origin, 'via': _chain(stack),
                'guards': list(guards), 'unreachable': unreachable, 'zip': zip_op or origin.get('cursor') == ZIP_CURSOR,
                'order': len(result.references) + len(result.reloads)}
        if movie is not None:
            base['movie'] = movie
        if age.values:
            base['age'] = sorted(age.values)
        if nodes.dynamic or rooms.dynamic:
            result.references.append({**base, 'target': None,
                                      'dynamic': {'node': nodes.dynamic or sorted(nodes.values),
                                                  'room': rooms.dynamic or sorted(rooms.values),
                                                  'age': age.dynamic or sorted(age.values)}})
            return
        room_names = [self.room_names.get(r, f'room{r}') for r in sorted(rooms.values)] or [room]
        if not nodes.values:
            if room_names == [room]:
                result.reloads.append(base)
                return
            # A room change that keeps the current node number.
            node_values = [result.owner[1]]
        else:
            node_values = sorted(nodes.values)
        for target_room in room_names:
            for node in node_values:
                result.references.append({**base, 'target': (target_room, node)})

    # -- calls ------------------------------------------------------------

    def _call(self, op, name, room, result, origin, stack, guards, state, unreachable):
        common = op.op in COMMON_CALLS
        index = COMMON_CALLS[op.op] if common else CALLS[op.op]
        if index >= len(op.args):
            return
        value = op.args[index]
        if op.op in (127, 132):
            value = abs(value)
        if value == 0:
            return
        if value < 0:
            result.dynamic_calls.append({'text': fmt.opcode(op), 'offset': op.offset, 'origin': origin,
                                         'via': _chain(stack), 'variable': fmt.var_name(-value)})
            return
        extra = [f'repeated by {name}'] if op.op in LOOP_CALLS else []
        self._enter(value, SHARED_ROOM if common else room, result, origin, stack, guards + extra, state,
                    unreachable, op, 'common script' if common else 'script', location_room=room)

    def _enter(self, node_id, script_room, result, origin, stack, guards, state, unreachable, op, how,
               location_room=None):
        location_room = location_room or script_room
        key = (script_room, node_id)
        result.calls.append({'target': key, 'how': how, 'offset': op.offset, 'origin': origin, 'via': _chain(stack)})
        room = self.db.rooms.get(script_room)
        callee = room.nodes.get(node_id) if room else None
        if callee is None:
            result.missing_calls.append({'target': key, 'how': how, 'text': fmt.opcode(op), 'offset': op.offset,
                                         'origin': origin, 'via': _chain(stack)})
            return
        if key in stack or len(stack) >= MAX_DEPTH:
            if len(stack) >= MAX_DEPTH:
                result.truncated.append({'target': key, 'offset': op.offset})
            return
        stack.append(key)
        for index, script in enumerate(callee.scripts):
            inner = guards if script.condition == 1 else guards + [
                f'{script_room} {node_id} script {index} when {fmt.condition(script.condition)}']
            # A helper called from a room still acts in that room: its own
            # calls and moves are resolved from where the player stands.
            self._script(script.ops, location_room, result, origin, stack, inner, state, unreachable)
        stack.pop()

    def _view(self, op, guards, origin, stack):
        name = fmt.op_name(op.op)
        args = op.args
        if name in ('nodeCubeInitIndex', 'nodeFrameInitIndex'):
            candidates, selector = args[1:], fmt.var_name(args[0])
        elif name == 'nodeFrameInitCond':
            candidates, selector = args[1:3], fmt.condition(args[0])
        else:
            candidates, selector = args[:1], ''
        return {'type': VIEW_OPS[op.op], 'op': name, 'offset': op.offset, 'selector': selector,
                'indexes': [v for v in candidates if v > 0],
                'dynamic': [fmt.var_name(-v) for v in candidates if v < 0],
                'guards': list(guards), 'origin': origin, 'via': _chain(stack)}


def _chain(stack):
    return [f'{room} {node}' for room, node in stack]
