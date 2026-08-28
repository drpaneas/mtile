package audio

import "testing"

func unpackMonoWords(words []uint32, samples int) []int8 {
	out := make([]int8, samples)
	for i := range out {
		out[i] = int8(byte(words[i/4] >> (uint(i&3) * 8)))
	}
	return out
}

func FuzzMixerByteAndWordPathsAgree(f *testing.F) {
	f.Add(uint8(12), uint16(MaxVolume), uint8(SourceSquare), int16(0))
	f.Add(uint8(4), uint16(255), uint8(SourceNoise), int16(MinPan))
	f.Add(uint8(8), uint16(129), uint8(SourcePulse), int16(MaxPan))
	f.Add(uint8(6), uint16(167), uint8(SourceTiltedSaw), int16(0))
	f.Add(uint8(5), uint16(119), uint8(SourceOrgan), int16(0))
	f.Add(uint8(7), uint16(167), uint8(SourcePhaser), int16(0))
	f.Fuzz(func(t *testing.T, rawVoices uint8, rawVolume uint16, rawKind uint8, rawPan int16) {
		voiceCount := 1 + int(rawVoices%MaxVoices)
		volume := rawVolume % (MaxVolume + 1)
		synthKinds := [...]SourceKind{
			SourceSquare, SourceTriangle, SourceTiltedSaw, SourceSaw, SourceNoise, SourcePulse, SourceOrgan, SourcePhaser,
		}
		kind := synthKinds[int(rawKind)%len(synthKinds)]
		pan := MinPan + int16(uint32(uint16(rawPan))%uint32(MaxPan-MinPan+1))

		var mixer Mixer
		if !mixer.Init(MaxVoices) {
			t.Fatal("initialize mixer")
		}
		for i := 0; i < voiceCount; i++ {
			if _, ok := mixer.Start(Request{
				Kind: kind, Step: uint32(1+i) << 23, Volume: volume, Pan: pan,
				NoiseSeed: uint16(i + 1), Priority: uint8(i),
			}); !ok {
				t.Fatalf("start voice %d/%d", i, voiceCount)
			}
		}

		const samples = 64
		byteMixer, wordMixer := mixer, mixer
		bytes := make([]int8, samples)
		words := make([]uint32, samples/4)
		byteMixer.MixMono(bytes)
		wordMixer.MixMonoWords(words)
		unpacked := unpackMonoWords(words, samples)
		for i := range bytes {
			if bytes[i] != unpacked[i] {
				t.Fatalf("mono sample %d differs: byte=%d word=%d", i, bytes[i], unpacked[i])
			}
		}

		stereoByteMixer, stereoWordMixer := mixer, mixer
		left, right := make([]int8, samples), make([]int8, samples)
		leftWords, rightWords := make([]uint32, samples/4), make([]uint32, samples/4)
		stereoByteMixer.MixStereo(left, right)
		stereoWordMixer.MixStereoWords(leftWords, rightWords)
		leftUnpacked, rightUnpacked := unpackMonoWords(leftWords, samples), unpackMonoWords(rightWords, samples)
		for i := range left {
			if left[i] != leftUnpacked[i] || right[i] != rightUnpacked[i] {
				t.Fatalf("stereo sample %d differs: byte=(%d,%d) word=(%d,%d)", i, left[i], right[i], leftUnpacked[i], rightUnpacked[i])
			}
		}
	})
}

func FuzzMusicPatternTransitions(f *testing.F) {
	f.Add([]byte{2, 0, 1, 2, 0, 1, 0, 2})
	f.Add([]byte{1, 0, 0, 0})
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) == 0 {
			raw = []byte{0}
		}
		patternCount := 1 + int(raw[0]%4)
		definitions := make([]SFXDefinition, 4)
		steps := make([]SFXStep, 4)
		for i := range definitions {
			definitions[i] = SFXDefinition{
				ID: uint8(i), Speed: 1, StepOffset: uint16(i), StepCount: 1,
				LoopStart: NoSFXLoop, LoopEnd: NoSFXLoop,
			}
			steps[i] = SFXStep{PhaseStep: uint32(i+1) << 24, Volume: MaxVolume / 4, Kind: SourceTriangle}
		}
		patterns := make([]MusicPattern, patternCount)
		at := 1
		for i := range patterns {
			patterns[i] = MusicPattern{ID: uint8(i), DurationSamples: uint32(4 * (1 + int(raw[at%len(raw)]%16)))}
			at++
			for channel := range patterns[i].Channels {
				value := raw[at%len(raw)] % 5
				at++
				if value == 4 {
					patterns[i].Channels[channel] = -1
				} else {
					patterns[i].Channels[channel] = int8(value)
				}
			}
			next := int(raw[at%len(raw)] % uint8(patternCount+1))
			at++
			if next == patternCount {
				patterns[i].Next = NoMusicPattern
			} else {
				patterns[i].Next = int8(next)
			}
		}

		var mixer Mixer
		var player SFXPlayer
		if !mixer.Init(4) || !player.Init(&mixer, definitions, steps) || !player.InitMusic(patterns) || !player.Music(0, 0) {
			t.Fatal("valid fuzz music graph failed initialization")
		}
		buffer := make([]int8, 512)
		player.MixMono(buffer)
		state := player.MusicState()
		if state.Active && int(state.PatternID) >= patternCount {
			t.Fatalf("transition escaped graph: state=%+v patterns=%d", state, patternCount)
		}
	})
}
