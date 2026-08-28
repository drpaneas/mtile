#!/usr/bin/env python3
"""Import, packed-render, assemble, and compare the indexed PICO-8 corpus."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import subprocess
import wave
from pathlib import Path
from typing import Any

from compare_pico8_corpus import (
    RATE,
    compare,
    compare_segment_sequence,
    decode,
    extract_features,
)
from import_pico8_audio import write_project


def segment_key(segment: dict[str, Any], limit_samples: int) -> str:
    identity = "\0".join(
        (
            segment["cart"],
            str(segment["entryPattern"]),
            str(segment["sfxID"]),
            str(limit_samples),
        )
    )
    return hashlib.sha256(identity.encode("utf-8")).hexdigest()[:20]


def segment_limits(
    segments: list[dict[str, Any]], reference_active_seconds: float | None
) -> list[int]:
    finite_samples = [
        samples_for_units(int(segment["renderDurationUnits"]))
        if segment.get("renderDurationUnits") is not None
        else (
            None
            if segment["indefinite"]
            else round(float(segment["naturalDurationSeconds"]) * RATE)
        )
        for segment in segments
    ]
    indefinite = [index for index, value in enumerate(finite_samples) if value is None]
    if reference_active_seconds is None:
        indefinite_total = len(indefinite) * 30 * RATE
    else:
        target = round(reference_active_seconds * RATE)
        indefinite_total = max(len(indefinite), target - sum(value or 0 for value in finite_samples))

    limits: list[int] = []
    remaining = indefinite_total
    for index, natural in enumerate(finite_samples):
        if natural is not None:
            if segments[index].get("renderDurationUnits") is not None:
                limits.append(natural)
                continue
            # The renderer stops on the graph's exact duration; one extra second
            # merely keeps the upper bound clear of float-to-sample rounding.
            limits.append(natural + RATE)
            continue
        slots = len([item for item in indefinite if item >= index])
        allocation = max(1, remaining // slots)
        limits.append(allocation)
        remaining -= allocation
    return limits


def samples_for_units(units: int) -> int:
    if units < 0:
        raise ValueError("SFX duration units must be non-negative")
    # Keep corpus boundaries bit-identical to runtime/audio.SFXSamplesForUnits.
    return units * 135 + (units * 3587 + 1837) // 3675


def read_wav(path: Path) -> tuple[wave._wave_params, bytes]:
    with wave.open(str(path), "rb") as source:
        params = source.getparams()
        if (
            params.nchannels != 1
            or params.sampwidth != 2
            or params.framerate != RATE
            or params.comptype != "NONE"
        ):
            raise ValueError(f"{path}: unexpected WAV format {params}")
        return params, source.readframes(params.nframes)


def concatenate_wavs(parts: list[Path], output: Path) -> int:
    payloads = []
    frames = 0
    for part in parts:
        params, payload = read_wav(part)
        frames += params.nframes
        payloads.append(payload)
    output.parent.mkdir(parents=True, exist_ok=True)
    with wave.open(str(output), "wb") as target:
        target.setnchannels(1)
        target.setsampwidth(2)
        target.setframerate(RATE)
        target.writeframes(b"".join(payloads))
    return frames


def build_renderer(root: Path, output: Path) -> Path:
    binary = output / "bin" / "render_mtile_music"
    binary.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(
        ["go", "build", "-o", str(binary), "./scripts/audio/render_mtile_music"],
        cwd=root,
        check=True,
    )
    return binary


def render_segment(
    renderer: Path,
    segment: dict[str, Any],
    limit_samples: int,
    output: Path,
) -> Path:
    key = segment_key(segment, limit_samples)
    project = output / "projects" / key
    rendered = output / "segments" / f"{key}.wav"
    write_project(
        Path(segment["cart"]),
        project,
        int(segment["entryPattern"] or 0),
        segment["sfxID"],
    )
    rendered.parent.mkdir(parents=True, exist_ok=True)
    command = [
        str(renderer),
        "--packed",
        "--max-samples",
        str(limit_samples),
    ]
    if segment["entryPattern"] is not None:
        command.extend(("--entry-pattern", str(segment["entryPattern"])))
    command.extend((str(project), str(rendered)))
    subprocess.run(command, check=True, stdout=subprocess.DEVNULL)
    return rendered


def safe_track_name(index: int, track: dict[str, Any]) -> str:
    album = "".join(
        character.lower() if character.isalnum() else "-"
        for character in track["album"]
    )
    album = recompact(album)
    return f"{index:03d}-{album}-{int(track['track']):02d}"


def recompact(value: str) -> str:
    while "--" in value:
        value = value.replace("--", "-")
    return value.strip("-")


def run(
    root: Path,
    manifest_path: Path,
    output: Path,
    album_filter: str | None = None,
    track_filter: int | None = None,
    limit_tracks: int | None = None,
) -> dict[str, Any]:
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    corpus = Path(manifest["corpus"])
    renderer = build_renderer(root, output)
    records = []
    selected = [
        track
        for track in manifest["tracks"]
        if (album_filter is None or album_filter.casefold() in track["album"].casefold())
        and (track_filter is None or int(track["track"]) == track_filter)
    ]
    if limit_tracks is not None:
        selected = selected[:limit_tracks]

    for index, track in enumerate(selected, 1):
        name = safe_track_name(index, track)
        reference = corpus / track["reference"] if track["reference"] else None
        reference_features = (
            extract_features(decode(reference)) if reference is not None else None
        )
        limits = segment_limits(
            track["segments"],
            reference_features.duration if reference_features is not None else None,
        )
        parts = [
            render_segment(renderer, segment, limit, output)
            for segment, limit in zip(track["segments"], limits, strict=True)
        ]
        candidate = output / "tracks" / f"{name}.wav"
        frames = concatenate_wavs(parts, candidate) if parts else 0
        metrics = None
        segmented_metrics = None
        if reference_features is not None and parts:
            metrics = compare(reference_features, extract_features(decode(candidate)))
            if len(parts) > 1:
                segmented_metrics = compare_segment_sequence(
                    reference_features,
                    [extract_features(decode(part)) for part in parts],
                )
        record = {
            "id": track["id"],
            "reference": reference.as_posix() if reference is not None else None,
            "candidate": candidate.as_posix() if parts else None,
            "candidateSeconds": frames / RATE,
            "segmentLimits": limits,
            "metrics": metrics,
            "segmentedMetrics": segmented_metrics,
        }
        records.append(record)
        print(
            f"[{index}/{len(selected)}] {track['id']}: "
            + (
                f"active-chroma={metrics['meanActiveChromaSimilarity']:.4f} "
                f"active-bands={metrics['meanActiveBandSimilarity']:.4f} "
                f"activity-iou={metrics['activityIntersectionOverUnion']:.4f}"
                if metrics is not None
                else "no comparable cart/reference pair"
            ),
            flush=True,
        )
        report = {
            "version": 1,
            "manifest": manifest_path.resolve().as_posix(),
            "tracks": records,
        }
        (output / "report.json").write_text(
            json.dumps(report, indent=2, ensure_ascii=False) + "\n",
            encoding="utf-8",
        )
    return {"version": 1, "manifest": manifest_path.resolve().as_posix(), "tracks": records}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--album")
    parser.add_argument("--track", type=int)
    parser.add_argument("--limit-tracks", type=int)
    args = parser.parse_args()
    if args.limit_tracks is not None and args.limit_tracks <= 0:
        parser.error("--limit-tracks must be positive")
    run(
        Path(__file__).resolve().parents[2],
        args.manifest.resolve(),
        args.output.resolve(),
        args.album,
        args.track,
        args.limit_tracks,
    )


if __name__ == "__main__":
    main()
