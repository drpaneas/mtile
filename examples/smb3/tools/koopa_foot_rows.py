#!/usr/bin/env python3
"""Print each standing Koopa pose ID and its last opaque row."""
import json
from pathlib import Path


def main():
    assets = Path(__file__).resolve().parents[1] / "assets"
    with (assets / "objsheet.json").open() as file:
        tiles = {tile["id"]: tile["pixels"] for tile in json.load(file)["sprites"]}
    with (assets / "poses.json").open() as file:
        poses = json.load(file)["poses"]
    standing = {
        f"{enemy}_{phase}"
        for enemy in ("green_koopa", "red_koopa", "green_paratroopa")
        for phase in (1, 2)
    }
    for pose_id, pose in enumerate(poses):
        if pose["name"] in standing:
            foot_row = max(
                part["y"] + y
                for part in pose["parts"]
                for y, row in enumerate(tiles[part["tile"]])
                if any(row)
            )
            print(pose_id, foot_row)


if __name__ == "__main__":
    main()
