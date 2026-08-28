#!/usr/bin/env python3
"""Generate matching PICO-8/Mtile four-note melody speed fixtures."""

from __future__ import annotations

import argparse
import json
import math
from pathlib import Path


PITCHES = (24, 24, 36, 36)
WAVEFORM = "square"
WAVEFORM_ID = 3
VOLUME = 5
PICO_SAMPLES_PER_SPEED_UNIT = 183
PICO_SAMPLE_RATE = 22050


def sfx_line(speed: int) -> str:
    header = f"00{speed:02x}0000"
    notes = "".join(f"{pitch:02x}{WAVEFORM_ID:x}{VOLUME:x}0" for pitch in PITCHES)
    return header + notes + "00000" * (32 - len(PITCHES))


def case_frames(speed: int, tick_rate: int, margin: int) -> int:
    samples = speed * len(PITCHES) * PICO_SAMPLES_PER_SPEED_UNIT
    return math.ceil(samples * tick_rate / PICO_SAMPLE_RATE) + margin


def write_pico_cart(path: Path, speeds: list[int]) -> list[int]:
    waits = [case_frames(speed, 60, 8) for speed in speeds]
    filename = f"pico-speeds-{speeds[0]:03d}-{speeds[-1]:03d}"
    lines = [
        "pico-8 cartridge // http://www.pico-8.com",
        "version 42",
        "__lua__",
        "-- generated exhaustive four-note speed conformance fixture",
        "case_id=-1",
        "wait_frames=30",
        f"case_waits={{{','.join(str(wait - 1) for wait in waits)}}}",
        "done=false",
        "shutdown_wait=0",
        "function _init()",
        f'  extcmd("set_filename","{filename}")',
        '  extcmd("audio_rec")',
        "end",
        "function _update60()",
        "  if done then",
        "    shutdown_wait-=1",
        '    if shutdown_wait<=0 then extcmd("shutdown") end',
        "    return",
        "  end",
        "  if wait_frames>0 then",
        "    wait_frames-=1",
        f"  elseif case_id<{len(speeds) - 1} then",
        "    case_id+=1",
        "    sfx(case_id,0)",
        "    wait_frames=case_waits[case_id+1]",
        "  else",
        '    extcmd("audio_end",1)',
        "    done=true",
        "    shutdown_wait=60",
        "  end",
        "end",
        "function _draw()",
        "  cls(1)",
        '  print("speed conformance",24,54,7)',
        f'  print("speed "..(case_id+{speeds[0]}),36,68,6)',
        "end",
        "__sfx__",
    ]
    lines.extend(sfx_line(speed) for speed in speeds)
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return waits


def write_project(path: Path, speeds: list[int]) -> list[int]:
    waits = [case_frames(speed, 30, 4) for speed in speeds]
    audio_dir = path / "assets" / "audio"
    audio_dir.mkdir(parents=True, exist_ok=True)
    (audio_dir / "sfx.json").write_text(
        json.dumps(
            {
                "version": 1,
                "sfx": [
                    {
                        "id": case_id,
                        "name": f"speed-{speed:03d}",
                        "speed": speed,
                        "steps": [
                            {
                                "pitch": pitch,
                                "waveform": WAVEFORM,
                                "volume": VOLUME,
                                "effect": "none",
                            }
                            for pitch in PITCHES
                        ],
                    }
                    for case_id, speed in enumerate(speeds)
                ],
            },
            indent=2,
        )
        + "\n",
        encoding="utf-8",
    )
    (audio_dir / "settings.json").write_text(
        '{\n  "version": 1,\n  "output": "mono",\n  "sampleRate": 16384,\n'
        '  "voiceProfile": "auto",\n  "masterVolume": 256\n}\n',
        encoding="utf-8",
    )
    assets = path / "assets"
    (assets / "map.json").write_text(
        '{\n  "width": 1,\n  "height": 1,\n  "cells": [0]\n}\n',
        encoding="utf-8",
    )
    (assets / "spritesheet.json").write_text(
        '{\n  "width": 8,\n  "height": 8,\n  "sprites": null\n}\n',
        encoding="utf-8",
    )
    (path / "project.json").write_text(
        '{\n  "activePalette": "aurora"\n}\n', encoding="utf-8"
    )
    (path / "game.lua").write_text(
        f"""-- generated exhaustive four-note speed conformance fixture
case_id = -1
wait_frames = 15
case_waits = {{{", ".join(str(wait - 1) for wait in waits)}}}

function _init()
  case_id = -1
  wait_frames = 15
end

function _update()
  if wait_frames > 0 then
    wait_frames = wait_frames - 1
  elseif case_id < {len(speeds) - 1} then
    case_id = case_id + 1
    sfx(case_id, 0)
    wait_frames = case_waits[case_id + 1]
  end
end

function _draw()
  cls(1)
  print("PICO SPEED CONFORMANCE", 8, 16, 15)
  print("SPEED: " .. (case_id + {speeds[0]}), 8, 48, 18)
end
""",
        encoding="utf-8",
    )
    return waits


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--start-speed", type=int, required=True)
    parser.add_argument("--end-speed", type=int, required=True)
    args = parser.parse_args()
    if not 1 <= args.start_speed <= args.end_speed <= 255:
        parser.error("speed range must satisfy 1 <= start <= end <= 255")
    if args.end_speed - args.start_speed + 1 > 64:
        parser.error("one fixture may contain at most 64 speeds")
    return args


def main() -> None:
    args = parse_args()
    args.output_dir.mkdir(parents=True, exist_ok=True)
    speeds = list(range(args.start_speed, args.end_speed + 1))
    pico_path = args.output_dir / "speeds.p8"
    project_path = args.output_dir / "mtile"
    pico_waits = write_pico_cart(pico_path, speeds)
    mtile_waits = write_project(project_path, speeds)
    manifest = {
        "version": 1,
        "metric": "four-note-melody-speed",
        "waveform": WAVEFORM,
        "pitches": list(PITCHES),
        "speeds": speeds,
        "picoSamplesPerSpeedUnit": PICO_SAMPLES_PER_SPEED_UNIT,
        "picoSampleRate": PICO_SAMPLE_RATE,
        "picoWaitFrames": pico_waits,
        "mtileWaitFrames": mtile_waits,
        "picoTotalFrames": 30 + sum(pico_waits) + 60,
        "mtileTotalFrames": 15 + sum(mtile_waits),
        "picoCart": pico_path.name,
        "mtileProject": "mtile",
    }
    (args.output_dir / "manifest.json").write_text(
        json.dumps(manifest, indent=2) + "\n", encoding="utf-8"
    )
    print(pico_path)
    print(project_path)


if __name__ == "__main__":
    main()
