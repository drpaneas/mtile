#!/usr/bin/env python3
"""Parse the audio sections of a text PICO-8 cartridge.

The module deliberately preserves PICO-8's numeric representation.  Corpus
import and conformance tools can therefore make policy decisions (for example,
how to group music patterns) without first translating away custom-instrument
references or pattern flags.
"""

from __future__ import annotations

import argparse
import json
from dataclasses import asdict, dataclass
from pathlib import Path


MAX_SFX = 64
MAX_SFX_NOTES = 32
MAX_MUSIC_PATTERNS = 64


@dataclass(frozen=True)
class Note:
    pitch: int
    waveform: int
    volume: int
    effect: int

    @property
    def custom_instrument(self) -> int | None:
        return self.waveform - 8 if self.waveform >= 8 else None


@dataclass(frozen=True)
class SFX:
    filters: int
    speed: int
    loop_start: int
    loop_end: int
    notes: tuple[Note, ...]


@dataclass(frozen=True)
class MusicPattern:
    flags: int
    channels: tuple[int | None, int | None, int | None, int | None]


@dataclass(frozen=True)
class CartAudio:
    version: int | None
    sfx: tuple[SFX, ...]
    music: tuple[MusicPattern, ...]


def sections(text: str) -> dict[str, list[str]]:
    result: dict[str, list[str]] = {}
    current: str | None = None
    for raw_line in text.splitlines():
        line = raw_line.rstrip("\r")
        if len(line) >= 4 and line.startswith("__") and line.endswith("__"):
            current = line[2:-2]
            result.setdefault(current, [])
        elif current is not None:
            result[current].append(line)
    return result


def parse_sfx_line(line: str, index: int) -> SFX:
    if len(line) != 168:
        raise ValueError(f"SFX {index}: expected 168 hex characters, found {len(line)}")
    try:
        int(line, 16)
    except ValueError as error:
        raise ValueError(f"SFX {index}: contains non-hexadecimal data") from error
    notes = []
    for note_index in range(MAX_SFX_NOTES):
        offset = 8 + note_index * 5
        encoded = line[offset : offset + 5]
        notes.append(
            Note(
                pitch=int(encoded[0:2], 16),
                waveform=int(encoded[2], 16),
                volume=int(encoded[3], 16),
                effect=int(encoded[4], 16),
            )
        )
    return SFX(
        filters=int(line[0:2], 16) & 0xFE,
        speed=int(line[2:4], 16),
        loop_start=int(line[4:6], 16),
        loop_end=int(line[6:8], 16),
        notes=tuple(notes),
    )


def parse_music_line(line: str, index: int) -> MusicPattern:
    if len(line) != 11 or line[2] != " ":
        raise ValueError(f"music pattern {index}: expected 'ff cccccccc', found {line!r}")
    try:
        flags = int(line[0:2], 16)
        encoded = bytes.fromhex(line[3:])
    except ValueError as error:
        raise ValueError(f"music pattern {index}: contains non-hexadecimal data") from error
    if len(encoded) != 4:
        raise ValueError(f"music pattern {index}: expected four channels")
    channels = tuple(None if value & 0x40 else value & 0x3F for value in encoded)
    return MusicPattern(flags=flags, channels=channels)  # type: ignore[arg-type]


def parse_cart_text(text: str) -> CartAudio:
    version = None
    for line in text.splitlines()[:4]:
        if line.startswith("version "):
            try:
                version = int(line.removeprefix("version "))
            except ValueError as error:
                raise ValueError(f"invalid cartridge version line: {line!r}") from error
    parsed_sections = sections(text)
    sfx_lines = [line for line in parsed_sections.get("sfx", []) if line]
    music_lines = [line for line in parsed_sections.get("music", []) if line]
    if len(sfx_lines) > MAX_SFX:
        raise ValueError(f"cartridge has {len(sfx_lines)} SFX (maximum {MAX_SFX})")
    if len(music_lines) > MAX_MUSIC_PATTERNS:
        raise ValueError(
            f"cartridge has {len(music_lines)} music patterns (maximum {MAX_MUSIC_PATTERNS})"
        )
    return CartAudio(
        version=version,
        sfx=tuple(parse_sfx_line(line, index) for index, line in enumerate(sfx_lines)),
        music=tuple(
            parse_music_line(line, index) for index, line in enumerate(music_lines)
        ),
    )


def load_cart(path: Path) -> CartAudio:
    return parse_cart_text(path.read_text(encoding="utf-8"))


def custom_instrument_graph(cart: CartAudio) -> dict[int, set[int]]:
    graph = {instrument: set() for instrument in range(min(8, len(cart.sfx)))}
    for instrument in graph:
        for note in cart.sfx[instrument].notes:
            if note.volume and note.custom_instrument is not None:
                graph[instrument].add(note.custom_instrument)
    return graph


def graph_cycles(graph: dict[int, set[int]]) -> list[list[int]]:
    cycles: set[tuple[int, ...]] = set()
    path: list[int] = []
    active: set[int] = set()

    def visit(node: int) -> None:
        if node in active:
            start = path.index(node)
            cycle = path[start:]
            rotations = [tuple(cycle[index:] + cycle[:index]) for index in range(len(cycle))]
            cycles.add(min(rotations))
            return
        if node in path or node not in graph:
            return
        path.append(node)
        active.add(node)
        for target in sorted(graph[node]):
            visit(target)
        active.remove(node)
        path.pop()

    for node in graph:
        visit(node)
    return [list(cycle) for cycle in sorted(cycles)]


def acyclic_depth(graph: dict[int, set[int]]) -> int | None:
    if graph_cycles(graph):
        return None
    memo: dict[int, int] = {}

    def depth(node: int) -> int:
        if node not in graph or not graph[node]:
            return 1
        if node not in memo:
            memo[node] = 1 + max(depth(target) for target in graph[node])
        return memo[node]

    return max((depth(node) for node in graph), default=0)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("cart", type=Path)
    args = parser.parse_args()
    cart = load_cart(args.cart)
    graph = custom_instrument_graph(cart)
    audible_custom_notes = sum(
        note.volume > 0 and note.custom_instrument is not None
        for sfx in cart.sfx
        for note in sfx.notes
    )
    print(
        json.dumps(
            {
                "version": cart.version,
                "sfx": len(cart.sfx),
                "music": len(cart.music),
                "audibleCustomNotes": audible_custom_notes,
                "customInstrumentGraph": {
                    str(node): sorted(edges) for node, edges in graph.items() if edges
                },
                "customInstrumentCycles": graph_cycles(graph),
                "customInstrumentDepth": acyclic_depth(graph),
                "firstSFX": asdict(cart.sfx[0]) if cart.sfx else None,
            },
            indent=2,
        )
    )


if __name__ == "__main__":
    main()
