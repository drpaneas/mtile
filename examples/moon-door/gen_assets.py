#!/usr/bin/env python3
"""Paint Moon Door's original tile and pose assets. Standard library only."""
import json
import math
from pathlib import Path
import random
import struct
import zlib

ROOT = Path(__file__).resolve().parent
HEX = [
    "08181f", "08181f", "102831", "173b41", "245154", "356a61",
    "518574", "82a68b", "b0c8a2", "e4dec0", "fff0c9", "24383e",
    "394c4d", "59665c", "7c8069", "a59a77", "d0bc8c", "9f724f",
    "714e3d", "49382f", "d99f62", "f1c782", "6d365d", "a35279",
    "d17e91", "efb0aa", "30475c", "496381", "7d93a4", "a5c6cb",
    "183837", "fff0c9",
]


class Canvas:
    def __init__(self, w, h, color=0):
        self.w, self.h = w, h
        self.p = [color] * (w * h)

    def dot(self, x, y, color):
        x, y = int(x), int(y)
        if 0 <= x < self.w and 0 <= y < self.h:
            self.p[y * self.w + x] = color

    def rect(self, x0, y0, x1, y1, color):
        for y in range(max(0, int(y0)), min(self.h, int(y1) + 1)):
            for x in range(max(0, int(x0)), min(self.w, int(x1) + 1)):
                self.p[y * self.w + x] = color

    def line(self, x0, y0, x1, y1, color):
        steps = max(abs(int(x1 - x0)), abs(int(y1 - y0)), 1)
        for step in range(steps + 1):
            self.dot(round(x0 + (x1 - x0) * step / steps),
                     round(y0 + (y1 - y0) * step / steps), color)

    def ellipse(self, x, y, rx, ry, color):
        for py in range(int(y - ry), int(y + ry) + 1):
            d = (py - y) / max(ry, 1)
            if abs(d) <= 1:
                dx = int(rx * math.sqrt(1 - d * d))
                self.rect(x - dx, py, x + dx, py, color)


