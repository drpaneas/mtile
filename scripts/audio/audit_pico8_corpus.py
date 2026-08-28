#!/usr/bin/env python3
"""Import and export every distinct PICO-8 cart entry in the paired corpus."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any

from compare_pico8_corpus import RATE
from run_pico8_corpus import build_renderer, read_wav, render_segment


def entry_identity(segment: dict[str, Any]) -> tuple[str, int | None, int | None]:
    return (
        str(segment["cartSource"]),
        segment.get("entryPattern"),
        segment.get("sfxID"),
    )


def collect_entries(manifest: dict[str, Any]) -> list[dict[str, Any]]:
    entries: dict[tuple[str, int | None, int | None], dict[str, Any]] = {}
    for track in manifest["tracks"]:
        for classification, key in (
            ("selected", "segments"),
            ("excluded-alternate", "excludedSegments"),
        ):
            for segment in track[key]:
                identity = entry_identity(segment)
                record = entries.setdefault(
                    identity,
                    {
                        "segment": segment,
                        "classifications": set(),
                        "tracks": set(),
                    },
                )
                record["classifications"].add(classification)
                record["tracks"].add(track["id"])
    result = []
    for identity in sorted(
        entries,
        key=lambda value: (
            value[0].casefold(),
            -1 if value[1] is None else value[1],
            -1 if value[2] is None else value[2],
        ),
    ):
        record = entries[identity]
        result.append(
            {
                "segment": record["segment"],
                "classifications": sorted(record["classifications"]),
                "tracks": sorted(record["tracks"], key=str.casefold),
            }
        )
    return result


def render_limit(segment: dict[str, Any], indefinite_seconds: int) -> int:
    natural = segment.get("naturalDurationSeconds")
    if natural is None or bool(segment.get("indefinite", False)):
        return indefinite_seconds * RATE
    # The renderer stops when finite music becomes inactive. One second of
    # headroom protects the exact rational endpoint from float conversion.
    return round(float(natural) * RATE) + RATE


def run(
    root: Path,
    manifest_path: Path,
    output: Path,
    indefinite_seconds: int,
    limit_entries: int | None = None,
) -> dict[str, Any]:
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    entries = collect_entries(manifest)
    if limit_entries is not None:
        entries = entries[:limit_entries]
    renderer = build_renderer(root, output)
    records = []
    for index, entry in enumerate(entries, 1):
        segment = entry["segment"]
        limit = render_limit(segment, indefinite_seconds)
        candidate = render_segment(renderer, segment, limit, output)
        params, _ = read_wav(candidate)
        record = {
            "cartSource": segment["cartSource"],
            "cart": segment["cart"],
            "sourceKind": segment["sourceKind"],
            "entryPattern": segment.get("entryPattern"),
            "sfxID": segment.get("sfxID"),
            "naturalDurationSeconds": segment.get("naturalDurationSeconds"),
            "indefinite": bool(segment.get("indefinite", False)),
            "classifications": entry["classifications"],
            "tracks": entry["tracks"],
            "candidate": candidate.as_posix(),
            "candidateSeconds": params.nframes / RATE,
            "limitSamples": limit,
        }
        records.append(record)
        print(
            f"[{index}/{len(entries)}] {segment['cartSource']} "
            f"entry={segment.get('entryPattern')} sfx={segment.get('sfxID')} "
            f"seconds={record['candidateSeconds']:.3f}",
            flush=True,
        )
        report = make_report(manifest_path, manifest, records, len(entries))
        (output / "audit-report.json").write_text(
            json.dumps(report, indent=2, ensure_ascii=False) + "\n",
            encoding="utf-8",
        )
    return make_report(manifest_path, manifest, records, len(entries))


def make_report(
    manifest_path: Path,
    manifest: dict[str, Any],
    records: list[dict[str, Any]],
    expected_entries: int,
) -> dict[str, Any]:
    carts = {record["cartSource"] for record in records}
    expected_carts = int(manifest["summary"]["carts"])
    complete = len(records) == expected_entries and len(carts) == expected_carts
    return {
        "version": 1,
        "manifest": manifest_path.resolve().as_posix(),
        "summary": {
            "expectedCarts": expected_carts,
            "exportedCarts": len(carts),
            "expectedEntries": expected_entries,
            "exportedEntries": len(records),
            "selectedEntries": sum(
                "selected" in record["classifications"] for record in records
            ),
            "excludedAlternateEntries": sum(
                "excluded-alternate" in record["classifications"]
                for record in records
            ),
            "complete": complete,
        },
        "entries": records,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--indefinite-seconds", type=int, default=30)
    parser.add_argument("--limit-entries", type=int)
    args = parser.parse_args()
    if args.indefinite_seconds <= 0 or args.indefinite_seconds > 1_800:
        parser.error("--indefinite-seconds must be in 1..1800")
    if args.limit_entries is not None and args.limit_entries <= 0:
        parser.error("--limit-entries must be positive")
    run(
        Path(__file__).resolve().parents[2],
        args.manifest.resolve(),
        args.output.resolve(),
        args.indefinite_seconds,
        args.limit_entries,
    )


if __name__ == "__main__":
    main()
