#!/usr/bin/env python3
"""Author grouped cloud planes; retain every original terrain cell and sprite."""

import json
from pathlib import Path


ASSETS = Path(__file__).resolve().parent / "assets"
WIDTH, HEIGHT, PERIOD = 60, 20, 30
CLOUD_TILE = 45


def main():
    map_path = ASSETS / "map.json"
    source = json.loads(map_path.read_text())
    old_width = source["width"]
    planes = {layer["id"]: layer["cells"] for layer in source["layers"]}
    # First run migrates BG1. Later regeneration preserves edited BG3 terrain.
    old_terrain = planes[1 if old_width == 30 else 3]
    terrain = [0] * (WIDTH * HEIGHT)
    for y in range(HEIGHT):
        terrain[y * WIDTH:y * WIDTH + PERIOD] = old_terrain[y * old_width:y * old_width + PERIOD]

    # Seventeen authored rectangles, grouped into three independently scrolled
    # motifs. Coordinates and widths are cells; one cell is eight pixels tall.
    motifs = (
        ((2, 2, 6), (17, 5, 7), (5, 9, 5), (22, 12, 8), (12, 15, 6), (1, 18, 4)),
        ((11, 3, 5), (1, 6, 8), (21, 9, 6), (8, 12, 5), (18, 16, 7), (4, 19, 6)),
        ((22, 1, 6), (7, 4, 7), (15, 8, 8), (0, 13, 6), (25, 17, 5)),
    )
    layers = []
    for bg, rectangles in enumerate(motifs):
        cells = [0] * (WIDTH * HEIGHT)
        for x, y, width in rectangles:
            for column in range(x, x + width):
                cells[y * WIDTH + column % PERIOD] = CLOUD_TILE
        for y in range(HEIGHT):
            cells[y * WIDTH + PERIOD:(y + 1) * WIDTH] = cells[y * WIDTH:y * WIDTH + PERIOD]
        layers.append({"id": bg, "cells": cells})
    layers.append({"id": 3, "cells": terrain})
    map_path.write_text(json.dumps({"version": 2, "width": WIDTH, "height": HEIGHT,
                                   "layers": layers}, indent=2) + "\n")

    sheet_path = ASSETS / "spritesheet.json"
    sheet = json.loads(sheet_path.read_text())
    sprites = {sprite["id"]: sprite for sprite in sheet["sprites"]}
    tile = {"id": CLOUD_TILE, "x": CLOUD_TILE % 30 * 8, "y": CLOUD_TILE // 30 * 8,
            "width": 8, "height": 8, "pixels": [[206] * 8 for _ in range(8)], "flags": 0}
    if CLOUD_TILE in sprites and sprites[CLOUD_TILE] != tile:
        raise ValueError(f"Sprite {CLOUD_TILE} is occupied; choose an unused cloud tile")
    sprites[CLOUD_TILE] = tile
    sheet["sprites"] = [sprites[key] for key in sorted(sprites)]
    sheet_path.write_text(json.dumps(sheet, indent=2) + "\n")


if __name__ == "__main__":
    main()
