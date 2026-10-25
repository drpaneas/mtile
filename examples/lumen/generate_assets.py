#!/usr/bin/env python3
"""Rebuild Lumen's original pixel artwork. Python standard library only.

The painting is authored as four independently scrolling GBA tile planes.
Characters, effects and interface artwork live in named poses on the OBJ page;
registered clips handle playback without gameplay frame arithmetic.
Run from any directory. Optional previews are written into /tmp.
"""

import json
import math
from pathlib import Path
import random
import re
import struct
import zlib


ROOT = Path(__file__).resolve().parent
W, H = 480, 160
ROOMS = 12
ROOM_W = 240
WORLD_W = ROOMS * ROOM_W
SEED = 73461
SKY_BANDS = [(0, 18, 2), (19, 43, 3), (44, 67, 4), (68, 86, 5), (87, 105, 6), (106, 117, 7)]
WATER_BANDS = [(118, 122, 17), (123, 131, 16), (132, 144, 15), (145, 159, 14)]

# 0 is transparent. 30 and 31 are invariant UI ink/paper across dawn banks.
HEX = [
    "10192c", "10192c", "17223a", "212e48", "2e3b56", "454762",
    "5d536e", "7b647e", "a17d93", "c99aab", "e6b2b2", "f5c7b0",
    "ffdfbb", "fff1d2", "172f3b", "1c4247", "24585a", "31726c",
    "499187", "70b2a1", "a9c8b4", "31364f", "4c4b67", "6b647f",
    "938196", "c0a2aa", "403045", "674558", "a76a79", "e8a292",
    "0e1728", "fff1d2",
]
DAWN = [
    "10192c", "50425f", "6b536c", "87617a", "a36f87", "c38693",
    "d3999d", "dfa8a7", "e9b5af", "efc0b6", "f5cbbb", "ffdab9",
    "ffe7c8", "fff1d2", "35495a", "415e66", "4c7777", "608c83",
    "7ba494", "a3bcab", "ccd7bd", "62566f", "7c6781", "9c7e93",
    "c098a4", "dfb3b5", "675168", "976675", "ca8a91", "f4b49e",
    "0e1728", "fff1d2",
]


def rgb(h):
    return tuple(int(h[i:i + 2], 16) for i in (0, 2, 4))


BASE = [rgb(h) for h in HEX]


class Canvas:
    def __init__(self, w, h, c=0):
        self.w, self.h = w, h
        self.p = [c] * (w * h)

    def dot(self, x, y, c):
        x, y = int(x), int(y)
        if 0 <= x < self.w and 0 <= y < self.h:
            self.p[y * self.w + x] = c

    def rect(self, x0, y0, x1, y1, c):
        for y in range(max(0, int(y0)), min(self.h, int(y1) + 1)):
            lo, hi = max(0, int(x0)), min(self.w, int(x1) + 1)
            if hi > lo:
                self.p[y * self.w + lo:y * self.w + hi] = [c] * (hi - lo)

    def line(self, x0, y0, x1, y1, c):
        x0, y0, x1, y1 = map(int, (x0, y0, x1, y1))
        dx, dy = abs(x1 - x0), -abs(y1 - y0)
        sx, sy = (1 if x0 < x1 else -1), (1 if y0 < y1 else -1)
        err = dx + dy
        while True:
            self.dot(x0, y0, c)
            if x0 == x1 and y0 == y1:
                break
            e = 2 * err
            if e >= dy:
                err += dy
                x0 += sx
            if e <= dx:
                err += dx
                y0 += sy

    def poly(self, vertices, c):
        ymin = max(0, math.floor(min(y for _, y in vertices)))
        ymax = min(self.h - 1, math.ceil(max(y for _, y in vertices)))
        for y in range(ymin, ymax + 1):
            xs = []
            for i, (x0, y0) in enumerate(vertices):
                x1, y1 = vertices[(i + 1) % len(vertices)]
                if (y0 <= y < y1) or (y1 <= y < y0):
                    xs.append(x0 + (y - y0) * (x1 - x0) / (y1 - y0))
            xs.sort()
            for i in range(0, len(xs) - 1, 2):
                self.rect(math.ceil(xs[i]), y, math.floor(xs[i + 1]), y, c)

    def ellipse(self, cx, cy, rx, ry, c):
        for y in range(int(cy - ry), int(cy + ry) + 1):
            d = (y - cy) / max(ry, 0.1)
            if abs(d) <= 1:
                dx = int(rx * math.sqrt(max(0, 1 - d * d)))
                self.rect(int(cx) - dx, y, int(cx) + dx, y, c)

    def paste(self, other, ox, oy):
        for y in range(other.h):
            for x in range(other.w):
                c = other.p[y * other.w + x]
                if c:
                    self.dot(ox + x, oy + y, c)


def png(path, canvas, scale=1):
    """Small RGB PNG encoder, keeping the generator dependency-free."""
    def chunk(kind, data):
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data) & 0xffffffff)
    display_colors = BASE + [tuple(round(color[ch] * 255 / 31) for ch in "rgb")
                             for color in make_palette()["colors"][32:]]
    raw = bytearray()
    for y in range(canvas.h):
        row = bytearray([0])
        for x in range(canvas.w):
            row.extend(bytes(display_colors[canvas.p[y * canvas.w + x]]) * scale)
        raw.extend(row * scale)
    data = b"\x89PNG\r\n\x1a\n"
    data += chunk(b"IHDR", struct.pack(">IIBBBBB", canvas.w * scale, canvas.h * scale, 8, 2, 0, 0, 0))
    data += chunk(b"IDAT", zlib.compress(bytes(raw), 9)) + chunk(b"IEND", b"")
    path.write_bytes(data)


def star(c, x, y, bright=12):
    c.dot(x, y, bright)
    c.dot(x - 1, y, 7)
    c.dot(x + 1, y, 7)
    c.dot(x, y - 1, 7)
    c.dot(x, y + 1, 7)


