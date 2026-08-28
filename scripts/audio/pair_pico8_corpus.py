#!/usr/bin/env python3
"""Pair the PICO-8 corpus carts with album tracks and supplied references."""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path
from typing import Any


LEADING_TRACK = re.compile(r"^(\d{1,2})(?:\D|$)")
INVENTION = re.compile(r"\binvention(?: no\.)?\s*(\d{1,2})", re.IGNORECASE)
ZELDA = re.compile(r"\bzelda\s+(\d{1,2})(?:\D|$)", re.IGNORECASE)

DOUBLE_DRAGON = {
    "ddintro": 1,
    "dd1": 2,
    "ddboss": 3,
    "ddcomplete": 4,
    "dd2": 5,
    "dd3": 6,
    "dd4": 7,
    "ddbossfinal": 8,
    "ddend": 9,
}

SONIC = {
    "sthh title": 1,
    "sthh green hill zone": 2,
    "sthh marble zone": 3,
    "sthh spring yard zone": 4,
    "sthh labyrinth zone": 5,
    "sthh star light zone": 6,
    "sthh scrap brain zone": 7,
    "sthh boss": 8,
    "sthh final zone": 9,
    "sthh special stage": 10,
    "sthh stage clear": 11,
    "sthh 1up": 12,
    "sthh emerald": 13,
    "sthh power up": 14,
    "sthh drowning": 15,
    "sthh continue": 16,
    "sthh game over": 17,
    "sthh all clear": 18,
    "sthh staff roll": 19,
    "sthh staff roll2": 19,
}

LAST_BOT = {
    "prelude": 1,
    "fallout": 2,
    "j-taylor made": 3,
    "journey to far lands": 4,
    "ableton": 5,
    "flashback robot": 6,
    "the root of sin": 7,
    "planet fever": 8,
    "reflection": 9,
}

TUNES_ENTRY_ORDER = {
    "Pico-8 Tunes Vol. 1 (2017)": [0, 6, 14, 18, 24, 30, 36, 42, 46, 48, 55, 59],
    "Pico-8 Tunes Vol. 2 (2019)": [6, 56, 21, 0, 31, 43, 49, 40, 39, 13],
}

# Slipways is an interactive cart whose album export changes between looping
# graph roots at authored pattern boundaries.  Static root discovery finds the
# available sections but cannot infer this performance order or how many loop
# cycles were recorded.  Durations are exact PICO SFX-speed units recovered
# from the reference's section boundaries.
SLIPWAYS_ARRANGEMENT = (
    (0, 27_520),
    (3, 36_224),
    (6, 16_384),
    (8, 20_480),
    (11, 32_768),
    (15, 20_480),
    (15, 16_384),
    (20, 65_536),
)


def cart_stem(source: str) -> str:
    name = Path(source).name
    for suffix in (".p8.png", ".p8", ".png"):
        if name.casefold().endswith(suffix):
            return name[: -len(suffix)].strip().casefold()
    return name.strip().casefold()


def without_part_suffix(stem: str) -> str:
    return re.sub(r"(?:\s+)?\d+[a-z]?$", "", stem).strip()


def natural_key(value: str) -> tuple[object, ...]:
    return tuple(
        int(part) if part.isdigit() else part.casefold()
        for part in re.split(r"(\d+)", value)
    )


def cart_sort_key(source: str) -> tuple[tuple[object, ...], tuple[object, ...]]:
    path = Path(source)
    return natural_key(path.parent.as_posix()), natural_key(cart_stem(source))


def cart_track(record: dict[str, Any]) -> int | None:
    album = record["album"]
    stem = cart_stem(record["source"])
    if album.startswith("Pico-8 Tunes Vol."):
        return None
    if (
        album.startswith("Celeste ")
        or album.startswith("Counterpoint")
        or album.startswith("Pico Mix")
    ):
        match = LEADING_TRACK.match(stem)
        return int(match.group(1)) if match else None
    if album.startswith("J"):
        match = INVENTION.search(stem)
        return int(match.group(1)) if match else None
    if album.startswith("Pico & Chill"):
        match = re.fullmatch(r"ambience([1-4])", stem)
        if match:
            return int(match.group(1)) * 2 - 1
        match = re.fullmatch(r"picochill([1-3])[a-z]?", stem)
        return int(match.group(1)) * 2 if match else None
    if album.startswith("Double Dragon"):
        return DOUBLE_DRAGON.get(stem)
    if album.startswith("Sonic the Hedgehog"):
        return SONIC.get(stem)
    if album.startswith("The Last Bot"):
        return LAST_BOT.get(without_part_suffix(stem))
    if album.startswith("The Legend of Zelda"):
        match = ZELDA.search(stem)
        return int(match.group(1)) if match else None
    return None


