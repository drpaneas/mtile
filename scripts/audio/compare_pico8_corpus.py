#!/usr/bin/env python3
"""Compare a full PICO-8 reference track with a Mtile capture.

The comparison is mastering-tolerant: gain and MP3 encoding may differ, while
active duration, energy phrasing, pitch-class content, and log-band timbre must
remain aligned.  It intentionally reports metrics without declaring a pass;
corpus-wide thresholds are calibrated only after known-good and known-bad
tracks have both been measured.
"""

from __future__ import annotations

import argparse
import json
import math
import subprocess
from dataclasses import dataclass
from pathlib import Path

import numpy as np


RATE = 16_384
WINDOW = 1_024
HOP = 256
MAX_LAG_SECONDS = 1.0
FEATURE_CHUNK_FRAMES = 2_048
SEGMENT_SEARCH_SECONDS = 12.0
SEGMENT_ANCHOR_SECONDS = 8.0
SEGMENT_EDGE_GUARD_SECONDS = 1.0


@dataclass(frozen=True)
class Features:
    duration: float
    envelope: np.ndarray
    chroma: np.ndarray
    bands: np.ndarray
    activity: np.ndarray


def decode(path: Path) -> np.ndarray:
    command = [
        "ffmpeg",
        "-v",
        "error",
        "-i",
        str(path),
        "-ac",
        "1",
        "-ar",
        str(RATE),
        "-f",
        "f32le",
        "-",
    ]
    result = subprocess.run(command, check=True, stdout=subprocess.PIPE)
    samples = np.frombuffer(result.stdout, dtype="<f4").astype(np.float64)
    if len(samples) < WINDOW:
        raise ValueError(f"{path}: decoded audio is shorter than one analysis window")
    return samples


