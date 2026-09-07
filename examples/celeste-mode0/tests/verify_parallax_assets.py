#!/usr/bin/env python3
"""Verify the authored cloud migration against a frozen pre-parallax project.

Usage: python3 examples/celeste/tests/verify_parallax_assets.py --baseline SNAPSHOT
SNAPSHOT can be the original project directory or its parent snapshot directory.
MTILE_PARALLAX_BASELINE supplies the default when --baseline is omitted.
The generator runs only inside a temporary copy; authored assets are read-only.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

CLOUD_TILE = 45
TERRAIN_REMAP = {
    31: 22, 41: 23, 60: 26, 61: 27, 62: 28, 63: 29, 64: 30, 65: 31, 66: 32, 67: 33,
    71: 24, 91: 34, 92: 35, 93: 36, 94: 37, 95: 38, 96: 39, 101: 25,
    103: 40, 104: 41, 105: 42, 122: 43, 128: 44,
}


def read(path):
    return json.loads(path.read_text(encoding="utf-8"))


def remap_cells(cells):
    return [TERRAIN_REMAP.get(tile, tile) for tile in cells]


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", type=Path, default=os.environ.get("MTILE_PARALLAX_BASELINE"))
    parser.add_argument("--project", type=Path, default=Path(__file__).resolve().parents[1])
    args = parser.parse_args()
    if args.baseline is None:
        parser.error("pass --baseline or set MTILE_PARALLAX_BASELINE to a pre-parallax snapshot")
    baseline = args.baseline
    if (baseline / "project" / "assets").is_dir():
        baseline = baseline / "project"

    old_map = read(baseline / "assets" / "map.json")
    new_map = read(args.project / "assets" / "map.json")
    require((old_map["width"], old_map["height"]) == (30, 20), "baseline must be the original 30x20 room")
    require((new_map["width"], new_map["height"]) == (60, 20), "adapted map must be 60x20")
    old_layers = {layer["id"]: layer["cells"] for layer in old_map["layers"]}
    new_layers = {layer["id"]: layer["cells"] for layer in new_map["layers"]}
    require(set(new_layers) == {0, 1, 2, 3}, "adapted map must contain all four planes")
    require(all(len(cells) == 1200 for cells in new_layers.values()), "each adapted plane must contain 1200 cells")
    for y in range(20):
        require(new_layers[3][y * 60:y * 60 + 30] == remap_cells(old_layers[1][y * 30:(y + 1) * 30]),
                f"original terrain changed in row {y}")
        require(not any(new_layers[3][y * 60 + 30:(y + 1) * 60]), f"terrain extends beyond room in row {y}")
        for bg in range(3):
            require(new_layers[bg][y * 60:y * 60 + 30] == new_layers[bg][y * 60 + 30:(y + 1) * 60],
                    f"cloud BG{bg} does not repeat in row {y}")
    for bg in range(3):
        require(set(new_layers[bg]) == {0, CLOUD_TILE},
                f"BG{bg} must use only empty cells and cloud tile {CLOUD_TILE}")

    old_sheet = read(baseline / "assets" / "spritesheet.json")
    new_sheet = read(args.project / "assets" / "spritesheet.json")
    require((new_sheet["width"], new_sheet["height"]) == (240, 160), "spritesheet canvas must stay 240x160")
    old_sprites = {sprite["id"]: sprite for sprite in old_sheet["sprites"]}
    new_sprites = {sprite["id"]: sprite for sprite in new_sheet["sprites"]}
    for old_id, new_id in {**{i: i for i in range(1, 8)}, **TERRAIN_REMAP}.items():
        if old_id not in old_sprites:
            continue
        got = new_sprites.get(new_id)
        require(got is not None, f"packed sprite {new_id} missing for original {old_id}")
        require(got["pixels"] == old_sprites[old_id]["pixels"] and got["flags"] == old_sprites[old_id]["flags"],
                f"original sprite {old_id} pixels/flags changed at packed id {new_id}")
    cloud = new_sprites[CLOUD_TILE]
    require(cloud["pixels"] == [[206] * 8 for _ in range(8)] and cloud["flags"] == 0,
            "cloud tile must be solid color 206 with no gameplay flags")

    with tempfile.TemporaryDirectory(prefix="mtile-cloud-assets-") as temporary:
        scratch = Path(temporary)
        (scratch / "assets").mkdir()
        paths = []
        for name in ("map.json", "spritesheet.json"):
            target = scratch / "assets" / name
            shutil.copy2(args.project / "assets" / name, target)
            paths.append(target)
        shutil.copy2(args.project / "generate_cloud_layers.py", scratch / "generate_cloud_layers.py")
        before = [hashlib.sha256(path.read_bytes()).digest() for path in paths]
        subprocess.run([sys.executable, str(scratch / "generate_cloud_layers.py")], check=True)
        require(before == [hashlib.sha256(path.read_bytes()).digest() for path in paths],
                "generator is not idempotent for the committed authored motifs")

    print(f"PASS: 600 terrain cells, {len(old_sprites)} original sprites, one cloud tile, three duplicated motifs, generator idempotence")


if __name__ == "__main__":
    main()
