"""Generate the original clockwork surveyor's object tiles and animation poses."""

import json
from pathlib import Path


ASSETS = Path(__file__).resolve().parent / "assets"
# Each mark is a 2×2 block in the Aurora palette. Dots leave the backing clear.
COLORS = {"s": 120, "t": 109, "m": 110, "l": 112, "o": 162, "g": 161, "w": 39}
SHELL = (
    ".....stttts.....",
    "...stmmllmmts...",
    "..stmmmmmmllts..",
    ".stmmmsssssssls.",
    ".stmmsoggggggos.",
    ".stmmsoowwwggos.",
    ".stmmsoowwwggos.",
    ".stmmmsoogggoos.",
    "..stmmsssssssts.",
    "...stmmmmmllts..",
    "....sttttttts...",
    "...sooooooooos..",
    "...soggoogoogs..",
    "....sssssssss...",
)


def frame(bob, stride, lamp):
    pixels = [[0] * 40 for _ in range(48)]

    def block(x, y, color, width=1, height=1):
        for row in range(2 * y, 2 * (y + height)):
            pixels[row][2 * x:2 * (x + width)] = [COLORS[color]] * (2 * width)

    # The two spring legs counter-swing while the instrument housing bobs.
    for anchor, offset, shade in ((7, -stride, "t"), (13, stride, "m")):
        block(anchor, 17 + bob, shade, height=3 - bob)
        block(min(anchor, anchor + offset), 19, shade, abs(offset) + 1)
        block(anchor + offset, 20, "s")
        block(anchor + offset - 1, 21, "o", 4)
    block(7, 1 + bob, "t", height=4)
    block(6, bob, lamp, 3, 2)
    for y, row in enumerate(SHELL, 4 + bob):
        for x, mark in enumerate(row, 2):
            if mark != ".":
                block(x, y, mark)
    return pixels


def main():
    sheet = {"width": 512, "height": 320, "sprites": []}
    poses = {"version": 1, "poses": []}
    clips = {
        "idle": [(0, 0, "g"), (1, 0, "g"), (1, 0, "w"), (0, 0, "w")],
        "walk": [(1, -2, "g"), (0, -1, "g"), (0, 1, "w"),
                 (1, 2, "w"), (0, 0, "g")],
    }
    for clip, frames in clips.items():
        for index, motion in enumerate(frames):
            pixels = frame(*motion)
            origin = len(poses["poses"]) * 40
            parts = []
            for y in range(0, 48, 8):
                for x in range(0, 40, 8):
                    tile = [row[x:x + 8] for row in pixels[y:y + 8]]
                    if any(any(row) for row in tile):
                        tile_id = (y // 8) * (sheet["width"] // 8) + (origin + x) // 8
                        sheet["sprites"].append({
                            "id": tile_id, "x": origin + x, "y": y,
                            "width": 8, "height": 8, "pixels": tile, "flags": 0,
                        })
                        parts.append({"tile": tile_id, "x": x, "y": y})
            poses["poses"].append({"name": f"{clip}_{index}", "parts": parts})
    for name, data in (("objsheet.json", sheet), ("poses.json", poses)):
        (ASSETS / name).write_text(json.dumps(data, indent=2) + "\n")


if __name__ == "__main__":
    main()
