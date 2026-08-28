#!/usr/bin/env python3

from __future__ import annotations

import unittest

from audit_pico8_corpus import collect_entries, render_limit
from compare_pico8_corpus import RATE


def segment(source: str, entry: int, *, seconds: float | None, indefinite: bool):
    return {
        "cartSource": source,
        "cart": f"/tmp/{source}",
        "sourceKind": "text",
        "entryPattern": entry,
        "sfxID": None,
        "naturalDurationSeconds": seconds,
        "indefinite": indefinite,
    }


class AuditPICO8CorpusTests(unittest.TestCase):
    def test_collect_entries_deduplicates_and_retains_classification(self) -> None:
        shared = segment("one.p8", 0, seconds=2.0, indefinite=False)
        alternate = segment("two.p8", 3, seconds=None, indefinite=True)
        manifest = {
            "tracks": [
                {
                    "id": "album/01",
                    "segments": [shared],
                    "excludedSegments": [shared, alternate],
                }
            ]
        }
        entries = collect_entries(manifest)
        self.assertEqual(len(entries), 2)
        self.assertEqual(
            entries[0]["classifications"], ["excluded-alternate", "selected"]
        )

    def test_render_limit_uses_full_finite_duration_and_caps_indefinite(self) -> None:
        finite = segment("finite.p8", 0, seconds=2.5, indefinite=False)
        indefinite = segment("loop.p8", 0, seconds=None, indefinite=True)
        self.assertEqual(render_limit(finite, 30), round(2.5 * RATE) + RATE)
        self.assertEqual(render_limit(indefinite, 30), 30 * RATE)


if __name__ == "__main__":
    unittest.main()
