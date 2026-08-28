#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("pico8_cart.py")
SPEC = importlib.util.spec_from_file_location("pico8_cart", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
pico8_cart = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = pico8_cart
SPEC.loader.exec_module(pico8_cart)

acyclic_depth = pico8_cart.acyclic_depth
custom_instrument_graph = pico8_cart.custom_instrument_graph
graph_cycles = pico8_cart.graph_cycles
parse_cart_text = pico8_cart.parse_cart_text


def sfx_line(notes: list[str], speed: int = 16, loop: tuple[int, int] = (0, 0)) -> str:
    encoded = notes + ["00000"] * (32 - len(notes))
    return f"00{speed:02x}{loop[0]:02x}{loop[1]:02x}" + "".join(encoded)


class PICO8CartTest(unittest.TestCase):
    def test_parses_sfx_and_music_without_losing_native_values(self) -> None:
        cart = parse_cart_text(
            "\n".join(
                [
                    "pico-8 cartridge // http://www.pico-8.com",
                    "version 42",
                    "__sfx__",
                    sfx_line(["18875", "21764"], speed=255, loop=(1, 2)),
                    "__music__",
                    "05 00417f02",
                ]
            )
        )
        self.assertEqual(cart.version, 42)
        self.assertEqual((cart.sfx[0].speed, cart.sfx[0].loop_start, cart.sfx[0].loop_end), (255, 1, 2))
        self.assertEqual((cart.sfx[0].notes[0].pitch, cart.sfx[0].notes[0].custom_instrument), (24, 0))
        self.assertEqual((cart.sfx[0].notes[0].volume, cart.sfx[0].notes[0].effect), (7, 5))
        self.assertEqual(cart.music[0].flags, 5)
        self.assertEqual(cart.music[0].channels, (0, None, None, 2))

    def test_reports_acyclic_depth_and_cycles(self) -> None:
        cart = parse_cart_text(
            "\n".join(
                [
                    "version 42",
                    "__sfx__",
                    sfx_line(["18870"]),
                    sfx_line(["18970"]),
                ]
            )
        )
        graph = custom_instrument_graph(cart)
        self.assertEqual(graph, {0: {0}, 1: {1}})
        self.assertEqual(graph_cycles(graph), [[0], [1]])
        self.assertIsNone(acyclic_depth(graph))

        cart = parse_cart_text(
            "\n".join(
                [
                    "version 42",
                    "__sfx__",
                    sfx_line(["18970"]),
                    sfx_line(["18070"]),
                ]
            )
        )
        graph = custom_instrument_graph(cart)
        self.assertEqual(graph, {0: {1}, 1: set()})
        self.assertEqual(graph_cycles(graph), [])
        self.assertEqual(acyclic_depth(graph), 2)


if __name__ == "__main__":
    unittest.main()
