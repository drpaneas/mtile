#!/usr/bin/env python3
"""Compare tile helpers with original Celeste state and actual atlas pixels.

Run from any directory with Python 3 and Lua 5.3 or newer installed:
    python3 examples/celeste-mode0/tests/verify_tile_helpers.py

This checks Lua helper behavior and authored pixel placement. Native GBA
renderer/timing verification is a separate check; this does not emulate it.
"""

import json
from pathlib import Path
import shutil
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[3]
TESTS = Path(__file__).resolve().parent
LUA = shutil.which("lua")
if not LUA:
    raise SystemExit("Install Lua 5.3 or newer, with its lua executable on PATH.")

subprocess.run(
    [LUA, str(TESTS / "tile_helper_state_parity.lua"), str(ROOT)],
    check=True,
)

sheet = json.loads((TESTS.parent / "assets/spritesheet.json").read_text())
with tempfile.TemporaryDirectory(prefix="mtile-tile-helper-parity-") as work:
    atlas = Path(work) / "atlas.lua"
    entries = []
    for sprite in sheet["sprites"]:
        rows = ["{" + ",".join(map(str, row)) + "}" for row in sprite["pixels"]]
        entries.append(f'[{sprite["id"]}]={{' + ",".join(rows) + "}")
    atlas.write_text("return {\n" + ",\n".join(entries) + "\n}\n")
    subprocess.run(
        [LUA, str(TESTS / "tile_effect_placement_parity.lua"), str(ROOT), str(atlas)],
        check=True,
    )
