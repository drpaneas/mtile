#!/usr/bin/env python3
"""Bake Celeste's small software effects into editable, immutable sprite cells.

Run this while authoring the project, never while the game is running. Every
pixel is an integer palette index, so the sprite editor can change these frames
just like the original artwork. No image processing dependency is required.
Letters and numbers are engine Mode 0 print glyphs, not authored sheet cells.
"""

import argparse
import json
from pathlib import Path


PROJECT = Path(__file__).resolve().parent
PLAYER_USED_BASE = 8
HAIR_BASE = 15
HAIR_AFTER_DASH = 17
AMBIENT_BASE = 46
DEATH_BASE = 54
AMBIENT_CLIPPED_BASE = 50
RETIRED_UI_BEGIN = 323
RETIRED_UI_END = 416
SHEET_COLUMNS = 30
# Baked cells from older four-color dash palettes. The live game only uses
# default poses 1..7 and the spent-dash remap 8..14 / 17..18.
UNUSED_BAKED = set(range(240, 254)) | {263, 264, 265, 266}


def sprite(tile_id, pixels):
    return {"id": tile_id, "x": tile_id % SHEET_COLUMNS * 8, "y": tile_id // SHEET_COLUMNS * 8,
            "width": 8, "height": 8, "pixels": pixels, "flags": 0}


def blank():
    return [[0] * 8 for _ in range(8)]


def hair_circle(radius, color):
    # These authored masks match mtile's integer midpoint circles exactly.
    # The center stays at (2, 2) in both frames, keeping one draw anchor.
    rows = {1: ((1, 2, 2), (2, 1, 3), (3, 2, 2)),
            2: ((0, 1, 3), (1, 0, 4), (2, 0, 4), (3, 0, 4), (4, 1, 3))}
    pixels = blank()
    for y, left, right in rows[radius]:
        for x in range(left, right + 1):
            pixels[y][x] = color
    return pixels


def solid_rectangle(width, height, color):
    pixels = blank()
    for y in range(height):
        for x in range(width):
            pixels[y][x] = color
    return pixels


def generated_sprites(original):
    """Return generated cells; all original tile IDs stay untouched."""
    output = {}

    # Spent-dash poses replace hair color 158 with 214 so Mode 0 does not need
    # a draw-time palette swap on the player sprite.
    for frame in range(1, 8):
        pixels = [[214 if pixel == 158 else pixel for pixel in row]
                  for row in original[frame]["pixels"]]
        tile_id = PLAYER_USED_BASE + frame - 1
        output[tile_id] = sprite(tile_id, pixels)

    for base, color in ((HAIR_BASE, 158), (HAIR_AFTER_DASH, 214)):
        for radius in (1, 2):
            tile_id = base + radius - 1
            output[tile_id] = sprite(tile_id, hair_circle(radius, color))

    for color_index, color in enumerate((11, 14)):
        for size in (1, 2):
            tile_id = AMBIENT_BASE + color_index * 2 + size - 1
            output[tile_id] = sprite(tile_id, solid_rectangle(size, size, color))

    # A shrinking square centered on a fractional position can quantize to a
    # rectangle, not only a square. Include all 1..5 width/height combinations
    # to preserve the existing floor/clipping behavior during death effects.
    for color_index, color in enumerate((249, 62)):
        for width in range(1, 6):
            for height in range(1, 6):
                tile_id = DEATH_BASE + color_index * 25 + (width - 1) * 5 + height - 1
                output[tile_id] = sprite(tile_id, solid_rectangle(width, height, color))
    return output


def clipped_ambient_sprites():
    # A 2x2 ambient particle clipped against a screen edge still needs only
    # one OBJ, keeping the fixed 25-particle sprite budget exact.
    output = {}
    for variant, color in enumerate((11, 14)):
        for shape, (width, height) in enumerate(((1, 2), (2, 1))):
            tile_id = AMBIENT_CLIPPED_BASE + variant * 2 + shape
            output[tile_id] = sprite(tile_id, solid_rectangle(width, height, color))
    return output


def drop_retired_ui(sprites):
    return {tile_id: item for tile_id, item in sprites.items()
            if tile_id < RETIRED_UI_BEGIN or tile_id >= RETIRED_UI_END}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="check generated assets without changing files")
    args = parser.parse_args()
    path = PROJECT / "assets" / "spritesheet.json"
    manifest_path = PROJECT / "assets" / "completion-ui.json"
    sheet = json.loads(path.read_text())
    sprites = {item["id"]: item for item in sheet["sprites"]}
    leftover_ui = [tile_id for tile_id in sprites if RETIRED_UI_BEGIN <= tile_id < RETIRED_UI_END]
    leftover_baked = sorted(tile_id for tile_id in sprites if tile_id in UNUSED_BAKED)
    generated = generated_sprites(sprites)
    generated.update(clipped_ambient_sprites())

    def packed_ids(items):
        used = sorted(items)
        if not used or used[0] != 1 or used != list(range(1, used[-1] + 1)):
            raise SystemExit(f"spritesheet IDs must be compact 1..N, got {used[:8]}...{used[-4:]}")
        return used[-1]

    if args.check:
        changed = [tile_id for tile_id, item in generated.items() if sprites.get(tile_id) != item]
        if changed:
            raise SystemExit(f"Generated frames differ at IDs {changed}; rerun {Path(__file__).name}")
        if leftover_ui:
            raise SystemExit(f"Retired UI cells still present at IDs {leftover_ui}; rerun {Path(__file__).name}")
        if leftover_baked:
            raise SystemExit(f"Unused baked cells still present at IDs {leftover_baked}; rerun {Path(__file__).name}")
        if manifest_path.exists():
            raise SystemExit("completion-ui.json is leftover authored font/panel data; delete it")
        packed_ids(sprites)
        if sheet.get("width") != SHEET_COLUMNS * 8 or sheet.get("height") != 160:
            raise SystemExit("spritesheet canvas must stay 240x160")
        print(f"Verified {len(generated)} authored effect/player cells.")
        return
    sprites = drop_retired_ui(sprites)
    for tile_id in UNUSED_BAKED:
        sprites.pop(tile_id, None)
    sprites.update(generated)
    packed_ids(sprites)
    sheet["width"] = SHEET_COLUMNS * 8
    sheet["height"] = 160
    sheet["sprites"] = [sprites[key] for key in sorted(sprites)]
    path.write_text(json.dumps(sheet, indent=2) + "\n")
    if manifest_path.exists():
        manifest_path.unlink()
    print(f"Authored {len(generated)} effect/player cells, IDs {min(generated)}-{max(generated)}.")


if __name__ == "__main__":
    main()
