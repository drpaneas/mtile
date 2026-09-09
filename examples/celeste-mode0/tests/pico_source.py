"""PICO source decoding for the Celeste port's PSG conversion checks.

This is port-specific tooling, not a Mtile asset format or runtime engine.
"""
from __future__ import annotations
from dataclasses import dataclass
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


def sfx_length(effect: SFX) -> int:
    if effect.loop_start > 0 and effect.loop_end == 0:
        return min(effect.loop_start, 32)
    return 32

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

