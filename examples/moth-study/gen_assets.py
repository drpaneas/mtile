#!/usr/bin/env python3
"""Paint Moth Study's original tile artwork using only Python's standard library.

The detailed painting is build-time art: Lua only draws a map and one pose/clip.
Run from any directory. Re-running replaces the example's authored assets.
"""
import json
import math
from pathlib import Path
import struct
import zlib

ROOT = Path(__file__).resolve().parent
HEX = [
    '162b2b', '162b2b', '29413d', '486257', '6d8770', '9bab87',
    'c7ccb0', 'e7dfbf', 'f7edcf', 'd9cba3', 'ba9d73', '89694e',
    '593d38', '8d4b3e', 'ba6747', 'df9560', 'edbc7f', 'ffdb9a',
    '7faaa0', 'b4d0b3', 'dbdfbd', 'a87c73', 'dba292', 'f3c8ad',
]
COLORS = [tuple(round(int(h[i:i+2], 16)*31/255) for i in (0, 2, 4)) for h in HEX]


class Canvas:
    def __init__(self, width, height, color=0):
        self.w, self.h = width, height
        self.p = [color]*(width*height)

    def dot(self, x, y, color):
        x, y = round(x), round(y)
        if 0 <= x < self.w and 0 <= y < self.h:
            self.p[y*self.w+x] = color

    def rect(self, x0, y0, x1, y1, color):
        for y in range(max(0, round(y0)), min(self.h, round(y1)+1)):
            for x in range(max(0, round(x0)), min(self.w, round(x1)+1)):
                self.dot(x, y, color)

    def line(self, x0, y0, x1, y1, color):
        count = max(abs(round(x1-x0)), abs(round(y1-y0)), 1)
        for i in range(count+1):
            self.dot(x0+(x1-x0)*i/count, y0+(y1-y0)*i/count, color)

    def poly(self, points, color):
        for y in range(max(0, math.floor(min(p[1] for p in points))), min(self.h, math.ceil(max(p[1] for p in points))+1)):
            xs = []
            for (x0, y0), (x1, y1) in zip(points, points[1:]+points[:1]):
                if y0 <= y < y1 or y1 <= y < y0:
                    xs.append(x0+(y-y0)*(x1-x0)/(y1-y0))
            xs.sort()
            for i in range(0, len(xs)-1, 2):
                self.rect(math.ceil(xs[i]), y, math.floor(xs[i+1]), y, color)

    def ellipse(self, x, y, rx, ry, color):
        for py in range(round(y-ry), round(y+ry)+1):
            fraction = (py-y)/max(ry, 1)
            if abs(fraction) <= 1:
                dx = math.floor(rx*math.sqrt(1-fraction*fraction))
                self.rect(x-dx, py, x+dx, py, color)

    def paste(self, other, x, y):
        for py in range(other.h):
            for px in range(other.w):
                color = other.p[py*other.w+px]
                if color:
                    self.dot(x+px, y+py, color)


# Original compact capitals, baked into the map rather than drawn by Lua.
FONT = {
'A':['010','101','111','101','101'], 'B':['110','101','110','101','110'],
'C':['011','100','100','100','011'], 'D':['110','101','101','101','110'],
'E':['111','100','110','100','111'], 'F':['111','100','110','100','100'],
'G':['011','100','101','101','011'], 'H':['101','101','111','101','101'],
'I':['111','010','010','010','111'], 'J':['001','001','001','101','010'],
'K':['101','101','110','101','101'], 'L':['100','100','100','100','111'],
'M':['10001','11011','10101','10001','10001'], 'N':['1001','1101','1011','1001','1001'],
'O':['010','101','101','101','010'], 'P':['110','101','110','100','100'],
'Q':['010','101','101','111','011'], 'R':['110','101','110','101','101'],
'S':['011','100','010','001','110'], 'T':['111','010','010','010','010'],
'U':['101','101','101','101','111'], 'V':['101','101','101','101','010'],
'W':['10001','10001','10101','11011','10001'], 'X':['101','101','010','101','101'],
'Y':['101','101','010','010','010'], 'Z':['111','001','010','100','111'],
' ':['000']*5, '/':['001','001','010','100','100'],
}


def lettering(canvas, message, x, y, color, scale=1, spacing=2):
    width = sum((len(FONT[ch][0])+spacing)*scale for ch in message)-spacing*scale
    if x is None:
        x = (canvas.w-width)//2
    for ch in message:
        for dy, row in enumerate(FONT[ch]):
            for dx, bit in enumerate(row):
                if bit == '1':
                    canvas.rect(x+dx*scale, y+dy*scale, x+(dx+1)*scale-1, y+(dy+1)*scale-1, color)
        x += (len(FONT[ch][0])+spacing)*scale