def reed(c, x, y, height, side=1, tone=16):
    c.line(x, y, x + side, y - height, tone)
    c.line(x, y - height // 3, x - side * 3, y - height + 3, tone)
    c.line(x + side, y - height // 2, x + side * 4, y - height + 1, tone)
    if height > 12:
        c.rect(x + side, y - height - 3, x + side + 1, y - height + 1, 27)
        c.dot(x + side, y - height - 3, 28)


def flower(c, x, y, tone=9):
    c.line(x, y, x, y - 4, 17)
    c.dot(x - 1, y - 2, 18)
    c.dot(x + 1, y - 3, 18)
    c.dot(x, y - 6, tone)
    c.rect(x - 1, y - 5, x + 1, y - 4, tone)
    c.dot(x, y - 5, 13)


def willow(c, x, y, mirror=1, scale=1):
    """A gnarled tree with separated strands, never a solid canopy blob."""
    def pt(a, b):
        return x + mirror * a * scale, y + b * scale
    def shape(points, color):
        c.poly([pt(a, b) for a, b in points], color)
    shape([(-5, 0), (5, 0), (0, -27), (1, -48), (-3, -64), (-5, -52), (-5, -27)], 14)
    shape([(-2, -5), (0, -5), (-2, -31), (-1, -51), (-3, -57), (-3, -28)], 16)
    for branch in [[(-2, -36), (-18, -52), (-32, -54)], [(-2, -48), (13, -61), (27, -57)], [(-3, -55), (-10, -68), (-27, -66)]]:
        for i in range(len(branch) - 1):
            a, b = branch[i], branch[i + 1]
            c.line(*pt(*a), *pt(*b), 14)
            c.line(*pt(a[0], a[1] + 1), *pt(b[0], b[1] + 1), 15)
    canopy = [(-40, -59), (-32, -68), (-16, -72), (-3, -71), (10, -68), (27, -62), (34, -54), (16, -52), (1, -58), (-17, -57), (-30, -53)]
    shape(canopy, 15)
    shape([(-38, -59), (-30, -65), (-17, -68), (-5, -67), (11, -64), (24, -60), (30, -55), (13, -56), (-3, -61), (-22, -59)], 16)
    rng = random.Random(SEED + x)
    for n in range(36):
        dx = rng.randint(-38, 29)
        top = -60 - int(7 * math.cos(dx / 30)) + rng.randint(-1, 2)
        length = rng.randint(12, 27) + (7 if abs(dx) > 23 else 0)
        col = 17 if n % 4 else 18
        for d in range(length):
            if d % 5 != 4:
                px, py = pt(dx + int(math.sin(d / 13) * 2), top + d)
                c.dot(px, py, col if d < length - 5 else 16)
                if d % 4 == 1:
                    c.dot(px + mirror * scale, py, 16)
    c.line(*pt(-5, -2), *pt(-14, 1), 15)
    c.line(*pt(3, -1), *pt(12, 2), 15)


def stone(c, x0, y0, x1, y1, color=22):
    c.rect(x0, y0, x1, y1, color)
    c.line(x0, y0, x1 - 1, y0, 24)
    c.line(x0, y0, x0, y1 - 1, 23)
    c.line(x0 + 1, y1, x1, y1, 21)
    c.line(x1, y0 + 1, x1, y1, 21)


def panorama():
    layers = [Canvas(W, H, 2), Canvas(W, H), Canvas(W, H), Canvas(W, H)]
    c = layers[0]
    rng = random.Random(SEED)
    # Atmospheric bands are deliberately quiet: a sky the moth can breathe in.
    for top, bottom, color in SKY_BANDS:
        c.rect(0, top, W - 1, bottom, color)
    # A single cross in one tile makes the slowest sky plane's motion visible.
    # It naturally moves above the viewport as the dawn strip slides upward.
    c.dot(118, 27, 13)
    for x, y in [(117, 27), (119, 27), (118, 26), (118, 28)]:
        c.dot(x, y, 9)
    # Sky ornaments share BG1 with the distant shore and water. They stay
    # transparent around their pixels while the separate opaque sky warms.
    c = layers[1]
    # Distant clouds carry lilac and rose through the blue.
    for x, y, length in [(71, 49, 49), (119, 54, 39), (281, 61, 58), (362, 72, 75), (6, 78, 66), (149, 84, 72), (400, 42, 47)]:
        c.rect(x, y, x + length, y, 6)
        c.rect(x + 6, y - 1, x + length - 8, y - 1, 5)
        c.rect(x + 20, y + 1, x + length + 5, y + 1, 5)
    # The moon is a single clear peach focal point, with hand-cut crater shapes.
    moon_canvas = Canvas(W, H)
    c = moon_canvas
    c.ellipse(344, 38, 29, 29, 8)
    c.ellipse(343, 37, 27, 27, 10)
    c.ellipse(340, 35, 25, 25, 11)
    c.ellipse(337, 32, 20, 22, 12)
    c.poly([(350, 15), (358, 20), (362, 28), (363, 36), (360, 45), (354, 52), (346, 56), (351, 47), (353, 34), (348, 25)], 10)
    c.poly([(326, 29), (332, 25), (338, 27), (339, 31), (334, 34), (329, 33)], 11)
    c.poly([(332, 46), (335, 42), (341, 43), (343, 47), (338, 50)], 11)
    c.rect(344, 20, 347, 21, 13)
    c.rect(321, 35, 323, 39, 13)
    c.rect(328, 20, 331, 21, 13)
    layers[1].paste(moon_canvas, -128, 0)
    c = layers[1]
    # Stars are placed compositionally, then a handful of faint points.
    for x, y in [(91, 19), (137, 31), (188, 14), (276, 25), (401, 18), (442, 35), (163, 58), (305, 46), (62, 62), (388, 52)]:
        star(c, x, y)
    for _ in range(47):
        x, y = rng.randrange(8, W - 8), rng.randrange(8, 79)
        if (x - 216) ** 2 + (y - 38) ** 2 > 35 ** 2:
            c.dot(x, y, 8 if rng.randrange(4) else 10)
    # Three linked points suggest the constellation followed in the game.
    c.line(99, 35, 117, 25, 4)
    c.line(117, 25, 137, 31, 4)
    for x, y in [(99, 35), (117, 25)]:
        c.dot(x, y, 10)
    c = layers[1]
    # Far hills and a drowned settlement, all low contrast.
    c.poly([(0, 109), (0, 98), (24, 91), (41, 93), (70, 86), (95, 90), (119, 81), (143, 88), (174, 85), (193, 94), (227, 91), (250, 83), (276, 91), (303, 87), (329, 94), (360, 90), (386, 81), (413, 90), (446, 85), (480, 95), (480, 117), (0, 117)], 22)
    c.poly([(0, 109), (32, 103), (69, 106), (107, 96), (135, 103), (169, 99), (203, 107), (234, 99), (269, 105), (302, 100), (340, 107), (371, 102), (406, 98), (442, 105), (480, 98), (480, 119), (0, 119)], 21)
    for x, y, w, h in [(111, 97, 5, 9), (124, 94, 7, 13), (135, 99, 4, 8), (294, 96, 6, 11), (304, 99, 8, 8), (379, 94, 6, 13)]:
        c.rect(x, y, x + w, y + h, 21)
        c.poly([(x - 1, y), (x + w // 2, y - 4), (x + w + 1, y)], 21)
        c.dot(x + 2, y + 3, 7)
    # Flat water carries colored ribbons rather than a costly runtime effect.
    c.rect(0, 118, W - 1, H - 1, 14)
    c.rect(0, 118, W - 1, 122, 17)
    c.rect(0, 123, W - 1, 131, 16)
    c.rect(0, 132, W - 1, 144, 15)
    # A repeated 64px water vocabulary gives rich ribbons without filling the
    # four-background character block with mostly one-off ripple tiles.
    ripple_rng = random.Random(SEED + 99)
    for y in range(120, 158, 2):
        for _ in range(3):
            x = ripple_rng.randrange(64)
            length = ripple_rng.randrange(3, 10)
            tone = 18 if y < 125 else (17 if y < 139 else 16)
            for ox in range(-64, W, 64):
                c.rect(ox + x, y, ox + x + length, y, tone)
    # Moon's broken reflection: warm, sparse, widening toward the viewer.
    for y, span, col in [(119, 16, 9), (122, 23, 10), (125, 14, 11), (128, 29, 9), (131, 20, 10), (134, 34, 8), (138, 24, 9), (143, 40, 7), (149, 32, 6)]:
        x = 216 + rng.randrange(-4, 5)
        c.rect(x - span // 2, y, x + span // 2, y, col)
        c.rect(x + span // 2 + 4, y, x + span // 2 + 8, y, col - 1)
        if y < 135:
            c.rect(x - span // 2 - 8, y, x - span // 2 - 4, y, col - 1)
    c = layers[2]
    # Banks: stepped islands, hanging roots, and glancing rim light.
    islands = [([(0, 111), (16, 108), (37, 111), (53, 108), (66, 114), (96, 114), (104, 119), (104, 123), (77, 126), (54, 123), (24, 125), (0, 122)], 15),
               ([(193, 115), (211, 111), (253, 112), (270, 116), (280, 122), (259, 126), (223, 125), (201, 122)], 15),
               ([(386, 116), (400, 112), (431, 111), (454, 108), (480, 111), (480, 125), (450, 126), (423, 123), (395, 125), (383, 121)], 15)]
    for shape, color in islands:
        c.poly(shape, color)
        for i in range(len(shape) - 1):
            a, b = shape[i], shape[i + 1]
            if i < 5:
                c.line(*a, *b, 18)
    # Left garden, with low steps under lantern one.
    stone(c, 66, 115, 102, 118)
    stone(c, 72, 111, 94, 114, 23)
    stone(c, 77, 109, 89, 110, 24)
    c.line(69, 116, 100, 116, 25)
    c.line(80, 117, 80, 119, 21)
    # The central moon gate: asymmetrical ruins and an intact pointed opening.
    # Everything is painted behind the lantern, keeping its inner air quiet.
    c.poly([(208, 113), (208, 70), (215, 57), (224, 47), (239, 42), (255, 46), (269, 57), (273, 69), (273, 112), (261, 112), (261, 70), (253, 60), (241, 54), (230, 58), (221, 69), (221, 113)], 21)
    c.poly([(210, 111), (210, 70), (217, 58), (226, 50), (239, 45), (253, 49), (265, 59), (269, 70), (269, 110), (262, 110), (262, 69), (254, 59), (241, 52), (229, 56), (219, 68), (219, 111)], 23)
    c.line(211, 68, 218, 57, 25)
    c.line(218, 57, 228, 49, 25)
    c.line(228, 49, 239, 45, 25)
    c.line(240, 45, 252, 49, 24)
    c.line(252, 49, 264, 60, 24)
    c.line(264, 60, 268, 69, 24)
    c.line(211, 71, 211, 109, 24)
    c.line(219, 71, 219, 109, 22)
    c.line(263, 71, 263, 109, 22)
    c.line(269, 72, 269, 109, 21)
    # Voussoir cuts and a cracked keystone.
    for a, b in [((220, 55), (226, 60)), ((230, 48), (233, 55)), ((241, 45), (241, 53)), ((253, 49), (249, 57)), ((264, 60), (257, 64))]:
        c.line(*a, *b, 21)
    for yy in [77, 90, 103]:
        c.line(210, yy, 218, yy, 21)
        c.line(262, yy + 3, 270, yy + 3, 21)
        c.dot(212, yy + 1, 25)
    c.line(215, 79, 213, 84, 21)
    c.line(213, 84, 215, 90, 21)
    stone(c, 206, 110, 222, 114, 23)
    stone(c, 259, 111, 275, 115, 22)
    stone(c, 202, 115, 276, 119)
    c.line(203, 116, 274, 116, 24)
    c.line(225, 116, 225, 119, 21)
    c.line(253, 116, 253, 119, 21)
    # Diamond ornament and hanging lantern chain.
    c.poly([(241, 46), (244, 49), (241, 52), (238, 49)], 25)
    c.dot(241, 49, 11)
    c.line(240, 54, 240, 66, 22)
    c.dot(240, 59, 25)
    # Climbing jasmine curls around the old stone.
    c.line(208, 109, 216, 94, 16)
    c.line(216, 94, 212, 77, 16)
    c.line(212, 77, 218, 63, 16)
    c.line(218, 63, 230, 53, 16)
    for x, y in [(210, 103), (214, 95), (213, 86), (214, 76), (219, 63), (224, 58), (229, 54)]:
        c.rect(x - 2, y, x, y + 1, 18)
        c.dot(x + 1, y - 1, 19)
        if y % 3:
            c.dot(x - 1, y - 2, 10)
    # Right pavilion remnant. Fluted columns and a broken ornamental pediment.
    stone(c, 398, 113, 441, 117)
    stone(c, 407, 108, 432, 112, 23)
    stone(c, 440, 76, 450, 111, 22)
    c.rect(440, 78, 442, 109, 24)
    c.line(445, 81, 445, 108, 21)
    stone(c, 437, 72, 453, 76, 23)
    c.poly([(434, 71), (442, 66), (447, 67), (452, 69), (451, 72)], 24)
    c.line(435, 71, 451, 71, 25)
    stone(c, 393, 96, 400, 112, 22)
    c.poly([(391, 95), (393, 91), (396, 93), (399, 90), (403, 95)], 23)
    c.line(395, 98, 395, 110, 24)
    # Fallen obelisk on the left, the garden's former boundary.
    c.poly([(132, 112), (143, 111), (152, 91), (148, 86), (144, 89)], 21)
    c.poly([(134, 110), (139, 109), (148, 90), (146, 88)], 23)
    c.line(138, 110, 147, 91, 24)
    stone(c, 129, 112, 147, 116, 22)
    c.line(136, 102, 144, 104, 21)
    # Garden trees frame the playable air instead of obscuring it.
    willow(c, 29, 115)
    willow(c, 480, 113, -1)
    # A few reflected columns, broken into short horizontal notes.
    for x, width in [(212, 8), (264, 7), (444, 9), (28, 7)]:
        for yy in range(125, 141, 3):
            c.rect(x + ((yy // 3) % 3) - 1, yy, x + width, yy, 21 if yy < 133 else 15)
    c = layers[3]
    # Thin edge reeds and pink seedheads.
    for x, y, h in [(8, 120, 14), (45, 122, 12), (55, 120, 16), (58, 121, 10), (98, 119, 9), (191, 120, 11), (198, 119, 15), (280, 122, 12), (284, 123, 9), (385, 122, 13), (459, 123, 15), (465, 122, 18)]:
        reed(c, x, y, h, -1 if x % 2 else 1)
    # Lilies have asymmetric notches, dark hulls, one-pixel cool rim light.
    for x, y, rx in [(110, 133, 7), (157, 148, 9), (305, 138, 9), (373, 151, 11), (40, 145, 8), (426, 136, 7), (237, 144, 6)]:
        c.ellipse(x, y + 1, rx, 2, 14)
        c.ellipse(x, y, rx, 2, 17)
        c.line(x - rx + 2, y - 1, x + rx - 2, y - 1, 19)
        c.line(x, y, x + 3, y + 2, 14)
        if x in [157, 305, 426]:
            c.poly([(x - 3, y - 1), (x - 4, y - 4), (x - 1, y - 3), (x, y - 6), (x + 2, y - 3), (x + 4, y - 4), (x + 3, y - 1)], 9)
            c.rect(x - 1, y - 3, x + 1, y - 1, 11)
            c.dot(x, y - 3, 13)
    # Foreground silhouettes create depth and a gently irregular lower frame.
    c.poly([(0, 151), (17, 150), (32, 154), (46, 153), (59, 157), (86, 155), (101, 159), (128, 157), (151, 160), (0, 160)], 1)
    c.poly([(331, 160), (355, 157), (374, 159), (400, 154), (419, 156), (438, 151), (457, 154), (480, 149), (480, 160)], 1)
    for x, y, h in [(4, 157, 17), (10, 155, 13), (23, 156, 20), (29, 159, 14), (78, 160, 12), (445, 159, 21), (454, 158, 17), (468, 156, 23), (476, 156, 13)]:
        reed(c, x, y, h, 1, 14)
    for x, y in [(34, 155), (52, 158), (91, 159), (434, 156), (460, 156)]:
        flower(c, x, y, 8)
    # A few translucent-looking hints are actual limited-palette pixels.
    for x, y in [(66, 99), (103, 105), (181, 106), (290, 111), (377, 105), (429, 94)]:
        c.dot(x, y, 19)
    return layers


def copy_tiles(dst, source, box, ox, oy, transparent=True):
    """Arrange authored eight-pixel building blocks without new tile patterns.

    Empty cells leave the existing composition alone. A nonempty cell replaces
    the complete destination cell, so overlapping ruins never silently create
    an extra character in the shared four-background tile bank.
    """
    x0, y0, x1, y1 = box
    assert all(n % 8 == 0 for n in (x0, y0, x1, y1, ox, oy))
    for sy in range(y0, y1, 8):
        for sx in range(x0, x1, 8):
            tile = [source.p[(sy + y) * source.w + sx + x]
                    for y in range(8) for x in range(8)]
            if transparent and not any(tile):
                continue
            for y in range(8):
                for x in range(8):
                    dst.dot(ox + sx - x0 + x, oy + sy - y0 + y, tile[y * 8 + x])


def long_panorama(original):
    """Twelve composed rooms made from the garden's original tile vocabulary.

    The first two rooms preserve the familiar entrance. Beyond them the same
    hand-painted stone and plants form an orchard, a drowned colonnade, open
    lagoons, jasmine gates, and the final shrine. Whole-cell rearrangements keep
    this much larger world within the same GBA background character block.
    """
    # Foreground travels faster than the playable garden and needs its own
    # generous right margin; otherwise the final room would lose its reeds.
    foreground_w = (math.ceil(((WORLD_W - ROOM_W) * 1.125 + ROOM_W) / 8) + 3) * 8
    far_w = (math.ceil(((WORLD_W - ROOM_W) / 4 + ROOM_W) / 8) + 1) * 8
    sky_w = math.ceil(((WORLD_W - ROOM_W) / 8 + ROOM_W) / 8) * 8
    layers = [Canvas(sky_w, H, 2), Canvas(far_w, H),
              Canvas(WORLD_W, H), Canvas(foreground_w, H)]
    for bg in (0, 1):
        # The slow sky travels at 1/8x and the distant shore at 1/4x. Their
        # complete finite paintings extend only as far as each camera can see.
        extent = layers[bg].w
        for x in range(0, extent, 8):
            sx = x % W
            if bg == 1 and x >= W and 184 <= sx < 248:
                # There is one moon. Repeat quiet sky/water around its later
                # position instead of copying the focal point into a second.
                sx = 8 + (sx - 184)
            copy_tiles(layers[bg], original[bg], (sx, 0, sx + 8, H), x, 0, False)
    copy_tiles(layers[2], original[2], (0, 0, W, H), 0, 0, False)
    copy_tiles(layers[3], original[3], (0, 0, W, H), 0, 0, False)

    # Each object box includes its mossy footing and, where appropriate,
    # broken reflections. All dimensions and placements are tile-aligned.
    objects = {
        "willow": (0, 32, 72, 144),
        "edge-willow": (416, 32, 480, 128),
        "gate": (200, 40, 280, 144),
        "pavilion": (384, 64, 456, 144),
        "obelisk": (128, 80, 160, 120),
        "steps": (64, 104, 112, 128),
        "left-bank": (0, 104, 112, 128),
        "island": (192, 104, 288, 128),
        "right-bank": (376, 104, 480, 128),
    }
    # room, block, local x, local y. Quiet water gaps separate the silhouettes;
    # architectural beats alternate with open air rather than repeating the
    # original two-room panorama six times.
    arrangement = [
        (2, "willow", 8, 32), (2, "steps", 120, 96),
        (2, "obelisk", 192, 88),
        (3, "pavilion", 8, 64), (3, "pavilion", 144, 56),
        (3, "island", 80, 112),
        (4, "willow", 0, 24), (4, "island", 120, 104),
        (4, "steps", 144, 104),
        (5, "gate", 136, 48), (5, "obelisk", 16, 88),
        (6, "left-bank", 0, 104), (6, "obelisk", 40, 80),
        (6, "steps", 136, 96), (6, "obelisk", 184, 72),
        (7, "willow", 0, 32), (7, "willow", 136, 40),
        (7, "island", 72, 112),
        (8, "steps", 24, 104), (8, "pavilion", 128, 56),
        (8, "island", 96, 104),
        (9, "gate", 136, 40),
        (10, "willow", 0, 32), (10, "obelisk", 120, 80),
        (10, "right-bank", 104, 104),
        (11, "gate", 136, 32), (11, "edge-willow", 224, 32),
        (11, "steps", 16, 104),
    ]
    for room, block, x, y in arrangement:
        copy_tiles(layers[2], original[2], objects[block], room * ROOM_W + x, y)

    # Reeds and lily islands have a separate rhythm from the stone. Choosing
    # different eight-pixel column sequences changes spacing without adding
    # mirrored tile patterns or covering the playable air.
    reed_bands = [(0, 104, 112, 160), (104, 104, 192, 160),
                  (192, 104, 288, 160), (288, 104, 384, 160),
                  (384, 104, 480, 160)]
    band_order = [2, 0, 4, 1, 3, 4, 2, 1, 0, 3, 1, 4, 0, 2, 3,
                  4, 1, 3, 0, 2, 4, 0, 1, 3, 2, 1, 4, 3, 0, 2]
    x = W
    for index in band_order:
        box = reed_bands[index]
        copy_tiles(layers[3], original[3], box, x, 104)
        x += box[2] - box[0]
        if x >= foreground_w:
            break
    assert x >= foreground_w
    return layers


def moth(frame):
    c = Canvas(16, 16)
    # Four silhouettes, each with the same thorax/pivot. The rose ocelli give
    # this tiny luna moth a recognizable character even on a moving screen.
    if frame == 0:
        left = [(7, 6), (4, 3), (1, 3), (0, 7), (3, 9), (1, 12), (4, 13), (7, 9)]
    elif frame == 1:
        left = [(7, 6), (4, 2), (2, 1), (1, 5), (4, 9), (3, 13), (5, 12), (7, 9)]
    elif frame == 2:
        left = [(7, 6), (5, 2), (5, 1), (4, 5), (6, 8), (5, 12), (7, 10)]
    else:
        left = [(7, 6), (4, 4), (2, 5), (2, 8), (4, 9), (3, 12), (5, 12), (7, 9)]
    right = [(15 - x, y) for x, y in left]
    c.poly(left, 18)
    c.poly(right, 18)
    c.poly([(x + (1 if x < 6 else 0), y - 1) for x, y in left], 20)
    c.poly([(x - (1 if x > 9 else 0), y - 1) for x, y in right], 20)
    c.line(6, 6, 3 if frame < 2 else 5, 4, 13)
    c.line(9, 6, 12 if frame < 2 else 10, 4, 13)
    if frame != 2:
        for xx in [4 if frame == 0 else 5, 11 if frame == 0 else 10]:
            c.rect(xx, 6, xx + 1, 7, 28)
            c.dot(xx, 6, 12)
    c.line(7, 6, 7, 11, 11)
    c.line(8, 6, 8, 10, 13)
    c.rect(7, 4, 8, 6, 31)
    c.dot(6, 3, 12)
    c.dot(9, 3, 12)
    c.dot(5, 2, 12)
    c.dot(10, 2, 12)
    c.dot(7, 12, 10)
    return c


def ember(frame):
    c = Canvas(8, 8)
    c.poly([(4, 0), (7, 4), (4, 7), (1, 4)], 27)
    c.poly([(4, 1), (6, 4), (4, 6), (2, 4)], 29 if frame % 2 else 11)
    c.rect(3, 3, 4, 4, 13)
    c.dot(4, 2, 31)
    if frame in [1, 3]:
        c.dot(0, 3, 10)
        c.dot(7, 6, 10)
    return c


def hazard(frame):
    c = Canvas(16, 16)
    c.poly([(1, 6), (4, 3), (7, 5), (10, 2), (14, 4), (13, 8), (15, 11), (11, 10), (8, 13), (5, 11), (1, 12), (3, 8)], 26)
    c.poly([(3, 6), (6, 5), (8, 6), (11, 4), (13, 6), (11, 8), (12, 10), (9, 9), (7, 11), (5, 9), (3, 10), (4, 8)], 21)
    c.line(3, 5 + frame, 5, 5 + frame, 7)
    c.line(10, 4 + frame, 12, 4 + frame, 7)
    c.dot(7, 7, 10)
    c.dot(10, 7, 10)
    return c


def lantern(lit):
    c = Canvas(16, 32)
    # A carved garden lantern: metal loop, flared roof, amber paper panes.
    c.rect(7, 0, 8, 2, 24)
    c.dot(6, 1, 24)
    c.dot(9, 1, 24)
    c.line(7, 3, 7, 5, 23)
    c.poly([(2, 8), (5, 5), (10, 5), (13, 8)], 24)
    c.line(3, 8, 12, 8, 25)
    c.rect(3, 9, 12, 10, 22)
    c.rect(4, 11, 11, 20, 28 if lit else 21)
    c.rect(5, 11, 10, 19, 11 if lit else 22)
    if lit:
        c.rect(6, 12, 9, 18, 12)
        c.rect(7, 13, 8, 17, 13)
        c.dot(7, 11, 31)
    else:
        c.line(5, 13, 10, 13, 23)
        c.dot(8, 16, 24)
    c.line(4, 10, 4, 20, 24)
    c.line(11, 10, 11, 20, 22)
    c.line(7, 10, 7, 20, 24 if not lit else 29)
    c.rect(3, 21, 12, 22, 23)
    c.line(4, 21, 11, 21, 25)
    c.rect(5, 23, 10, 24, 22)
    c.line(7, 25, 7, 28, 23)
    c.dot(6, 28, 28)
    c.dot(8, 28, 28)
    if lit:
        c.dot(7, 30, 11)
        c.dot(8, 29, 12)
    return c


def bloom(lit):
    c = Canvas(16, 16)
    c.line(7, 14, 7, 7, 17)
    c.poly([(7, 11), (2, 8), (3, 12), (7, 13)], 18)
    c.poly([(8, 11), (13, 7), (12, 12), (8, 13)], 19)
    if lit:
        c.poly([(7, 9), (3, 7), (2, 4), (5, 5), (7, 1), (9, 5), (13, 3), (12, 7), (9, 9)], 10)
        c.poly([(6, 7), (5, 4), (7, 5), (8, 2), (9, 6), (11, 5), (10, 8)], 12)
        c.rect(7, 6, 8, 8, 13)
    else:
        c.poly([(6, 8), (5, 5), (7, 2), (9, 4), (9, 7), (8, 9)], 27)
        c.line(7, 3, 7, 7, 28)
    return c


def wordmark():
    """Custom high-contrast carved capitals; each has an intentional flourish."""
    c = Canvas(128, 32)
    # Five 18px capitals with a two-pixel stroke and bracketed serifs.
    glyphs = {
        "L": ["1100000", "1100000", "1100000", "1100000", "1100000", "1100000", "1111111"],
        "U": ["1100011", "1100011", "1100011", "1100011", "1100011", "1100011", "0111110"],
        "M": ["1100011", "1110111", "1111111", "1101011", "1100011", "1100011", "1100011"],
        "E": ["1111111", "1100000", "1100000", "1111110", "1100000", "1100000", "1111111"],
        "N": ["1100011", "1110011", "1111011", "1101111", "1100111", "1100011", "1100011"],
    }
    origins = [7, 30, 53, 76, 99]
    for letter, ox in zip("LUMEN", origins):
        rows = glyphs[letter]
        for y, row in enumerate(rows):
            for x, bit in enumerate(row):
                if bit == "1":
                    c.rect(ox + x * 2, 5 + y * 3, ox + x * 2 + 1, 7 + y * 3, 25)
                    c.rect(ox + x * 2, 4 + y * 3, ox + x * 2 + 1, 6 + y * 3, 31)
        # Small bracket serifs, then fine upper-left highlights.
        c.rect(ox - 2, 4, ox + 5, 5, 31)
        if letter != "L":
            c.rect(ox + 9, 4, ox + 15, 5, 31)
        c.rect(ox - 2, 23, ox + 5, 24, 31)
        if letter in "UMN":
            c.rect(ox + 9, 23, ox + 15, 24, 31)
    # Broken orbit and diamond ornaments, aligned to optical center.
    c.line(19, 30, 51, 30, 8)
    c.line(75, 30, 108, 30, 8)
    c.poly([(63, 27), (67, 30), (63, 31), (60, 30)], 11)
    c.dot(63, 29, 31)
    c.dot(0, 13, 9)
    c.dot(125, 13, 9)
    # One offline navy pixel outline keeps the cream lettering legible when
    # the slower moon plane passes behind it. Original ink and ornaments win.
    original = Canvas(c.w, c.h)
    original.p = c.p.copy()
    for y in range(c.h):
        for x in range(c.w):
            if original.p[y * c.w + x] == 31:
                for dy in (-1, 0, 1):
                    for dx in (-1, 0, 1):
                        c.dot(x + dx, y + dy, 30)
    c.paste(original, 0, 0)
    return c


def make_palette():
    colors = []
    targets = [rgb(h) for h in DAWN]
    for stage in range(8):
        f = min(stage, 6) / 6
        for i, (a, b) in enumerate(zip(BASE, targets)):
            if i in [0, 30, 31]:
                b = a
            channels = [round((a[k] * (1 - f) + b[k] * f) * 31 / 255) for k in range(3)]
            colors.append(dict(zip("rgb", channels)))
    return {"name": "lumen", "colors": colors}


def text(c, value, x, y, color):
    """Use mTile's own 4x5 font when baking interface poses."""
    for ch in value:
        columns = FONT[ord(ch) - 32]
        for dx, bits in enumerate(columns):
            for dy in range(5):
                if bits & (1 << dy):
                    c.dot(x + dx, y + dy, color)
        x += 6


def outline_ink(c, color):
    """Bake a one-pixel eight-neighbor navy outline without runtime objects."""
    original = Canvas(c.w, c.h)
    original.p = c.p.copy()
    for y in range(c.h):
        for x in range(c.w):
            if original.p[y * c.w + x] == color:
                for dy in (-1, 0, 1):
                    for dx in (-1, 0, 1):
                        c.dot(x + dx, y + dy, 30)
    c.paste(original, 0, 0)
    return c


def label(value, color=31, shadow=True, width=None, height=8):
    c = Canvas(width or math.ceil((len(value) * 6) / 8) * 8, height)
    if shadow:
        text(c, value, 1, 1, 30)
    text(c, value, 0, 0, color)
    return c


def load_font():
    source = (ROOT.parent.parent / "runtime" / "text.go").read_text()
    source = source[source.index("var font4x5"):source.index("func glyphColumns")]
    columns = re.findall(r"\{(0x[0-9a-f]+), (0x[0-9a-f]+), (0x[0-9a-f]+), (0x[0-9a-f]+)\}", source)
    assert len(columns) == 95
    return [tuple(int(n, 16) for n in row) for row in columns]


FONT = load_font()


class PoseBank:
    def __init__(self):
        self.canvas = Canvas(512, 320)
        self.poses = []
        self.constants = []
        self.x, self.y, self.row_h = 0, 0, 0

    def add(self, symbol, name, c):
        width = math.ceil(c.w / 8) * 8
        height = math.ceil(c.h / 8) * 8
        if self.x + width > 512:
            self.x, self.y, self.row_h = 0, self.y + self.row_h, 0
        assert self.y + height <= self.canvas.h, "keep artwork on one editor OBJ page"
        self.canvas.paste(c, self.x, self.y)
        parts = []
        for ty in range(height // 8):
            for tx in range(width // 8):
                parts.append({"tile": (self.y // 8 + ty) * 64 + self.x // 8 + tx,
                              "x": tx * 8, "y": ty * 8})
        index = len(self.poses)
        self.poses.append({"name": name, "parts": parts})
        self.constants.append((symbol, index))
        self.x += width
        self.row_h = max(self.row_h, height)
        return index


def interface(bank):
    # The strip is reused eight times per screen edge. The resident bank shares
    # its identical opaque ink tiles; labels retain independent semantic poses.
    bank.add("UI_STRIP", "interface/ink-strip", Canvas(32, 16, 30))
    header = Canvas(240, 16, 30)
    text(header, "LUMEN", 9, 5, 31)
    text(header, "LANTERNS", 65, 5, 24)
    text(header, "/6", 126, 5, 24)
    header.line(8, 15, 231, 15, 21)
    bank.add("UI_HEADER", "interface/header", header)
    footer = Canvas(240, 16, 30)
    footer.line(8, 0, 231, 0, 21)
    text(footer, "EMBERS", 9, 4, 24)
    text(footer, "A DASH", 96, 4, 24)
    footer.rect(135, 6, 156, 9, 21)
    footer.rect(136, 7, 155, 8, 30)
    bank.add("UI_FOOTER", "interface/footer", footer)
    for digit in range(4):
        bank.add("UI_DIGIT" + str(digit), "interface/lantern-count-" + str(digit), label(str(digit), 12, False))
    for lit in (False, True):
        c = Canvas(8, 8)
        shade = 13 if lit else 21
        c.line(1, 2, 3, 5, shade)
        c.line(3, 5, 5, 2, shade)
        c.dot(3, 1, shade)
        bank.add("UI_WING_LIT" if lit else "UI_WING_DARK", "interface/wing-full" if lit else "interface/wing-empty", c)
    for lit in (False, True):
        c = Canvas(8, 8)
        c.ellipse(3, 3, 2, 2, 12 if lit else 21)
        bank.add("UI_EMBER_LIT" if lit else "UI_EMBER_DARK", "interface/ember-full" if lit else "interface/ember-empty", c)
    for stage in range(9):
        c = Canvas(24, 8)
        if stage:
            c.rect(0, 0, round(stage * 17 / 8), 0, 12 if stage == 8 else 19)
            if stage == 8:
                c.rect(0, 1, 17, 1, 12)
        bank.add("UI_DASH" + str(stage), "interface/dash-charge-" + str(stage), c)
    for symbol, name, value, color in [
        ("UI_LIGHT", "light-hint", "B LIGHT", 24),
        ("UI_LIGHT_READY", "light-ready", "B LIGHT", 12),
        ("UI_GATHER", "gather-hint", "3 TO LIGHT", 24),
        ("TITLE_SUBTITLE", "subtitle", "KEEPER OF THE TIDE", 13),
        ("TITLE_BEGIN", "begin", "A / START TO BEGIN", 31),
        ("TITLE_CONTROLS", "controls", "D-PAD FLY   A DASH   B LIGHT", 24),
        ("MESSAGE_GATHER", "gather", "GATHER THREE EMBERS", 31),
        ("MESSAGE_NEEDS", "needs", "A LANTERN NEEDS THREE EMBERS", 31),
        ("MESSAGE_REMEMBERS", "remembers", "THE GARDEN REMEMBERS", 12),
        ("MESSAGE_BRING", "bring", "BRING EMBERS TO A LANTERN", 31),
        ("MESSAGE_FIND", "find", "FIND A LANTERN. PRESS B.", 12),
        ("MESSAGE_DASH", "dash", "DASH THROUGH THE SHADOWS", 24),
        ("DAWN_RESTART", "another-dawn", "A / START  ANOTHER DAWN", 31),
        ("LANTERN_HINT", "button-hint", "B", 12),
    ]:
        if symbol in ("TITLE_CONTROLS", "DAWN_RESTART"):
            art = Canvas(240, 16, 30)
            text(art, value, 42 if symbol == "TITLE_CONTROLS" else 54,
                 3 if symbol == "TITLE_CONTROLS" else 4, color)
        elif symbol.startswith("MESSAGE_"):
            art = Canvas(math.ceil(len(value) * 6 / 8) * 8, 8)
            # One-pixel inset keeps every outline edge inside the existing cell.
            text(art, value, 1, 1, color)
            outline_ink(art, color)
        else:
            art = label(value, color)
        bank.add(symbol, "lettering/" + name, art)
    ornament = Canvas(32, 8)
    ornament.line(1, 0, 30, 0, 12)
    bank.add("UI_ORNAMENT", "interface/ornament", ornament)
    panel = Canvas(176, 64, 30)
    panel.rect(2, 2, 173, 61, 21)
    panel.rect(3, 3, 172, 60, 30)
    panel.line(71, 6, 104, 6, 12)
    bank.add("UI_PANEL", "interface/pause-panel", panel)
    pause = Canvas(176, 64)
    pause.p = panel.p.copy()
    text(pause, "A MOMENT OF STILLNESS", 25, 15, 31)
    text(pause, "A / START TO RESUME", 34, 39, 24)
    bank.add("UI_PAUSE", "lettering/pause", pause)
    rest = Canvas(176, 64)
    rest.p = panel.p.copy()
    text(rest, "REST YOUR WINGS", 43, 15, 31)
    text(rest, "A  RETURN TO THE LIGHT", 25, 35, 13)
    text(rest, "B  A NEW JOURNEY", 43, 50, 24)
    bank.add("UI_REST", "lettering/rest", rest)
    dawn = Canvas(144, 32)
    text(dawn, "THE GARDEN WAKES", 19, 1, 31)
    text(dawn, "YOU BROUGHT THE MORNING", 1, 13, 31)
    dawn.line(51, 24, 80, 24, 12)
    outline_ink(dawn, 31)
    bank.add("UI_DAWN", "lettering/dawn", dawn)
    # Append the new journey counts so editor-authored existing pose IDs remain
    # stable; animation clips still refer to the same saved frame numbers.
    for digit in range(4, 7):
        bank.add("UI_DIGIT" + str(digit), "interface/lantern-count-" + str(digit), label(str(digit), 12, False))


def sprites(canvas):
    result = []
    for ty in range(canvas.h // 8):
        for tx in range(canvas.w // 8):
            pixels = [[canvas.p[(ty * 8 + y) * canvas.w + tx * 8 + x] for x in range(8)] for y in range(8)]
            if tx == 0 and ty == 0 or any(any(row) for row in pixels):
                result.append({"id": ty * (canvas.w // 8) + tx, "x": tx * 8, "y": ty * 8,
                               "width": 8, "height": 8, "pixels": pixels, "flags": 0})
    return {"width": canvas.w, "height": canvas.h, "sprites": result}


def value_runs(values):
    """Return inclusive-start/exclusive-end runs of identical layout values."""
    runs = []
    start = 0
    for index in range(1, len(values) + 1):
        if index == len(values) or values[index] != values[start]:
            runs.append((start, index, values[start]))
            start = index
    return runs


def emit_lookup_tree(source, runs, argument, leaf, indent="  "):
    """Emit a balanced numeric range tree; every reachable leaf returns."""
    if len(runs) == 1:
        source.append(indent + "return " + leaf(runs[0][2]))
        return
    middle = len(runs) // 2
    boundary = runs[middle - 1][1]
    source.append(indent + "if " + argument + " < " + str(boundary) + " then")
    emit_lookup_tree(source, runs[:middle], argument, leaf, indent + "  ")
    source.append(indent + "else")
    emit_lookup_tree(source, runs[middle:], argument, leaf, indent + "  ")
    source.append(indent + "end")


def terrain_lua(canvases, layouts):
    """Immutable tile lookups with no Lua strings, tables, or runtime heap."""
    source = [
        "-- Generated by generate_assets.py. Immutable numeric tile lookups.",
        "-- These twelve rooms share one compact display map; landscape.lua",
        "-- updates incoming columns through mTile's existing mset() API.",
        "-- Balanced run trees keep artwork in ROM without strings or tables.",
    ]
    unique_rows, row_ids, plane_rows = [], {}, []
    for canvas, cells in zip(canvases, layouts):
        cols = canvas.w // 8
        ids = []
        for row in range(20):
            values = tuple(cells[row * cols:(row + 1) * cols])
            if values not in row_ids:
                row_ids[values] = len(unique_rows)
                unique_rows.append(values)
            ids.append(row_ids[values])
        plane_rows.append(ids)
    for index, values in enumerate(unique_rows):
        source.append("function terrain_row_" + str(index) + "(col)")
        emit_lookup_tree(source, value_runs(values), "col", str)
        source.append("end")
    for name, canvas, rows in zip(("far", "garden", "front"), canvases, plane_rows):
        cols = canvas.w // 8
        source.append("TERRAIN_" + name.upper() + "_COLS = " + str(cols))
        source.append("function terrain_" + name + "_at(col, row)")
        source.append("  if col < 0 or col >= " + str(cols) +
                      " or row < 0 or row >= 20 then return 0 end")
        emit_lookup_tree(source, value_runs(rows), "row",
                         lambda index: "terrain_row_" + str(index) + "(col)")
        source.append("end")
    return "\n".join(source) + "\n"


def main():
    layers = long_panorama(panorama())
    # Water on BG1 covers these rows. Keeping an opaque gradient underneath
    # prevents holes while independently scrolling the four finite planes.
    layers[0].rect(0, 118, layers[0].w - 1, H - 1, 7)
    # The slowest plane is a 256px repeating sky, duplicated side by side in
    # the hardware map. Its continuous 480px strip holds all seven dawn views.
    # The other three long paintings remain immutable numeric ROM lookups;
    # landscape.lua keeps only each camera's incoming columns in this same
    # compact display map. No ROM map streaming or heap is needed.
    sky = Canvas(256, 480)
    copy_tiles(sky, layers[0], (0, 0, 256, H), 0, 0, False)
    for y0, y1, color in [(160, 183, 39), (184, 207, 71), (208, 231, 103),
                          (232, 255, 135), (256, 279, 167), (280, 303, 199),
                          (304, 327, 200), (328, 351, 201), (352, 383, 202),
                          (384, 415, 203), (416, 479, 204)]:
        sky.rect(0, y0, sky.w - 1, y1, color)
    blank = (0,) * 64
    patterns, tiles = {blank: 0}, [blank]
    planes = [[0] * (64 * 60) for _ in range(4)]
    assert sum(len(plane) for plane in planes) < 16384

    def canvas_tiles(canvas):
        cells = []
        for ty in range(canvas.h // 8):
            for tx in range(canvas.w // 8):
                tile = tuple(canvas.p[(ty * 8 + y) * canvas.w + tx * 8 + x]
                             for y in range(8) for x in range(8))
                if tile not in patterns:
                    patterns[tile] = len(tiles)
                    tiles.append(tile)
                cells.append(patterns[tile])
        return cells

    sky_cells = canvas_tiles(sky)
    terrain_cells = [canvas_tiles(canvas) for canvas in layers[1:]]
    for ty in range(60):
        for tx in range(64):
            planes[0][ty * 64 + tx] = sky_cells[ty * 32 + tx % 32]
    for bg, (canvas, cells) in enumerate(zip(layers[1:], terrain_cells), 1):
        cols = canvas.w // 8
        for ty in range(20):
            for tx in range(64):
                tile = cells[ty * cols + tx % 32]
                planes[bg][ty * 64 + tx] = tile
                planes[bg][(ty + 20) * 64 + tx] = tile
    height = math.ceil(len(tiles) / 32) * 8
    assert 32 * (height // 8) <= 768, "four Mode 0 BGs share at most 768 cells"
    sheet = Canvas(256, height)
    for n, tile in enumerate(tiles):
        for y in range(8):
            for x in range(8):
                sheet.dot((n % 32) * 8 + x, (n // 32) * 8 + y, tile[y * 8 + x])

    bank = PoseBank()
    frames = {}
    for symbol, clip, count, generator in [
        ("MOTH", "moth-flight", 4, moth),
        ("EMBER", "ember-glow", 4, ember),
        ("SHADOW", "shadow-drift", 2, hazard),
    ]:
        frames[clip] = [bank.add(symbol + str(i), clip + "/" + str(i), generator(i)) for i in range(count)]
    bank.add("LANTERN_DARK", "lantern/unlit", lantern(False))
    lantern_frames = []
    for i in range(3):
        art = lantern(True)
        if i:
            art.rect(6, 12, 9, 18, 11 if i == 1 else 12)
            art.rect(7, 13 + i % 2, 8, 16 + i % 2, 13)
        lantern_frames.append(bank.add("LANTERN_LIT" + str(i), "lantern/flame-" + str(i), art))
    bank.add("BLOOM_BUD", "jasmine/bud", bloom(False))
    bloom_frames = []
    for i in range(3):
        art = bloom(False) if i == 0 else bloom(True)
        if i == 1:
            art.rect(0, 0, 15, 1, 0)
            art.rect(0, 0, 3, 6, 0)
            art.rect(12, 0, 15, 6, 0)
        bloom_frames.append(bank.add("BLOOM_OPEN" + str(i), "jasmine/opening-" + str(i), art))
    bank.add("WORDMARK", "lettering/lumen", wordmark())
    spark = Canvas(8, 8)
    star(spark, 3, 3, 31)
    spark.dot(3, 1, 12)
    spark.dot(3, 5, 12)
    bank.add("GLINT", "light/glint", spark)
    for symbol, name, color in [("MOTE_GOLD", "gold-mote", 12), ("MOTE_MINT", "mint-mote", 19),
                                 ("PARTICLE_WHITE", "white-particle", 31), ("PARTICLE_ROSE", "rose-particle", 28),
                                 ("PARTICLE_MINT", "mint-particle", 19), ("PARTICLE_GOLD", "gold-particle", 12)]:
        art = Canvas(8, 8)
        art.dot(3, 3, color)
        bank.add(symbol, "light/" + name, art)
    ripple = Canvas(8, 8)
    ripple.line(0, 0, 5, 0, 17)
    bank.add("RIPPLE", "water/ripple", ripple)
    reflection = Canvas(32, 32)
    for row in range(5):
        radius = 10 - row
        center = 16 + (row % 3) - 1
        reflection.line(center - radius, row * 5, center + radius, row * 5, 10 + row % 3)
    bank.add("REFLECTION", "water/lantern-reflection", reflection)
    interface(bank)

    clips = [
        {"name": "moth-flight", "frames": frames["moth-flight"], "fps": 10, "loop": True},
        {"name": "moth-dash", "frames": frames["moth-flight"], "fps": 18, "loop": True},
        {"name": "ember-glow", "frames": frames["ember-glow"], "fps": 5, "loop": True},
        {"name": "shadow-drift", "frames": frames["shadow-drift"], "fps": 4, "loop": True},
        {"name": "lantern-lit", "frames": lantern_frames, "fps": 6, "loop": True},
        {"name": "bloom-opening", "frames": bloom_frames, "fps": 6, "loop": False},
        {"name": "dash-recharge", "frames": list(range(dict(bank.constants)["UI_DASH0"],
                                                     dict(bank.constants)["UI_DASH8"] + 1)),
         "fps": 6, "loop": False},
    ]
    assets = ROOT / "assets"
    (assets / "palettes").mkdir(parents=True, exist_ok=True)
    output = [
        (assets / "spritesheet.json", sprites(sheet)),
        (assets / "map.json", {"version": 2, "width": 64, "height": 60,
                                "layers": [{"id": i, "cells": cells} for i, cells in enumerate(planes)]}),
        (assets / "objsheet.json", sprites(bank.canvas)),
        (assets / "poses.json", {"version": 1, "poses": bank.poses,
                                "variants": [{"name": "glow", "colors": [
                                    {"from": 20, "to": 13}, {"from": 18, "to": 19}, {"from": 28, "to": 29}]}]}),
        (assets / "animation.json", {"version": 1, "pages": [
            {"name": "Lumen actors and lettering", "x": 0, "y": 0, "width": 512, "height": 320}], "clips": clips}),
        (assets / "palettes" / "lumen.json", make_palette()),
    ]
    for path, obj in output:
        path.write_text(json.dumps(obj, separators=(",", ":")) + "\n")
    (ROOT / "src" / "art.lua").write_text(
        "-- Generated by generate_assets.py. Poses are authored on the OBJ page.\n" +
        "\n".join(symbol + " = " + str(index) for symbol, index in bank.constants) + "\n")
    (ROOT / "src" / "terrain.lua").write_text(terrain_lua(layers[1:], terrain_cells))

    # A camera-correct contact sheet makes the twelve composed rooms easy to
    # review. Directly flattening world coordinates would misrepresent the
    # independently moving sky, distant shore, and foreground.
    night, dawn = Canvas(ROOM_W * 4, H * 3), Canvas(ROOM_W * 4, H * 3)
    for room in range(ROOMS):
        scene_x = room * ROOM_W
        for target, sky_y in ((night, 0), (dawn, 288)):
            view = Canvas(ROOM_W, H)
            for y in range(H):
                for x in range(ROOM_W):
                    view.dot(x, y, sky.p[(sky_y + y) * sky.w + (scene_x // 8 + x) % sky.w])
            for canvas, speed in zip(layers[1:], (0.25, 1, 1.125)):
                camera = math.floor(scene_x * speed)
                for y in range(H):
                    for x in range(ROOM_W):
                        color = canvas.p[y * canvas.w + camera + x]
                        if color:
                            view.dot(x, y, color)
            target.paste(view, room % 4 * ROOM_W, room // 4 * H)
    png(Path("/tmp/lumen-panorama.png"), night, 2)
    png(Path("/tmp/lumen-12-rooms.png"), night, 2)
    png(Path("/tmp/lumen-dawn-panorama.png"), dawn, 2)
    png(Path("/tmp/lumen-obj-atlas.png"), bank.canvas, 2)
    print(f"Lumen: {len(tiles)} unique BG tiles; {32 * (height // 8)} allocated; four compact 64x60 planes.")
    print(f"World: {ROOMS} rooms, {WORLD_W}px; terrain column counts " +
          "/".join(str(canvas.w // 8) for canvas in layers[1:]) + ".")
    print(f"OBJ: {len(bank.poses)} poses, {len(clips)} named clips; editor canvas 512x320.")
    for symbol, index in bank.constants:
        print(f"{symbol}={index}")


if __name__ == "__main__":
    main()
