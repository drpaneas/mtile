#!/usr/bin/env python3
"""Tests for deterministic PICO-8 audio-project import."""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).parent))
from import_pico8_audio import pattern_duration_units, write_project
from pico8_cart import CartAudio, MusicPattern, Note, SFX


def cart_text(*, music: str = "00 00010203") -> str:
    silent = "00000" * 32
    audible = "18070" + "00000" * 31
    sfx = "00010000" + audible + "\n" + "00010000" + silent + "\n" * 62
    return (
        "pico-8 cartridge // http://www.pico-8.com\n"
        "version 42\n"
        "__sfx__\n"
        f"{sfx}"
        "__music__\n"
        f"{music}\n"
    )


class ImportPICO8AudioTests(unittest.TestCase):
    def test_looping_sfx_still_sets_music_pattern_duration(self) -> None:
        effect = SFX(
            filters=0,
            speed=20,
            loop_start=0,
            loop_end=4,
            notes=tuple(Note(24, 0, 7, 0) for _ in range(32)),
        )
        cart = CartAudio(
            version=42,
            sfx=(effect,),
            music=(MusicPattern(0, (0, None, None, None)),),
        )
        self.assertEqual(pattern_duration_units(cart, cart.music[0]), 640)

    def test_non_looping_channel_sets_duration_after_looping_channel(self) -> None:
        notes = tuple(Note(24, 0, 7, 0) for _ in range(32))
        looping = SFX(
            filters=0, speed=20, loop_start=0, loop_end=4, notes=notes
        )
        finite = SFX(filters=0, speed=4, loop_start=0, loop_end=0, notes=notes)
        cart = CartAudio(
            version=42,
            sfx=(looping, finite),
            music=(MusicPattern(0, (0, 1, None, None)),),
        )
        self.assertEqual(pattern_duration_units(cart, cart.music[0]), 128)

    def test_all_looping_channels_use_longest_duration(self) -> None:
        notes = tuple(Note(24, 0, 7, 0) for _ in range(32))
        short = SFX(filters=0, speed=16, loop_start=0, loop_end=32, notes=notes)
        long = SFX(filters=0, speed=44, loop_start=0, loop_end=32, notes=notes)
        cart = CartAudio(
            version=42,
            sfx=(short, long),
            music=(MusicPattern(0, (0, 1, None, None)),),
        )
        self.assertEqual(pattern_duration_units(cart, cart.music[0]), 44 * 32)

    def test_generated_project_starts_music_once_in_init(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            cart = root / "fixture.p8"
            output = root / "project"
            cart.write_text(cart_text(), encoding="utf-8")

            write_project(cart, output, 0)

            game = (output / "game.lua").read_text(encoding="utf-8")
            self.assertIn("function _init()\n music(0)\nend", game)
            self.assertEqual(game.count("music(0)"), 1)
            self.assertNotIn("started", game)

    def test_can_start_one_sfx_for_channel_isolation(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            cart = root / "fixture.p8"
            output = root / "project"
            cart.write_text(cart_text(), encoding="utf-8")

            write_project(cart, output, 0, sfx_id=1)

            game = (output / "game.lua").read_text(encoding="utf-8")
            self.assertIn("function _init()\n sfx(1, 0)\nend", game)
            self.assertNotIn("music(", game)


if __name__ == "__main__":
    unittest.main()
