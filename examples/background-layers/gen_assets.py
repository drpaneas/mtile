#!/usr/bin/env python3
"""Regenerate the editor-friendly version-2 four-background assets."""

import json
from pathlib import Path


EXAMPLE_DIR = Path(__file__).resolve().parent
ASSET_DIR = EXAMPLE_DIR / "assets"
PALETTE_DIR = ASSET_DIR / "palettes"
MAP_WIDTH = 60
MOTIF_WIDTH = 30
STRIP_HEIGHT = 20


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
            "00000000",
            "00050000",
            "00555000",
            "00050000",
            "00000000",
            "00000000",
            "00000000",
            "00000000",
        ],
        [
            "00055000",
            "00555500",
            "05545550",
            "05545550",
            "05555550",
            "05555550",
            "00555500",
            "00055000",
        ],
        [
            "00066000",
            "00676600",
            "06677660",
            "06777760",
            "66666666",
            "66666666",
            "66666666",
            "66666666",
        ],
        [
            "66666666",
            "66766666",
            "66666666",
            "66667666",
            "66666666",
            "67666666",
            "66666666",
            "66666676",
        ],
        [
            "00099000",
            "00999900",
            "09999990",
            "99999999",
            "88888888",
            "88888888",
            "88888888",
            "88888888",
        ],
        [
            "88888888",
            "88988888",
            "88888888",
            "88889888",
            "88888888",
            "89888888",
            "88888888",
            "88888898",
        ],
        [
            "AAAAAAAA",
            "A9AA9AAA",
            "BBBBBBBB",
            "BBABBBAB",
            "BBBBBBBB",
            "BABBABBB",
            "BBBBBBBB",
            "BBBABBBA",
        ],
        [
            "BBBBBBBB",
            "BBABBBAB",
            "BBBBBBBB",
            "BABBABBB",
            "BBBBBBBB",
            "BBBABBBA",
            "BBBBBBBB",
            "ABBBABBB",
        ],
        [
            "00000000",
            "00044000",
            "04444400",
            "44444440",
            "04444444",
            "00444440",
            "00000000",
            "00000000",
        ],
        [
            "000DD000",
            "00DDDD00",
            "000FF000",
            "00CCCC00",
            "0CCCCCC0",
            "00EEEE00",
            "00E00E00",
            "0EE00EE0",
        ],
    ]
    return {
        # Use the editor's native 30x20 grid even though this example authors
        # only the first row. Empty slots remain available for experimentation.
        "width": 240,
        "height": 160,
        "sprites": [sprite(tile_id, rows) for tile_id, rows in enumerate(tiles)],
    }


def make_map():
    layers = [[0] * (MAP_WIDTH * STRIP_HEIGHT) for _ in range(4)]

    def put(layer, x, y, tile_id):
        layers[layer][y * MAP_WIDTH + x] = tile_id

    # BG0: sparse transparent details over cls()'s night-sky backdrop.
    for x in range(0, MOTIF_WIDTH, 4):
        put(0, x, 2 + (x * 3) % 9, 1)
    put(0, 11, 3, 2)
    put(0, 19, 8, 9)
    put(0, 28, 5, 9)

    # BG1: a low-frequency distant mountain silhouette.
    far_heights = [7, 8, 9, 11, 10, 9, 8, 7]
    for x in range(MOTIF_WIDTH):
        top = STRIP_HEIGHT - far_heights[x % len(far_heights)]
        put(1, x, top, 3)
        for y in range(top + 1, STRIP_HEIGHT):
            put(1, x, y, 4)

    # BG2: taller, darker hills in front of the mountains.
    near_heights = [4, 5, 6, 7, 6, 5, 4, 5]
    for x in range(MOTIF_WIDTH):
        top = STRIP_HEIGHT - near_heights[x % len(near_heights)]
        put(2, x, top, 5)
        for y in range(top + 1, STRIP_HEIGHT):
            put(2, x, y, 6)

    # BG3: the full-speed foreground floor.
    for x in range(MOTIF_WIDTH):
        put(3, x, 17, 7)
        put(3, x, 18, 8)
        put(3, x, 19, 8)

    # The second screen is an exact copy of the first. A 240-pixel camera
    # period can therefore rebase from source column 29 to zero without a
    # visible seam; only that layer refreshes its hardware window.
    for layer in range(4):
        for y in range(STRIP_HEIGHT):
            for x in range(MOTIF_WIDTH):
                source = y * MAP_WIDTH + x
                layers[layer][source + MOTIF_WIDTH] = layers[layer][source]

    return {
        "version": 2,
        "width": MAP_WIDTH,
        "height": STRIP_HEIGHT,
        "layers": [
            {"id": layer, "cells": cells}
            for layer, cells in enumerate(layers)
        ],
    }


def make_palette():
    # Keep the scene's hand-authored first row, then expose a complete color
    # cube plus grayscale ramp so Sprite Lab has 256 real paintable entries.
    rgb = [
        (0, 0, 0),
        (1, 2, 6),
        (2, 4, 10),
        (4, 8, 15),
        (12, 20, 28),
        (31, 31, 24),
        (8, 6, 14),
        (14, 10, 20),
        (2, 10, 10),
        (4, 18, 12),
        (8, 24, 10),
        (10, 6, 3),
        (28, 5, 5),
        (28, 18, 10),
        (4, 8, 22),
        (2, 2, 3),
    ]
    levels = [0, 6, 12, 18, 24, 31]
    for red in levels:
        for green in levels:
            for blue in levels:
                rgb.append((red, green, blue))
    for value in range(24):
        channel = round(value * 31 / 23)
        rgb.append((channel, channel, channel))
    if len(rgb) != 256:
        raise ValueError(f"palette has {len(rgb)} colors, want 256")
    return {
        "name": "parallax",
        "colors": [{"r": r, "g": g, "b": b} for r, g, b in rgb],
    }


def main():
    write_json(ASSET_DIR / "spritesheet.json", make_spritesheet())
    write_json(ASSET_DIR / "map.json", make_map())
    write_json(PALETTE_DIR / "parallax.json", make_palette())


if __name__ == "__main__":
    main()
