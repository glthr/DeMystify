"""Graph semantics, attribution, and regression coverage for the supplied CDs."""

from collections import Counter
from pathlib import Path
import os
import unittest

from riven_graph import native, scripts
from riven_graph.build import analyze, build, key


class ProjectionTests(unittest.TestCase):
    def test_projection_collapses_pairs_and_excludes_zip_reset_and_new_game(self):
        graph = {
            "start_card": "a",
            "nodes": [{"id": value} for value in "abcdef"],
            "edges": [
                {"source": "a", "target": "b", "kind": "script"},
                {"source": "a", "target": "b", "kind": "native"},
                {"source": "b", "target": "b", "kind": "script"},
                {"source": "b", "target": "c", "kind": "zip"},
                {"source": "b", "target": "d", "kind": "native-ending"},
                {"source": "b", "target": "e", "kind": "native-new-game"},
                {"source": "b", "target": "f", "kind": "native-timer"},
            ],
        }
        result = analyze(graph)
        self.assertEqual(result["unique_pairs"], 3)
        self.assertEqual(result["reachable_from_start"], 3)
        self.assertEqual(result["maximum_distance_from_start"], 2)
        self.assertEqual(result["most_distant_from_start"], ["f"])
        self.assertEqual(result["self_loops"], ["b"])
        self.assertEqual(result["isolated"], list("cde"))
        self.assertEqual(result["sources"], list("acde"))
        self.assertEqual(result["sinks"], list("cdef"))

    def test_all_redirects_resolve_requested_destination_without_rewriting_caller(self):
        rmaps = {s: [] for s in native.STACK_IDS.values()}
        for src_stack, src_rmap, dst_stack, dst_rmap, _ in native.SPECIAL_REDIRECTS:
            for stack, rmap in ((src_stack, src_rmap), (dst_stack, dst_rmap)):
                if rmap not in rmaps[stack]:
                    rmaps[stack].append(rmap)
        for src_stack, src_rmap, dst_stack, dst_rmap, _ in native.SPECIAL_REDIRECTS:
            with self.subTest(stack=src_stack, rmap=hex(src_rmap)):
                literal = native.local_card_id(src_stack, src_rmap, rmaps)
                target = native.resolve_destination(src_stack, literal, rmaps)
                self.assertEqual(target["literal_stack"], src_stack)
                self.assertEqual(target["literal_card_id"], literal)
                self.assertEqual(target["stack"], dst_stack)
                self.assertEqual(target["card_id"], native.local_card_id(dst_stack, dst_rmap, rmaps))
                self.assertTrue(target["redirected"])


class SuppliedDiscGraphTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        root = Path(__file__).resolve().parents[1]
        fixture = Path(os.environ.get("RIVEN_TEST_DATA", root / "riven-the-sequel-to-myst_202302"))
        if not fixture.is_dir() or len([p for p in fixture.iterdir() if p.suffix.lower() == ".iso"]) != 5:
            raise unittest.SkipTest("The five supplied Riven CD images are not available")
        cls.graph, cls.details, cls.manifest, cls.ast = build([fixture])
        cls.nodes = {n["id"]: n for n in cls.graph["nodes"]}
        cls.cards = {c["id"]: c for c in cls.ast["cards"]}

    def test_supplied_disc_regression_counts_and_new_game_entry(self):
        self.assertEqual({s["id"]: s["card_count"] for s in self.graph["stacks"]}, {
            "aspit": 8, "bspit": 498, "gspit": 267, "jspit": 814,
            "ospit": 89, "pspit": 47, "rspit": 37, "tspit": 547,
        })
        stats = self.graph["stats"]
        self.assertEqual(stats["card_count"], 2307)
        self.assertEqual(stats["node_count"], 2312)
        self.assertEqual(stats["virtual_node_count"], 5)
        self.assertEqual(stats["hotspot_count"], 8498)
        self.assertEqual(stats["command_count"], 45491)
        self.assertEqual(stats["parse_errors"], 0)
        self.assertEqual(self.graph["start_card"], "tspit:155")
        self.assertEqual(self.nodes["tspit:155"]["global_id"], 0x6E9A)
        self.assertEqual(self.ast["names"]["aspit"][5][0], "tspit")

    def test_edge_identity_and_every_native_source_are_attributed(self):
        edge_keys = [(e["source"], e["target"], e["kind"]) for e in self.graph["edges"]]
        self.assertEqual(len(edge_keys), len(set(edge_keys)))
        for edge in self.graph["edges"]:
            with self.subTest(source=edge["source"], target=edge["target"], kind=edge["kind"]):
                self.assertIn(edge["source"], self.nodes)
                self.assertIn(edge["target"], self.nodes)
                self.assertTrue(edge["evidence"])
                self.assertEqual(edge["cross_stack"], self.nodes[edge["source"]]["stack"] != self.nodes[edge["target"]]["stack"])
                for evidence in edge["evidence"]:
                    if edge["kind"].startswith("native"):
                        self.assertTrue(evidence["engine_source"].startswith(native.SOURCE_BASE))
                    self.assertIn("literal_target", evidence)

    def test_dynamic_returns_keep_graph_source_and_reach_card_detail(self):
        returns = [u for u in self.graph["unresolved_navigation"] if u.get("kind") == "dynamic-return"]
        self.assertEqual({u["source"] for u in returns}, {"aspit:5", "aspit:6", "aspit:7"})
        for record in returns:
            self.assertTrue(record["engine_source"].startswith(native.SOURCE_BASE))
            self.assertIn(record, self.details[record["source"]]["unresolved"])
        for record in self.graph["unresolved_navigation"]:
            self.assertIn(record["source"], self.nodes)

    def test_every_literal_navigation_callsite_has_edge_evidence(self):
        # Reconcile all 6,980 original opcode-2/27 callsites, including nested
        # branch guards, against the emitted graph. Native facts are separate.
        def callsite(source, evidence):
            return (source, evidence["resource"], evidence["event"],
                    evidence["byte_offset"], evidence.get("hotspot_ordinal"))

        emitted = {}
        for edge in self.graph["edges"]:
            if edge["kind"] != "script":
                continue
            for evidence in edge["evidence"]:
                identity = callsite(edge["source"], evidence)
                self.assertNotIn(identity, emitted)
                emitted[identity] = evidence
        self.assertFalse(any(u.get("opcode") in (2, 27) for u in self.graph["unresolved_navigation"]))

        expected = set()
        for card in self.ast["cards"]:
            stack = card["stack"]
            contexts = [(None, card["scripts"])] + [(h, h["scripts"]) for h in card["hotspots"]]
            for hotspot, typed_scripts in contexts:
                for script in typed_scripts:
                    for command, guards in scripts.walk_commands([script]):
                        if command["opcode"] not in (2, 27):
                            continue
                        resource = f'{"CARD" if hotspot is None else "HSPT"}:{card["card_id"]}'
                        identity = (card["id"], resource, script["event"], command["offset"],
                                    None if hotspot is None else hotspot["ordinal"])
                        expected.add(identity)
                        self.assertIn(identity, emitted)
                        evidence = emitted[identity]
                        self.assertEqual(evidence["guards"], [
                            {**guard, "variable_name": scripts.name_at(self.ast["names"][stack], 4, guard["variable"])}
                            for guard in guards
                        ])
                        if command["opcode"] == 2:
                            literal = key(stack, command["args"][0])
                        else:
                            target_stack = scripts.name_at(self.ast["names"][stack], 5, command["stack_name_id"]).lower()
                            literal = key(target_stack, native.local_card_id(target_stack, command["global_card_id"], self.ast["rmaps"]))
                        self.assertEqual(evidence["literal_target"], literal)
        self.assertEqual(len(expected), 6980)
        self.assertEqual(set(emitted), expected)

    def test_every_redirected_edge_retains_original_target_and_engine_chain(self):
        redirected_rmaps = set()
        for edge in self.graph["edges"]:
            for evidence in edge["evidence"]:
                literal_stack, literal_card = evidence["literal_target"].split(":")
                resolved = native.resolve_destination(literal_stack, int(literal_card), self.ast["rmaps"])
                self.assertEqual(edge["target"], key(resolved["stack"], resolved["card_id"]))
                self.assertEqual(evidence.get("native_redirects", []), resolved["redirect_chain"])
                for redirect in resolved["redirect_chain"]:
                    redirected_rmaps.add((redirect["source_stack"], redirect["source_rmap"]))
        self.assertEqual(redirected_rmaps, {(s, r) for s, r, *_ in native.SPECIAL_REDIRECTS})

    def test_native_movement_and_endings_are_separate_from_script_and_zip(self):
        kinds = Counter(e["kind"] for e in self.graph["edges"])
        self.assertEqual(kinds, {"script": 6063, "zip": 606, "native": 10,
                                 "native-timer": 3, "native-ending": 4})
        # Global inventory returns must never become a shortcut to arbitrary
        # cards. The trap book's one actual script link remains preserved.
        for source in ("aspit:5", "aspit:6"):
            self.assertFalse(any(e["source"] == source for e in self.graph["edges"]))
        trap_exits = [e for e in self.graph["edges"] if e["source"] == "aspit:7"]
        self.assertEqual([(e["target"], e["kind"]) for e in trap_exits], [("ospit:9", "script")])
        timers = [(e["source"], e["target"]) for e in self.graph["edges"] if e["kind"] == "native-timer"]
        self.assertEqual(timers, [("jspit:621", "jspit:627"), ("jspit:627", "jspit:629"), ("jspit:629", "jspit:632")])

    def test_absent_card_references_survive_as_five_explicit_virtual_nodes(self):
        virtual_ids = {n["id"] for n in self.graph["nodes"] if n.get("virtual")}
        self.assertEqual(virtual_ids, {"bspit:466", "jspit:740", "tspit:361", "tspit:477", "tspit:478"})
        # Preserve reference endpoints without inventing shipped CARD bytes or
        # outgoing scripts for these absent resources.
        self.assertFalse(virtual_ids.intersection(self.cards))
        self.assertFalse(any(e["source"] in virtual_ids for e in self.graph["edges"]))
        missing_edges = [e for e in self.graph["edges"] if e["target"] in virtual_ids]
        self.assertEqual(len(missing_edges), 6)
        self.assertEqual(sum(len(e["evidence"]) for e in missing_edges), 8)
        self.assertEqual({e["target"] for e in missing_edges}, virtual_ids)
        for edge in missing_edges:
            self.assertEqual(edge["kind"], "script")
            self.assertTrue(all(ev.get("missing_target") for ev in edge["evidence"]))

    def test_zip_targets_are_visited_place_candidates_with_matching_names(self):
        for edge in self.graph["edges"]:
            if edge["kind"] != "zip":
                continue
            source, target = self.cards[edge["source"]], self.cards[edge["target"]]
            self.assertEqual(source["stack"], target["stack"])
            self.assertTrue(target["zip_mode_place"])
            for evidence in edge["evidence"]:
                self.assertEqual(evidence["zip_name"], target["name"])
                self.assertEqual(evidence["hotspot_name"], target["name"])
                self.assertEqual(evidence["opcode"], 45)

    def test_supplied_disc_projection_regression(self):
        analysis = self.graph["analysis"]
        self.assertEqual(analysis["unique_pairs"], 6072)
        self.assertEqual(analysis["reachable_from_start"], 2262)
        self.assertEqual(analysis["maximum_distance_from_start"], 116)
        self.assertEqual(sum(map(len, analysis["strongly_connected_components"])), 2312)


if __name__ == "__main__":
    unittest.main()
