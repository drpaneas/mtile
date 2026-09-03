package audio

// pulsePeriods contains Game Boy pulse-channel period-register values indexed
// by conventional MIDI note. MIDI C2..B7 (36..107) is copied exactly from
// hUGEDriver's hUGE_note_table.inc. The remaining high notes extend that table
// for mtile's wider editor range. 0xFFFF marks notes below the hardware range.
var pulsePeriods = [152]uint16{
	65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535,
	65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535, 65535,
	65535, 65535, 65535, 65535, 44, 156, 262, 363, 457, 547, 631, 710, 786, 854, 923, 986,
	1046, 1102, 1155, 1205, 1253, 1297, 1339, 1379, 1417, 1452, 1486, 1517, 1546, 1575, 1602, 1627,
	1650, 1673, 1694, 1714, 1732, 1750, 1767, 1783, 1798, 1812, 1825, 1837, 1849, 1860, 1871, 1881,
	1890, 1899, 1907, 1915, 1923, 1930, 1936, 1943, 1949, 1954, 1959, 1964, 1969, 1974, 1978, 1982,
	1985, 1988, 1992, 1995, 1998, 2001, 2004, 2006, 2009, 2011, 2013, 2015, 2017, 2018, 2020, 2022,
	2023, 2025, 2026, 2027, 2028, 2029, 2030, 2031, 2032, 2033, 2034, 2035, 2036, 2036, 2037, 2038,
	2038, 2039, 2039, 2040, 2040, 2041, 2041, 2041, 2042, 2042, 2042, 2043, 2043, 2043, 2044, 2044,
	2044, 2044, 2045, 2045, 2045, 2045, 2045, 2045,
}

// NativeTonePeriod returns the nearest register period for CH1, CH2, or a
// 32-sample CH3 waveform. Call NativeSoundPeriod when the concrete sound is
// available so CH3's GBA-only 64-sample dimension is pitched correctly.
func NativeTonePeriod(kind SoundKind, note uint8) (uint16, bool) {
	index := int(note)
	switch kind {
	case SoundCH1, SoundCH2:
	case SoundCH3:
		// A 32-sample CH3 waveform completes a cycle at half the pulse-channel
		// frequency for the same register value. Move one octave up the period
		// table so the editor's conventional MIDI pitch remains physical truth.
		index += 12
	default:
		return 0, false
	}
	if index >= len(pulsePeriods) || pulsePeriods[index] == 0xFFFF {
		return 0, false
	}
	return pulsePeriods[index], true
}

// NativeSoundPeriod returns the hardware period for one concrete PSG sound.
// CH3 advances one 4-bit sample at 2,097,152/(2048-period) Hz, so a 64-sample
// waveform needs twice the sample clock of a 32-sample waveform to preserve
// the same musical pitch.
func NativeSoundPeriod(sound Sound, note uint8) (uint16, bool) {
	return nativeSoundPeriod(&sound, note)
}

// nativeSoundPeriod is the target hot-path form. Sound contains a complete
// 64-sample CH3 waveform, so passing it by pointer avoids copying the tagged
// union on every scheduler note event.
func nativeSoundPeriod(sound *Sound, note uint8) (uint16, bool) {
	if sound == nil {
		return 0, false
	}
	if sound.Kind != SoundCH3 || sound.Wave.Wave.Length != 64 {
		return NativeTonePeriod(sound.Kind, note)
	}
	// A 64-sample waveform needs another octave of sample-clock compensation
	// beyond the ordinary 32-sample CH3 mapping.
	index := int(note) + 24
	if index >= len(pulsePeriods) || pulsePeriods[index] == 0xFFFF {
		return 0, false
	}
	return pulsePeriods[index], true
}

// NativeSoundNoteValid reports whether a note can drive this concrete sound.
// Ordinary CH4 sounds use a fixed NR43 value and accept any percussion-row
// label. A GB Studio-relative noise table needs GB Studio's C3..B8 base range.
func NativeSoundNoteValid(sound Sound, note uint8) bool {
	return nativeSoundNoteValid(&sound, note)
}

func nativeSoundNoteValid(sound *Sound, note uint8) bool {
	if sound == nil {
		return false
	}
	if sound.Kind != SoundCH4 {
		_, ok := nativeSoundPeriod(sound, note)
		return ok
	}
	for _, step := range sound.Macro.Steps {
		if step.HasNoiseNote {
			return note >= 36 && note <= 107
		}
	}
	return true
}

// NativeGBStudioNoisePolynomial reproduces hUGEDriver's get_note_poly for a
// MIDI note plus one instrument-subpattern offset. GB Studio stores its first
// tracker note as ID zero, while Native Audio stores conventional MIDI notes
// (MIDI 36 = C2); subtracting 36 converts between those numeric domains.
func NativeGBStudioNoisePolynomial(note uint8, offset int8) (divisor, shift uint8, ok bool) {
	if note < 36 || note > 107 || offset < -36 || offset > 35 {
		return 0, 0, false
	}
	noteID := uint8(int(note) - 36 + int(offset))
	inverted := ^(noteID + 192)
	polynomial := inverted
	if inverted >= 7 {
		shiftValue := inverted/4 - 1
		polynomial = (shiftValue << 4) | (shiftValue >> 4) | (inverted&3 + 4)
	}
	return polynomial & 7, polynomial >> 4, true
}

// NativeSoundNoteRange returns the inclusive MIDI-note range that the
// concrete hardware sound can represent. CH3's range depends on whether Wave
// RAM is configured for 32 or 64 samples, so callers must use the actual
// sound rather than only its channel kind.
func NativeSoundNoteRange(sound Sound) (uint8, uint8, bool) {
	if sound.Kind == SoundCH4 {
		return 0, 0, false
	}
	minimum, maximum := -1, -1
	for note := 0; note <= 127; note++ {
		if _, ok := nativeSoundPeriod(&sound, uint8(note)); !ok {
			continue
		}
		if minimum < 0 {
			minimum = note
		}
		maximum = note
	}
	if minimum < 0 {
		return 0, 0, false
	}
	return uint8(minimum), uint8(maximum), true
}
