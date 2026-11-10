#!/usr/bin/env python3
"""Replay the generated SMB3 cartridge through the mTile player helper."""

import argparse
import hashlib
import json
import math
from pathlib import Path
import shutil
import struct
import subprocess
import sys
import tempfile
import threading
import zlib


WIDTH, HEIGHT, SAMPLE_RATE = 240, 160, 48_000
RGBA_SIZE = WIDTH * HEIGHT * 4
KEY_A, KEY_B, KEY_START, KEY_RIGHT, KEY_UP = 1, 2, 8, 16, 64
STEP, QUIT, READ_MEMORY = 1, 3, 6
# The pinned PocketNES release places its 2 KiB NES RAM and 8 KiB cartridge RAM
# consecutively at the start of GBA IWRAM (src/mem.s and src/gba_cart_my.ld).
NES_RAM = 0x03000000
NES_MEMORY_SIZE = 0x2800

# GBA key masks and zero-based mTile helper frame numbers. The one-ROM
# PocketNES build boots straight into SMB3.
INPUT = {
    450: KEY_START, 451: 0,
    700: KEY_START, 701: 0,
    920: KEY_START, 921: 0,
    1060: KEY_RIGHT, 1070: 0,
    1130: KEY_UP, 1140: 0,
    1200: KEY_A, 1210: 0,
    # World 1-1 clear from the Mesen replay with NES masks converted to GBA
    # masks and a 289-frame start offset for this particular route.
    1369: KEY_RIGHT | KEY_B,
    1393: KEY_RIGHT | KEY_B | KEY_A,
    1433: KEY_RIGHT | KEY_B,
    1460: KEY_RIGHT | KEY_B | KEY_A,
    1492: KEY_RIGHT | KEY_B,
    1506: KEY_RIGHT | KEY_B | KEY_A,
    1538: KEY_RIGHT | KEY_B,
    1574: KEY_RIGHT | KEY_B | KEY_A,
    1606: KEY_RIGHT | KEY_B,
    1689: KEY_RIGHT | KEY_B | KEY_A,
    1721: KEY_RIGHT | KEY_B,
    1810: KEY_RIGHT | KEY_B | KEY_A,
    1846: KEY_RIGHT | KEY_B,
    1929: KEY_RIGHT | KEY_B | KEY_A,
    1965: KEY_RIGHT | KEY_B,
    1998: KEY_RIGHT | KEY_B | KEY_A,
    2034: KEY_RIGHT | KEY_B,
    2073: KEY_RIGHT | KEY_B | KEY_A,
    2109: KEY_RIGHT | KEY_B,
    2136: KEY_RIGHT | KEY_B | KEY_A,
    2172: KEY_RIGHT | KEY_B,
    2192: KEY_RIGHT | KEY_B | KEY_A,
    2228: KEY_RIGHT | KEY_B,
    2256: KEY_RIGHT | KEY_B | KEY_A,
    2274: KEY_RIGHT | KEY_B,
    2451: KEY_RIGHT | KEY_B | KEY_A,
    2489: KEY_RIGHT | KEY_B,
    2659: 0,
    # Map navigation and the retimed World 1-2 slope run.
    3039: KEY_RIGHT, 3043: 0,
    3130: KEY_RIGHT, 3140: 0,
    3219: KEY_A, 3223: 0,
    3399: KEY_RIGHT | KEY_B,
    3439: KEY_RIGHT | KEY_B | KEY_A,
    3459: KEY_RIGHT | KEY_B,
    3489: 0,
}
CHECKPOINTS = {
    650: "title",
    1000: "world-1-map",
    1080: "map-right",
    1180: "map-level-1",
    1330: "world-1-1-spawn",
    1385: "running",
    1405: "jumping",
    1453: "landing-scroll",
    2474: "world-1-1-goal",
    2889: "cleared-map",
    3190: "map-level-2",
    3350: "world-1-2-spawn",
    3483: "slope-contact",
    3484: "slope-next-frame",
    3518: "slope-climb",
}
# The source Mesen replay measured these three positions and slope shapes.
SLOPE_LANDMARKS = {
    "slope-contact": (3105, [179, 338], 13),
    "slope-next-frame": (3106, [181, 337], 13),
    "slope-climb": (3140, [248, 335], 12),
}


def check(condition, message):
    if not condition:
        raise RuntimeError(message)


def read_exact(pipe, size):
    data = pipe.read(size)
    check(len(data) == size, f"helper stopped after {len(data)} of {size} bytes")
    return data


def send(process, operation, keys=0, payload=b""):
    process.stdin.write(struct.pack("<BBH", operation, 0, keys) + payload)
    process.stdin.flush()