def frame_rms(samples: np.ndarray, window: int = 256, hop: int = 128) -> np.ndarray:
    count = 1 + max(0, (len(samples) - window) // hop)
    frames = np.lib.stride_tricks.as_strided(
        samples,
        shape=(count, window),
        strides=(samples.strides[0] * hop, samples.strides[0]),
        writeable=False,
    )
    return np.sqrt(np.mean(frames * frames, axis=1))


def trim_active(samples: np.ndarray) -> np.ndarray:
    rms = frame_rms(samples)
    threshold = max(1e-4, float(np.max(rms)) * 0.005)
    active = np.flatnonzero(rms >= threshold)
    if len(active) == 0:
        raise ValueError("recording contains no active audio")
    start = max(0, int(active[0]) * 128 - WINDOW // 2)
    end = min(len(samples), int(active[-1]) * 128 + 256 + WINDOW // 2)
    return samples[start:end]


def normalized_rows(values: np.ndarray) -> np.ndarray:
    norms = np.linalg.norm(values, axis=1, keepdims=True)
    return values / np.maximum(norms, 1e-30)


def extract_features(samples: np.ndarray) -> Features:
    samples = trim_active(samples)
    count = 1 + (len(samples) - WINDOW) // HOP
    frequencies = np.fft.rfftfreq(WINDOW, 1.0 / RATE)
    window = np.hanning(WINDOW)
    band_edges = np.geomspace(40.0, RATE / 2.0, 49)
    band_masks = [
        (frequencies >= band_edges[index])
        & (frequencies < band_edges[index + 1])
        for index in range(48)
    ]
    audible = np.flatnonzero((frequencies >= 40.0) & (frequencies <= 7_500.0))
    midi = np.rint(69.0 + 12.0 * np.log2(frequencies[audible] / 440.0)).astype(int)

    rms_chunks = []
    band_chunks = []
    chroma_chunks = []
    for first in range(0, count, FEATURE_CHUNK_FRAMES):
        chunk_count = min(FEATURE_CHUNK_FRAMES, count - first)
        offset = first * HOP
        chunk_samples = samples[offset:]
        frames = np.lib.stride_tricks.as_strided(
            chunk_samples,
            shape=(chunk_count, WINDOW),
            strides=(samples.strides[0] * HOP, samples.strides[0]),
            writeable=False,
        )
        power = np.abs(np.fft.rfft(frames * window, axis=1)) ** 2
        rms_chunks.append(np.sqrt(np.mean(frames * frames, axis=1)))
        bands = np.stack(
            [np.sum(power[:, mask], axis=1) for mask in band_masks], axis=1
        )
        band_chunks.append(
            normalized_rows(
                np.log1p(
                    bands
                    / np.maximum(np.sum(bands, axis=1, keepdims=True), 1e-30)
                    * 1e6
                )
            )
        )
        chroma = np.zeros((chunk_count, 12), dtype=np.float64)
        magnitude = np.sqrt(power[:, audible])
        for pitch_class in range(12):
            chroma[:, pitch_class] = np.sum(
                magnitude[:, midi % 12 == pitch_class], axis=1
            )
        chroma_chunks.append(normalized_rows(chroma))

    rms = np.concatenate(rms_chunks)
    activity_threshold = max(1e-4, float(np.max(rms)) * 0.005)
    activity = rms >= activity_threshold
    envelope = np.log(rms + 1e-7)
    envelope = (envelope - np.mean(envelope)) / max(float(np.std(envelope)), 1e-9)
    return Features(
        len(samples) / RATE,
        envelope,
        np.concatenate(chroma_chunks),
        np.concatenate(band_chunks),
        activity,
    )


def aligned_slices(length_left: int, length_right: int, lag: int) -> tuple[slice, slice]:
    left_start = max(0, lag)
    right_start = max(0, -lag)
    length = min(length_left - left_start, length_right - right_start)
    return slice(left_start, left_start + length), slice(right_start, right_start + length)


def correlation(left: np.ndarray, right: np.ndarray) -> float:
    if len(left) < 2 or float(np.std(left)) == 0 or float(np.std(right)) == 0:
        return 0.0
    return float(np.corrcoef(left, right)[0, 1])


def best_alignment(left: Features, right: Features) -> tuple[int, float]:
    max_lag = round(MAX_LAG_SECONDS * RATE / HOP)
    best = (-math.inf, 0)
    for lag in range(-max_lag, max_lag + 1):
        one, two = aligned_slices(len(left.envelope), len(right.envelope), lag)
        if one.stop - one.start < round(2.0 * RATE / HOP):
            continue
        envelope_score = correlation(left.envelope[one], right.envelope[two])
        active_left = left.activity[one]
        active_right = right.activity[two]
        intersection = active_left & active_right
        union = active_left | active_right
        if np.any(intersection):
            chroma_score = float(
                np.mean(
                    np.sum(
                        left.chroma[one][intersection]
                        * right.chroma[two][intersection],
                        axis=1,
                    )
                )
            )
        else:
            chroma_score = 0.0
        activity_score = float(np.sum(intersection)) / max(1, int(np.sum(union)))
        score = 0.2 * envelope_score + 0.6 * chroma_score + 0.2 * activity_score
        if score > best[0] or (score == best[0] and abs(lag) < abs(best[1])):
            best = (score, lag)
    return best[1], best[0]


def compare(reference: Features, candidate: Features) -> dict[str, object]:
    lag, alignment_score = best_alignment(reference, candidate)
    one, two = aligned_slices(len(reference.envelope), len(candidate.envelope), lag)
    envelope = correlation(reference.envelope[one], candidate.envelope[two])
    chroma = np.sum(reference.chroma[one] * candidate.chroma[two], axis=1)
    bands = np.sum(reference.bands[one] * candidate.bands[two], axis=1)
    reference_activity = reference.activity[one]
    candidate_activity = candidate.activity[two]
    active_intersection = reference_activity & candidate_activity
    active_union = reference_activity | candidate_activity
    if np.any(active_intersection):
        active_chroma = chroma[active_intersection]
        active_bands = bands[active_intersection]
    else:
        active_chroma = np.zeros(1, dtype=np.float64)
        active_bands = np.zeros(1, dtype=np.float64)
    segment_frames = max(1, round(RATE / HOP))
    segment_chroma = [
        float(np.mean(chroma[index : index + segment_frames]))
        for index in range(0, len(chroma), segment_frames)
    ]
    return {
        "referenceActiveSeconds": reference.duration,
        "candidateActiveSeconds": candidate.duration,
        "durationErrorSeconds": candidate.duration - reference.duration,
        "durationRatio": candidate.duration / reference.duration,
        "lagFrames": lag,
        "lagSeconds": lag * HOP / RATE,
        "alignmentScore": alignment_score,
        "envelopeCorrelation": envelope,
        "activityIntersectionOverUnion": float(np.sum(active_intersection))
        / max(1, int(np.sum(active_union))),
        "referenceActiveFrameRatio": float(np.mean(reference_activity)),
        "candidateActiveFrameRatio": float(np.mean(candidate_activity)),
        "meanChromaSimilarity": float(np.mean(chroma)),
        "p10ChromaSimilarity": float(np.percentile(chroma, 10)),
        "meanActiveChromaSimilarity": float(np.mean(active_chroma)),
        "p10ActiveChromaSimilarity": float(np.percentile(active_chroma, 10)),
        "meanBandSimilarity": float(np.mean(bands)),
        "p10BandSimilarity": float(np.percentile(bands, 10)),
        "meanActiveBandSimilarity": float(np.mean(active_bands)),
        "p10ActiveBandSimilarity": float(np.percentile(active_bands, 10)),
        "worstChromaSecond": int(np.argmin(segment_chroma)),
        "worstChromaSecondScore": min(segment_chroma),
    }


def feature_match_score(
    reference: Features,
    candidate: Features,
    reference_start: int,
    candidate_start: int,
    length: int,
) -> float:
    if length <= 0:
        return -math.inf
    reference_slice = slice(reference_start, reference_start + length)
    candidate_slice = slice(candidate_start, candidate_start + length)
    active = reference.activity[reference_slice] & candidate.activity[candidate_slice]
    if not np.any(active):
        return -math.inf
    chroma = np.sum(
        reference.chroma[reference_slice][active]
        * candidate.chroma[candidate_slice][active],
        axis=1,
    )
    bands = np.sum(
        reference.bands[reference_slice][active]
        * candidate.bands[candidate_slice][active],
        axis=1,
    )
    return 0.75 * float(np.mean(chroma)) + 0.25 * float(np.mean(bands))


def locate_segment(
    reference: Features,
    candidate: Features,
    expected_start: int,
    minimum_start: int = 0,
) -> tuple[int, float]:
    """Locate one rendered cart inside a mastered multipart reference.

    Album tracks assembled from several PICO-8 captures can contain short
    crossfades or edits at cart boundaries. A global fixed-lag comparison then
    misaligns every later cart even though each cart is note-exact. Search a
    guarded interior anchor near the expected ordered position, first at a
    coarse cadence and then at single-frame resolution.
    """

    frames_per_second = RATE / HOP
    guard = min(
        round(SEGMENT_EDGE_GUARD_SECONDS * frames_per_second),
        max(0, len(candidate.chroma) // 4),
    )
    anchor_length = min(
        round(SEGMENT_ANCHOR_SECONDS * frames_per_second),
        len(candidate.chroma) - 2 * guard,
    )
    if anchor_length <= 0:
        guard = 0
        anchor_length = len(candidate.chroma)
    maximum_start = len(reference.chroma) - guard - anchor_length
    if maximum_start < minimum_start:
        raise ValueError("reference is too short to locate ordered segment")
    radius = round(SEGMENT_SEARCH_SECONDS * frames_per_second)
    lower = max(minimum_start, expected_start - radius)
    upper = min(maximum_start, expected_start + radius)
    if lower > upper:
        lower = upper = min(max(minimum_start, expected_start), maximum_start)

    stride = max(1, round(frames_per_second / 4.0))
    candidates = list(range(lower, upper + 1, stride))
    if not candidates or candidates[-1] != upper:
        candidates.append(upper)
    coarse_start, coarse_score = max(
        (
            (
                start,
                feature_match_score(
                    reference,
                    candidate,
                    start + guard,
                    guard,
                    anchor_length,
                ),
            )
            for start in candidates
        ),
        key=lambda item: item[1],
    )
    refine_lower = max(lower, coarse_start - stride)
    refine_upper = min(upper, coarse_start + stride)
    return max(
        (
            (
                start,
                feature_match_score(
                    reference,
                    candidate,
                    start + guard,
                    guard,
                    anchor_length,
                ),
            )
            for start in range(refine_lower, refine_upper + 1)
        ),
        key=lambda item: item[1],
        default=(coarse_start, coarse_score),
    )


def compare_segment_sequence(
    reference: Features, candidates: list[Features]
) -> dict[str, object]:
    """Compare ordered cart exports independently inside one album track."""

    if not candidates:
        raise ValueError("segmented comparison requires at least one candidate")
    frames_per_second = RATE / HOP
    guard_limit = round(SEGMENT_EDGE_GUARD_SECONDS * frames_per_second)
    expected_start = 0
    minimum_start = 0
    records: list[dict[str, object]] = []
    chroma_total = 0.0
    band_total = 0.0
    active_total = 0
    for index, candidate in enumerate(candidates):
        start, anchor_score = locate_segment(
            reference, candidate, expected_start, minimum_start
        )
        guard = min(guard_limit, max(0, len(candidate.chroma) // 8))
        length = min(
            len(candidate.chroma) - 2 * guard,
            len(reference.chroma) - start - guard,
        )
        if length <= 0:
            raise ValueError(f"segment {index} has no comparable interior frames")
        reference_slice = slice(start + guard, start + guard + length)
        candidate_slice = slice(guard, guard + length)
        active = (
            reference.activity[reference_slice] & candidate.activity[candidate_slice]
        )
        if np.any(active):
            chroma = np.sum(
                reference.chroma[reference_slice][active]
                * candidate.chroma[candidate_slice][active],
                axis=1,
            )
            bands = np.sum(
                reference.bands[reference_slice][active]
                * candidate.bands[candidate_slice][active],
                axis=1,
            )
            active_frames = int(np.sum(active))
            mean_chroma = float(np.mean(chroma))
            mean_bands = float(np.mean(bands))
            chroma_total += float(np.sum(chroma))
            band_total += float(np.sum(bands))
            active_total += active_frames
        else:
            active_frames = 0
            mean_chroma = 0.0
            mean_bands = 0.0
        records.append(
            {
                "index": index,
                "referenceStartSeconds": start / frames_per_second,
                "candidateActiveSeconds": candidate.duration,
                "anchorScore": anchor_score,
                "activeFrames": active_frames,
                "meanActiveChromaSimilarity": mean_chroma,
                "meanActiveBandSimilarity": mean_bands,
            }
        )
        expected_start = start + len(candidate.chroma)
        minimum_start = start + 1
    return {
        "segments": records,
        "meanActiveChromaSimilarity": chroma_total / max(1, active_total),
        "meanActiveBandSimilarity": band_total / max(1, active_total),
        "activeFrames": active_total,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("reference", type=Path)
    parser.add_argument("candidate", type=Path)
    parser.add_argument("--json", type=Path)
    args = parser.parse_args()
    result = compare(
        extract_features(decode(args.reference)),
        extract_features(decode(args.candidate)),
    )
    encoded = json.dumps(result, indent=2) + "\n"
    print(encoded, end="")
    if args.json is not None:
        args.json.parent.mkdir(parents=True, exist_ok=True)
        args.json.write_text(encoded, encoding="utf-8")


if __name__ == "__main__":
    main()
