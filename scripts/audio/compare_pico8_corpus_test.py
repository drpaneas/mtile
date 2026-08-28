#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import math
import sys
import unittest
from pathlib import Path

import numpy as np


MODULE_PATH = Path(__file__).with_name("compare_pico8_corpus.py")
SPEC = importlib.util.spec_from_file_location("compare_pico8_corpus", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
corpus = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = corpus
SPEC.loader.exec_module(corpus)


def melody(frequencies: tuple[float, ...]) -> np.ndarray:
    pieces = []
    for frequency in frequencies:
        times = np.arange(corpus.RATE // 2) / corpus.RATE
        pieces.append(0.5 * np.sin(2 * math.pi * frequency * times))
    return np.concatenate(pieces)


class CorpusComparisonTests(unittest.TestCase):
    def test_identical_melody_has_near_perfect_metrics(self) -> None:
        source = melody((220.0, 330.0, 440.0, 550.0, 660.0))
        features = corpus.extract_features(source)
        result = corpus.compare(features, features)
        self.assertAlmostEqual(result["durationRatio"], 1.0)
        self.assertGreater(result["envelopeCorrelation"], 0.99)
        self.assertGreater(result["meanChromaSimilarity"], 0.99)
        self.assertGreater(result["meanBandSimilarity"], 0.99)

    def test_changed_notes_reduce_pitch_similarity(self) -> None:
        reference = corpus.extract_features(melody((220.0, 330.0, 440.0, 550.0)))
        changed = corpus.extract_features(melody((277.0, 415.0, 554.0, 698.0)))
        result = corpus.compare(reference, changed)
        self.assertLess(result["meanChromaSimilarity"], 0.5)

    def test_sparse_matching_notes_use_active_audio_metrics(self) -> None:
        tone = melody((220.0,))[: corpus.RATE // 16]
        silence = np.zeros(corpus.RATE // 4)
        source = np.concatenate([tone, silence] * 8)
        result = corpus.compare(
            corpus.extract_features(source), corpus.extract_features(source)
        )
        self.assertLess(result["meanChromaSimilarity"], 0.5)
        self.assertGreater(result["meanActiveChromaSimilarity"], 0.99)
        self.assertGreater(result["meanActiveBandSimilarity"], 0.99)
        self.assertGreater(result["activityIntersectionOverUnion"], 0.99)

    def test_segmented_comparison_recovers_mastering_gap(self) -> None:
        first = melody((220.0, 247.0, 277.0, 330.0) * 6)
        second = melody((392.0, 440.0, 494.0, 587.0) * 6)
        reference = np.concatenate([first, np.zeros(corpus.RATE), second])
        global_result = corpus.compare(
            corpus.extract_features(reference),
            corpus.extract_features(np.concatenate([first, second])),
        )
        segmented = corpus.compare_segment_sequence(
            corpus.extract_features(reference),
            [corpus.extract_features(first), corpus.extract_features(second)],
        )
        self.assertLess(global_result["meanActiveChromaSimilarity"], 0.95)
        self.assertGreater(segmented["meanActiveChromaSimilarity"], 0.99)
        self.assertGreater(segmented["meanActiveBandSimilarity"], 0.99)
        self.assertAlmostEqual(
            segmented["segments"][1]["referenceStartSeconds"],
            len(first) / corpus.RATE + 1.0,
            delta=0.1,
        )


if __name__ == "__main__":
    unittest.main()
