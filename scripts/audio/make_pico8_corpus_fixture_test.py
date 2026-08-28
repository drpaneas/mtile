#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("make_pico8_corpus_fixture.py")
SPEC = importlib.util.spec_from_file_location("make_pico8_corpus_fixture", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
fixture = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = fixture
SPEC.loader.exec_module(fixture)


class CorpusFixtureTests(unittest.TestCase):
    def test_replaces_only_lua_and_preserves_audio_sections(self) -> None:
        source = "header\n__lua__\nold()\n__sfx__\nabc\n__music__\ndef\n"
        result = fixture.replace_lua(source, fixture.capture_lua(7, 123, "track"))
        self.assertNotIn("old()", result)
        self.assertIn('extcmd("set_filename","track")', result)
        self.assertIn("music(7,0,15)", result)
        self.assertIn("capture_frames=123", result)
        self.assertTrue(result.endswith("__sfx__\nabc\n__music__\ndef\n"))

    def test_inserts_lua_section_in_audio_only_export(self) -> None:
        result = fixture.replace_lua(
            "pico-8 cartridge\nversion 42\n__sfx__\nabc\n", "replacement\n"
        )
        self.assertEqual(
            result,
            "pico-8 cartridge\nversion 42\n__lua__\nreplacement\n__sfx__\nabc\n",
        )

    def test_can_capture_one_sfx_instead_of_music(self) -> None:
        lua = fixture.capture_lua(0, 600, "isolated", sfx_id=8)
        self.assertIn("sfx(8,0)", lua)
        self.assertNotIn("music(", lua)

    def test_can_isolate_music_channels(self) -> None:
        lua = fixture.capture_lua(40, 600, "isolated", channel_mask=2)
        self.assertIn("music(40,0,2)", lua)

    def test_can_silence_other_channels_without_changing_music_rows(self) -> None:
        note = "24930"
        lines = [f"01010000{note * 32}" for _ in range(5)]
        source = (
            "header\n__sfx__\n"
            + "\n".join(lines)
            + "\n__music__\n00 01020304\n"
        )
        result = fixture.isolate_music_channel(source, 1)
        self.assertIn("00 01020304", result)
        muted = "24900" * 32
        self.assertEqual(result.count(muted), 3)
        self.assertEqual(result.count(note * 32), 2)

    def test_rejects_sfx_shared_by_kept_and_silenced_channels(self) -> None:
        note = "24930"
        source = (
            "header\n__sfx__\n01010000"
            + note * 32
            + "\n__music__\n00 01010101\n"
        )
        with self.assertRaisesRegex(ValueError, "shared music SFX IDs: 1"):
            fixture.isolate_music_channel(source, 1)


if __name__ == "__main__":
    unittest.main()
