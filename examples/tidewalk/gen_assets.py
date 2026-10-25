#!/usr/bin/env python3
"""Rebuild Tidewalk's original, hand-composed pixel art; standard library only.

The canvases are asset authoring tools, not drawing code used by the game.
Run from any directory: python3 examples/tidewalk/gen_assets.py
Artwork and generator follow the repository's MIT license. No imported images.
"""

import json
import math
from pathlib import Path
import random
import struct
import zlib

ROOT = Path(__file__).resolve().parent
W, H = 480, 160
HEX = [
    "192f45", "bad9d6", "cee7df", "e7eee0", "fff4d9", "f7d4b4",
    "e5b7ad", "d6979d", "ac92a7", "7e889e", "667d94", "49788b",
    "367a88", "3e9295", "59aba3", "7dbeb0", "a6d0b9", "d9dfb6",
    "263c50", "38535e", "4f7470", "739382", "a8b692", "d3c29a",
    "9a785d", "715449", "493e40", "b66c64", "df8d79", "efb294",
    "182c40", "fff2d0",
]
RGB = [tuple(int(h[i:i + 2], 16) for i in (0, 2, 4)) for h in HEX]


class Canvas:
    def __init__(self, w, h, color=0):
        self.w, self.h, self.p = w, h, [color] * (w * h)

    def dot(self, x, y, color):
        x, y = int(x), int(y)
        if 0 <= x < self.w and 0 <= y < self.h:
            self.p[y * self.w + x] = color

    def rect(self, x0, y0, x1, y1, color):
        for y in range(max(0, int(y0)), min(self.h, int(y1) + 1)):
            lo, hi = max(0, int(x0)), min(self.w, int(x1) + 1)
            self.p[y * self.w + lo:y * self.w + hi] = [color] * max(0, hi - lo)

    def line(self, x0, y0, x1, y1, color):
        x0, y0, x1, y1 = map(int, (x0, y0, x1, y1))
        dx, dy, sx, sy = abs(x1 - x0), -abs(y1 - y0), 1 if x0 < x1 else -1, 1 if y0 < y1 else -1
        err = dx + dy
        while True:
            self.dot(x0, y0, color)
            if x0 == x1 and y0 == y1:
                break
            e = 2 * err
            if e >= dy:
                err += dy
                x0 += sx
            if e <= dx:
                err += dx
                y0 += sy

    def ellipse(self, x, y, rx, ry, color):
        for py in range(y - ry, y + ry + 1):
            half = int(rx * math.sqrt(max(0, 1 - ((py - y) / ry) ** 2)))
            self.rect(x - half, py, x + half, py, color)

    def poly(self, vertices, color):
        for y in range(max(0, min(p[1] for p in vertices)), min(self.h, max(p[1] for p in vertices) + 1)):
            xs = []
            for i, (ax, ay) in enumerate(vertices):
                bx, by = vertices[(i + 1) % len(vertices)]
                if (ay <= y < by) or (by <= y < ay):
                    xs.append(ax + (y - ay) * (bx - ax) / (by - ay))
            xs.sort()
            for i in range(0, len(xs) - 1, 2):
                self.rect(math.ceil(xs[i]), y, math.floor(xs[i + 1]), y, color)

    def paste(self, other, x, y):
        for py in range(other.h):
            for px in range(other.w):
                color = other.p[py * other.w + px]
                if color:
                    self.dot(x + px, y + py, color)


def png(path, canvas, scale=1):
    def chunk(kind, data):
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data) & 0xffffffff)
    raw = bytearray()
    # Preview the actual 5-bit GBA palette rather than a more saturated source.
    colors = [bytes(round(round(v * 31 / 255) * 255 / 31) for v in rgb) for rgb in RGB]
    for y in range(canvas.h):
        row = b"\0" + b"".join(colors[canvas.p[y * canvas.w + x]] * scale for x in range(canvas.w))
        raw.extend(row * scale)
    data = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", canvas.w * scale, canvas.h * scale, 8, 2, 0, 0, 0))
    path.write_bytes(data + chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b""))


