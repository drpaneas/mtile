#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
import wave
from pathlib import Path


SCRIPT_DIR = Path(__file__).parent
sys.path.insert(0, str(SCRIPT_DIR))
SPEC = importlib.util.spec_from_file_location(
    "run_pico8_corpus", SCRIPT_DIR / "run_pico8_corpus.py"
)
assert SPEC is not None and SPEC.loader is not None
runner = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = runner
SPEC.loader.exec_module(runner)


class CorpusRunnerTest(unittest.TestCase):
    def test_allocates_reference_residual_to_indefinite_segments(self) -> None:
        segments = [
            {"indefinite": False, "naturalDurationSeconds": 2.0},
            {"indefinite": True, "naturalDurationSeconds": None},
            {"indefinite": True, "naturalDurationSeconds": None},
        ]
        limits = runner.segment_limits(segments, 8.0)
        self.assertEqual(limits[0], 3 * runner.RATE)
        self.assertEqual(limits[1:], [3 * runner.RATE, 3 * runner.RATE])

    def test_explicit_speed_units_override_indefinite_allocation(self) -> None:
        segments = [
            {
                "indefinite": True,
                "naturalDurationSeconds": None,
                "renderDurationUnits": 4096,
            }
        ]
        self.assertEqual(
            runner.segment_limits(segments, 999.0),
            [runner.samples_for_units(4096)],
        )

    def test_concatenates_target_rate_mono_wavs(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            parts = []
            for index, frames in enumerate((3, 5)):
                part = root / f"{index}.wav"
                with wave.open(str(part), "wb") as target:
                    target.setnchannels(1)
                    target.setsampwidth(2)
                    target.setframerate(runner.RATE)
                    target.writeframes(bytes([index, 0]) * frames)
                parts.append(part)
            output = root / "joined.wav"
            self.assertEqual(runner.concatenate_wavs(parts, output), 8)
            with wave.open(str(output), "rb") as joined:
                self.assertEqual(joined.getnframes(), 8)


if __name__ == "__main__":
    unittest.main()
