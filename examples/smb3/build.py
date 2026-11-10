#!/usr/bin/env python3
"""Import assets from a local SMB3 NES ROM, then build the native mTile GBA game."""

import argparse
import json
from pathlib import Path
import subprocess
import sys
import tempfile


PROJECT = Path(__file__).resolve().parent
sys.path.insert(0, str(PROJECT / "tools"))

from compose_course import write_courses
from extract_assets import extract, read_rom
from extract_levels import extract_courses
from extract_sound import extract_sound


def import_assets(rom_path):
    rom = read_rom(rom_path)
    data = extract_courses(rom)
    courses = [data["world1-1"], data["world1-2"]]

    with tempfile.TemporaryDirectory(prefix=".smb3-import-", dir=PROJECT) as work:
        work = Path(work)
        assets = work / "assets"
        extract(rom, courses, assets, include_status_bar=True, include_title=True, include_world_map=True)
        extract_sound(rom, assets / "audio")
        graphics = json.loads((assets / "tile-ids.json").read_text())
        write_courses(courses, graphics, work / "course_data.lua")

        for source in sorted(assets.rglob("*.json")):
            if source.name == "tile-ids.json":
                continue
            destination = PROJECT / source.relative_to(work)
            destination.parent.mkdir(parents=True, exist_ok=True)
            source.replace(destination)
        (work / "course_data.lua").replace(PROJECT / "course_data.lua")
    print(f"Imported World 1-1 and 1-2 into {PROJECT}", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("rom", type=Path, help="path to your NTSC Super Mario Bros. 3 NES ROM")
    parser.add_argument("--assets-only", action="store_true", help="prepare the editable project without running the GBA build")
    parser.add_argument("-o", "--output", type=Path, default=PROJECT / "game.gba")
    args = parser.parse_args()

    try:
        if not args.assets_only and args.output.resolve() == args.rom.resolve():
            raise ValueError("the GBA output must be different from the source ROM")
        import_assets(args.rom)
        if not args.assets_only:
            root = PROJECT.parents[1]
            subprocess.run(
                ["go", "run", "./cmd/mtile", "build", "-o", str(args.output.resolve()), str(PROJECT)],
                cwd=root,
                check=True,
            )
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"smb3: {error}\n")


if __name__ == "__main__":
    main()
