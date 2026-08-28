package performance

// PulseWaveform materializes the four Game Boy pulse duties in CH3's native
// 32-nibble Wave RAM format. Unsupported values fall back to 50%.
func PulseWaveform(duty int) [32]uint8 {
	high := 16
	switch duty {
	case 12:
		high = 4
	case 25:
		high = 8
	case 50:
		high = 16
	case 75:
		high = 24
	}
	var wave [32]uint8
	for i := 0; i < high; i++ {
		wave[i] = 15
	}
	return wave
}
