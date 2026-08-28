#!/usr/bin/env python3
"""Generate matching PICO-8 and Mtile single-note SFX fixtures."""

from __future__ import annotations

import argparse
import json
import math
from pathlib import Path


WAVEFORMS = {
    "triangle": 0,
    "tilted-saw": 1,
    "saw": 2,
    "square": 3,
    "pulse": 4,
    "organ": 5,
    "noise": 6,
    "phaser": 7,
}

EFFECTS = {
    "none": 0,
    "slide": 1,
    "vibrato": 2,
    "drop": 3,
    "fade-in": 4,
    "fade-out": 5,
    "arpeggio-fast": 6,
    "arpeggio-slow": 7,
}


def encode_filters(noiz: bool, buzz: bool, detune: int, reverb: int, dampen: int) -> int:
    """Return PICO-8's mixed-radix SFX filter byte (editor flag clear)."""
    return (
        (2 if noiz else 0)
        + (4 if buzz else 0)
        + detune * 8
        + reverb * 24
        + dampen * 72
    )


def pico_sfx_line(
    pitch: int,
    waveform: int,
    volume: int,
    effect: int,
    speed: int,
    filters: int,
) -> str:
    # PICO-8 text carts store editor mode, speed, loop start/end, then 32
    # five-nibble notes: pitch, waveform, volume, effect. Only the first note
    # is audible in this fixture so every exported WAV is one isolated case.
    # PICO-8 requires the SFX editor-mode bit in filtered text-cart records.
    # It is UI metadata only; generated Mtile definitions retain filters
    # with that bit clear.
    cart_filters = filters | (1 if filters else 0)
    header = f"{cart_filters:02x}{speed:02x}0000"
    note = f"{pitch:02x}{waveform:x}{volume:x}{effect:x}"
    return header + note + "00000" * 31


