#!/usr/bin/env python3
"""Regenerate the deterministic current-API coin-block example assets."""

import json
from pathlib import Path


EXAMPLE_DIR = Path(__file__).resolve().parent
ASSET_DIR = EXAMPLE_DIR / "assets"


def write_json(destination, value):
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


def pixels(rows):
    if len(rows) != 8 or any(len(row) != 8 for row in rows):
        raise ValueError("every tile must be exactly 8x8")
    return [[int(color, 16) for color in row] for row in rows]


def sprite(tile_id, rows):
    return {
        "id": tile_id,
        "x": tile_id * 8,
        "y": 0,
        "width": 8,
        "height": 8,
        "pixels": pixels(rows),
        "flags": 0,
    }


def make_spritesheet():
    tiles = [
        ["00000000"] * 8,
        [
            "88888888",
            "89889889",
            "77777777",
            "77977797",
            "77777777",
            "79777977",
            "77777777",
            "77797779",
        ],
        [
            "77777777",
            "77977797",
            "77777777",
            "79777977",
            "77777777",
            "77797779",
            "77777777",
            "79777797",
        ],
        [
            "000CC000",
            "00CCCC00",
            "00066000",
            "00666600",
            "00333300",
            "00300300",
            "00E00E00",
            "0EE00EE0",
        ],
        [
            "66666666",
            "65555556",
            "65000556",
            "65005556",
            "65005006",
            "65555556",
            "65505556",
            "66666666",
        ],
        [
            "AAAAAAAA",
            "A444444A",
            "A400044A",
            "A400444A",
            "A400400A",
            "A444444A",
            "A440444A",
            "AAAAAAAA",
        ],
        [
            "AAAAAAAA",
            "ABBBBBBA",
            "ABAAAABA",
            "ABBBBBBA",
            "ABAAAABA",
            "ABBBBBBA",
            "ABBBBBBA",
            "AAAAAAAA",
        ],
        [
            "00055000",
            "00555500",
            "05566550",
            "05566550",
            "05566550",
            "05566550",
            "00555500",
            "00055000",
        ],
    ]
    return {
        "width": 240,
        "height": 160,
        "sprites": [sprite(tile_id, rows) for tile_id, rows in enumerate(tiles)],
    }


def make_map():
    width, height = 30, 20
    cells = [0] * (width * height)
    for x in range(width):
        cells[17 * width + x] = 1
        cells[18 * width + x] = 2
        cells[19 * width + x] = 2
    cells[14 * width + 14] = 4
    return {"width": width, "height": height, "cells": cells}


def make_palette():
    colors = [
        (0, 0, 0),
        (1, 2, 6),
        (2, 4, 10),
        (4, 20, 28),
        (31, 31, 31),
        (31, 27, 4),
        (31, 15, 3),
        (14, 7, 3),
        (3, 23, 9),
        (2, 13, 7),
        (22, 22, 24),
        (12, 12, 15),
        (28, 5, 5),
        (31, 14, 18),
        (5, 11, 31),
        (24, 26, 31),
    ]
    return {
        "name": "coin-block",
        "colors": [{"r": r, "g": g, "b": b} for r, g, b in colors],
    }


def main():
    write_json(ASSET_DIR / "spritesheet.json", make_spritesheet())
    write_json(ASSET_DIR / "map.json", make_map())
    write_json(ASSET_DIR / "palettes" / "coin-block.json", make_palette())
    write_json(
        ASSET_DIR / "audio" / "settings.json",
        {"version": 2, "profile": "gameboy-psg", "leftLevel": 7, "rightLevel": 7, "psgRatio": 100},
    )
    write_json(ASSET_DIR / "audio" / "sounds.json", {"version": 2, "sounds": None})
    write_json(
        ASSET_DIR / "audio" / "music.json",
        {"version": 2, "ppqn": 96, "clips": None, "parts": None, "songs": None},
    )


if __name__ == "__main__":
    main()
