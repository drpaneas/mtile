#!/usr/bin/env python3
"""Capture Coin Quest from a real ROM in mtile-player (requires Pillow)."""

import argparse
import hashlib
import json
from pathlib import Path
import struct
import subprocess

from PIL import Image

from capture_preview import read_exact


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--player", type=Path, required=True)
    parser.add_argument("--rom", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    args.out.mkdir(parents=True, exist_ok=True)
    with subprocess.Popen([str(args.player.resolve()), str(args.rom.resolve())],
                          stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                          stderr=subprocess.DEVNULL) as player:
        try:
            if read_exact(player.stdout, 16)[:4] != b"GLUP":
                raise RuntimeError("unexpected player protocol")
            for tick in range(145):
                # Move right from the starting position to collect the first coin.
                keys = 0x10 if 30 <= tick < 118 else 0
                player.stdin.write(struct.pack("<BBH", 1, 0, keys))
                player.stdin.flush()
                header = read_exact(player.stdout, 20)
                if header[:4] != b"FRAM":
                    raise RuntimeError("unexpected frame header")
                _, samples, overflow, telemetry = struct.unpack("<4I", header[4:])
                rgba = read_exact(player.stdout, 240 * 160 * 4)
                read_exact(player.stdout, samples * 4 + telemetry)
                if overflow:
                    raise RuntimeError("audio overflow during capture")
            player.stdin.write(bytes([3, 0, 0, 0]))
            player.stdin.flush()
            player.wait(timeout=10)
            if player.returncode:
                raise RuntimeError(f"player exited with status {player.returncode}")
        finally:
            if player.poll() is None:
                player.kill()
                player.wait()
    Image.frombytes("RGBA", (240, 160), rgba).convert("RGB").resize(
        (720, 480), Image.Resampling.NEAREST).save(args.out / "preview.png")
    (args.out / "capture.json").write_text(json.dumps({
        "template": "coin-quest", "rom_sha256": hashlib.sha256(args.rom.read_bytes()).hexdigest(),
        "lcd_frames": 145, "input": "right on zero-based LCD frames 30 through 117",
        "scale": "3x nearest neighbour", "audio": "not included in still image",
    }, indent=2) + "\n")


if __name__ == "__main__":
    main()