def read_nes_memory(process):
    send(process, READ_MEMORY, payload=struct.pack("<II", NES_RAM, NES_MEMORY_SIZE))
    marker, status, size = struct.unpack("<4sII", read_exact(process.stdout, 12))
    check(marker == b"DATA" and size <= NES_MEMORY_SIZE, "invalid memory reply")
    payload = read_exact(process.stdout, size)
    check(status == 0, f"helper memory read failed: {payload.decode(errors='replace')}")
    check(size == NES_MEMORY_SIZE, f"expected {NES_MEMORY_SIZE} NES bytes, received {size}")
    return payload


def game_state(memory):
    def word(low, high):
        return memory[low] + 256 * memory[high]

    def signed(offset):
        return struct.unpack_from("b", memory, offset)[0]

    state = {
        "update_select": memory[0x100],
        "world": memory[0x727] + 1,
        "world_enter_state": memory[0x728],
        "level_layout": word(0x26B9, 0x26BA),
        "goal_state": memory[0x2174],
    }
    if state["update_select"] == 192 and state["world_enter_state"] == 0:
        state["map_position"] = [word(0x79, 0x77), memory[0x75]]
    if state["update_select"] == 192 and state["world_enter_state"] == 3:
        state.update(
            position=[word(0x90, 0x75), word(0xA2, 0x87)],
            velocity=[signed(0xBD), signed(0xCF)],
            camera_x=word(0xFD, 0x12),
            nes_pad_held=memory[0x17],
            in_air=memory[0xD8],
            slope_shape=memory[0x60D],
        )
    return state


def replay(helper, rom, timeout):
    with tempfile.TemporaryDirectory(prefix="mtile-smb3-verify-") as work:
        private_rom = Path(work) / "game.gba"
        shutil.copyfile(rom, private_rom)
        with tempfile.TemporaryFile() as stderr, subprocess.Popen(
            [str(helper), str(private_rom)],
            cwd=work,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=stderr,
        ) as process:
            expired = threading.Event()

            def stop_at_deadline():
                expired.set()
                process.kill()

            watchdog = threading.Timer(timeout, stop_at_deadline)
            watchdog.start()
            try:
                return read_replay(process)
            except (OSError, RuntimeError) as error:
                if expired.is_set():
                    raise RuntimeError(f"helper replay timed out after {timeout:g}s") from error
                stderr.seek(max(0, stderr.seek(0, 2) - 2048))
                tail = stderr.read().decode(errors="replace").strip()
                raise RuntimeError(f"{error}\n{tail}".strip()) from error
            finally:
                watchdog.cancel()
                watchdog.join()
                if process.poll() is None:
                    process.kill()


def read_replay(process):
    reader = process.stdout
    handshake = struct.unpack("<4s4HI", read_exact(reader, 16))
    check(handshake == (b"GLUP", 3, WIDTH, HEIGHT, 0, SAMPLE_RATE),
          f"expected mTile helper protocol v3, received {handshake!r}")
    video_digest = hashlib.sha256()
    audio_digest = hashlib.sha256()
    checkpoints = {}
    images = {}
    keys = 0
    sample_count = 0
    audible_frames = 0
    frame_count = max(CHECKPOINTS) + 1

    for number in range(frame_count):
        keys = INPUT.get(number, keys)
        send(process, STEP, keys)
        marker, frame, samples, overflow, telemetry_size = struct.unpack(
            "<4sIIII", read_exact(reader, 20)
        )
        check(marker == b"FRAM" and frame == number, f"unexpected frame {marker!r} {frame}")
        check(0 < samples <= 4096 and not overflow, f"bad audio packet at frame {frame}")
        check(12 <= telemetry_size <= 1024, f"bad telemetry size at frame {frame}")
        rgba = read_exact(reader, RGBA_SIZE)
        pcm = read_exact(reader, samples * 4)
        telemetry = read_exact(reader, telemetry_size)
        kind, version, size, _, keyinput = struct.unpack_from("<4s4H", telemetry)
        check((kind, version, size) == (b"TELM", 1, telemetry_size),
              f"bad telemetry packet at frame {frame}")
        check((~keyinput & 0x3FF) == keys, f"GBA key mask mismatch at frame {frame}")

        video_digest.update(rgba)
        audio_digest.update(struct.pack("<I", samples))
        audio_digest.update(pcm)
        sample_count += samples
        audible_frames += any(pcm)
        if frame in CHECKPOINTS:
            label = CHECKPOINTS[frame]
            checkpoints[label] = {
                "frame": frame,
                "rgba_sha256": hashlib.sha256(rgba).hexdigest(),
                **game_state(read_nes_memory(process)),
            }
            images[label] = rgba

    send(process, QUIT)
    check(process.wait() == 0, "helper exited unsuccessfully")
    for label, (mesen_frame, _, _) in SLOPE_LANDMARKS.items():
        checkpoints[label]["mesen_reference_frame"] = mesen_frame
    return {
        "dimensions": [WIDTH, HEIGHT],
        "frames": frame_count,
        "pcm_sample_rate": SAMPLE_RATE,
        "pcm_stereo_sample_frames": sample_count,
        "frames_with_nonzero_pcm": audible_frames,
        "video_sha256": video_digest.hexdigest(),
        "pcm_with_frame_lengths_sha256": audio_digest.hexdigest(),
        "checkpoints": checkpoints,
    }, images


