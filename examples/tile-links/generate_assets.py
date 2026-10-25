#!/usr/bin/env python3
"""Regenerate the Windy Meadow assets.

Shared tiles hold the map art: ground, the cursor, and two marker tiles. The
Poses page (objsheet.json) holds each object painted upright, and every pose
is exactly what Sprite Lab's REGISTER POSE records for that selection. The
flower marker is linked to the "sway" clip and the lamp marker to the "lamp"
pose (assets/tilelinks.json), so map() draws those objects wherever the
markers are painted. Run from this directory: python3 generate_assets.py
"""
import json
import os
import random
import shutil

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", ".."))

SKY, DIRT, DIRT_DARK, GRASS, GRASS_DARK = 24, 35, 34, 44, 43
STEM, PETAL, PETAL_EDGE, CENTER = 44, 29, 28, 40
POST, POST_DARK, GLOW, FLAME = 9, 6, 39, 37
MARK_FLOWER, MARK_LAMP, CURSOR = 29, 39, 15

MAP_W, MAP_H = 64, 20
FLOWER_ROW, LAMP_ROW = 15, 14
FLOWER, LAMP = 4, 5  # shared tile IDs of the two markers


def tile(fn):
    return [[fn(x, y) for x in range(8)] for y in range(8)]


def flower(lean):
    """An 8x16 flower: a head on the top tile, a stem on the bottom tile."""
    def head(x, y):
        dx, dy = x - 3 - lean, y - 4
        if (dx, dy) == (0, 0):
            return CENTER
        if abs(dx) + abs(dy) == 1:
            return PETAL
        if abs(dx) + abs(dy) == 2 and dy >= -1:
            return PETAL_EDGE
        if y == 7 and x == 3 + lean:
            return STEM
        return 0

    def stem(x, y):
        bend = lean if y < 3 else 0
        if x == 3 + bend:
            return STEM
        if y in (3, 4) and x == (4 if lean <= 0 else 2):
            return STEM  # a leaf on the side away from the wind
        return 0
    return tile(head), tile(stem)


def lamp():
    """An 8x24 lamp: a glowing cap, then two post tiles."""
    def cap(x, y):
        if 2 <= x <= 5 and 3 <= y <= 6:
            return FLAME if 3 <= x <= 4 and 4 <= y <= 5 else GLOW
        if 1 <= x <= 6 and y in (2, 7):
            return POST_DARK
        return 0
    post = tile(lambda x, y: (POST if x == 3 else POST_DARK) if x in (3, 4) else 0)
    base = tile(lambda x, y: POST_DARK if (x in (3, 4) or (y >= 6 and 2 <= x <= 5)) else 0)
    return tile(cap), post, base


def capture_pose(name, sheet_cols, tiles, col, row, width, height):
    """Mirror Sprite Lab's REGISTER POSE: each painted cell of the selected
    rectangle becomes a part at its position inside the selection."""
    parts = []
    for dy in range(height):
        for dx in range(width):
            tile = (row + dy) * sheet_cols + col + dx
            if any(any(p != 0 for p in line) for line in tiles.get(tile, [[0]])):
                parts.append({"tile": tile, "x": dx * 8, "y": dy * 8})
    return {"name": name, "parts": parts}


def sprite_records(tiles, sheet_cols):
    return [{"id": i, "x": (i % sheet_cols) * 8, "y": (i // sheet_cols) * 8,
             "width": 8, "height": 8, "pixels": pixels, "flags": 0}
            for i, pixels in sorted(tiles.items())]


def main():
    # Shared tiles: map art only. The two markers are what Map Lab paints.
    shared = {
        1: tile(lambda x, y: (GRASS if y < 2 else GRASS_DARK if y < 3 else DIRT)),
        2: tile(lambda x, y: DIRT_DARK if (x * 3 + y * 5) % 11 == 0 else DIRT),
        3: tile(lambda x, y: CURSOR if (y == 0 or y == 7) and (x < 3 or x > 4) or (x == 0 or x == 7) and (y < 3 or y > 4) else 0),
        FLOWER: flower(0)[0],
        LAMP: tile(lambda x, y: MARK_LAMP if 2 <= x <= 5 and 2 <= y <= 5 else (POST_DARK if x in (3, 4) else 0)),
    }
    sheet = {"width": 240, "height": 160, "sprites": sprite_records(shared, 30)}

    # Poses page (OBJ sheet): each object painted upright, exactly as it
    # appears in the game. Flower frames are columns 0-2, the lamp column 3.
    obj_cols = 64
    obj = {}
    for col, lean in ((0, -1), (1, 0), (2, 1)):
        top, bottom = flower(lean)
        obj[col], obj[obj_cols + col] = top, bottom
    cap, post, base = lamp()
    obj[3], obj[obj_cols + 3], obj[2 * obj_cols + 3] = cap, post, base
    objsheet = {"width": 512, "height": 320, "sprites": sprite_records(obj, obj_cols)}

    poses = {"version": 1, "poses": [
        capture_pose("left", obj_cols, obj, 0, 0, 1, 2),
        capture_pose("still", obj_cols, obj, 1, 0, 1, 2),
        capture_pose("right", obj_cols, obj, 2, 0, 1, 2),
        capture_pose("lamp", obj_cols, obj, 3, 0, 1, 3),
    ]}
    animation = {"version": 1, "pages": [{"name": "OBJ 1", "x": 0, "y": 0, "width": 512, "height": 320}],
                 "clips": [{"name": "sway", "frames": [1, 0, 1, 2], "fps": 4, "loop": True}]}
    links = {"version": 1, "links": [{"tile": FLOWER, "animation": "sway"}, {"tile": LAMP, "pose": 3}]}

    cells = [0] * (MAP_W * MAP_H)
    for x in range(MAP_W):
        cells[17 * MAP_W + x] = 1
        cells[18 * MAP_W + x] = 2
        cells[19 * MAP_W + x] = 2
    rnd = random.Random(4)
    for x in range(1, MAP_W - 1):
        if x % 12 == 6:
            cells[LAMP_ROW * MAP_W + x] = LAMP
        elif rnd.random() < 0.45:
            cells[FLOWER_ROW * MAP_W + x] = FLOWER
    map_data = {"width": MAP_W, "height": MAP_H, "cells": cells}

    assets = os.path.join(HERE, "assets")
    os.makedirs(os.path.join(assets, "palettes"), exist_ok=True)
    shutil.copy(os.path.join(REPO, "examples", "hello", "assets", "palettes", "aurora.json"),
                os.path.join(assets, "palettes", "aurora.json"))
    for name, value in (("spritesheet.json", sheet), ("objsheet.json", objsheet), ("poses.json", poses),
                        ("animation.json", animation), ("tilelinks.json", links), ("map.json", map_data)):
        with open(os.path.join(assets, name), "w") as f:
            json.dump(value, f, indent=2)
            f.write("\n")
    with open(os.path.join(HERE, "project.json"), "w") as f:
        json.dump({"activePalette": "aurora", "renderer": "mode0"}, f, indent=2)
        f.write("\n")


if __name__ == "__main__":
    main()
