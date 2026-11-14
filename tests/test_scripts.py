"""Small binary fixtures targeting bytecode alignment and navigation semantics."""

import unittest

from riven_graph.scripts import (
    ParseError,
    format_scripts,
    parse_card,
    parse_hotspots,
    parse_names,
    parse_script,
    walk_commands,
)


NESTED_SWITCH = bytes.fromhex("""
    0001
      0008 0002 0004 0002
        0001 0001
          0008 0002 0007 0002
            0003 0001 0002 0001 0037
            ffff 0001 0002 0001 0038
        ffff 0001
          001b 0003 0002 1234 abcd
""")


class ScriptTests(unittest.TestCase):
    def test_nested_switch_default_keeps_both_guards(self):
        script = parse_script(NESTED_SWITCH)
        flat = list(walk_commands([script]))
        moves = {command["args"][0]: guards for command, guards in flat
                 if command["opcode"] == 2}
        self.assertEqual(set(moves), {55, 56})
        self.assertEqual([(g["variable"], g["value"]) for g in moves[55]], [(4, 1), (7, 3)])
        self.assertEqual([(g["variable"], g["value"]) for g in moves[56]], [(4, 1), (7, 65535)])
        self.assertEqual(moves[56][1]["excluded_values"], [3])
        stack_command, default_guards = flat[-1]
        self.assertEqual(default_guards[0]["excluded_values"], [1])
        self.assertEqual(default_guards[0]["value"], 65535)
        self.assertEqual(stack_command["end_offset"], len(NESTED_SWITCH))

    def test_change_stack_uses_unsigned_32_bit_rmap(self):
        script = parse_script(bytes.fromhex("0001 001b 0003 0002 fedc ba98"))
        command = script["commands"][0]
        self.assertEqual(command["stack_name_id"], 2)
        self.assertEqual(command["global_card_id"], 0xFEDCBA98)
        self.assertEqual(command["offset"], 2)
        self.assertEqual(command["end_offset"], 12)
        text = format_scripts([script], {5: ["aspit", "bspit", "tspit"]})
        self.assertIn('change_stack("tspit", rmap=0xfedcba98)', text)

    def test_stored_movie_opcode_is_not_an_immediate_command(self):
        # One command schedules activateSLST(2) after the 32-bit movie time.
        script = parse_script(bytes.fromhex("0001 0026 0005 0007 0001 2345 0028 0002"))
        self.assertEqual(len(script["commands"]), 1)
        command = script["commands"][0]
        self.assertEqual(command["opcode"], 38)
        self.assertEqual(command["movie_slot"], 7)
        self.assertEqual(command["movie_time"], 0x12345)
        self.assertEqual(command["stored_opcode"], 40)
        self.assertEqual(command["stored_args"], [2])

    def test_hotspot_signed_fields_and_script_boundary(self):
        data = bytes.fromhex("""
            0002
              0012 ffff fffb 0001 0260 0188 0099 00c8 0003 fffe 0001
              0001 0000 0001 0002 0001 0016
              0013 0004 0005 0005 0005 0006 0000 0000 0004 0000 0000
              0000
        """)
        first, second = parse_hotspots(data)
        self.assertEqual((first["offset"], first["end_offset"]), (2, 36))
        self.assertEqual(first["name_id"], -1)
        self.assertEqual(first["rect"], [-5, 1, 608, 392])
        self.assertEqual(first["transition_offset"], -2)
        self.assertEqual(first["flags"], 1)
        self.assertEqual(first["scripts"][0]["event"], "mouse_down")
        self.assertEqual(first["scripts"][0]["commands"][0]["offset"], 30)
        self.assertEqual(first["scripts"][0]["commands"][0]["args"], [22])
        self.assertEqual(second["offset"], 36)
        self.assertFalse(second["valid_rect"])
        self.assertEqual(second["scripts"], [])
        self.assertEqual(second["end_offset"], len(data))

    def test_card_offsets_stay_resource_relative(self):
        # Signed name ID, Zip marker, one CardLoad typed script.
        card = parse_card(bytes.fromhex("ffff 0001 0001 0006 0001 0002 0001 0037"))
        self.assertEqual(card["name_id"], -1)
        self.assertEqual(card["zip_mode_place"], 1)
        self.assertEqual(card["scripts"][0]["offset"], 6)
        self.assertEqual(card["scripts"][0]["commands"][0]["offset"], 10)
        self.assertEqual(card["consumed_bytes"], 16)

    def test_truncation_and_trailing_bytes_are_rejected(self):
        # Every proper prefix is incomplete, including branch body boundaries.
        for cut in range(len(NESTED_SWITCH)):
            with self.subTest(cut=cut), self.assertRaises(ParseError):
                parse_script(NESTED_SWITCH[:cut])
        with self.assertRaisesRegex(ParseError, "trailing bytes"):
            parse_script(NESTED_SWITCH + b"\0\0")
        with self.assertRaises(ParseError):
            parse_card(bytes.fromhex("ffff 0000 0000 0000"))
        with self.assertRaises(ParseError):
            parse_hotspots(bytes.fromhex("0000 0000"))

    def test_name_offsets_use_string_base_and_preserve_numeric_ids(self):
        # Offsets intentionally differ from the separate alphabetical index.
        data = bytes.fromhex("0003 0006 0000 000b 0001 0000 0002") + b"alpha\0beta\0gamma\0"
        self.assertEqual(parse_names(data), ["beta", "alpha", "gamma"])
        self.assertEqual(parse_names(bytes.fromhex("0000")), [])

    def test_invalid_name_pointer_and_missing_terminator(self):
        with self.assertRaisesRegex(ParseError, "outside resource"):
            parse_names(bytes.fromhex("0001 0010 0000") + b"a\0")
        with self.assertRaisesRegex(ParseError, "unterminated"):
            parse_names(bytes.fromhex("0001 0000 0000") + b"a")


if __name__ == "__main__":
    unittest.main()