def fern(c, x, y, size, mirror=1):
    c.line(x, y, x + mirror * 4, y - size, 5)
    for j in range(3, size, 3):
        cx = x + mirror * j * 4 // size
        width = max(2, (size - j) // 3)
        c.line(cx, y - j, cx - width, y - j - 3, 6)
        c.line(cx, y - j, cx + width, y - j - 4, 5)
        c.dot(cx - width, y - j - 3, 7)


def blossom(c, x, y):
    c.line(x, y, x, y - 5, 6)
    c.dot(x - 1, y - 3, 7)
    c.rect(x - 2, y - 8, x + 2, y - 6, 23)
    c.rect(x - 1, y - 9, x + 1, y - 5, 24)
    c.dot(x, y - 7, 10)
    c.dot(x + 1, y - 8, 25)


def background():
    c = Canvas(240, 160, 1)
    rng = random.Random(4157)
    for y0, y1, color in [(21, 39, 2), (40, 62, 3), (63, 89, 4),
                           (90, 112, 3), (113, 159, 2)]:
        c.rect(0, y0, 239, y1, color)
    # Distant reed island, a slim crescent, and interrupted reflected light.
    c.ellipse(120, 36, 11, 11, 9)
    c.ellipse(124, 32, 10, 10, 2)
    for _ in range(62):
        x, y = rng.randrange(240), rng.randrange(18, 87)
        c.dot(x, y, 7 if y > 58 else 6)
    for x in range(0, 240, 3):
        top = 83 + round(8 * math.sin(x / 14))
        c.line(x, 105, x + 2, top, 3)
    for y in range(107, 144, 4):
        for x in range(rng.randrange(7), 240, 19):
            c.line(x, y, x + rng.randrange(3, 9), y, 4)
    # One light road draws the eye to the gate, behind the framing foliage.
    for y in range(116, 146):
        half = 18 + (y - 116) * 2
        c.rect(120 - half, y, 120 + half, y, 12 if y % 8 else 11)
        if y % 8 == 1:
            c.line(121 - half, y, 119 + half, y, 14)
        if y % 8 > 1:
            for x in range(120 - half + ((y // 8) % 2) * 16, 121 + half, 32):
                c.dot(x, y, 11)
    # Weathered masonry, with a real opening painted behind the pose.
    c.ellipse(120, 67, 33, 31, 11)
    c.rect(87, 67, 153, 122, 11)
    c.ellipse(120, 66, 30, 28, 14)
    c.rect(90, 66, 150, 121, 13)
    c.ellipse(120, 68, 25, 24, 16)
    c.rect(95, 68, 145, 120, 15)
    c.ellipse(120, 70, 20, 23, 18)
    c.rect(100, 70, 140, 119, 18)
    c.ellipse(120, 73, 16, 21, 26)
    c.rect(104, 73, 136, 119, 26)
    for y in range(76, 119):
        c.rect(105, y, 135, y, 27 if y < 92 else 4)
    c.ellipse(121, 76, 5, 5, 10)
    c.ellipse(123, 74, 4, 4, 27)
    for x, y in [(109, 67), (130, 72), (115, 85), (129, 83)]:
        c.dot(x, y, 10)
    for y in range(95, 118, 4):
        half = (y - 93) // 3
        c.line(120 - half, y, 121 + half, y, 8 if y < 109 else 7)
    for y in range(73, 121, 12):
        c.line(88, y, 99, y, 11)
        c.line(141, y, 152, y, 11)
        c.line(90, y + 1, 99, y + 1, 15)
        c.line(141, y + 1, 150, y + 1, 15)
    # Keystone and an engraved rising moon.
    c.rect(116, 36, 124, 44, 18)
    c.rect(117, 36, 123, 42, 20)
    c.ellipse(120, 39, 2, 2, 10)
    c.dot(121, 38, 18)
    c.rect(92, 120, 148, 123, 14)
    c.line(92, 120, 148, 120, 16)
    c.rect(87, 124, 153, 127, 12)
    c.line(87, 124, 153, 124, 15)
    # Old trees frame the opening rather than competing with it.
    for x, mirror in [(17, 1), (222, -1)]:
        c.line(x, 138, x + mirror * 8, 52, 1)
        c.line(x + mirror * 1, 135, x + mirror * 9, 52, 11)
        c.line(x + mirror * 7, 92, x + mirror * 34, 64, 11)
        c.line(x + mirror * 7, 74, x + mirror * 21, 40, 11)
        for branch in range(11):
            bx = x + mirror * (6 + branch * 5)
            by = 51 + branch * 2
            c.ellipse(bx, by, 13, 6, 3)
            c.ellipse(bx - mirror * 2, by - 2, 11, 3, 4)
            for j in range(4):
                c.line(bx + j * 3 - 4, by + 2,
                       bx + j * 3 - 6, by + 10 + (j % 2) * 6, 4)
    # Moss and flowers follow the architecture, not a uniform random scatter.
    for x in [91, 99, 143, 149]:
        for y in range(63 + (x % 4) * 2, 116, 7):
            c.dot(x, y, 6)
            c.line(x, y, x - 3, y + 2, 5)
    for x in range(0, 240, 7):
        if 72 < x < 166:
            continue
        y = 135 + rng.randrange(7)
        fern(c, x, y, rng.randrange(10, 23), -1 if x > 120 else 1)
    for x, y in [(40, 130), (49, 138), (57, 128), (67, 138),
                 (174, 134), (184, 128), (193, 139), (204, 132)]:
        blossom(c, x, y)
    for x, y in [(55, 74), (72, 103), (161, 88), (190, 68), (44, 111)]:
        c.dot(x, y, 21)
        c.dot(x + 1, y, 7)
    # Quiet ink bands reserve readable space for real runtime captions.
    c.rect(0, 0, 239, 15, 1)
    c.rect(0, 146, 239, 159, 1)
    c.line(8, 145, 231, 145, 4)
    return c


def door(frame):
    c = Canvas(48, 64)
    # Each leaf folds towards its hinge; the aperture belongs to the map.
    width = [16, 14, 10, 7, 4, 2][frame]
    for y in range(0, 64):
        # The top of each panel follows the arch painted in BG0.
        for x in range(8, 41):
            if y < 17 and ((x - 24) / 17) ** 2 + ((y - 17) / 17) ** 2 > 1:
                continue
            left = x < 8 + width
            right = x > 40 - width
            if left or right:
                edge = 8 + width - 1 if left else 41 - width
                color = 18 if (x - 8) % 5 else 19
                if x == edge:
                    color = 20
                c.dot(x, y, color)
                if y in (24, 49, 61):
                    c.dot(x, y, 17)
                if y in (25, 50) and x % 7 == 2:
                    c.dot(x, y, 21)
    for x in (8, 40):
        c.line(x, 19, x, 62, 20)
    if frame < 5:
        # Small crescent-shaped handles move with their panel, not game code.
        for x in (6 + width, 42 - width):
            c.ellipse(x, 38, 1, 3, 21)
            c.dot(x, 37, 10)
    return c


def sheet(canvas):
    tiles = []
    for ty in range(canvas.h // 8):
        for tx in range(canvas.w // 8):
            pixels = [canvas.p[(ty * 8 + y) * canvas.w + tx * 8:
                               (ty * 8 + y) * canvas.w + tx * 8 + 8] for y in range(8)]
            if (tx == 0 and ty == 0) or any(any(row) for row in pixels):
                tiles.append(dict(id=ty * (canvas.w // 8) + tx, x=tx * 8, y=ty * 8,
                                  width=8, height=8, pixels=pixels, flags=0))
    return dict(width=canvas.w, height=canvas.h, sprites=tiles)


def png(path, canvas, scale=3):
    def chunk(name, data):
        return struct.pack(">I", len(data)) + name + data + struct.pack(">I", zlib.crc32(name + data) & 0xffffffff)
    colors = [bytes(int(h[i:i + 2], 16) for i in (0, 2, 4)) for h in HEX]
    raw = b"".join((b"\0" + b"".join(colors[c] * scale for c in canvas.p[y * canvas.w:(y + 1) * canvas.w])) * scale
                   for y in range(canvas.h))
    path.write_bytes(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", canvas.w * scale, canvas.h * scale, 8, 2, 0, 0, 0))
                     + chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b""))


def main():
    painted = background()
    blank = (0,) * 64
    tiles, ids, cells = [blank], {blank: 0}, []
    for ty in range(20):
        for tx in range(30):
            tile = tuple(painted.p[(ty * 8 + y) * 240 + tx * 8 + x] for y in range(8) for x in range(8))
            if tile not in ids:
                ids[tile] = len(tiles)
                tiles.append(tile)
            cells.append(ids[tile])
    shared = Canvas(240, 160)
    for i, tile in enumerate(tiles):
        for y in range(8):
            for x in range(8):
                shared.dot((i % 30) * 8 + x, (i // 30) * 8 + y, tile[y * 8 + x])
    objects = Canvas(512, 320)
    poses = []
    for frame in range(6):
        art = door(frame)
        ox = frame * 48
        parts = []
        for ty in range(8):
            for tx in range(6):
                nonempty = False
                for y in range(8):
                    for x in range(8):
                        color = art.p[(ty * 8 + y) * 48 + tx * 8 + x]
                        objects.dot(ox + tx * 8 + x, ty * 8 + y, color)
                        nonempty |= color != 0
                if nonempty:
                    parts.append(dict(tile=ty * 64 + frame * 6 + tx, x=tx * 8, y=ty * 8))
        poses.append(dict(name=f"door_{frame}", parts=parts))
    colors = [dict(zip("rgb", [round(int(h[i:i + 2], 16) * 31 / 255) for i in (0, 2, 4)])) for h in HEX]
    colors.extend([dict(r=0, g=0, b=0) for _ in range(256 - len(colors))])
    assets = ROOT / "assets"
    (assets / "palettes").mkdir(parents=True, exist_ok=True)
    values = {
        "spritesheet.json": sheet(shared),
        "map.json": dict(version=2, width=30, height=20,
                         layers=[dict(id=i, cells=cells if i == 0 else [0] * 600) for i in range(4)]),
        "objsheet.json": sheet(objects),
        "poses.json": dict(version=1, poses=poses),
        "animation.json": dict(version=1, pages=[dict(name="Door leaves", x=0, y=0, width=512, height=320)],
                               clips=[dict(name="open", frames=list(range(6)), fps=8, loop=False)]),
        "palettes/moon-door.json": dict(name="moon-door", colors=colors),
    }
    for path, value in values.items():
        (assets / path).write_text(json.dumps(value, separators=(",", ":")) + "\n")
    # Offline art proof, explicitly not the native ROM capture in the README.
    png(Path("/tmp/mtile-moon-door-art.png"), painted)
    print(f"Painted {len(tiles)} shared tiles, 6 poses, 1 nonlooping clip")


if __name__ == "__main__":
    main()
