#!/usr/bin/env python3
"""Original, deterministic native tile art. Python standard library only."""

import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
ASSETS = HERE / "assets"
COLORS = [
    (0, 0, 0), (2, 3, 5), (3, 5, 8), (6, 9, 13),
    (11, 14, 16), (22, 25, 25), (29, 28, 24), (28, 22, 11),
    (21, 14, 6), (6, 5, 5), (11, 8, 6), (4, 9, 9),
    (8, 14, 11), (15, 19, 13), (20, 11, 9), (25, 17, 14),
]


def write(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


def rect(image, x, y, w, h, color):
    for py in range(max(0, y), min(len(image), y + h)):
        for px in range(max(0, x), min(len(image[0]), x + w)):
            image[py][px] = color


def line(image, x0, y0, x1, y1, color):
    dx, dy = abs(x1 - x0), -abs(y1 - y0)
    sx, sy = 1 if x0 < x1 else -1, 1 if y0 < y1 else -1
    error = dx + dy
    while True:
        rect(image, x0, y0, 1, 1, color)
        if x0 == x1 and y0 == y1:
            return
        double = 2 * error
        if double >= dy:
            error += dy
            x0 += sx
        if double <= dx:
            error += dx
            y0 += sy


def ellipse(image, x, y, rx, ry, color):
    for py in range(y - ry, y + ry + 1):
        for px in range(x - rx, x + rx + 1):
            if (px - x) ** 2 * ry ** 2 + (py - y) ** 2 * rx ** 2 <= rx ** 2 * ry ** 2:
                rect(image, px, py, 1, 1, color)


def star(image, x, y, color):
    line(image, x - 2, y, x + 2, y, color)
    line(image, x, y - 2, x, y + 2, color)


def lantern(image, x, y):
    line(image, x, y - 12, x, y - 2, 8)
    rect(image, x - 4, y - 1, 9, 2, 8)
    rect(image, x - 3, y + 1, 7, 9, 7)
    rect(image, x - 1, y + 2, 3, 6, 6)
    rect(image, x - 4, y + 10, 9, 2, 8)


def pot(image, x, y, leaf=12):
    rect(image, x - 8, y, 17, 3, 15)
    rect(image, x - 6, y + 3, 13, 9, 14)
    rect(image, x - 4, y + 12, 9, 2, 10)
    line(image, x, y - 22, x, y, 13)
    for dy, direction in [(-4, -1), (-9, 1), (-14, -1), (-19, 1)]:
        ellipse(image, x + 5 * direction, y + dy, 6, 2, leaf)
        line(image, x, y + dy + 2, x + direction * 8, y + dy - 1, 13)


def doorway(image, left, warm):
    x = 0 if left else 222
    wall, trim, glow = (10, 8, 7) if warm else (2, 4, 5)
    rect(image, x, 96, 18, 40, wall)
    rect(image, x + 3, 100, 12, 36, 1)
    rect(image, x + (0 if left else 15), 96, 3, 40, trim)
    rect(image, x, 96, 18, 3, trim)
    for yy in (103, 111, 119, 127):
        rect(image, x + (0 if left else 17), yy, 1, 4, glow)
    rect(image, x, 134, 18, 2, glow)


def room(warm):
    image = [[9 if warm else 1 for _ in range(240)] for _ in range(160)]
    wall, seam, trim = (10, 9, 8) if warm else (2, 1, 3)
    rect(image, 0, 19, 240, 117, wall)
    for y in range(25, 136, 13):
        line(image, 0, y, 239, y, seam)
        for x in range(8 + (y // 13 % 2) * 15, 240, 30):
            line(image, x, y - 11, x, y, seam)
    for x in (18, 214):
        rect(image, x, 22, 8, 114, trim)
        rect(image, x + 2, 24, 2, 110, 7 if warm else 4)
        rect(image, x - 2, 21, 12, 4, trim)
        rect(image, x - 2, 131, 12, 5, trim)
    # A broad, deeply inset arched window is the scene's quiet focal point.
    ellipse(image, 120, 68, 71, 42, trim)
    rect(image, 49, 68, 143, 42, trim)
    ellipse(image, 120, 68, 66, 37, 11 if warm else 1)
    rect(image, 54, 68, 133, 36, 11 if warm else 1)
    rect(image, 45, 107, 151, 5, 7 if warm else 4)
    rect(image, 42, 112, 157, 3, trim)
    if warm:
        # Distant terraced garden, individual stems, and low warm sunlight.
        ellipse(image, 155, 65, 15, 15, 8)
        ellipse(image, 155, 65, 11, 11, 7)
        for x, height in [(58, 9), (72, 14), (88, 8), (104, 13), (121, 9), (140, 15), (169, 11), (181, 15)]:
            rect(image, x, 104 - height, 9, height, 12)
            rect(image, x + 2, 104 - height, 3, 2, 13)
        for x, y in [(70, 61), (93, 43), (112, 82), (138, 47), (173, 83)]:
            rect(image, x, y, 2, 2, 13)
    else:
        ellipse(image, 119, 64, 19, 19, 5)
        ellipse(image, 127, 58, 17, 18, 1)
        for x, y in [(66, 67), (83, 52), (98, 83), (145, 47), (162, 64), (176, 85), (137, 96)]:
            star(image, x, y, 5)
        for x, y in [(73, 87), (95, 36), (144, 80), (164, 95), (113, 91), (179, 64)]:
            rect(image, x, y, 1, 1, 4)
    for x in (85, 119, 153):
        rect(image, x, 35 if x == 119 else 44, 2, 63 if x == 119 else 54, trim)
        rect(image, x + 2, 38 if x == 119 else 47, 1, 60 if x == 119 else 51, 7 if warm else 4)
    rect(image, 55, 81, 131, 2, trim)
    if warm:
        # Hanging baskets and an abundant bench, leaving the walking path clear.
        for x, y in [(42, 55), (198, 60)]:
            line(image, x, 22, x, y - 6, 8)
            ellipse(image, x, y, 11, 5, 12)
            ellipse(image, x + 7, y - 3, 6, 4, 13)
            rect(image, x - 8, y + 3, 17, 3, 14)
            rect(image, x - 5, y + 6, 11, 4, 10)
            line(image, x - 8, y + 3, x - 12, y + 17, 12)
        rect(image, 53, 127, 145, 4, 8)
        rect(image, 60, 131, 4, 5, 10)
        rect(image, 187, 131, 4, 5, 10)
        for x in (64, 92, 151, 179):
            pot(image, x, 113, 12 if x % 2 else 11)
        ellipse(image, 119, 114, 11, 8, 12)
        for x, y in [(111, 108), (120, 101), (128, 109)]:
            ellipse(image, x, y, 4, 4, 15)
            rect(image, x - 1, y - 1, 3, 3, 7)
        pot(image, 119, 117)
        lantern(image, 207, 86)
        doorway(image, True, False)
    else:
        # Brass telescope, chart cabinet, and a little globe.
        rect(image, 49, 121, 39, 14, 10)
        rect(image, 47, 118, 43, 4, 8)
        for x in (53, 64, 75):
            rect(image, x, 125, 8, 7, 9)
            rect(image, x + 3, 127, 2, 1, 7)
        line(image, 65, 116, 65, 100, 8)
        ellipse(image, 65, 105, 8, 8, 4)
        line(image, 59, 102, 70, 108, 5)
        line(image, 65, 97, 65, 111, 3)
        line(image, 163, 111, 149, 135, 8)
        line(image, 163, 111, 177, 135, 8)
        line(image, 163, 111, 163, 135, 7)
        for off in range(-5, 6):
            line(image, 148, 112 + off, 180, 92 + off, 8)
        line(image, 148, 107, 180, 87, 7)
        line(image, 180, 87, 185, 97, 5)
        line(image, 182, 88, 187, 98, 8)
        rect(image, 144, 108, 5, 8, 3)
        lantern(image, 37, 84)
        doorway(image, False, True)
    # Walkway: one continuous pale edge and staggered stonework, no collision.
    rect(image, 0, 136, 240, 3, 8 if warm else 4)
    rect(image, 0, 139, 240, 9, 10 if warm else 2)
    for x in range(0, 240, 24):
        rect(image, x, 140, 1, 8, 9 if warm else 1)
        rect(image, x + 3, 140, 17, 1, 8 if warm else 3)
    rect(image, 0, 148, 240, 12, 1)
    return image


def keeper():
    # A small original cloaked keeper; one complete 16x24 static pose.
    rows = [
        "0000000000000000", "0000011111000000", "0000122222100000",
        "0001233333210000", "0001233333210000", "0001235553210000",
        "0001231513210000", "0001235553210000", "0000127772100000",
        "0001237883210000", "0001227882210000", "0012222222221000",
        "0012233333221000", "0012233333221000", "0012233333221880",
        "0012233333221768", "0012233333221768", "0012223332221880",
        "0001222222210000", "0001111111110000", "0000110011000000",
        "0000110011000000", "0000110011000000", "0004440044400000",
    ]
    return [[int(ch, 16) for ch in row] for row in rows]


def tile_record(index, tile, columns):
    return {"id": index, "x": index % columns * 8, "y": index // columns * 8,
            "width": 8, "height": 8, "pixels": tile, "flags": 0}


def main():
    scenes = [room(False), room(True)]
    tiles = [[[0] * 8 for _ in range(8)]]
    lookup = {tuple([0] * 64): 0}
    cells = [0] * (60 * 20)
    for number, image in enumerate(scenes):
        for cy in range(20):
            for cx in range(30):
                tile = [row[cx * 8:cx * 8 + 8] for row in image[cy * 8:cy * 8 + 8]]
                key = tuple(value for row in tile for value in row)
                if key not in lookup:
                    # Mode 0 text owns shared-sheet IDs 480..511. Keep those
                    # editor-locked slots empty; backgrounds can use later IDs.
                    if len(tiles) == 480:
                        tiles.extend([[[0] * 8 for _ in range(8)] for _ in range(32)])
                    lookup[key] = len(tiles)
                    tiles.append(tile)
                cells[cy * 60 + number * 30 + cx] = lookup[key]
    if len(tiles) > 600:
        raise ValueError("the original scene must fit the editable 240x160 sheet")
    write(ASSETS / "spritesheet.json", {"width": 240, "height": 160,
          "sprites": [tile_record(i, tile, 30) for i, tile in enumerate(tiles)]})
    write(ASSETS / "map.json", {"width": 60, "height": 20, "cells": cells})
    hero, parts, objects = keeper(), [], []
    for cy in range(3):
        for cx in range(2):
            index = cy * 64 + cx
            tile = [row[cx * 8:cx * 8 + 8] for row in hero[cy * 8:cy * 8 + 8]]
            objects.append(tile_record(index, tile, 64))
            parts.append({"tile": index, "x": cx * 8, "y": cy * 8})
    write(ASSETS / "objsheet.json", {"width": 512, "height": 320, "sprites": objects})
    write(ASSETS / "poses.json", {"version": 1, "poses": [{"name": "lantern_keeper", "parts": parts}]})
    write(ASSETS / "palettes" / "lantern-rooms.json", {
          "name": "lantern-rooms", "colors": [{"r": r, "g": g, "b": b} for r, g, b in COLORS]})
    print(f"lantern-rooms: {len(tiles)} sheet slots including 32 reserved for text, one static pose, two BG0 rooms")


if __name__ == "__main__":
    main()
