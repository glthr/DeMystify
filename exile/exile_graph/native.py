"""Navigation performed by the engine, outside the scripts stored in the game.

These transitions come from reading the ScummVM implementation of the Myst III
engine at a pinned commit. They are kept apart from the script references and
carry a link to the code they come from. ScummVM is a reimplementation: it
documents the behaviour of the original interpreter without being its source.
"""
from __future__ import annotations

from .walker import Walk, _fresh

COMMIT = 'aed0208e772b128d3f953207a297fc521c32833a'
BASE = f'https://github.com/scummvm/scummvm/blob/{COMMIT}/engines/myst3/'

BOOT = ('XXXX', 1)        # loaded when the program starts
START = ('TOHO', 101)     # reached by the new-game script MENU 98
MENU_MAIN, MENU_NEW, MENU_LOAD, MENU_SAVE, MENU_SETTINGS = 100, 98, 200, 300, 599

MENU_ACTIONS = [
    (MENU_NEW, 'main menu action 1, New game, once confirmed', 'menu.cpp#L241-L260'),
    (MENU_LOAD, 'main menu action 2, Load game, once confirmed', 'menu.cpp#L261-L282'),
    (MENU_SAVE, 'main menu actions 1, 2, 3 and 5, when the player chooses to save first', 'menu.cpp#L241-L310'),
]

# Inventory items that open a book from wherever the player stands. The
# place left is saved and restored when the book is closed.
BOOKS = [
    {'item': 277, 'name': "Atrus's journal", 'target': ('JRNL', 100)},
    {'item': 279, 'name': "Saavedro's journal", 'target': ('JRNL', 200)},
    {'item': 480, 'name': 'Tomahna linking book', 'target': ('NACH', 220)},
    {'item': 481, 'name': 'Releeshahn book', 'target': ('JRNL', 300)},
]


# The five alignment panels of the electromagnet room. Their scripts leave for "the node in variable 32", which no
# script sets: the engine's routine for this puzzle stores the node number modulo 100 there (114 returns to 14).
TESLA_ROOM, TESLA_NODES, TESLA_VARIABLE = 'ENEM', (114, 116, 118, 120, 122), 'v32'


def tesla_returns(db):
    out = []
    room = db.rooms.get(TESLA_ROOM)
    if room is None:
        return out
    for index, node_id in enumerate(TESLA_NODES):
        if node_id in room.nodes and node_id % 100 in room.nodes:
            reason = f'puzzle routine 2 stores {node_id % 100} in variable 32 when the panel is drawn'
            out.append({'source': (TESLA_ROOM, node_id), 'target': (TESLA_ROOM, node_id % 100), 'unreachable': False,
                        'zip': False, 'origin': {'kind': 'native', 'index': index, 'condition': reason,
                                                 'conditional': False},
                        'text': 'goToNodeTrans1(-32)', 'offset': None, 'via': [], 'guards': [],
                        'engine': BASE + 'puzzles.cpp#L263-L293'})
    return out


def references(db, walker):
    """Engine transitions whose source and destination are both fixed."""
    out = tesla_returns(db)
    menu = db.rooms.get('MENU')
    if menu is None or MENU_MAIN not in menu.nodes:
        return out
    owner = ('MENU', MENU_MAIN)
    for index, (target, reason, where) in enumerate(MENU_ACTIONS):
        if target not in menu.nodes:
            continue
        out.append({'source': owner, 'target': ('MENU', target), 'unreachable': False, 'zip': False,
                    'origin': {'kind': 'native', 'index': index, 'condition': reason, 'conditional': True},
                    'text': f'Menu::goToNode({target})', 'offset': None, 'via': [], 'guards': [reason],
                    'engine': BASE + where})
    if MENU_SETTINGS in menu.nodes:
        # Action 4 runs the scripts of MENU 599 from the main menu.
        result = Walk(owner)
        origin = {'kind': 'native', 'index': len(MENU_ACTIONS), 'condition': 'main menu action 4, Settings',
                  'conditional': True}
        stack = [('MENU', MENU_SETTINGS)]
        state = _fresh()
        for script in menu.nodes[MENU_SETTINGS].scripts:
            walker._script(script.ops, 'MENU', result, origin, stack, [], state, False)
        for reference in result.references:
            out.append({**reference, 'source': owner, 'engine': BASE + 'menu.cpp#L289-L294'})
    return out


def metadata():
    return {
        'reference': f'https://github.com/scummvm/scummvm/tree/{COMMIT}/engines/myst3',
        'boot': {'node': 'XXXX:1', 'note': 'The program loads this node first; its script leads to the main menu.',
                 'source': BASE + 'myst3.cpp#L203-L211'},
        'newGame': {'node': 'TOHO:101', 'note': 'The script of MENU 98 resets the game state and goes to room '
                                                '301, node 101.'},
        'savedReturns': [
            {'mechanism': 'main menu', 'entry': 'MENU:100',
             'note': 'Escape opens the main menu from any place and saves that place; MENU 97 returns to it.',
             'source': BASE + 'menu.cpp#L330-L364'},
            {'mechanism': 'saved game', 'entry': None,
             'note': 'Loading a game goes to the place stored in the save.',
             'source': BASE + 'myst3.cpp#L1600-L1637'},
            *[{'mechanism': 'inventory book', 'item': book['item'], 'name': book['name'],
               'entry': f'{book["target"][0]}:{book["target"][1]}',
               'note': 'Opened from the inventory at any place; the place left is saved and restored on closing.',
               'source': BASE + 'inventory.cpp#L247-L304'} for book in BOOKS],
        ],
        'note': 'Saved returns depend on where the player was. They are listed here and are not expanded into '
                'edges from or to every place.',
    }