def write_pico_cart(
    path: Path,
    waveform: str,
    effect: str,
    speed: int,
    volume: int,
    pitches: list[int],
    filters: int,
) -> None:
    reverb = (filters // 24) % 3
    # Leave eight 60 Hz frames per reverb level for the captured 1/2-feedback
    # tail. This keeps isolated exports complete and prevents adjacent sweep
    # cases from overlapping.
    trigger_frames = math.ceil(speed / 2) + 4 + reverb * 8
    final_case = len(pitches) - 1
    lines = [
        "pico-8 cartridge // http://www.pico-8.com",
        "version 42",
        "__lua__",
        "-- generated exhaustive single-note audio conformance fixture",
        "case_id=-1",
        "wait_frames=30",
        "done=false",
        "shutdown_wait=0",
        "function _init()",
        "  extcmd(\"set_filename\",\"pico-notes\")",
        "  extcmd(\"audio_rec\")",
        "end",
        "function _update60()",
        "  if done then",
        "    shutdown_wait-=1",
        "    if shutdown_wait<=0 then extcmd(\"shutdown\") end",
        "    return",
        "  end",
        "  if wait_frames>0 then",
        "    wait_frames-=1",
        f"  elseif case_id<{final_case} then",
        "    case_id+=1",
        "    sfx(case_id,0)",
        f"    wait_frames={trigger_frames - 1}",
        "  else",
        "    extcmd(\"audio_end\",1)",
        "    done=true",
        "    shutdown_wait=60",
        "  end",
        "end",
        "function _draw()",
        "  cls(1)",
        "  print(\"audio conformance\",24,54,7)",
        "  print(\"case \"..case_id,40,68,6)",
        "end",
        "__sfx__",
    ]
    waveform_id = WAVEFORMS[waveform]
    effect_id = EFFECTS[effect]
    lines.extend(
        pico_sfx_line(pitch, waveform_id, volume, effect_id, speed, filters)
        for pitch in pitches
    )
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def write_mtile_project(
    path: Path,
    waveform: str,
    effect: str,
    speed: int,
    volume: int,
    pitches: list[int],
    filter_settings: dict[str, object],
) -> None:
    reverb = int(filter_settings.get("reverb", 0))
    trigger_frames = math.ceil(speed * 183 * 30 / 22050) + 2 + reverb * 4
    final_case = len(pitches) - 1
    audio_dir = path / "assets" / "audio"
    audio_dir.mkdir(parents=True, exist_ok=True)
    (path / "assets").mkdir(parents=True, exist_ok=True)

    authored_filters = {
        key: value
        for key, value in filter_settings.items()
        if value is not False and value != 0
    }
    sfx = {
        "version": 1,
        "sfx": [
            {
                "id": case_id,
                "name": f"{waveform}-{pitch:02d}",
                "speed": speed,
                **({"filters": authored_filters} if authored_filters else {}),
                "steps": [
                    {
                        "pitch": pitch,
                        "waveform": waveform,
                        "volume": volume,
                        "effect": effect,
                    }
                ],
            }
            for case_id, pitch in enumerate(pitches)
        ],
    }
    (audio_dir / "sfx.json").write_text(
        json.dumps(sfx, indent=2) + "\n", encoding="utf-8"
    )
    (audio_dir / "settings.json").write_text(
        json.dumps(
            {
                "version": 1,
                "output": "mono",
                "sampleRate": 16384,
                "voiceProfile": "auto",
                "masterVolume": 256,
            },
            indent=2,
        )
        + "\n",
        encoding="utf-8",
    )
    (path / "assets" / "map.json").write_text(
        '{\n  "width": 1,\n  "height": 1,\n  "cells": [0]\n}\n', encoding="utf-8"
    )
    (path / "assets" / "spritesheet.json").write_text(
        '{\n  "width": 8,\n  "height": 8,\n  "sprites": null\n}\n', encoding="utf-8"
    )
    (path / "project.json").write_text(
        '{\n  "activePalette": "aurora"\n}\n', encoding="utf-8"
    )
    (path / "game.lua").write_text(
        """-- generated exhaustive single-note audio conformance fixture
case_id = -1
wait_frames = 15

function _init()
  case_id = -1
  wait_frames = 15
end

function _update()
  if wait_frames > 0 then
    wait_frames = wait_frames - 1
  elseif case_id < %d then
    case_id = case_id + 1
    sfx(case_id, 0)
    wait_frames = %d
  end
end

function _draw()
  cls(1)
  print("PICO SFX CONFORMANCE", 8, 16, 15)
  print("WAVE: %s", 8, 48, 18)
  print("EFFECT: %s", 8, 64, 18)
  print("PITCH: " .. case_id, 8, 80, 18)
end
"""
        % (final_case, trigger_frames - 1, waveform.upper(), effect.upper()),
        encoding="utf-8",
    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--waveform", choices=WAVEFORMS, default="triangle")
    parser.add_argument("--effect", choices=EFFECTS, default="none")
    parser.add_argument("--speed", type=int, default=16)
    parser.add_argument("--volume", type=int, default=5)
    parser.add_argument("--noiz", action="store_true")
    parser.add_argument("--buzz", action="store_true")
    parser.add_argument("--detune", type=int, choices=range(3), default=0)
    parser.add_argument("--reverb", type=int, choices=range(3), default=0)
    parser.add_argument("--dampen", type=int, choices=range(3), default=0)
    parser.add_argument(
        "--pitch",
        type=int,
        action="append",
        help="pitch to include (repeatable); defaults to all 64 pitches",
    )
    args = parser.parse_args()
    if not 1 <= args.speed <= 255:
        parser.error("--speed must be in 1..255")
    if not 0 <= args.volume <= 7:
        parser.error("--volume must be in 0..7")
    if args.pitch is not None and any(pitch < 0 or pitch > 63 for pitch in args.pitch):
        parser.error("--pitch must be in 0..63")
    return args


def main() -> None:
    args = parse_args()
    args.output_dir.mkdir(parents=True, exist_ok=True)
    pico_path = args.output_dir / f"{args.waveform}-{args.effect}.p8"
    project_path = args.output_dir / "mtile"
    pitches = args.pitch if args.pitch is not None else list(range(64))
    filter_settings = {
        "noiz": args.noiz,
        "buzz": args.buzz,
        "detune": args.detune,
        "reverb": args.reverb,
        "dampen": args.dampen,
    }
    filters = encode_filters(**filter_settings)
    pico_trigger_frames = math.ceil(args.speed / 2) + 4 + args.reverb * 8
    mtile_trigger_frames = (
        math.ceil(args.speed * 183 * 30 / 22050) + 2 + args.reverb * 4
    )
    write_pico_cart(
        pico_path, args.waveform, args.effect, args.speed, args.volume, pitches, filters
    )
    write_mtile_project(
        project_path,
        args.waveform,
        args.effect,
        args.speed,
        args.volume,
        pitches,
        filter_settings,
    )
    manifest = {
        "version": 1,
        "waveform": args.waveform,
        "waveformID": WAVEFORMS[args.waveform],
        "effect": args.effect,
        "effectID": EFFECTS[args.effect],
        "speed": args.speed,
        "volume": args.volume,
        "filters": filter_settings,
        "filterByte": filters,
        "pitches": pitches,
        "picoCart": pico_path.name,
        "mtileProject": "mtile",
        "picoTriggerIntervalFrames": pico_trigger_frames,
        "mtileTriggerIntervalFrames": mtile_trigger_frames,
    }
    (args.output_dir / "manifest.json").write_text(
        json.dumps(manifest, indent=2) + "\n", encoding="utf-8"
    )
    print(pico_path)
    print(project_path)


if __name__ == "__main__":
    main()