def sprig(c, x, y, direction, height):
    tip_x = x+direction*12
    c.line(x, y, tip_x, y-height, 4)
    c.line(x+1, y, tip_x+1, y-height, 6)
    for i in range(1, 6):
        py = y-i*height/6
        px = x+direction*i*2
        side = -1 if i%2 else 1
        ex = px+side*(9+i%3)
        ey = py-6
        c.poly([(px, py), (ex-3, ey+2), (ex, ey-3), (ex+2, ey+3)], 5)
        c.line(px, py, ex, ey, 3)
        c.dot(ex, ey-2, 6)
    c.ellipse(tip_x, y-height-3, 3, 5, 21)
    c.ellipse(tip_x-1, y-height-4, 2, 3, 22)
    c.dot(tip_x-1, y-height-5, 23)


def background():
    c = Canvas(240, 160, 1)
    # A printed botanical plate, with warm paper and a quiet dark cloth margin.
    c.rect(9, 7, 231, 153, 2)
    c.rect(7, 5, 230, 152, 9)
    c.rect(8, 6, 229, 151, 8)
    c.rect(11, 9, 226, 148, 7)
    c.rect(12, 10, 225, 147, 8)
    for y in range(12, 147):
        for x in range(13, 225):
            if (x*13+y*23)%227 == 0:
                c.dot(x, y, 7)
    # Fine corner registration marks and a single editorial hierarchy.
    for x, dx in [(17, 1), (220, -1)]:
        for y, dy in [(15, 1), (142, -1)]:
            c.line(x, y, x+dx*8, y, 10)
            c.line(x, y, x, y+dy*8, 10)
    lettering(c, 'THE QUIET GARDEN', None, 17, 4, spacing=2)
    lettering(c, 'MOTH STUDY', None, 29, 2, scale=2, spacing=1)
    c.line(78, 43, 161, 43, 9)
    c.ellipse(120, 79, 37, 30, 7)
    c.ellipse(120, 78, 35, 28, 8)
    # Unfilled moon diagram, fine guide points, and botanical marginalia.
    for a in range(0, 360, 4):
        angle = math.radians(a)
        c.dot(120+33*math.cos(angle), 79+29*math.sin(angle), 9)
    for x, y in [(78, 79), (162, 79), (120, 46), (120, 112)]:
        c.dot(x, y, 10)
        c.dot(x-1, y, 9)
        c.dot(x+1, y, 9)
    sprig(c, 52, 122, -1, 62)
    sprig(c, 185, 124, 1, 69)
    sprig(c, 64, 124, 1, 37)
    sprig(c, 173, 123, -1, 34)
    # Pressed coral petals at the foot of each stem.
    for x, y in [(47, 120), (56, 126), (182, 126), (194, 120)]:
        c.ellipse(x, y, 3, 2, 22)
        c.dot(x-1, y-1, 23)
    c.line(80, 122, 158, 122, 9)
    lettering(c, 'SPECIMEN II', None, 129, 3, spacing=2)
    lettering(c, 'EVENING COLLECTION', None, 139, 4, spacing=1)
    return c


def moth(width=1, lift=0):
    wings = Canvas(64, 64)
    # Forewings and scalloped hindwings; geometry is original to this example.
    left = [(30, 27), (23, 13), (12, 5), (5, 6), (3, 15), (6, 26), (16, 35), (28, 40)]
    right = [(34, 27), (41, 15), (52, 6), (59, 7), (61, 17), (57, 28), (46, 37), (35, 40)]
    hind_l = [(30, 33), (17, 29), (9, 35), (12, 45), (18, 50), (22, 48), (27, 50), (32, 39)]
    hind_r = [(34, 33), (48, 30), (55, 36), (52, 47), (46, 51), (42, 49), (37, 50), (32, 39)]
    for points in [hind_l, hind_r, left, right]:
        wings.poly(points, 12)
    for points in [[(29, 34), (18, 31), (12, 36), (15, 44), (20, 47), (25, 46), (30, 39)],
                   [(35, 34), (47, 32), (52, 37), (49, 45), (44, 48), (39, 46), (34, 39)]]:
        wings.poly(points, 18)
    for points in [[(29, 28), (21, 14), (11, 7), (6, 9), (6, 18), (10, 26), (20, 34), (29, 38)],
                   [(35, 28), (43, 16), (53, 8), (58, 10), (58, 20), (54, 27), (44, 35), (35, 38)]]:
        wings.poly(points, 14)
    # Honey-colored leading edges and cream etched veins.
    for ax, bx in [(8, 28), (56, 36)]:
        wings.line(ax, 10, bx, 33, 16)
        wings.line(ax+1, 12, bx, 34, 15)
        for py, ex in [(19, ax), (27, ax+3 if ax<32 else ax-3)]:
            wings.line(32, 35, ex, py, 17)
    for x, y in [(16, 24), (48, 25)]:
        wings.ellipse(x, y, 6, 6, 16)
        wings.ellipse(x, y, 5, 5, 12)
        wings.ellipse(x, y, 3, 4, 18)
        wings.ellipse(x, y, 2, 2, 2)
        wings.dot(x-1, y-2, 20)
    for x, y in [(21, 39), (44, 40)]:
        wings.ellipse(x, y, 4, 4, 20)
        wings.ellipse(x, y, 3, 3, 3)
        wings.dot(x-1, y-1, 19)
    for x, y in [(11, 11), (15, 14), (20, 18), (54, 12), (50, 16), (46, 19), (15, 40), (48, 42)]:
        wings.dot(x, y, 17)
    c = Canvas(64, 64)
    # Width projection makes a folded wing frame, keeping every pose's origin.
    for y in range(64):
        for x in range(64):
            color = wings.p[y*64+x]
            if color:
                c.dot(32+(x-32)*width, y+lift, color)
    # The right-pointing head and off-center cream mark make facing perceptible.
    c.poly([(31, 23), (35, 24), (36, 34), (33, 45), (31, 46), (29, 37), (29, 29)], 12)
    c.line(31, 27, 32, 43, 15)
    c.line(33, 29, 33, 40, 17)
    for y in [31, 35, 39]:
        c.line(30, y, 34, y, 11)
    c.ellipse(33, 24, 4, 4, 13)
    c.ellipse(34, 23, 3, 3, 16)
    c.dot(37, 22, 1)
    c.dot(36, 21, 20)
    c.line(31, 21, 26, 16, 12)
    c.line(35, 20, 40, 14, 12)
    c.dot(25, 16, 15)
    c.dot(41, 14, 17)
    c.line(30, 33, 25, 36, 12)
    c.line(35, 34, 40, 37, 12)
    return c


