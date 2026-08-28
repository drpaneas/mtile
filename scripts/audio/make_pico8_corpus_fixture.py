#!/usr/bin/env python3
"""Wrap a text PICO-8 audio cart in a deterministic WAV capture harness."""

from __future__ import annotations

import argparse
import re
from pathlib import Path


SECTION = re.compile(r"^__[a-z0-9_]+__$", re.MULTILINE)
MUSIC_LINE = re.compile(r"^([0-9a-fA-F]{2}) ([0-9a-fA-F]{8})$", re.MULTILINE)
SFX_LINE = re.compile(r"^([0-9a-fA-F]{8})([0-9a-fA-F]{160})$", re.MULTILINE)


def capture_lua(
    entry_pattern: int,
    capture_frames: int,
    output_name: str,
    sfx_id: int | None = None,
    channel_mask: int = 15,
) -> str:
    start = (
        f"sfx({sfx_id},0)"
        if sfx_id is not None
        else f"music({entry_pattern},0,{channel_mask})"
    )
    return f"""wait_frames=30
capture_frames={capture_frames}
elapsed_frames=0
started=false
done=false
function _init()
 extcmd("set_filename","{output_name}")
 extcmd("audio_rec")
end
function _update60()
 if done then return end
 if wait_frames>0 then
  wait_frames-=1
 elseif not started then
  {start}
  started=true
 elseif elapsed_frames<capture_frames then
  elapsed_frames+=1
 else
  extcmd("audio_end",1)
  done=true
 end
end
"""


def replace_lua(text: str, lua: str) -> str:
    marker = "__lua__"
    marker_start = text.find(marker)
    if marker_start < 0:
        # PICO-8's text export omits an empty Lua section. Audio-only PNG
        # carts therefore need a new section rather than a replacement.
        first_section = SECTION.search(text)
        insert_at = first_section.start() if first_section is not None else len(text)
        prefix = text[:insert_at]
        if prefix and not prefix.endswith("\n"):
            prefix += "\n"
        suffix = text[insert_at:]
        return prefix + marker + "\n" + lua + suffix
    content_start = text.find("\n", marker_start)
    if content_start < 0:
        content_start = len(text)
    else:
        content_start += 1
    next_section = SECTION.search(text, content_start)
    content_end = next_section.start() if next_section is not None else len(text)
    suffix = text[content_end:]
    if suffix and not suffix.startswith("\n") and not lua.endswith("\n"):
        lua += "\n"
    return text[:content_start] + lua + suffix


def isolate_music_channel(text: str, channel: int) -> str:
    if channel < 0 or channel > 3:
        raise ValueError("music channel must be in 0..3")
    music_marker = text.find("__music__")
    sfx_marker = text.find("__sfx__")
    if music_marker < 0:
        raise ValueError("cart has no __music__ section")
    if sfx_marker < 0:
        raise ValueError("cart has no __sfx__ section")

    music_start = text.find("\n", music_marker) + 1
    music_next = SECTION.search(text, music_start)
    music_end = music_next.start() if music_next is not None else len(text)
    keep: set[int] = set()
    silence: set[int] = set()
    for match in MUSIC_LINE.finditer(text[music_start:music_end]):
        encoded = [
            int(match.group(2)[index : index + 2], 16) for index in range(0, 8, 2)
        ]
        for index, value in enumerate(encoded):
            if value & 0x40:
                continue
            (keep if index == channel else silence).add(value & 0x3F)
    overlap = keep & silence
    if overlap:
        ids = ", ".join(str(value) for value in sorted(overlap))
        raise ValueError(f"cannot isolate shared music SFX IDs: {ids}")

    sfx_start = text.find("\n", sfx_marker) + 1
    sfx_next = SECTION.search(text, sfx_start)
    sfx_end = sfx_next.start() if sfx_next is not None else len(text)
    sfx_id = -1

    def mute_sfx(match: re.Match[str]) -> str:
        nonlocal sfx_id
        sfx_id += 1
        notes = match.group(2)
        if sfx_id not in silence:
            return match.group(0)
        muted = [notes[index : index + 5] for index in range(0, len(notes), 5)]
        muted = [note[:3] + "0" + note[4:] for note in muted]
        return match.group(1) + "".join(muted)

    # Keep every music-row assignment intact: PICO derives pattern duration
    # from those assignments. Muting note volumes isolates the audible channel
    # without changing transport timing, loop selection, or pattern changes.
    sfx = SFX_LINE.sub(mute_sfx, text[sfx_start:sfx_end])
    return text[:sfx_start] + sfx + text[sfx_end:]


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("cart", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--entry-pattern", type=int, default=0, choices=range(64))
    parser.add_argument("--sfx-id", type=int, choices=range(64))
    parser.add_argument("--channel-mask", type=int, default=15, choices=range(1, 16))
    parser.add_argument("--solo-channel", type=int, choices=range(4))
    parser.add_argument("--capture-frames", type=int, required=True)
    parser.add_argument("--output-name", default="pico-corpus")
    args = parser.parse_args()
    if args.capture_frames <= 0:
        parser.error("--capture-frames must be positive")
    if not re.fullmatch(r"[A-Za-z0-9._-]+", args.output_name):
        parser.error("--output-name may contain only letters, digits, '.', '_' and '-'")
    text = args.cart.read_text(encoding="utf-8")
    wrapped = replace_lua(
        text,
        capture_lua(
            args.entry_pattern,
            args.capture_frames,
            args.output_name,
            args.sfx_id,
            args.channel_mask,
        ),
    )
    if args.solo_channel is not None:
        wrapped = isolate_music_channel(wrapped, args.solo_channel)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(wrapped, encoding="utf-8")


if __name__ == "__main__":
    main()
