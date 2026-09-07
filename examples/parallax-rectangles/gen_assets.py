#!/usr/bin/env python3
"""Generate a small editable four-plane scene, using only existing mtile APIs."""

import json
from pathlib import Path


ASSETS = Path(__file__).resolve().parent / "assets"
WIDTH, HEIGHT, PERIOD = 60, 20, 30


def write(path, data):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, separators=(",", ":")) + "\n", encoding="utf-8")


def main():
    tiles = [[[color] * 8 for _ in range(8)] for color in (0, 2, 3, 4, 5)]
    # A small original placeholder actor; transparent index 0 stays empty.
    tiles.append([[int(c) for c in row] for row in (
        "00666600", "06666660", "00777700", "00677600",
        "00666600", "06666660", "00700700", "07700770",
    )])
    write(ASSETS / "spritesheet.json", {
        "width": 240, "height": 160,
        "sprites": [{"id": i, "x": i * 8, "y": 0, "width": 8, "height": 8,
                     "pixels": pixels, "flags": 0} for i, pixels in enumerate(tiles)],
    })

    layers = [[0] * (WIDTH * HEIGHT) for _ in range(4)]

    def rectangle(layer, x, y, width, height, tile):
        for py in range(y, y + height):
            for px in range(x, x + width):
                layers[layer][py * WIDTH + px % PERIOD] = tile

    # Independent authored motifs. Each plane needs only one solid tile;
    # rectangles are map cells, never per-frame pixel drawing operations.
    for layer, rectangles in enumerate((
        ((2, 5, 7, 1), (14, 10, 6, 2), (24, 15, 4, 1)),
        ((8, 7, 6, 1), (19, 12, 8, 1), (0, 16, 5, 1)),
        ((20, 4, 6, 2), (4, 11, 5, 1), (13, 15, 8, 1)),
    )):
        for args in rectangles:
            rectangle(layer, *args, layer + 1)

    rectangle(3, 0, 18, 30, 2, 4)
    rectangle(3, 0, 3, 1, 15, 4)
    rectangle(3, 29, 3, 1, 15, 4)
    rectangle(3, 8, 13, 5, 1, 4)
    rectangle(3, 20, 9, 5, 1, 4)

    # Duplicate the motif so the 31-column visible window can cross x=240.
    for cells in layers:
        for y in range(HEIGHT):
            for x in range(PERIOD):
                cells[y * WIDTH + x + PERIOD] = cells[y * WIDTH + x]
    write(ASSETS / "map.json", {"version": 2, "width": WIDTH, "height": HEIGHT,
          "layers": [{"id": i, "cells": cells} for i, cells in enumerate(layers)]})

    # 5-bit RGB, with all 256 entries available to the Sprite Lab.
    colors = [(0, 0, 0), (3, 4, 9), (7, 8, 15), (12, 13, 21),
              (20, 18, 25), (14, 12, 21), (29, 8, 12), (31, 23, 19), (31, 30, 28)]
    colors += [(i % 32, i % 32, i % 32) for i in range(256 - len(colors))]
    write(ASSETS / "palettes" / "parallax.json", {"name": "parallax", "colors":
          [{"r": r, "g": g, "b": b} for r, g, b in colors]})


if __name__ == "__main__":
    main()
