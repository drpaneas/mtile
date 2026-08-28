#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import json
import math
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("check_pico_conformance.py")
SPEC = importlib.util.spec_from_file_location("check_pico_conformance", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
conformance = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(conformance)


def write_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value), encoding="utf-8")


class CheckPICOConformanceTest(unittest.TestCase):
    def test_json_value_replaces_non_finite_metrics(self) -> None:
        value = {"measured": 1.0, "unavailable": [math.nan, math.inf]}
        self.assertEqual(
            conformance.json_value(value),
            {"measured": 1.0, "unavailable": [None, None]},
        )

    def test_filter_matrix_shape_is_stable(self) -> None:
        self.assertEqual(len(conformance.FILTER_GROUP_CASES), 29)
        self.assertEqual(sum(conformance.FILTER_GROUP_CASES.values()), 92)

    def test_instrument_summary_requires_every_waveform_and_pitch(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            result = {
                "fixture": {"waveform": ""},
                "cases": [
                    {"pitch": pitch, "pass": True} for pitch in range(64)
                ],
                "summary": {
                    "cases": 64,
                    "failures": 0,
                    "maxAbsolutePitchCents": 1.0,
                    "minTimbreCorrelation": 0.99,
                    "medianAmplitudeRatio": 1.0,
                }
            }
            for waveform in conformance.WAVEFORMS:
                result["fixture"]["waveform"] = waveform
                write_json(root / waveform / "result.json", result)
            rows = conformance.instrument_summary(root)
            self.assertEqual(len(rows), 8)
            self.assertEqual(sum(int(row["cases"]) for row in rows), 512)

            result["summary"]["failures"] = 1
            write_json(root / "organ" / "result.json", result)
            with self.assertRaisesRegex(ValueError, "organ"):
                conformance.instrument_summary(root)

    def test_speed_summary_requires_exact_unique_range(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            rows = [
                {
                    "speed": speed,
                    "pass": True,
                    "durationDeltaMS": -1.5,
                    "transitionDeltaMS": 1.0,
                    "lowPitchDeltaCents": -2.0,
                    "highPitchDeltaCents": 3.0,
                }
                for speed in range(1, 256)
            ]
            write_json(root / "speeds" / "result.json", {"speeds": rows})
            summary = conformance.speed_summary(root)
            self.assertEqual(summary["cases"], 255)
            self.assertEqual(summary["maxAbsoluteDurationDeltaMS"], 1.5)

            rows.pop()
            write_json(root / "speeds" / "result.json", {"speeds": rows})
            with self.assertRaisesRegex(ValueError, "incomplete"):
                conformance.speed_summary(root)


if __name__ == "__main__":
    unittest.main()
