#!/usr/bin/env python3
"""Cross-correlate mGBA's internal stereo stream with the shared Go reference."""

from __future__ import annotations

import argparse
import hashlib
import wave
from pathlib import Path

import numpy as np


EXPECTED_RATE = 32768
ALIGNMENT_WINDOW = 8192
EVALUATION_SKIP = 4096
EVALUATION_FRAMES = 32768


def read_stereo(path: Path) -> np.ndarray:
    with wave.open(str(path), "rb") as wav:
        if wav.getnchannels() != 2 or wav.getsampwidth() != 2:
            raise ValueError(f"{path}: expected stereo signed 16-bit PCM")
        if wav.getframerate() != EXPECTED_RATE:
            raise ValueError(
                f"{path}: expected {EXPECTED_RATE} Hz, found {wav.getframerate()} Hz"
            )
        frames = wav.getnframes()
        samples = np.frombuffer(wav.readframes(frames), dtype="<i2")
    return samples.reshape(-1, 2).astype(np.float64)


def correlation(left: np.ndarray, right: np.ndarray) -> float:
    left = left - np.mean(left)
    right = right - np.mean(right)
    denominator = float(np.linalg.norm(left) * np.linalg.norm(right))
    if denominator == 0:
        return 0.0
    return float(np.dot(left, right) / denominator)


def alignment(capture: np.ndarray, reference: np.ndarray) -> tuple[int, float]:
    active = np.flatnonzero(np.any(capture != 0, axis=1))
    if not len(active):
        raise ValueError("capture contains no nonzero stereo samples")
    first = int(active[0])
    length = min(ALIGNMENT_WINDOW, len(reference), len(capture) - first)
    if length < ALIGNMENT_WINDOW // 2:
        raise ValueError("capture is too short for alignment")

    best = (first, -1.0)
    low = max(0, first - 32)
    high = min(len(capture) - length, first + 512)
    for start in range(low, high + 1):
        left = correlation(capture[start : start + length, 0], reference[:length, 0])
        right = correlation(capture[start : start + length, 1], reference[:length, 1])
        score = (left + right) / 2
        if score > best[1]:
            best = (start, score)
    return best


def gain(actual: np.ndarray, expected: np.ndarray) -> float:
    denominator = float(np.dot(expected, expected))
    return float(np.dot(actual, expected) / denominator) if denominator else 0.0


def longest_silent_run(samples: np.ndarray) -> int:
    silent = np.all(samples == 0, axis=1)
    edges = np.flatnonzero(np.diff(np.r_[False, silent, False]))
    runs = edges[1::2] - edges[::2]
    return int(np.max(runs)) if len(runs) else 0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("capture", type=Path)
    parser.add_argument("reference", type=Path)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    capture = read_stereo(args.capture)
    reference = read_stereo(args.reference)
    start, alignment_score = alignment(capture, reference)

    available = min(
        EVALUATION_FRAMES,
        len(reference) - EVALUATION_SKIP,
        len(capture) - start - EVALUATION_SKIP,
    )
    if available < EVALUATION_FRAMES:
        raise ValueError(
            f"only {available} aligned evaluation frames; need {EVALUATION_FRAMES}"
        )
    expected = reference[EVALUATION_SKIP : EVALUATION_SKIP + available]
    actual = capture[
        start + EVALUATION_SKIP : start + EVALUATION_SKIP + available
    ]

    left_correlation = correlation(actual[:, 0], expected[:, 0])
    right_correlation = correlation(actual[:, 1], expected[:, 1])
    left_swapped = correlation(actual[:, 0], expected[:, 1])
    right_swapped = correlation(actual[:, 1], expected[:, 0])
    channel_correlation = correlation(actual[:, 0], actual[:, 1])
    left_gain = gain(actual[:, 0], expected[:, 0])
    right_gain = gain(actual[:, 1], expected[:, 1])
    # The square-wave left reference never contains zero. Any simultaneous
    # zero run after the settled prefix therefore indicates paired silence,
    # including a dropout after the one-second correlation window.
    settled_capture = capture[start + EVALUATION_SKIP :]
    silent_run = longest_silent_run(settled_capture)
    digest = hashlib.sha256(args.capture.read_bytes()).hexdigest()

    print(
        f"capture frames={len(capture)} rate={EXPECTED_RATE} Hz "
        f"sha256={digest}"
    )
    print(f"alignment start={start} frames score={alignment_score:.6f}")
    print(
        f"own correlation left={left_correlation:.6f} "
        f"right={right_correlation:.6f}"
    )
    print(
        f"swapped correlation left/right={left_swapped:.6f} "
        f"right/left={right_swapped:.6f}"
    )
    print(
        f"inter-channel correlation={channel_correlation:.6f} "
        f"gain left={left_gain:.6f} right={right_gain:.6f} "
        f"longest silence={silent_run} frames"
    )

    failures: list[str] = []
    if alignment_score < 0.999:
        failures.append(f"alignment correlation {alignment_score:.6f} < 0.999")
    if min(left_correlation, right_correlation) < 0.999:
        failures.append("one or both own-channel correlations are below 0.999")
    if max(abs(left_swapped), abs(right_swapped), abs(channel_correlation)) > 0.10:
        failures.append("cross-channel correlation exceeds 0.10")
    if not 0.99 <= left_gain <= 1.01 or not 0.99 <= right_gain <= 1.01:
        failures.append("captured/reference gain is outside 1%")
    if silent_run:
        failures.append(f"aligned capture contains a {silent_run}-frame silence")
    if failures:
        raise SystemExit("stereo capture FAILED: " + "; ".join(failures))
    print("stereo capture PASS")


if __name__ == "__main__":
    main()
