#!/usr/bin/env python3
"""Replace a text cart's Lua with a deterministic one-SFX audio recorder."""

from __future__ import annotations

import argparse
from pathlib import Path


def replace_lua(text: str, call: str, frames: int) -> str:
    lines = text.splitlines()
    try:
        lua = lines.index("__lua__")
    except ValueError as error:
        raise ValueError("source cart has no __lua__ section") from error
    section_end = next(
        (index for index in range(lua + 1, len(lines)) if lines[index].startswith("__") and lines[index].endswith("__")),
        len(lines),
    )
    recorder = [
        "wait_frames=30",
        "done=false",
        "function _init()",
        ' extcmd("set_filename","pico-probe")',
        ' extcmd("audio_rec")',
        "end",
        "function _update60()",
        " if done then return end",
        " if wait_frames>0 then",
        "  wait_frames-=1",
        " elseif wait_frames==0 then",
        f"  {call}",
        "  wait_frames=-1",
        f" elseif wait_frames>-{frames} then",
        "  wait_frames-=1",
        " else",
        '  extcmd("audio_end",1)',
        "  done=true",
        " end",
        "end",
    ]
    return "\n".join(lines[: lua + 1] + recorder + lines[section_end:]) + "\n"


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--sfx", type=int, choices=range(64))
    source.add_argument("--music", type=int, choices=range(64))
    parser.add_argument("--offset", type=int, default=0, choices=range(32))
    parser.add_argument("--frames", type=int, default=180)
    args = parser.parse_args()
    if args.frames < 1:
        parser.error("--frames must be positive")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(
        replace_lua(
            args.source.read_text(encoding="utf-8"),
            f"music({args.music})" if args.music is not None else f"sfx({args.sfx},0,{args.offset})",
            args.frames,
        ),
        encoding="utf-8",
    )


if __name__ == "__main__":
    main()
