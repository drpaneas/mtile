import json, os, shutil, sys, random
REPO = "/Users/pgeorgia/gocode/src/github.com/drpaneas/mtile"
OUT = "/tmp/tlbench"

def tile(fn):
    return [[fn(x, y) for x in range(8)] for y in range(8)]

def flower_tiles(lean):
    # 8x16 flower: petals on top tile, stem on bottom tile, leaning by `lean`.
    top = tile(lambda x, y: 9 if abs(x - 3 - lean) + abs(y - 4) <= 2 and y < 7 else (12 if (x - 3 - lean, y - 4) == (0, 0) else 0))
    bottom = tile(lambda x, y: 11 if x == 3 + (lean if y < 3 else 0) else (11 if y == 4 and x in (2, 4) else 0))
    return top, bottom

def sheet():
    sprites = []
    def put(i, pixels):
        sprites.append({"id": i, "x": (i % 16) * 8, "y": (i // 16) * 8, "width": 8, "height": 8, "pixels": pixels, "flags": 0})
    put(1, tile(lambda x, y: 5 if y < 2 else 4))                       # ground
    for i, lean in ((2, -1), (4, 0), (6, 1)):
        top, bottom = flower_tiles(lean)
        put(i, top); put(i + 1, bottom)
    put(8, tile(lambda x, y: 14 if x in (0, 7) or y in (0, 7) else 0))  # marker: animation
    put(9, tile(lambda x, y: 13 if x in (0, 7) or y in (0, 7) else 0))  # marker: pose
    return {"width": 128, "height": 16, "sprites": sprites}

POSES = {"version": 1, "poses": [
    {"name": "flower_%d" % k, "parts": [{"tile": t, "x": 0, "y": 0}, {"tile": t + 1, "x": 0, "y": 8}]}
    for k, t in enumerate((2, 4, 6))]}
ANIM = {"version": 1, "pages": [], "clips": [{"name": "sway", "frames": [0, 1, 2, 1], "fps": 6, "loop": True}]}
LINKS = {"version": 1, "links": [{"tile": 8, "animation": "sway"}, {"tile": 9, "pose": 1}]}

def make_map(w, h, count, seed):
    rnd = random.Random(seed)
    cells = [0] * (w * h)
    for x in range(w):
        for y in range(h - 2, h):
            cells[y * w + x] = 1
    spots = [(x, y) for y in range(1, h - 3) for x in range(w)]
    rnd.shuffle(spots)
    for i, (x, y) in enumerate(sorted(spots[:count], key=lambda p: (p[1], p[0]))):
        cells[y * w + x] = 8 if i % 3 else 9
    return {"width": w, "height": h, "cells": cells}

def lua(w, h, scroll, oracle):
    cam_update = "cam = (cam + 2) %% %d" % (w * 8 - 240) if scroll else ""
    head = "cam = 0\n"
    init = ""
    draw_links = ""
    if oracle:
        head += "links = {}\n"
        init = """function _init()
  for y = 0, %d do
    for x = 0, %d do
      local t = mget(x, y)
      if t == 8 or t == 9 then
        add(links, {x * 8, y * 8, t})
        mset(x, y, 0)
      end
    end
  end
end
""" % (h - 1, w - 1)
        draw_links = """  for l in all(links) do
    if l[3] == 8 then anim("sway", l[1], l[2]) else pspr(1, l[1], l[2]) end
  end
"""
    return head + init + """function _update()
  %s
end

function _draw()
  cls(1)
  camera(cam, 0)
  map()
%s  camera()
end
""" % (cam_update, draw_links)

def project(name, w, h, count, scroll, seed=7):
    for variant in ("feature", "oracle"):
        d = os.path.join(OUT, name, variant)
        shutil.rmtree(d, ignore_errors=True)
        os.makedirs(os.path.join(d, "assets", "palettes"))
        shutil.copy(os.path.join(REPO, "examples/hello/assets/palettes/aurora.json"), os.path.join(d, "assets/palettes/aurora.json"))
        files = {"project.json": {"activePalette": "aurora", "renderer": "mode4"},
                 "assets/spritesheet.json": sheet(), "assets/poses.json": POSES,
                 "assets/animation.json": ANIM, "assets/map.json": make_map(w, h, count, seed)}
        if variant == "feature":
            files["assets/tilelinks.json"] = LINKS
        for path, value in files.items():
            with open(os.path.join(d, path), "w") as f:
                json.dump(value, f)
        with open(os.path.join(d, "game.lua"), "w") as f:
            f.write(lua(w, h, scroll, variant == "oracle"))

project("screen12", 30, 20, 12, False)
project("dense120", 30, 20, 120, False)
project("scroll200", 256, 20, 200, True)
project("stream250", 1024, 20, 250, True)
print("generated")
project("level1500", 1024, 20, 1500, True, seed=11)