def cloud(c, x, y, length):
    c.rect(x - length, y, x + length, y + 3, 6)
    for dx, rx, ry in [(-length // 2, length // 2, 4), (0, length // 2, 7), (length // 2, length // 3, 4)]:
        c.ellipse(x + dx, y - 1, rx, ry, 3)
    c.line(x - length + 2, y + 2, x + length - 2, y + 2, 5)
    c.line(x - length // 3, y - 6, x + length // 4, y - 6, 4)


def gull(c, x, y, color=31):
    c.line(x - 3, y, x, y + 2, color)
    c.line(x, y + 2, x + 3, y, color)


def grass(c, x, y):
    for dx, h in [(-4, 8), (-2, 5), (0, 11), (2, 8), (4, 5)]:
        c.line(x, y, x + dx, y - h, 20)
        c.dot(x + dx, y - h, 22)
    c.dot(x - 3, y - 10, 28)
    c.dot(x, y - 12, 31)


def pot(c, x, y):
    c.rect(x - 4, y - 5, x + 4, y - 4, 29)
    c.rect(x - 3, y - 3, x + 3, y, 27)
    c.line(x - 2, y - 3, x - 2, y - 1, 28)
    grass(c, x, y - 6)


def window(c, x, y, w=8, h=11):
    c.rect(x - 1, y - 1, x + w, y + h, 25)
    c.rect(x, y, x + w - 1, y + h - 1, 11)
    c.line(x + 1, y + 1, x + w - 2, y + 1, 15)
    c.line(x + w // 2, y, x + w // 2, y + h - 1, 4)
    c.line(x, y + h // 2, x + w - 1, y + h // 2, 4)
    c.rect(x - 2, y + h + 1, x + w + 1, y + h + 2, 3)


def cottage(c):
    c.rect(23, 88, 78, 126, 23)
    c.rect(25, 90, 75, 125, 3)
    c.rect(68, 90, 75, 125, 5)
    for y in range(95, 123, 6):
        c.line(26, y, 74, y, 2)
    c.poly([(18, 89), (50, 67), (83, 89)], 26)
    c.poly([(20, 87), (50, 68), (80, 87)], 11)
    for y in range(72, 88, 4):
        half = (y - 67) * 3 // 2
        c.line(50 - half, y, 50 + half, y, 13)
        for x in range(50 - half, 50 + half, 9):
            c.dot(x, y + 1, 10)
    c.line(19, 89, 82, 89, 4)
    c.rect(65, 69, 71, 81, 5)
    c.rect(64, 68, 72, 70, 3)
    window(c, 31, 99, 10, 12)
    window(c, 61, 99, 8, 12)
    c.rect(47, 101, 56, 125, 19)
    c.rect(48, 102, 54, 125, 13)
    c.rect(49, 104, 53, 112, 15)
    c.dot(53, 118, 31)
    c.rect(43, 94, 58, 97, 27)
    c.line(45, 95, 56, 95, 31)
    c.rect(22, 126, 79, 127, 25)
    pot(c, 18, 125)
    pot(c, 82, 125)


def lighthouse(c):
    c.poly([(409, 126), (414, 59), (434, 59), (441, 126)], 23)
    c.poly([(412, 125), (417, 60), (429, 60), (434, 125)], 3)
    c.poly([(429, 60), (433, 60), (439, 125), (433, 125)], 5)
    for top in (74, 98):
        c.poly([(416 - (top - 60) // 12, top), (434 + (top - 60) // 14, top), (436 + (top - 60) // 14, top + 9), (415 - (top - 60) // 12, top + 9)], 27)
        c.rect(418, top, 430, top + 8, 28)
        c.line(419, top + 1, 428, top + 1, 29)
    c.rect(414, 57, 435, 60, 26)
    c.rect(416, 44, 433, 56, 10)
    c.rect(418, 45, 431, 54, 17)
    c.rect(421, 46, 428, 53, 31)
    c.line(425, 44, 425, 55, 25)
    c.line(418, 44, 418, 55, 25)
    c.line(431, 44, 431, 55, 25)
    c.poly([(412, 43), (425, 33), (438, 43)], 26)
    c.poly([(415, 42), (425, 35), (435, 42)], 27)
    c.line(425, 29, 425, 34, 25)
    c.dot(425, 28, 31)
    c.rect(411, 55, 438, 56, 26)
    c.line(411, 51, 411, 56, 26)
    c.line(438, 51, 438, 56, 26)
    c.line(411, 51, 438, 51, 26)
    window(c, 421, 87, 5, 7)
    c.rect(418, 113, 429, 126, 25)
    c.rect(420, 115, 427, 126, 19)
    c.line(421, 116, 421, 125, 13)
    c.dot(426, 121, 31)
    c.rect(405, 127, 446, 129, 5)
    pot(c, 402, 126)
    pot(c, 448, 126)


def scenery():
    far = Canvas(W, H, 1)
    for y, tone in [(40, 2), (67, 5), (84, 6), (95, 9), (102, 11)]:
        far.rect(0, y, W - 1, H - 1, tone)
    far.ellipse(119, 43, 16, 16, 5)
    far.ellipse(119, 41, 14, 14, 4)
    cloud(far, 44, 35, 24)
    cloud(far, 194, 51, 33)
    cloud(far, 329, 29, 34)
    cloud(far, 443, 57, 25)
    # Two quiet headlands create obvious landmarks for the slower plane.
    far.poly([(145, 94), (162, 88), (170, 81), (184, 79), (199, 68), (209, 65), (221, 73), (238, 75), (252, 91), (273, 95)], 8)
    far.poly([(166, 94), (183, 87), (194, 78), (208, 75), (227, 84), (244, 90), (261, 94)], 9)
    far.poly([(305, 95), (324, 89), (344, 80), (355, 77), (366, 84), (389, 87), (402, 95)], 8)
    far.line(157, 96, 276, 96, 10)
    far.line(321, 97, 400, 97, 10)
    gull(far, 160, 40, 7)
    gull(far, 171, 44, 7)
    gull(far, 292, 60, 7)

    near = Canvas(W, H)
    near.rect(0, 0, W - 1, 14, 30)
    near.line(0, 14, W - 1, 14, 20)
    for y, tone in [(96, 12), (103, 13), (112, 14), (120, 13), (144, 11)]:
        near.rect(0, y, W - 1, H - 1, tone)
    for y in range(98, 128, 7):
        for x in range((y * 3) % 24, W, 32):
            near.line(x, y, x + 9, y, 15 if y < 113 else 16)
            near.line(x + 13, y + 3, x + 16, y + 3, 12)
    # A single uninterrupted walkway: scenery never changes movement rules.
    near.rect(0, 128, W - 1, 130, 23)
    near.line(0, 128, W - 1, 128, 31)
    near.rect(0, 131, W - 1, 139, 24)
    near.line(0, 139, W - 1, 139, 25)
    near.rect(0, 140, W - 1, 143, 26)
    for x in range(0, W, 16):
        near.line(x, 131, x, 138, 25)
        near.line(x + 2, 132, x + 2, 137, 23)
        near.dot(x + 12, 132, 26)
        near.dot(x + 12, 137, 26)
        near.line(x + 5, 135, x + 9, 135, 25)
    for x in range(10, W, 48):
        near.rect(x, 142, x + 5, 153, 25)
        near.line(x, 143, x, 152, 24)
        near.line(x + 8, 149, x + 21, 149, 13)
    cottage(near)
    lighthouse(near)
    # Foreground landmarks are sharp and detailed; distant landmarks are soft.
    for x in (111, 263, 361):
        near.rect(x, 104, x + 2, 127, 25)
        near.line(x, 105, x, 126, 23)
        near.rect(x - 3, 99, x + 5, 106, 26)
        near.rect(x - 2, 100, x + 4, 104, 17)
        near.rect(x, 100, x + 2, 103, 31)
        near.line(x - 4, 98, x + 6, 98, 25)
        near.dot(x + 1, 97, 23)
    for x in (151, 308):
        near.rect(x, 121, x + 29, 123, 24)
        near.rect(x + 3, 124, x + 5, 127, 25)
        near.rect(x + 24, 124, x + 26, 127, 25)
        near.rect(x + 2, 114, x + 27, 118, 23)
        near.line(x + 2, 116, x + 27, 116, 24)
    # A striped harbour marker is the first-screen destination.
    near.rect(210, 112, 215, 127, 3)
    near.rect(210, 117, 215, 121, 27)
    near.line(209, 111, 216, 111, 25)
    near.dot(212, 109, 31)
    for x in (93, 192, 284, 389, 463):
        pot(near, x, 126)
    near.rect(0, 151, W - 1, 159, 30)
    near.line(0, 151, W - 1, 151, 20)
    return far, near


def sailor():
    c = Canvas(16, 32)
    c.ellipse(8, 29, 6, 1, 25)
    c.rect(4, 25, 6, 28, 18)
    c.rect(9, 25, 11, 28, 18)
    c.rect(3, 28, 6, 29, 30)
    c.rect(9, 28, 12, 29, 30)
    c.poly([(5, 14), (10, 14), (13, 23), (11, 26), (4, 26), (2, 23)], 27)
    c.rect(5, 15, 10, 24, 28)
    c.line(5, 16, 5, 23, 29)
    c.line(9, 17, 9, 24, 27)
    c.rect(2, 20, 3, 23, 5)
    c.rect(12, 20, 13, 23, 5)
    c.rect(5, 10, 10, 14, 5)
    c.rect(5, 9, 11, 11, 4)
    c.rect(4, 8, 11, 10, 25)
    c.rect(4, 6, 11, 8, 3)
    c.rect(5, 5, 10, 5, 31)
    c.line(3, 9, 12, 9, 11)
    c.dot(10, 12, 30)
    c.dot(11, 13, 29)
    c.rect(6, 15, 9, 16, 4)
    return c


def write_assets(far, near, hero):
    # Reserve rows 0–3 for one easily addressed 16×32 sprite: spr(1, ..., 2, 4).
    tiles = [(0,) * 64] * 128
    lookup = {(0,) * 64: 0}
    planes = []
    for canvas in (far, near):
        cells = []
        for ty in range(20):
            for tx in range(60):
                tile = tuple(canvas.p[(ty * 8 + y) * W + tx * 8 + x] for y in range(8) for x in range(8))
                if tile not in lookup:
                    # Mode 0 text stamps glyphs into shared IDs 480..511.
                    # Keep artwork out of that reserved range in both renderers.
                    if len(tiles) == 480:
                        tiles.extend([(0,) * 64] * 32)
                    lookup[tile] = len(tiles)
                    tiles.append(tile)
                cells.append(lookup[tile])
        planes.append(cells)
    for ty in range(4):
        for tx in range(2):
            tiles[1 + tx + ty * 32] = tuple(hero.p[(ty * 8 + y) * 16 + tx * 8 + x] for y in range(8) for x in range(8))
    sheet = Canvas(256, math.ceil(len(tiles) / 32) * 8)
    for i, tile in enumerate(tiles):
        for y in range(8):
            for x in range(8):
                sheet.dot((i % 32) * 8 + x, (i // 32) * 8 + y, tile[y * 8 + x])
    sprites = []
    for ty in range(sheet.h // 8):
        for tx in range(32):
            sprites.append({"id": ty * 32 + tx, "x": tx * 8, "y": ty * 8, "width": 8, "height": 8,
                            "pixels": [sheet.p[(ty * 8 + y) * 256 + tx * 8:(ty * 8 + y) * 256 + tx * 8 + 8] for y in range(8)], "flags": 0})
    assets = ROOT / "assets"
    (assets / "palettes").mkdir(parents=True, exist_ok=True)
    palette = [{"r": round(r * 31 / 255), "g": round(g * 31 / 255), "b": round(b * 31 / 255)} for r, g, b in RGB]
    palette.extend([{"r": 0, "g": 0, "b": 0}] * (256 - len(palette)))
    for path, obj in [
        (assets / "spritesheet.json", {"width": sheet.w, "height": sheet.h, "sprites": sprites}),
        (assets / "map.json", {"version": 2, "width": 60, "height": 20,
                               "layers": [{"id": i, "cells": cells} for i, cells in enumerate(planes)] +
                                         [{"id": i, "cells": [0] * 1200} for i in (2, 3)]}),
        (assets / "palettes" / "tidewalk.json", {"name": "tidewalk", "colors": palette}),
    ]:
        path.write_text(json.dumps(obj, separators=(",", ":")) + "\n")
    print(f"Tidewalk: {len(lookup) - 1} unique background tiles, 8 sailor tiles; {sheet.w}×{sheet.h} sheet")


def preview(far, near, hero, scroll, x):
    c = Canvas(240, 160, 1)
    for y in range(H):
        for px in range(240):
            c.dot(px, y, far.p[y * W + px + scroll // 4])
            color = near.p[y * W + px + scroll]
            if color:
                c.dot(px, y, color)
    c.paste(hero, x - scroll, 98)
    return c


def main():
    far, near = scenery()
    hero = sailor()
    write_assets(far, near, hero)
    # Runtime captures own preview.png; regeneration must preserve that proof.
    png(Path("/tmp/tidewalk-art.png"), preview(far, near, hero, 0, 88), 3)
    contact = Canvas(480, 160)
    contact.paste(preview(far, near, hero, 0, 88), 0, 0)
    contact.paste(preview(far, near, hero, 240, 440), 240, 0)
    png(ROOT / "coast.png", contact, 2)


if __name__ == "__main__":
    main()
