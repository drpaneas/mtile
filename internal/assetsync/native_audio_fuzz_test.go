package assetsync

import "testing"

// FuzzNativeAudioCompilerNeverPanics keeps arbitrary authoring values at the
// strict v2 compiler boundary. Invalid combinations must return diagnostics;
// valid combinations must compile to runtime data without panicking.
func FuzzNativeAudioCompilerNeverPanics(f *testing.F) {
	f.Add([]byte{0, 50, 12, 2, 0, 0, 60, 24, 7, 7, 2, 0, 0, 0, 0, 0})
	f.Add([]byte{3, 15, 7, 13, 15, 3, 36, 24, 0, 0, 0, 1, 1, 1, 1, 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 16 {
			return
		}
		kindIndex := int(data[0] % 4)
		kind := []string{"ch1", "ch2", "ch3", "ch4"}[kindIndex]
		route := []string{"none", "right", "left", "both"}[data[11]%4]
		sound := NativeSoundAsset{ID: 0, Name: "fuzz", Kind: kind, Route: route}
		switch kindIndex {
		case 0, 1:
			duty := []float64{12.5, 25, 50, 75, 33}[data[1]%5]
			sound.Pulse = &NativePulseAsset{
				Duty: duty, Length: int(data[12]), UseLength: data[13]&1 != 0,
				Envelope: NativeEnvelopeAsset{Initial: int(data[2]), Direction: []string{"down", "up", "sideways"}[data[14]%3], Pace: int(data[3])},
			}
			if kindIndex == 0 {
				sound.Pulse.Sweep = &NativeSweepAsset{Pace: int(data[4]), Direction: []string{"up", "down", "sideways"}[data[15]%3], Shift: int(data[5])}
			}
		case 2:
			count := 32
			if data[1]&1 != 0 {
				count = 64
			}
			samples := make([]int, count)
			for index := range samples {
				samples[index] = int(data[index%len(data)] % 20)
			}
			sound.Wave = &NativeWaveAsset{Samples: samples, Level: []int{0, 25, 50, 75, 100, 33}[data[2]%6], StartBank: int(data[3]), Length: int(data[4]), UseLength: data[5]&1 != 0}
		case 3:
			sound.Noise = &NativeNoiseAsset{Width: []int{7, 15, 9}[data[1]%3], Divisor: int(data[2]), Shift: int(data[3]), Length: int(data[4]), UseLength: data[5]&1 != 0, Envelope: NativeEnvelopeAsset{Initial: int(data[6]), Direction: []string{"down", "up", "sideways"}[data[14]%3], Pace: int(data[7])}}
		}
		sounds := NativeSoundLibrary{Version: NativeAudioSchemaVersion, Sounds: []NativeSoundAsset{sound}}
		clip := NativeClipAsset{ID: 0, Name: "fuzz clip", Kind: kind, LengthTicks: 96, Events: []NativeEventAsset{{Tick: int(data[12] % 96), DurationTicks: 1 + int(data[13]), Note: int(data[6]), SoundID: 0, Priority: int(data[7]), Legato: data[15]&1 != 0}}}
		laneID := 0
		lanes := NativeLaneAssets{}
		switch kindIndex {
		case 0:
			lanes.CH1 = &laneID
		case 1:
			lanes.CH2 = &laneID
		case 2:
			lanes.CH3 = &laneID
		case 3:
			lanes.CH4 = &laneID
		}
		music := NativeMusicLibrary{Version: NativeAudioSchemaVersion, PPQN: 96, Clips: []NativeClipAsset{clip}, Parts: []NativePartAsset{{ID: 0, Name: "fuzz part", LengthTicks: 96, Lanes: lanes}}, Songs: []NativeSongAsset{{ID: 0, Name: "fuzz song", Tempo: 20 + int(data[8]), Sequence: []int{0}}}}
		settings := DefaultNativeAudioSettings()
		settings.LeftLevel, settings.RightLevel = int(data[9]), int(data[10])
		_, _ = CompileNativeAudio(settings, sounds, music)
	})
}