def reference_track(record: dict[str, Any]) -> int | None:
    if record["track"] is not None:
        return int(record["track"])
    match = INVENTION.search(Path(record["source"]).name)
    return int(match.group(1)) if match else None


def chosen_entry(record: dict[str, Any]) -> dict[str, Any]:
    entries = record["entries"]
    if not entries:
        if record["fallbackSFX"] is None:
            raise ValueError(f'{record["source"]}: no audible entry or fallback SFX')
        return {
            "entryPattern": None,
            "sfxID": int(record["fallbackSFX"]),
            "durationSeconds": None,
            "indefinite": False,
        }
    by_id = {int(entry["entryPattern"]): entry for entry in entries}
    called = [
        by_id[int(entry)]
        for entry in record["literalMusicCalls"]
        if int(entry) in by_id
    ]
    return called[0] if called else entries[0]


def segment(record: dict[str, Any], entry: dict[str, Any]) -> dict[str, Any]:
    return {
        "cartSource": record["source"],
        "cart": record["cart"],
        "sourceKind": record["sourceKind"],
        "entryPattern": entry.get("entryPattern"),
        "sfxID": entry.get("sfxID"),
        "naturalDurationSeconds": entry.get("durationSeconds"),
        "indefinite": bool(entry.get("indefinite", False)),
        "audibleCustomNotes": int(record["audibleCustomNotes"]),
        "customInstrumentDepth": int(record["customInstrumentDepth"]),
    }


