#!/usr/bin/env python3
"""Index every playable PICO-8 audio cart and supplied MP3 reference."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
from pathlib import Path

from import_pico8_audio import pattern_duration_units, pattern_has_audio, pattern_next
from pico8_cart import (
    CartAudio,
    acyclic_depth,
    custom_instrument_graph,
    graph_cycles,
    load_cart,
)


MUSIC_CALL = re.compile(r"\bmusic\s*\(\s*(\d+)")
TRACK_NUMBER = re.compile(r"^(\d{1,2})(?:\D|$)")


def text_cart(path: Path) -> bool:
    if not path.is_file() or path.suffix.lower() in {".mp3", ".png", ".gif"}:
        return False
    try:
        prefix = path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return False
    return "__sfx__" in prefix and "__music__" in prefix


def cart_sources(
    corpus: Path, converted_dir: Path
) -> list[tuple[Path, Path, str]]:
    records: list[tuple[Path, Path, str]] = []
    converted_sources: set[str] = set()
    manifest_path = converted_dir / "manifest.json"
    if manifest_path.is_file():
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        for record in manifest.get("carts", []):
            source = Path(str(record["source"]))
            cart = converted_dir / str(record["cart"])
            if not cart.is_file():
                raise FileNotFoundError(f"converted cart is missing: {cart}")
            converted_sources.add(source.as_posix())
            records.append((source, cart.resolve(), "png"))
    for path in corpus.rglob("*"):
        if not text_cart(path):
            continue
        relative = path.relative_to(corpus)
        if relative.as_posix() in converted_sources:
            continue
        records.append((relative, path.resolve(), "text"))
    records.sort(key=lambda item: item[0].as_posix().casefold())
    return records


def active_patterns(cart: CartAudio) -> dict[int, object]:
    return {
        pattern_id: pattern
        for pattern_id, pattern in enumerate(cart.music)
        if pattern_has_audio(pattern)
    }


def graph_entry_patterns(cart: CartAudio) -> list[int]:
    patterns = active_patterns(cart)
    if not patterns:
        return []
    next_by_id = {
        pattern_id: pattern_next(patterns, pattern_id) for pattern_id in patterns
    }
    incoming = {
        target
        for target in next_by_id.values()
        if target >= 0 and target in patterns
    }
    entries = sorted(set(patterns) - incoming)

    def reachable(start: int) -> set[int]:
        seen: set[int] = set()
        current = start
        while current in patterns and current not in seen:
            seen.add(current)
            current = next_by_id[current]
        return seen

    covered: set[int] = set()
    for entry in entries:
        covered.update(reachable(entry))
    # Pure cycles have no in-degree-zero root. Add the lowest stable pattern
    # from each uncovered component so every audible graph is represented.
    while covered != set(patterns):
        entry = min(set(patterns) - covered)
        entries.append(entry)
        covered.update(reachable(entry))
    return sorted(entries)


def entry_timeline(cart: CartAudio, entry: int) -> dict[str, object]:
    patterns = active_patterns(cart)
    sequence: list[int] = []
    seen: dict[int, int] = {}
    current = entry
    duration_units = 0
    indefinite = False
    cycle: list[int] = []
    while current in patterns:
        if current in seen:
            cycle = sequence[seen[current] :]
            indefinite = True
            break
        seen[current] = len(sequence)
        sequence.append(current)
        units = pattern_duration_units(cart, patterns[current])
        if units == 0:
            indefinite = True
        elif not indefinite:
            duration_units += units
        current = pattern_next(patterns, current)
        if current < 0:
            break
    return {
        "entryPattern": entry,
        "patterns": sequence,
        "cycle": cycle,
        "durationUnits": None if indefinite else duration_units,
        "durationSeconds": None if indefinite else duration_units * 499712 / 3675 / 16384,
        "indefinite": indefinite,
    }


def literal_music_calls(path: Path) -> list[int]:
    text = path.read_text(encoding="utf-8")
    if "__lua__" not in text:
        return []
    lua = text.split("__lua__", 1)[1].split("__gfx__", 1)[0]
    return sorted({int(match.group(1)) for match in MUSIC_CALL.finditer(lua)})


def first_audible_sfx(cart: CartAudio) -> int | None:
    for sfx_id, effect in enumerate(cart.sfx):
        if any(note.volume for note in effect.notes):
            return sfx_id
    return None


def cart_record(corpus: Path, source: Path, cart_path: Path, kind: str) -> dict[str, object]:
    cart = load_cart(cart_path)
    graph = custom_instrument_graph(cart)
    entries = graph_entry_patterns(cart)
    audible_custom = sum(
        note.volume > 0 and note.custom_instrument is not None
        for effect in cart.sfx
        for note in effect.notes
    )
    return {
        "source": source.as_posix(),
        "cart": cart_path.as_posix(),
        "sourceKind": kind,
        "album": source.parts[0] if source.parts else "",
        "literalMusicCalls": literal_music_calls(cart_path),
        "entries": [entry_timeline(cart, entry) for entry in entries],
        "fallbackSFX": first_audible_sfx(cart) if not entries else None,
        "sfxCount": len(cart.sfx),
        "musicPatternCount": len(cart.music),
        "audibleCustomNotes": audible_custom,
        "customInstrumentDepth": acyclic_depth(graph),
        "customInstrumentCycles": graph_cycles(graph),
    }


def media_duration(path: Path) -> float:
    result = subprocess.run(
        [
            "ffprobe",
            "-v",
            "error",
            "-show_entries",
            "format=duration",
            "-of",
            "default=noprint_wrappers=1:nokey=1",
            str(path),
        ],
        check=True,
        capture_output=True,
        text=True,
    )
    return float(result.stdout.strip())


def mp3_records(corpus: Path) -> list[dict[str, object]]:
    records = []
    for path in sorted(corpus.rglob("*.mp3"), key=lambda item: item.as_posix().casefold()):
        relative = path.relative_to(corpus)
        match = TRACK_NUMBER.match(path.name)
        records.append(
            {
                "source": relative.as_posix(),
                "album": relative.parts[0] if relative.parts else "",
                "track": int(match.group(1)) if match else None,
                "durationSeconds": media_duration(path),
            }
        )
    return records


def inventory(corpus: Path, converted_dir: Path) -> dict[str, object]:
    carts = [
        cart_record(corpus, source, cart, kind)
        for source, cart, kind in cart_sources(corpus, converted_dir)
    ]
    references = mp3_records(corpus)
    return {
        "version": 1,
        "corpus": corpus.resolve().as_posix(),
        "convertedCarts": converted_dir.resolve().as_posix(),
        "summary": {
            "carts": len(carts),
            "textCarts": sum(record["sourceKind"] == "text" for record in carts),
            "pngCarts": sum(record["sourceKind"] == "png" for record in carts),
            "entryPoints": sum(len(record["entries"]) for record in carts),
            "mp3References": len(references),
            "customCarts": sum(record["audibleCustomNotes"] > 0 for record in carts),
        },
        "carts": carts,
        "references": references,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("corpus", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--converted-dir", type=Path, required=True)
    args = parser.parse_args()
    result = inventory(args.corpus.resolve(), args.converted_dir.resolve())
    encoded = json.dumps(result, indent=2, ensure_ascii=False) + "\n"
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(encoded, encoding="utf-8")
    print(json.dumps(result["summary"], indent=2))


if __name__ == "__main__":
    main()
