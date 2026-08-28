#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


SCRIPT_DIR = Path(__file__).parent
sys.path.insert(0, str(SCRIPT_DIR))
SPEC = importlib.util.spec_from_file_location(
    "pair_pico8_corpus", SCRIPT_DIR / "pair_pico8_corpus.py"
)
assert SPEC is not None and SPEC.loader is not None
pairing = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = pairing
SPEC.loader.exec_module(pairing)


def cart(
    album: str, source: str, entries: list[int], calls: list[int] | None = None
) -> dict[str, object]:
    return {
        "album": album,
        "source": source,
        "cart": f"/converted/{Path(source).name}.p8",
        "sourceKind": "png",
        "literalMusicCalls": calls or [],
        "entries": [
            {
                "entryPattern": entry,
                "durationSeconds": float(entry + 1),
                "indefinite": False,
            }
            for entry in entries
        ],
        "fallbackSFX": None,
        "audibleCustomNotes": 0,
        "customInstrumentDepth": 0,
    }


class PairingTest(unittest.TestCase):
    def test_maps_multipart_and_music_call_entry(self) -> None:
        album = "Celeste Soundtrack (Pico-8 Edition) (2019)"
        inventory = {
            "corpus": "/corpus",
            "convertedCarts": "/converted",
            "carts": [
                cart(album, f"{album}/Carts/03 song2.p8", [0, 20]),
                cart(album, f"{album}/Carts/03 song.p8", [0, 12], [12]),
            ],
            "references": [
                {
                    "album": album,
                    "track": 3,
                    "source": f"{album}/03 Song.mp3",
                    "durationSeconds": 14.0,
                }
            ],
        }
        result = pairing.pair(inventory)
        self.assertEqual(result["summary"]["coveredCarts"], 2)
        self.assertEqual(result["summary"]["pairedReferences"], 1)
        track = result["tracks"][0]
        self.assertEqual(
            [item["entryPattern"] for item in track["segments"]], [12, 0]
        )

    def test_expands_tunes_entries_into_tracks(self) -> None:
        album = "Pico-8 Tunes Vol. 1 (2017)"
        entries = [0, 6, 14, 18, 24, 30, 36, 42, 46, 48, 55, 59]
        inventory = {
            "corpus": "/corpus",
            "convertedCarts": "/converted",
            "carts": [cart(album, f"{album}/Tunes.p8.png", entries)],
            "references": [],
        }
        result = pairing.pair(inventory)
        self.assertEqual(
            [track["track"] for track in result["tracks"]], list(range(1, 13))
        )
        self.assertEqual(result["summary"]["coveredCarts"], 1)
        self.assertEqual(result["summary"]["segmentEntries"], 12)

    def test_uses_extra_graph_root_when_it_matches_reference_duration(self) -> None:
        album = "Pico & Chill (2021)"
        inventory = {
            "corpus": "/corpus",
            "convertedCarts": "/converted",
            "carts": [cart(album, f"{album}/Carts/ambience4.p8.png", [0, 20])],
            "references": [
                {
                    "album": album,
                    "track": 7,
                    "source": f"{album}/07 Ambience.mp3",
                    "durationSeconds": 22.0,
                }
            ],
        }
        result = pairing.pair(inventory)
        self.assertEqual(
            [item["entryPattern"] for item in result["tracks"][0]["segments"]],
            [0, 20],
        )

    def test_known_album_name_exceptions(self) -> None:
        cases = {
            "Double Dragon Soundtrack (Pico-8 Edition) (2016)": (
                "ddbossfinal.p8",
                8,
            ),
            "Sonic the Hedgehog Soundtrack (Pico-8 Edition) (2016)": (
                "sthh staff roll2.p8",
                19,
            ),
            "The Last Bot (2016)": ("J-Taylor Made5.p8", 3),
            "Pico & Chill (2021)": ("picochill1b.p8.png", 2),
            "The Legend of Zelda - A Link to the Past Soundtrack (Pico-8 Edition) (2015)": (
                "Zelda 30 Epilogue 2.p8",
                30,
            ),
        }
        for album, (name, expected) in cases.items():
            with self.subTest(name=name):
                self.assertEqual(
                    pairing.cart_track(
                        {"album": album, "source": f"{album}/Carts/{name}"}
                    ),
                    expected,
                )

    def test_reconstructs_slipways_interactive_arrangement(self) -> None:
        album = "Pico Mix (2018)"
        entries = [0, 3, 6, 8, 11, 15, 20, 52]
        inventory = {
            "corpus": "/corpus",
            "convertedCarts": "/converted",
            "carts": [cart(album, f"{album}/Carts/13 slipways.p8.png", entries)],
            "references": [
                {
                    "album": album,
                    "track": 13,
                    "source": f"{album}/13 Slipways.mp3",
                    "durationSeconds": 1963.86,
                }
            ],
        }
        track = pairing.pair(inventory)["tracks"][0]
        self.assertEqual(
            [item["entryPattern"] for item in track["segments"]],
            [entry for entry, _ in pairing.SLIPWAYS_ARRANGEMENT],
        )
        self.assertEqual(
            [item["renderDurationUnits"] for item in track["segments"]],
            [units for _, units in pairing.SLIPWAYS_ARRANGEMENT],
        )
        self.assertEqual(
            [item["entryPattern"] for item in track["excludedSegments"]], [52]
        )


if __name__ == "__main__":
    unittest.main()
