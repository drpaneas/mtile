#!/usr/bin/env python3
"""Import a text PICO-8 cart's complete SFX/music data into a Mtile project."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from pico8_cart import CartAudio, MusicPattern, SFX, load_cart


WAVEFORMS = (
    "triangle",
    "tilted-saw",
    "saw",
    "square",
    "pulse",
    "organ",
    "noise",
    "phaser",
    "custom-0",
    "custom-1",
    "custom-2",
    "custom-3",
    "custom-4",
    "custom-5",
    "custom-6",
    "custom-7",
)
EFFECTS = (
    "none",
    "slide",
    "vibrato",
    "drop",
    "fade-in",
    "fade-out",
    "arpeggio-fast",
    "arpeggio-slow",
)


def sfx_length(effect: SFX) -> int:
    if effect.loop_start > 0 and effect.loop_end == 0:
        return min(effect.loop_start, 32)
    return 32


def authored_filters(encoded: int) -> dict[str, object]:
    filters: dict[str, object] = {}
    if encoded & 2:
        filters["noiz"] = True
    if encoded & 4:
        filters["buzz"] = True
    for key, value in (
        ("detune", (encoded // 8) % 3),
        ("reverb", (encoded // 24) % 3),
        ("dampen", (encoded // 72) % 3),
    ):
        if value:
            filters[key] = value
    return filters


def authored_sfx(cart: CartAudio) -> dict[str, object]:
    effects: list[dict[str, object]] = []
    for sfx_id, effect in enumerate(cart.sfx):
        length = sfx_length(effect)
        filters = authored_filters(effect.filters)
        record: dict[str, object] = {
            "id": sfx_id,
            "name": f"pico-sfx-{sfx_id:02d}",
            "speed": effect.speed,
            "steps": [
                {
                    "pitch": note.pitch,
                    "waveform": WAVEFORMS[note.waveform],
                    "volume": note.volume,
                    "effect": EFFECTS[note.effect],
                }
                for note in effect.notes[:length]
            ],
        }
        if filters:
            record["filters"] = filters
        if effect.loop_start < effect.loop_end <= length:
            record["loop"] = {"start": effect.loop_start, "end": effect.loop_end}
        effects.append(record)
    return {"version": 1, "sfx": effects}


def pattern_has_audio(pattern: MusicPattern) -> bool:
    return any(channel is not None for channel in pattern.channels)


def pattern_duration_units(cart: CartAudio, pattern: MusicPattern) -> int:
    fallback = 0
    for sfx_id in pattern.channels:
        if sfx_id is None or sfx_id >= len(cart.sfx):
            continue
        effect = cart.sfx[sfx_id]
        duration = effect.speed * sfx_length(effect)
        fallback = max(fallback, duration)
        if effect.loop_start >= effect.loop_end:
            return duration
    # PICO normally follows the left-most non-looping channel. If every
    # assigned channel loops, it waits for the longest channel's full authored
    # length. Shorter loops continue until that shared pattern boundary.
    if fallback:
        return fallback
    # This only occurs for an empty pattern, which authored_music omits.
    return 0


def pattern_next(patterns: dict[int, MusicPattern], pattern_id: int) -> int:
    pattern = patterns[pattern_id]
    if pattern.flags & 4:
        return -1
    if pattern.flags & 2:
        for candidate in range(pattern_id, -1, -1):
            if candidate in patterns and patterns[candidate].flags & 1:
                return candidate
        return min(patterns)
    later = [candidate for candidate in patterns if candidate > pattern_id]
    return min(later) if later else -1


def authored_music(cart: CartAudio) -> dict[str, object]:
    patterns = {
        pattern_id: pattern
        for pattern_id, pattern in enumerate(cart.music)
        if pattern_has_audio(pattern)
    }
    authored = []
    for pattern_id, pattern in patterns.items():
        authored.append(
            {
                "id": pattern_id,
                "name": f"pico-pattern-{pattern_id:02d}",
                "channels": [channel if channel is not None else -1 for channel in pattern.channels],
                "next": pattern_next(patterns, pattern_id),
                "durationUnits": pattern_duration_units(cart, pattern),
            }
        )
    return {"version": 1, "patterns": authored}


def write_project(
    cart_path: Path, output: Path, entry_pattern: int, sfx_id: int | None = None
) -> None:
    cart = load_cart(cart_path)
    sfx = authored_sfx(cart)
    music = authored_music(cart)
    pattern_ids = {int(pattern["id"]) for pattern in music["patterns"]}  # type: ignore[index]
    if pattern_ids and entry_pattern not in pattern_ids:
        raise ValueError(f"entry pattern {entry_pattern} is not present in {cart_path}")

    audio_dir = output / "assets" / "audio"
    audio_dir.mkdir(parents=True, exist_ok=True)
    (audio_dir / "sfx.json").write_text(
        json.dumps(sfx, indent=2, ensure_ascii=False) + "\n", encoding="utf-8"
    )
    (audio_dir / "music.json").write_text(
        json.dumps(music, indent=2, ensure_ascii=False) + "\n", encoding="utf-8"
    )
    (audio_dir / "settings.json").write_text(
        '{\n  "version": 1,\n  "output": "mono",\n  "sampleRate": 16384,\n  "voiceProfile": "auto",\n  "masterVolume": 256\n}\n',
        encoding="utf-8",
    )
    assets = output / "assets"
    (assets / "map.json").write_text(
        '{\n  "width": 1,\n  "height": 1,\n  "cells": [0]\n}\n', encoding="utf-8"
    )
    (assets / "spritesheet.json").write_text(
        '{\n  "width": 8,\n  "height": 8,\n  "sprites": null\n}\n', encoding="utf-8"
    )
    (output / "project.json").write_text(
        '{\n  "activePalette": "aurora"\n}\n', encoding="utf-8"
    )
    if sfx_id is not None:
        if sfx_id >= len(cart.sfx):
            raise ValueError(f"SFX {sfx_id} is not present in {cart_path}")
        start = f"sfx({sfx_id}, 0)"
    elif pattern_ids:
        start = f"music({entry_pattern})"
    else:
        audible = next(
            (
                sfx_id
                for sfx_id, effect in enumerate(cart.sfx)
                if any(note.volume for note in effect.notes[: sfx_length(effect)])
            ),
            None,
        )
        if audible is None:
            raise ValueError(f"{cart_path} contains no audible SFX or music")
        start = f"sfx({audible}, 0)"
    (output / "game.lua").write_text(
        f"""function _init()
 {start}
end

function _update()
end

function _draw()
 cls(1)
 print("PICO-8 AUDIO CORPUS", 8, 16, 15)
 print("{cart_path.name}", 8, 32, 18)
end
""",
        encoding="utf-8",
    )


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("cart", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--entry-pattern", type=int, default=0, choices=range(64))
    parser.add_argument("--sfx-id", type=int, choices=range(64))
    args = parser.parse_args()
    write_project(
        args.cart.resolve(), args.output.resolve(), args.entry_pattern, args.sfx_id
    )


if __name__ == "__main__":
    main()
