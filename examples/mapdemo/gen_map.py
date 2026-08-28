#!/usr/bin/env python3
"""Regenerate mapdemo/assets/map.json at the maximum contiguous world width."""

import json
from pathlib import Path

# Lua numbers are signed 16:16 fixed point, so positive world coordinates end
# at 32767 pixels. 4095 whole 8-pixel tiles use 32760 pixels: 136 complete
# native screens plus 15 tiles. A 137th complete screen would need 32880
# pixels and wrap. The authored three-screen pattern repeats across that width.
FULL_SCREENS = 136
SCREEN_TILES_W = 30
EXTRA_TILES = 15
PATTERN_SCREENS = 3
MW, MH = FULL_SCREENS * SCREEN_TILES_W + EXTRA_TILES, 20
PATTERN_W = PATTERN_SCREENS * SCREEN_TILES_W
OUT = Path(__file__).resolve().parent / "assets" / "map.json"


def put(cells, x, y, t):
    if 0 <= x < MW and 0 <= y < MH:
        cells[y * MW + x] = t


def fill_row(cells, y, x0, x1, t):
    for x in range(x0, min(x1 + 1, MW)):
        put(cells, x, y, t)


def main():
    cells = [0] * (MW * MH)

    # --- screen 1 (cols 0-29): original room ---
    for x in range(0, 30):
        put(cells, x, 18, 1)
        put(cells, x, 19, 2)
    for x in range(22, 30):
        put(cells, x, 17, 1)
        if x > 25:
            put(cells, x, 16, 1)
    for x in range(22, 30):
        put(cells, x, 18, 2)
    for x in range(26, 30):
        put(cells, x, 17, 2)
    for x in range(4, 9):
        put(cells, x, 13, 3)
    for x in range(12, 16):
        put(cells, x, 10, 3)
    for x in range(19, 23):
        put(cells, x, 7, 3)
    for y in range(14, 18):
        put(cells, 2, y, 4)
    for x in range(9, 12):
        put(cells, x, 15, 7)
    for x, y in [
        (5, 12), (7, 12), (13, 9), (14, 9), (20, 6), (3, 17), (10, 17),
        (15, 17), (24, 16), (27, 15),
    ]:
        put(cells, x, y, 5)
    for x, y in [(6, 12), (21, 6), (12, 17), (25, 16)]:
        put(cells, x, y, 6)

    # --- screen 2 (cols 30-59): rolling hills + platforms ---
    for x in range(30, 60):
        put(cells, x, 18, 1)
        put(cells, x, 19, 2)
    for x in range(34, 42):
        put(cells, x, 16, 1)
        put(cells, x, 17, 2)
    for x in range(48, 56):
        put(cells, x, 15, 1)
        put(cells, x, 16, 2)
    fill_row(cells, 11, 33, 38, 3)
    fill_row(cells, 8, 45, 50, 3)
    fill_row(cells, 5, 52, 57, 3)
    for y in range(12, 17):
        put(cells, 58, y, 4)
    for x, y in [(36, 15), (40, 10), (47, 7), (54, 4), (32, 17), (51, 14)]:
        put(cells, x, y, 5)
    for x, y in [(37, 10), (53, 14)]:
        put(cells, x, y, 6)

    # --- screen 3 (cols 60-89): mesa + descent ---
    for x in range(60, 90):
        put(cells, x, 18, 1)
        put(cells, x, 19, 2)
    for x in range(62, 72):
        put(cells, x, 14, 3)
    for x in range(74, 84):
        put(cells, x, 11, 3)
    for x in range(80, 88):
        put(cells, x, 7, 3)
    for x in range(66, 70):
        put(cells, x, 17, 7)
    for y in range(10, 18):
        put(cells, 85, y, 4)
    for x, y in [(64, 13), (70, 10), (78, 6), (83, 16), (87, 8)]:
        put(cells, x, y, 5)
    for x, y in [(71, 13), (82, 10)]:
        put(cells, x, y, 6)

    # Repeat the three authored rooms to fill the maximum-width continuous
    # fixed-point world. The map is generated data, so compact JSON keeps the
    # repository file manageable while preserving the ordinary asset schema.
    for y in range(MH):
        row = y * MW
        for x in range(PATTERN_W, MW):
            cells[row + x] = cells[row + x % PATTERN_W]

    filled = sum(1 for c in cells if c)
    data = {"width": MW, "height": MH, "cells": cells}
    OUT.write_text(json.dumps(data, separators=(",", ":")) + "\n")
    print(
        f"wrote {OUT}: {MW}x{MH}, {FULL_SCREENS} screens + {EXTRA_TILES} tiles, "
        f"{filled}/{MW * MH} cells ({100 * filled // (MW * MH)}%)"
    )


if __name__ == "__main__":
    main()
