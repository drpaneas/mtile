#!/usr/bin/env python3
"""Generate matching transition-aware PICO-8 and Mtile effect fixtures."""

from __future__ import annotations

import argparse
import json
import math
from dataclasses import dataclass
from pathlib import Path


WAVEFORM = "square"
WAVEFORM_ID = 3
EFFECT_IDS = {
    "none": 0,
    "slide": 1,
    "vibrato": 2,
    "drop": 3,
    "fade-in": 4,
    "fade-out": 5,
    "arpeggio-fast": 6,
    "arpeggio-slow": 7,
}


@dataclass(frozen=True)
class Note:
    pitch: int
    volume: int
    effect: str


@dataclass(frozen=True)
class Case:
    name: str
    speed: int
    notes: tuple[Note, ...]
    check_pitch: bool = True
    check_envelope: bool = False


def arp(effect: str, speed: int) -> tuple[Note, ...]:
    return tuple(Note(pitch, 5, effect) for pitch in (24, 28, 31, 36))


CASES = (
    Case("none", 32, (Note(36, 5, "none"),)),
    Case(
        "slide-up",
        32,
        (Note(24, 3, "slide"), Note(48, 6, "none")),
        check_envelope=True,
    ),
    Case(
        "slide-down",
        32,
        (Note(48, 6, "slide"), Note(24, 3, "none")),
        check_envelope=True,
    ),
    Case("vibrato", 64, (Note(36, 5, "vibrato"),)),
    Case("drop", 64, (Note(48, 5, "drop"),)),
    Case(
        "fade-in",
        64,
        (Note(36, 5, "fade-in"),),
        check_pitch=False,
        check_envelope=True,
    ),
    Case(
        "fade-out",
        64,
        (Note(36, 5, "fade-out"),),
        check_pitch=False,
        check_envelope=True,
    ),
    Case("arpeggio-fast", 18, arp("arpeggio-fast", 18)),
    Case("arpeggio-slow", 18, arp("arpeggio-slow", 18)),
    Case("arpeggio-fast-speed8", 8, arp("arpeggio-fast", 8)),
    Case("arpeggio-slow-speed8", 8, arp("arpeggio-slow", 8)),
)


def pico_sfx_line(case: Case) -> str:
    header = f"00{case.speed:02x}0000"
    notes = "".join(
        f"{note.pitch:02x}{WAVEFORM_ID:x}{note.volume:x}{EFFECT_IDS[note.effect]:x}"
        for note in case.notes
    )
    return header + notes + "00000" * (32 - len(case.notes))


def write_pico_cart(path: Path) -> list[int]:
    waits = [math.ceil(case.speed * len(case.notes) * 183 * 60 / 22050) + 8 for case in CASES]
    wait_values = ",".join(str(wait - 1) for wait in waits)
    lines = [
        "pico-8 cartridge // http://www.pico-8.com",
        "version 42",
        "__lua__",
        "-- generated transition-aware PICO effect conformance fixture",
        "case_id=-1",
        "wait_frames=30",
        f"case_waits={{{wait_values}}}",
        "done=false",
        "shutdown_wait=0",
        "function _init()",
        "  extcmd(\"set_filename\",\"pico-effects\")",
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
        f"  elseif case_id<{len(CASES) - 1} then",
        "    case_id+=1",
        "    sfx(case_id,0)",
        "    wait_frames=case_waits[case_id+1]",
        "  else",
        "    extcmd(\"audio_end\",1)",
        "    done=true",
        "    shutdown_wait=60",
        "  end",
        "end",
        "function _draw()",
        "  cls(1)",
        "  print(\"effect conformance\",24,54,7)",
        "  print(\"case \"..case_id,40,68,6)",
        "end",
        "__sfx__",
    ]
    lines.extend(pico_sfx_line(case) for case in CASES)
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return waits


def write_mtile_project(path: Path) -> list[int]:
    waits = [math.ceil(case.speed * len(case.notes) * 183 * 30 / 22050) + 4 for case in CASES]
    audio_dir = path / "assets" / "audio"
    audio_dir.mkdir(parents=True, exist_ok=True)
    sfx = {
        "version": 1,
        "sfx": [
            {
                "id": case_id,
                "name": case.name,
                "speed": case.speed,
                "steps": [
                    {
                        "pitch": note.pitch,
                        "waveform": WAVEFORM,
                        "volume": note.volume,
                        "effect": note.effect,
                    }
                    for note in case.notes
                ],
            }
            for case_id, case in enumerate(CASES)
        ],
    }
    (audio_dir / "sfx.json").write_text(
        json.dumps(sfx, indent=2) + "\n", encoding="utf-8"
    )
    (audio_dir / "settings.json").write_text(
        '{\n  "version": 1,\n  "output": "mono",\n  "sampleRate": 16384,\n'
        '  "voiceProfile": "auto",\n  "masterVolume": 256\n}\n',
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
    wait_values = ", ".join(str(wait - 1) for wait in waits)
    (path / "game.lua").write_text(
        f"""-- generated transition-aware PICO effect conformance fixture
case_id = -1
wait_frames = 15
case_waits = {{{wait_values}}}

function _init()
  case_id = -1
  wait_frames = 15
end

function _update()
  if wait_frames > 0 then
    wait_frames = wait_frames - 1
  elseif case_id < {len(CASES) - 1} then
    case_id = case_id + 1
    sfx(case_id, 0)
    wait_frames = case_waits[case_id + 1]
  end
end

function _draw()
  cls(1)
  print("PICO EFFECT CONFORMANCE", 8, 16, 15)
  print("CASE: " .. case_id, 8, 48, 18)
end
""",
        encoding="utf-8",
    )
    return waits


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output-dir", type=Path, required=True)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    args.output_dir.mkdir(parents=True, exist_ok=True)
    pico_path = args.output_dir / "effects.p8"
    project_path = args.output_dir / "mtile"
    pico_waits = write_pico_cart(pico_path)
    mtile_waits = write_mtile_project(project_path)
    manifest = {
        "version": 1,
        "waveform": WAVEFORM,
        "cases": [
            {
                "id": case_id,
                "name": case.name,
                "speed": case.speed,
                "notes": [note.__dict__ for note in case.notes],
                "checkPitch": case.check_pitch,
                "checkEnvelope": case.check_envelope,
                "picoWaitFrames": pico_waits[case_id],
                "mtileWaitFrames": mtile_waits[case_id],
            }
            for case_id, case in enumerate(CASES)
        ],
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