def verify_landmarks(result):
    frames = result["checkpoints"]
    check(frames["title"]["update_select"] == 32, "SMB3 title did not appear")
    for label, position in (("world-1-map", [32, 64]), ("map-right", [64, 64]),
                            ("map-level-1", [64, 32]), ("cleared-map", [64, 32]),
                            ("map-level-2", [128, 32])):
        check(frames[label]["world"] == 1 and frames[label].get("map_position") == position,
              f"{label} did not reach World 1 map position {position}")
    spawn, run, jump, land = (frames[label] for label in
                             ("world-1-1-spawn", "running", "jumping", "landing-scroll"))
    for point in (spawn, run, jump, land):
        check(point["world"] == 1 and point["level_layout"] == 48002 and "position" in point,
              f"World 1-1 was inactive at frame {point['frame']}")
    check(spawn["position"] == [24, 384] and spawn["velocity"] == [0, 0],
          "unexpected World 1-1 starting position or velocity")
    check(run["position"][0] > spawn["position"][0] and run["velocity"][0] > 0
          and run["nes_pad_held"] == 0x41, "Right+B did not produce running")
    check(jump["position"][1] < run["position"][1] and jump["velocity"][1] < 0
          and jump["nes_pad_held"] == 0xC1, "A did not produce a running jump")
    check(land["position"][1] == spawn["position"][1] and land["velocity"][1] == 0
          and land["camera_x"] > spawn["camera_x"], "did not land and scroll right")
    check(frames["world-1-1-goal"]["goal_state"] == 129
          and frames["cleared-map"]["goal_state"] == 129, "World 1-1 clear was not recorded")

    spawn = frames["world-1-2-spawn"]
    check(spawn["world"] == 1 and spawn["level_layout"] == 44842
          and spawn.get("position") == [24, 368], "unexpected World 1-2 spawn")
    for label, (_, position, shape) in SLOPE_LANDMARKS.items():
        point = frames[label]
        check(point["level_layout"] == 44842 and point.get("position") == position
              and point.get("slope_shape") == shape and point.get("in_air") == 0,
              f"{label} did not match grounded position {position} and slope shape {shape}")

    visible_states = ("title", "world-1-map", "world-1-1-spawn", "jumping",
                      "world-1-1-goal", "cleared-map", "world-1-2-spawn", "slope-climb")
    hashes = {frames[label]["rgba_sha256"] for label in visible_states}
    check(len(hashes) == len(visible_states), "major checkpoint screenshots did not all change")
    check(result["frames_with_nonzero_pcm"] > 100, "expected non-silent game audio")


def write_png(path, rgba):
    def chunk(tag, data):
        return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data))

    stride = WIDTH * 4
    rows = b"".join(b"\0" + rgba[start:start + stride] for start in range(0, RGBA_SIZE, stride))
    path.write_bytes(
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", struct.pack(">2I5B", WIDTH, HEIGHT, 8, 6, 0, 0, 0))
        + chunk(b"IDAT", zlib.compress(rows))
        + chunk(b"IEND", b"")
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("rom", type=Path, help="locally generated SMB3 .gba cartridge")
    parser.add_argument("helper", type=Path, help="path to the mtile-player executable")
    parser.add_argument("--output", type=Path, help="save PNG checkpoints and verification.json")
    parser.add_argument("--timeout", type=float, default=60, help="seconds allowed per replay")
    args = parser.parse_args()
    if not math.isfinite(args.timeout) or args.timeout <= 0:
        parser.error("--timeout must be positive")
    try:
        rom, helper = args.rom.resolve(strict=True), args.helper.resolve(strict=True)
        first, images = replay(helper, rom, args.timeout)
        verify_landmarks(first)
        second, _ = replay(helper, rom, args.timeout)
        check(first == second, "fresh replays produced different video, audio, or game state")
        report = {
            "cartridge_sha256": hashlib.sha256(rom.read_bytes()).hexdigest(),
            "identical_runs": 2,
            **first,
        }
        rendered = json.dumps(report, indent=2) + "\n"
        if args.output:
            args.output.mkdir(parents=True, exist_ok=True)
            for label, rgba in images.items():
                write_png(args.output / f"{first['checkpoints'][label]['frame']:04}-{label}.png", rgba)
            (args.output / "verification.json").write_text(rendered, encoding="utf-8")
        print(rendered, end="")
    except (OSError, RuntimeError) as error:
        print(f"verify: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
