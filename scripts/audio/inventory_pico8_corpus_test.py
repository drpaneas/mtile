#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


SCRIPT_DIR = Path(__file__).parent
sys.path.insert(0, str(SCRIPT_DIR))
SPEC = importlib.util.spec_from_file_location(
    "inventory_pico8_corpus", SCRIPT_DIR / "inventory_pico8_corpus.py"
)
assert SPEC is not None and SPEC.loader is not None
inventory = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = inventory
SPEC.loader.exec_module(inventory)

from pico8_cart import CartAudio, MusicPattern, Note, SFX  # noqa: E402


def effect(speed: int = 4) -> SFX:
    notes = tuple(Note(24, 0, 7, 0) for _ in range(32))
    return SFX(filters=0, speed=speed, loop_start=0, loop_end=0, notes=notes)


class InventoryTest(unittest.TestCase):
    def test_finds_each_finite_graph_root_and_duration(self) -> None:
        cart = CartAudio(
            version=42,
            sfx=(effect(),),
            music=(
                MusicPattern(0, (0, None, None, None)),
                MusicPattern(4, (0, None, None, None)),
                MusicPattern(4, (0, None, None, None)),
            ),
        )
        self.assertEqual(inventory.graph_entry_patterns(cart), [0, 2])
        timeline = inventory.entry_timeline(cart, 0)
        self.assertEqual(timeline["patterns"], [0, 1])
        self.assertEqual(timeline["durationUnits"], 256)
        self.assertFalse(timeline["indefinite"])

    def test_pure_loop_gets_one_stable_entry(self) -> None:
        cart = CartAudio(
            version=42,
            sfx=(effect(),),
            music=(
                MusicPattern(1, (0, None, None, None)),
                MusicPattern(2, (0, None, None, None)),
            ),
        )
        self.assertEqual(inventory.graph_entry_patterns(cart), [0])
        timeline = inventory.entry_timeline(cart, 0)
        self.assertEqual(timeline["patterns"], [0, 1])
        self.assertEqual(timeline["cycle"], [0, 1])
        self.assertIsNone(timeline["durationUnits"])
        self.assertTrue(timeline["indefinite"])


if __name__ == "__main__":
    unittest.main()
