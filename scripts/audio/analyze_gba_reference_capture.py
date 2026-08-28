#!/usr/bin/env python3
"""Validate deterministic mGBA captures from the native audio truth ROMs."""

from __future__ import annotations

import argparse
import math
import struct
import wave
from pathlib import Path


CASE_COUNTS = {
    "ch1": 12,
    "ch2": 10,
    "ch3": 10,
    "ch4": 10,
    "fifoa": 6,
    "fifob": 6,
    "all": 2,
}

CASE_SECONDS = 2

# Sweep, envelope, and hardware-length cases deliberately change or end during
# their two-second slot. They are checked near the trigger instead of being
# required to remain audible in the settled middle of the slot.
DYNAMIC_CASES = {
    "ch1": {4, 5, 6, 7, 8, 9},
    "ch2": {4, 5, 6},
    "ch3": {8},
    "ch4": {5, 6, 7},
}

SILENT_CASES = {"ch3": {9}}


def rms(values: list[int]) -> float:
    if not values:
        return 0.0
    # A stopped GBA wave channel can hold its last DAC sample, so an emulator
    # capture may contain a non-zero but perfectly constant DC level. DC is
    # inaudible and must not be mistaken for an active tone. Measure the RMS of
    # the mean-centred signal (equivalent to its standard deviation) instead.
    mean = sum(values) / len(values)
    return math.sqrt(sum((value - mean) ** 2 for value in values) / len(values))


def load_stereo(path: Path) -> tuple[int, list[int], list[int]]:
    with wave.open(str(path), "rb") as source:
        if source.getnchannels() != 2 or source.getsampwidth() != 2:
            raise ValueError("capture must be stereo signed 16-bit PCM")
        rate = source.getframerate()
        frames = source.readframes(source.getnframes())
    samples = struct.unpack(f"<{len(frames) // 2}h", frames)
    return rate, list(samples[0::2]), list(samples[1::2])


def settled_segments(
    left: list[int], right: list[int], count: int, rate: int
) -> list[tuple[float, float]]:
    segment = rate * CASE_SECONDS
    if min(len(left), len(right)) < segment * count:
        raise ValueError("capture is shorter than its case table")
    out: list[tuple[float, float]] = []
    for index in range(count):
        start = index * segment + segment // 5
        end = (index + 1) * segment - segment // 10
        out.append((rms(left[start:end]), rms(right[start:end])))
    return out


def require_audible(kind: str, segments: list[tuple[float, float]]) -> None:
    silent_index = 5 if kind in ("fifoa", "fifob") else -1
    for index, (left, right) in enumerate(segments):
        peak = max(left, right)
        if index == silent_index or index in SILENT_CASES.get(kind, set()):
            if peak > 64:
                raise ValueError(
                    f"{kind} case {index} is not silent: rms={left:.1f}/{right:.1f}"
                )
        elif index in DYNAMIC_CASES.get(kind, set()):
            continue
        elif peak < 128:
            raise ValueError(
                f"{kind} case {index} is unexpectedly silent: rms={left:.1f}/{right:.1f}"
            )


def case_window_rms(
    left: list[int],
    right: list[int],
    rate: int,
    index: int,
    start_fraction: float,
    end_fraction: float,
) -> tuple[float, float]:
    segment = rate * CASE_SECONDS
    begin = index * segment
    start = begin + int(segment * start_fraction)
    end = begin + int(segment * end_fraction)
    return rms(left[start:end]), rms(right[start:end])


def require_dynamic_trigger(
    kind: str, left: list[int], right: list[int], rate: int
) -> None:
    for index in DYNAMIC_CASES.get(kind, set()):
        left_rms, right_rms = case_window_rms(
            left, right, rate, index, 0.02, 0.08
        )
        if max(left_rms, right_rms) < 128:
            raise ValueError(
                f"{kind} case {index} has no audible trigger: "
                f"rms={left_rms:.1f}/{right_rms:.1f}"
            )


def require_hardware_length_stop(
    kind: str, left: list[int], right: list[int], rate: int, index: int
) -> None:
    left_rms, right_rms = case_window_rms(left, right, rate, index, 0.70, 0.90)
    if max(left_rms, right_rms) > 64:
        raise ValueError(
            f"{kind} case {index} hardware length did not stop: "
            f"rms={left_rms:.1f}/{right_rms:.1f}"
        )


def require_route(
    kind: str, segments: list[tuple[float, float]], index: int, side: str
) -> None:
    left, right = segments[index]
    own, other = (left, right) if side == "left" else (right, left)
    if own < 128 or own < other * 4:
        raise ValueError(
            f"{kind} case {index} {side} routing failed: rms={left:.1f}/{right:.1f}"
        )


def require_half_level(kind: str, segments: list[tuple[float, float]]) -> None:
    full = max(segments[0])
    half = max(segments[4])
    ratio = half / full if full else 0.0
    if ratio < 0.45 or ratio > 0.55:
        raise ValueError(
            f"{kind} 50% level ratio is {ratio:.3f}, want approximately 0.5"
        )


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--kind", choices=sorted(CASE_COUNTS), required=True)
    parser.add_argument("--input", type=Path, required=True)
    args = parser.parse_args()

    rate, left, right = load_stereo(args.input)
    if rate != 32768:
        raise ValueError(f"capture rate is {rate}, want 32768 Hz")
    segments = settled_segments(left, right, CASE_COUNTS[args.kind], rate)
    require_audible(args.kind, segments)
    require_dynamic_trigger(args.kind, left, right, rate)

    if args.kind == "ch1":
        require_route(args.kind, segments, 10, "left")
        require_route(args.kind, segments, 11, "right")
    if args.kind == "ch2":
        require_route(args.kind, segments, 7, "left")
        require_route(args.kind, segments, 8, "right")
        require_hardware_length_stop(args.kind, left, right, rate, 6)
    if args.kind == "ch3":
        require_hardware_length_stop(args.kind, left, right, rate, 8)
    if args.kind == "ch4":
        require_route(args.kind, segments, 8, "left")
        require_route(args.kind, segments, 9, "right")
        require_hardware_length_stop(args.kind, left, right, rate, 7)
    if args.kind in ("fifoa", "fifob"):
        require_route(args.kind, segments, 1, "left")
        require_route(args.kind, segments, 2, "right")
        require_half_level(args.kind, segments)

    print(f"{args.kind}: PASS ({len(left)} frames at {rate} Hz)")
    for index, (left_rms, right_rms) in enumerate(segments):
        print(f"  case {index}: rms L={left_rms:.1f} R={right_rms:.1f}")


if __name__ == "__main__":
    main()
