#!/usr/bin/env python3
"""Hit the first real World 1-1 power-up block and collect its mushroom."""

import argparse
import json
from pathlib import Path
import struct
import subprocess

import verify_native as native

BOOT = ((72, 0), (8, 8), (20, 0), (8, 16), (48, 0),
        (8, 64), (48, 0), (8, 1), (6, 0))
# Apply inputs to simulation ticks so a build's GBA scan alignment does
# not change the side of the first high block that Mario hits.
APPROACH = ((10, 14, 19), (14, 82, 18), (82, 84, 19))


def jump_ready(state):
    return (state["tick"] >= 104 and 220 <= state["player_x"] <= 223
            and state["player_y"] == 384 and state["player_vx"] == 40
            and state["player_vy"] == 0 and state["player_power"] == 0)


def input_at(tick, jump_tick):
    if jump_tick is None:
        if tick >= 84:
            return 18
        return next((keys for start, end, keys in APPROACH if start <= tick < end), 0)
    elapsed = tick - jump_tick
    if elapsed < 48:
        return 35
    if 150 <= elapsed < 174:
        return 34
    return 0


def replay(emulator, output):
    for count, mask in BOOT:
        for _ in range(count):
            _, _, _, state = emulator.step(mask)
    assert state["stage"] == 2 and state["course"] == 1, state
    jump_tick = None
    for _ in range(460):
        tick = state["tick"]
        if jump_tick is None:
            if jump_ready(state):
                jump_tick = tick
            elif tick > 124 or state["player_x"] > 223:
                raise ValueError(f"missed running-jump launch window: {state}")
        frame, _, _, state = emulator.step(input_at(tick, jump_tick))
        assert state["stage"] == 2 and state["course"] == 1, state
        if state["player_power"] == 1:
            assert jump_tick is not None and 150 < state["tick"] - jump_tick < 210 and state["player_x"] < 170, state
            acquired = dict(state, frame=frame, jump_tick=jump_tick)
            for _ in range(4):
                _, rgba, _, state = emulator.step(0)
            acquired["mario_sprite_pixels"] = native.mario_sprite_pixels(rgba, state, powered=True)
            if output:
                native.write_png(output, rgba)
            return acquired
    raise ValueError(f"first World 1-1 mushroom was never acquired: {state}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("rom", "elf", "helper"):
        parser.add_argument(name, type=Path)
    parser.add_argument("--png", type=Path)
    args = parser.parse_args()
    native.WATCHES = (*native.WATCHES, "player_power")
    address, offsets = native.read_game_layout(args.elf, "arm-none-eabi-nm", "llvm-dwarfdump")
    with subprocess.Popen([str(args.helper), str(args.rom)], stdin=subprocess.PIPE,
                          stdout=subprocess.PIPE, stderr=subprocess.DEVNULL) as process:
        emulator = native.Preview(process, address, offsets)
        try:
            print(json.dumps(replay(emulator, args.png), indent=2))
        finally:
            emulator.write(struct.pack("<BBH", 3, 0, 0))
            process.wait(timeout=3)


if __name__ == "__main__":
    main()
