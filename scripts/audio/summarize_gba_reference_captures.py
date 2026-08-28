#!/usr/bin/env python3
"""Write a deterministic manifest for the native GBA audio truth captures."""

from __future__ import annotations

import argparse
import hashlib
import json
import wave
from pathlib import Path


KINDS = ("ch1", "ch2", "ch3", "ch4")


def digest(path: Path) -> str:
    value = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            value.update(block)
    return value.hexdigest()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--runtime", type=Path, required=True)
    parser.add_argument("--captures", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    entries = []
    for kind in KINDS:
        rom = args.runtime / f"audioref-{kind}.gba"
        capture = args.captures / f"audioref-{kind}.wav"
        with wave.open(str(capture), "rb") as wav:
            if wav.getnchannels() != 2 or wav.getsampwidth() != 2:
                raise ValueError(f"{capture} is not stereo signed 16-bit PCM")
            entries.append(
                {
                    "kind": kind,
                    "rom": {
                        "bytes": rom.stat().st_size,
                        "sha256": digest(rom),
                    },
                    "capture": {
                        "frames": wav.getnframes(),
                        "rateHz": wav.getframerate(),
                        "sha256": digest(capture),
                    },
                }
            )

    document = {
        "version": 1,
        "engine": "mGBA 0.10.3 internal mAVStream callback",
        "format": "stereo signed 16-bit PCM",
        "cases": entries,
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(document, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