def tile(image, x, y):
    return tuple(tuple(image.p[(y+dy)*image.w+x+dx] for dx in range(8)) for dy in range(8))


def sprite(number, pixels, columns):
    return {'id': number, 'x': (number%columns)*8, 'y': (number//columns)*8,
            'width': 8, 'height': 8, 'pixels': pixels, 'flags': 0}


def write_json(path, data):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, separators=(',', ':'))+'\n')


def png(path, image, scale=1):
    def chunk(kind, data):
        return struct.pack('>I', len(data))+kind+data+struct.pack('>I', zlib.crc32(kind+data)&0xffffffff)
    raw = bytearray()
    for y in range(image.h):
        row = bytearray([0])
        for x in range(image.w):
            row.extend(bytes(round(v*255/31) for v in COLORS[image.p[y*image.w+x]])*scale)
        raw.extend(row*scale)
    data = b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR', struct.pack('>IIBBBBB', image.w*scale, image.h*scale, 8, 2, 0, 0, 0))
    path.write_bytes(data+chunk(b'IDAT', zlib.compress(raw, 9))+chunk(b'IEND', b''))


def main():
    bg = background()
    tiles = [tuple(tuple(0 for _ in range(8)) for _ in range(8))]
    indices = {tiles[0]: 0}
    cells = []
    for y in range(0, 160, 8):
        for x in range(0, 240, 8):
            pixels = tile(bg, x, y)
            if pixels not in indices:
                indices[pixels] = len(tiles)
                tiles.append(pixels)
            cells.append(indices[pixels])
    assert len(tiles) <= 600
    assets = ROOT/'assets'
    write_json(assets/'spritesheet.json', {'width': 240, 'height': 160,
               'sprites': [sprite(i, p, 30) for i, p in enumerate(tiles)]})
    write_json(assets/'map.json', {'width': 30, 'height': 20, 'cells': cells})
    frames = [moth(), moth(.94, 1), moth(), moth(.68, -1), moth(.32, -2), moth(.68, -1)]
    sheet = Canvas(512, 320)
    poses, objects = [], []
    for frame_id, frame in enumerate(frames):
        ox, oy = frame_id*72, 0
        sheet.paste(frame, ox, oy)
        parts = []
        for y in range(0, 64, 8):
            for x in range(0, 64, 8):
                number = ((oy+y)//8)*64+(ox+x)//8
                pixels = tile(sheet, ox+x, oy+y)
                objects.append(sprite(number, pixels, 64))
                parts.append({'tile': number, 'x': x, 'y': y})
        poses.append({'name': ['rest', 'breathe', 'open', 'half', 'folded', 'half-return'][frame_id], 'parts': parts})
    write_json(assets/'objsheet.json', {'width': 512, 'height': 320, 'sprites': objects})
    write_json(assets/'poses.json', {'version': 1, 'poses': poses})
    write_json(assets/'animation.json', {'version': 1, 'pages': [
        {'name': 'Moth studies', 'x': 0, 'y': 0, 'width': 512, 'height': 320}], 'clips': [
        {'name': 'idle', 'frames': [0, 1], 'fps': 2, 'loop': True},
        {'name': 'flutter', 'frames': [2, 3, 4, 5], 'fps': 10, 'loop': True}]})
    palette = COLORS+[(0, 0, 0)]*(256-len(COLORS))
    write_json(assets/'palettes'/'herbarium.json', {'name': 'herbarium', 'colors': [dict(zip('rgb', color)) for color in palette]})
    preview = background()
    preview.paste(frames[0], 88, 48)
    png(Path('/tmp/mtile-moth-study-art.png'), preview, 3)
    png(Path('/tmp/mtile-moth-frames.png'), sheet, 2)
    print(f'{len(tiles)} shared tiles, {len(poses)} poses, 2 clips')


if __name__ == '__main__':
    main()
