#!/usr/bin/env python3
"""Generate matching one-note PICO-8/Mtile custom-instrument fixtures."""

from __future__ import annotations

import argparse
import json
import math
from pathlib import Path

from generate_pico_sfx_fixture import EFFECTS, WAVEFORMS, encode_filters


def sfx_line(
    notes: list[tuple[int, int, int, int]],
    speed: int,
    loop: tuple[int, int] | None = None,
    filters: int = 0,
) -> str:
    if len(notes) > 32:
        raise ValueError("an SFX can contain at most 32 notes")
    loop_start, loop_end = loop or (0, 0)
    # PICO-8's editor-mode bit must be set when the row uses either modern
    # filters or custom-instrument waveform nibbles.
    editor_mode = filters != 0 or any(waveform >= 8 for _, waveform, _, _ in notes)
    header = f"{(filters | (1 if editor_mode else 0)):02x}{speed:02x}{loop_start:02x}{loop_end:02x}"
    encoded = [f"{pitch:02x}{waveform:x}{volume:x}{effect:x}" for pitch, waveform, volume, effect in notes]
    return header + "".join(encoded + ["00000"] * (32 - len(encoded)))


def parse_note(value: str) -> tuple[int, str, int, str]:
    try:
        pitch_text, waveform, volume_text, effect = value.split(":")
        pitch, volume = int(pitch_text), int(volume_text)
    except ValueError as error:
        raise argparse.ArgumentTypeError(
            "notes use PITCH:WAVEFORM:VOLUME:EFFECT"
        ) from error
    if pitch not in range(64) or waveform not in WAVEFORMS or volume not in range(8) or effect not in EFFECTS:
        raise argparse.ArgumentTypeError(f"invalid custom-instrument note: {value}")
    return pitch, waveform, volume, effect


def write_pico_cart(
    path: Path,
    inner_notes: list[tuple[int, str, int, str]],
    inner_speed: int,
    inner_loop: bool,
    inner_filters: int,
    outer_pitch: int,
    outer_volume: int,
    outer_effect: str,
    outer_speed: int,
    outer_filters: int,
) -> None:
    inner_encoded = [
        (pitch, WAVEFORMS[waveform], volume, EFFECTS[effect])
        for pitch, waveform, volume, effect in inner_notes
    ]
    outer_note = [(outer_pitch, 8, outer_volume, EFFECTS[outer_effect])]
    blank = sfx_line([], 1)
    duration_frames = math.ceil(outer_speed / 2) + 8
    lines = [
        "pico-8 cartridge // http://www.pico-8.com",
        "version 42",
        "__lua__",
        "wait_frames=30",
        "done=false",
        "function _init()",
        ' extcmd("set_filename","pico-custom")',
        ' extcmd("audio_rec")',
        "end",
        "function _update60()",
        " if done then return end",
        " if wait_frames>0 then",
        "  wait_frames-=1",
        " elseif wait_frames==0 then",
        "  sfx(8,0)",
        "  wait_frames=-1",
        f" elseif wait_frames>-{duration_frames} then",
        "  wait_frames-=1",
        " else",
        '  extcmd("audio_end",1)',
        "  done=true",
        " end",
        "end",
        "__sfx__",
        sfx_line(
            inner_encoded,
            inner_speed,
            (0, len(inner_encoded)) if inner_loop else None,
            inner_filters,
        ),
        *([blank] * 7),
        sfx_line(outer_note, outer_speed, filters=outer_filters),
    ]
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def write_mtile_project(
    path: Path,
    inner_notes: list[tuple[int, str, int, str]],
    inner_speed: int,
    inner_loop: bool,
    outer_pitch: int,
    outer_volume: int,
    outer_effect: str,
    outer_speed: int,
) -> None:
    audio_dir = path / "assets" / "audio"
    audio_dir.mkdir(parents=True, exist_ok=True)
    sfx = {
        "version": 1,
        "sfx": [
            {
                "id": 0,
                "name": "custom-instrument-0",
                "speed": inner_speed,
                **({"loop": {"start": 0, "end": len(inner_notes)}} if inner_loop else {}),
                "steps": [
                    {"pitch": pitch, "waveform": waveform, "volume": volume, "effect": effect}
                    for pitch, waveform, volume, effect in inner_notes
                ],
            },
            {
                "id": 8,
                "name": "outer-note",
                "speed": outer_speed,
                "steps": [
                    {
                        "pitch": outer_pitch,
                        "waveform": "custom-0",
                        "volume": outer_volume,
                        "effect": outer_effect,
                    }
                ],
            },
        ],
    }
    (audio_dir / "sfx.json").write_text(json.dumps(sfx, indent=2) + "\n", encoding="utf-8")
    (audio_dir / "settings.json").write_text(
        '{\n  "version": 1,\n  "output": "mono",\n  "sampleRate": 16384,\n  "voiceProfile": "auto",\n  "masterVolume": 256\n}\n',
        encoding="utf-8",
    )
    assets = path / "assets"
    (assets / "map.json").write_text('{"width":1,"height":1,"cells":[0]}\n', encoding="utf-8")
    (assets / "spritesheet.json").write_text('{"width":8,"height":8,"sprites":null}\n', encoding="utf-8")
    (path / "project.json").write_text('{"activePalette":"aurora"}\n', encoding="utf-8")
    wait_frames = math.ceil(outer_speed * 183 * 30 / 22050) + 2
    (path / "game.lua").write_text(
        f"""wait_frames = 15
played = false
function _update()
 if wait_frames > 0 then
  wait_frames = wait_frames - 1
 elseif not played then
  sfx(8, 0)
  played = true
  wait_frames = {wait_frames}
 end
end
""",
        encoding="utf-8",
    )


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output_dir", type=Path)
    parser.add_argument("--inner-note", action="append", type=parse_note)
    parser.add_argument("--inner-speed", type=int, default=4)
    parser.add_argument("--no-inner-loop", action="store_true")
    parser.add_argument("--outer-pitch", type=int, default=24, choices=range(64))
    parser.add_argument("--outer-volume", type=int, default=7, choices=range(8))
    parser.add_argument("--outer-effect", choices=EFFECTS, default="none")
    parser.add_argument("--outer-speed", type=int, default=32)
    parser.add_argument("--inner-filter", type=int, default=0, choices=range(215))
    parser.add_argument("--outer-filter", type=int, default=0, choices=range(215))
    args = parser.parse_args()
    if args.inner_speed not in range(1, 256) or args.outer_speed not in range(1, 256):
        parser.error("speeds must be in 1..255")
    for name, value in (("inner", args.inner_filter), ("outer", args.outer_filter)):
        if value & 1:
            parser.error(f"--{name}-filter must have the editor bit clear")
    inner_notes = args.inner_note or [(24, "triangle", 7, "none")]
    args.output_dir.mkdir(parents=True, exist_ok=True)
    write_pico_cart(
        args.output_dir / "custom.p8",
        inner_notes,
        args.inner_speed,
        not args.no_inner_loop,
        args.inner_filter,
        args.outer_pitch,
        args.outer_volume,
        args.outer_effect,
        args.outer_speed,
        args.outer_filter,
    )
    write_mtile_project(
        args.output_dir / "mtile",
        inner_notes,
        args.inner_speed,
        not args.no_inner_loop,
        args.outer_pitch,
        args.outer_volume,
        args.outer_effect,
        args.outer_speed,
    )
    print(args.output_dir / "custom.p8")


if __name__ == "__main__":
    main()
