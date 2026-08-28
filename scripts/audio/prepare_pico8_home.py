#!/usr/bin/env python3
"""Create an isolated PICO-8 home with deterministic reference-audio gains."""

from __future__ import annotations

import argparse
from pathlib import Path


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("home", type=Path)
    args = parser.parse_args()
    args.home.mkdir(parents=True, exist_ok=True)
    (args.home / "config.txt").write_text(
        "volume 256\nsound_volume 256\nmusic_volume 256\n",
        encoding="utf-8",
    )


if __name__ == "__main__":
    main()
