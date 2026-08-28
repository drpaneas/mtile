#!/usr/bin/env python3
"""Unit tests for the GBA reference-capture signal measurements."""

import unittest

from analyze_gba_reference_capture import rms


class SignalMeasurementTests(unittest.TestCase):
    def test_constant_nonzero_dac_level_is_silent(self) -> None:
        self.assertEqual(rms([7680] * 256), 0.0)

    def test_alternating_wave_has_ac_energy(self) -> None:
        self.assertGreater(rms([-1024, 1024] * 128), 1000.0)

    def test_dc_offset_does_not_change_ac_energy(self) -> None:
        centred = rms([-1024, 1024] * 128)
        offset = rms([6656, 8704] * 128)
        self.assertAlmostEqual(offset, centred)


if __name__ == "__main__":
    unittest.main()
