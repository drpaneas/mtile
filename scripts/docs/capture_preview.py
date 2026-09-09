#!/usr/bin/env python3
"""Capture a README preview from a real mtile-player ROM run (requires Pillow)."""

import argparse
import hashlib
import json
from pathlib import Path
import struct
import subprocess

from PIL import Image


def read_exact(stream, size):
    data = bytearray()
    while len(data) < size:
        chunk = stream.read(size - len(data))
        if not chunk:
            raise RuntimeError("player closed its output during capture")
        data.extend(chunk)
    return bytes(data)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--player", required=True, type=Path)
    parser.add_argument("--rom", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    args.out.mkdir(parents=True, exist_ok=True)
    frames = []
    with subprocess.Popen([str(args.player.resolve()), str(args.rom.resolve())],
                          stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                          stderr=subprocess.DEVNULL) as player:
        try:
            if read_exact(player.stdout, 16)[:4] != b"GLUP":
                raise RuntimeError("unexpected player protocol")
            for tick in range(270):
                player.stdin.write(bytes([1, 0, 0, 0]))
                player.stdin.flush()
                header = read_exact(player.stdout, 20)
                if header[:4] != b"FRAM":
                    raise RuntimeError("unexpected frame header")
                _, samples, overflow, telemetry = struct.unpack("<4I", header[4:])
                rgba = read_exact(player.stdout, 240 * 160 * 4)
                read_exact(player.stdout, samples * 4 + telemetry)
                if overflow:
                    raise RuntimeError("audio overflow during capture")
                if tick >= 30 and (tick - 30) % 4 == 0:
                    frames.append(Image.frombytes("RGBA", (240, 160), rgba)
                                  .convert("RGB").resize((720, 480), Image.Resampling.NEAREST))
            player.stdin.write(bytes([3, 0, 0, 0]))
            player.stdin.flush()
            player.wait(timeout=10)
            if player.returncode:
                raise RuntimeError(f"player exited with status {player.returncode}")
        finally:
            if player.poll() is None:
                player.kill()
                player.wait()
    frames[0].save(args.out / "background-layers.png")
    durations = [10 * (round((i + 1) * 400 / 59.73) - round(i * 400 / 59.73))
                 for i in range(len(frames))]
    frames[0].save(args.out / "background-layers.gif", save_all=True,
                   append_images=frames[1:], duration=durations, loop=0, optimize=True)
    (args.out / "capture.json").write_text(json.dumps({
        "project": "examples/background-layers", "renderer": "project setting (mode0)",
        "rom_sha256": hashlib.sha256(args.rom.read_bytes()).hexdigest(),
        "frames": len(frames), "warmup_lcd_frames": 30, "sample_every_lcd_frames": 4,
        "input": "none", "scale": "3x nearest neighbour", "audio": "omitted from GIF",
    }, indent=2) + "\n")
    print(f"Captured {len(frames)} frames in {args.out}")


if __name__ == "__main__":
    main()