def select_album_segments(
    primary: list[dict[str, Any]],
    extras: list[dict[str, Any]],
    reference_seconds: float | None,
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    candidates = primary + extras
    if (
        reference_seconds is None
        or not primary
        or any(item["indefinite"] for item in candidates)
        or len(candidates) > 20
    ):
        return primary, extras

    primary_duration = sum(
        float(item["naturalDurationSeconds"]) for item in primary
    )
    if abs(primary_duration - reference_seconds) <= max(
        15.0, reference_seconds * 0.1
    ):
        return primary, extras

    # Audio-only album carts sometimes contain both multipart continuations
    # and alternate/unused takes. The MP3 duration identifies the intended
    # subset without guessing from suffixes: always retain the first authored
    # segment, then minimize duration error with the fewest selection edits.
    best_indices = set(range(len(primary)))
    best_score = (
        abs(primary_duration - reference_seconds),
        0,
    )
    optional = len(candidates) - 1
    for mask in range(1 << optional):
        indices = {0}
        for bit in range(optional):
            if mask & (1 << bit):
                indices.add(bit + 1)
        duration = sum(
            float(candidates[index]["naturalDurationSeconds"]) for index in indices
        )
        edits = sum(index not in indices for index in range(len(primary))) + sum(
            index in indices for index in range(len(primary), len(candidates))
        )
        score = (abs(duration - reference_seconds), edits)
        if score < best_score:
            best_score = score
            best_indices = indices
    selected = [item for index, item in enumerate(candidates) if index in best_indices]
    excluded = [item for index, item in enumerate(candidates) if index not in best_indices]
    return selected, excluded


def apply_known_arrangement(
    album: str,
    track: int,
    selected: list[dict[str, Any]],
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    if not (album == "Pico Mix (2018)" and track == 13):
        return selected, []
    by_entry = {int(item["entryPattern"]): item for item in selected}
    expected = {0, 3, 6, 8, 11, 15, 20, 52}
    if set(by_entry) != expected:
        raise ValueError(
            f"{album}/{track:02d}: unexpected Slipways entry set {sorted(by_entry)}"
        )
    arranged = []
    used = set()
    for entry, duration_units in SLIPWAYS_ARRANGEMENT:
        item = dict(by_entry[entry])
        item["renderDurationUnits"] = duration_units
        arranged.append(item)
        used.add(entry)
    return arranged, [by_entry[entry] for entry in sorted(set(by_entry) - used)]


def pair(inventory: dict[str, Any]) -> dict[str, Any]:
    refs: dict[tuple[str, int], dict[str, Any]] = {}
    for record in inventory["references"]:
        track = reference_track(record)
        if track is None:
            raise ValueError(f'cannot determine reference track: {record["source"]}')
        key = (record["album"], track)
        if key in refs:
            raise ValueError(f"duplicate reference for {key}")
        refs[key] = record

    grouped: dict[tuple[str, int], list[dict[str, Any]]] = {}
    extras: dict[tuple[str, int], list[dict[str, Any]]] = {}
    covered_carts: set[str] = set()
    for record in inventory["carts"]:
        album = record["album"]
        if album.startswith("Pico-8 Tunes Vol."):
            by_id = {
                int(entry["entryPattern"]): entry for entry in record["entries"]
            }
            entry_order = TUNES_ENTRY_ORDER.get(album)
            if entry_order is None or set(entry_order) != set(by_id):
                raise ValueError(f"{record['source']}: unexpected Tunes entry set")
            for track, entry_id in enumerate(entry_order, 1):
                entry = by_id[entry_id]
                grouped.setdefault((album, track), []).append(segment(record, entry))
            covered_carts.add(record["source"])
            continue
        track = cart_track(record)
        if track is None:
            raise ValueError(f'cannot determine cart track: {record["source"]}')
        entries = record["entries"]
        if album.startswith("Pico Mix") and track == 13:
            selected = entries
        else:
            selected = [chosen_entry(record)]
        authored_alternate = (
            album.startswith("The Last Bot")
            and cart_stem(record["source"]) == "planet fever8"
        )
        for entry in selected:
            destination = extras if authored_alternate else grouped
            destination.setdefault((album, track), []).append(segment(record, entry))
        selected_entries = {entry.get("entryPattern") for entry in selected}
        for entry in entries:
            if entry.get("entryPattern") not in selected_entries:
                extras.setdefault((album, track), []).append(segment(record, entry))
        covered_carts.add(record["source"])

    for segments in list(grouped.values()) + list(extras.values()):
        segments.sort(
            key=lambda item: (
                cart_sort_key(item["cartSource"]),
                -1
                if item["entryPattern"] is None
                else int(item["entryPattern"]),
            )
        )

    all_keys = sorted(
        set(grouped) | set(refs), key=lambda item: (item[0].casefold(), item[1])
    )
    tracks = []
    for album, track in all_keys:
        reference = refs.get((album, track))
        selected, excluded = select_album_segments(
            grouped.get((album, track), []),
            extras.get((album, track), []),
            float(reference["durationSeconds"]) if reference else None,
        )
        selected, arrangement_excluded = apply_known_arrangement(
            album, track, selected
        )
        excluded.extend(arrangement_excluded)
        selected.sort(key=lambda item: (cart_sort_key(item["cartSource"]), item["entryPattern"] or 0))
        excluded.sort(key=lambda item: (cart_sort_key(item["cartSource"]), item["entryPattern"] or 0))
        tracks.append(
            {
                "id": f"{album}/{track:02d}",
                "album": album,
                "track": track,
                "reference": reference["source"] if reference else None,
                "referenceDurationSeconds": reference["durationSeconds"]
                if reference
                else None,
                "segments": selected,
                "excludedSegments": excluded,
            }
        )

    all_carts = {record["source"] for record in inventory["carts"]}
    paired_refs = sum(
        track["reference"] is not None and bool(track["segments"])
        for track in tracks
    )
    return {
        "version": 1,
        "corpus": inventory["corpus"],
        "convertedCarts": inventory["convertedCarts"],
        "summary": {
            "tracks": len(tracks),
            "carts": len(all_carts),
            "coveredCarts": len(covered_carts),
            "segmentEntries": sum(len(track["segments"]) for track in tracks),
            "excludedSegmentEntries": sum(
                len(track["excludedSegments"]) for track in tracks
            ),
            "references": len(refs),
            "pairedReferences": paired_refs,
            "unpairedReferences": len(refs) - paired_refs,
            "cartOnlyTracks": sum(track["reference"] is None for track in tracks),
        },
        "uncoveredCarts": sorted(all_carts - covered_carts, key=str.casefold),
        "tracks": tracks,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("inventory", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    result = pair(json.loads(args.inventory.read_text(encoding="utf-8")))
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(
        json.dumps(result, indent=2, ensure_ascii=False) + "\n", encoding="utf-8"
    )
    print(json.dumps(result["summary"], indent=2))


if __name__ == "__main__":
    main()
