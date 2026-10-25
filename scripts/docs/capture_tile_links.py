#!/usr/bin/env python3
"""Capture Windy Meadow's real animation and scrolling (requires Pillow)."""

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
    warmup, sample_every = 30, 4
    segments = [(120, 0, "still"), (60, 0x10, "right"),
                (60, 0x20, "left"), (120, 0, "still")]
    inputs = [keys for count, keys, _ in segments for _ in range(count)]
    frames, stationary = [], set()
    with subprocess.Popen([str(args.player.resolve()), str(args.rom.resolve())],
                          stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                          stderr=subprocess.DEVNULL) as player:
        try:
            if read_exact(player.stdout, 16)[:4] != b"GLUP":
                raise RuntimeError("unexpected player protocol")
            for tick in range(warmup + len(inputs)):
                keys = inputs[tick - warmup] if tick >= warmup else 0
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
                captured = tick - warmup
                if captured >= 0 and captured % sample_every == 0:
                    frame = Image.frombytes("RGBA", (240, 160), rgba).convert("RGB")
                    frames.append(frame)
                    if captured < segments[0][0]:
                        stationary.add(frame.crop((0, 112, 240, 144)).tobytes())
            player.stdin.write(bytes([3, 0, 0, 0]))
            player.stdin.flush()
            player.wait(timeout=10)
            if player.returncode:
                raise RuntimeError(f"player exited with status {player.returncode}")
        finally:
            if player.poll() is None:
                player.kill()
                player.wait()
    if len(stationary) < 3:
        raise RuntimeError("flower artwork did not animate with the camera still")
    colors = sorted({color for frame in frames for _, color in frame.getcolors(256)})
    if len(colors) > 256:
        raise RuntimeError("capture exceeds the GIF palette; refusing to alter colors")
    palette = Image.new("P", (1, 1))
    palette.putpalette([channel for color in colors for channel in color]
                       + [0] * (768 - 3 * len(colors)))
    # Focus the documentation preview on the flowers and lamps, retaining the
    # native controls line below them instead of the mostly empty sky.
    crop = (0, 104, 240, 160)
    encoded = []
    for frame in frames:
        cropped = frame.crop(crop)
        scaled = cropped.resize((cropped.width * 3, cropped.height * 3),
                                Image.Resampling.NEAREST)
        paletted = scaled.quantize(palette=palette, dither=Image.Dither.NONE)
        if paletted.convert("RGB").tobytes() != scaled.tobytes():
            raise RuntimeError("GIF palette changed captured pixels")
        encoded.append(paletted)
    durations = [10 * (round((i + 1) * sample_every * 100 / 59.7275)
                       - round(i * sample_every * 100 / 59.7275))
                 for i in range(len(encoded))]
    encoded[0].save(args.out / "play.gif", save_all=True,
                    append_images=encoded[1:], duration=durations,
                    loop=0, optimize=True, disposal=1)
    (args.out / "capture.json").write_text(json.dumps({
        "project": "examples/tile-links", "renderer": "project setting (mode0)",
        "rom_sha256": hashlib.sha256(args.rom.read_bytes()).hexdigest(),
        "player_sha256": hashlib.sha256(args.player.read_bytes()).hexdigest(),
        "sampled_frames": len(frames), "warmup_lcd_frames": warmup,
        "sample_every_lcd_frames": sample_every,
        "input_segments": [{"lcd_frames": count, "keys": keys, "action": action}
                           for count, keys, action in segments],
        "stationary_flower_images": len(stationary),
        "duration_ms": sum(durations), "loop": "forever",
        "crop_native_pixels": crop,
        "scale": "3x nearest neighbour", "palette": "exact captured colors",
        "audio": "omitted from GIF",
    }, indent=2) + "\n")
    print(f"Captured {len(frames)} frames; {len(stationary)} stationary flower images; "
          f"{sum(durations)} ms in {args.out / 'play.gif'}")


if __name__ == "__main__":
    main()
