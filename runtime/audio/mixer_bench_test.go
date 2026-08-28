package audio

import "testing"

func benchmarkMixMono(b *testing.B, voices int) {
	var m Mixer
	if !m.Init(voices) {
		b.Fatalf("Init(%d) failed", voices)
	}
	for i := 0; i < voices; i++ {
		if _, ok := m.Start(Request{
			Kind: SourceTriangle, Step: uint32(1<<23 + i*7919), Volume: 32,
		}); !ok {
			b.Fatalf("start voice %d", i)
		}
	}
	var dst [256]int8
	b.ReportAllocs()
	b.SetBytes(int64(len(dst)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.MixMono(dst[:])
	}
}

func BenchmarkMixMono4(b *testing.B)  { benchmarkMixMono(b, 4) }
func BenchmarkMixMono8(b *testing.B)  { benchmarkMixMono(b, 8) }
func BenchmarkMixMono12(b *testing.B) { benchmarkMixMono(b, 12) }

func benchmarkMixStereo(b *testing.B, voices int) {
	var m Mixer
	if !m.Init(voices) {
		b.Fatalf("Init(%d) failed", voices)
	}
	for i := 0; i < voices; i++ {
		pan := int16(-256 + i*512/max(1, voices-1))
		if _, ok := m.Start(Request{
			Kind: SourceTriangle, Step: uint32(1<<23 + i*7919), Volume: 32, Pan: pan,
		}); !ok {
			b.Fatalf("start voice %d", i)
		}
	}
	var left, right [256]int8
	b.ReportAllocs()
	b.SetBytes(int64(len(left) + len(right)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.MixStereo(left[:], right[:])
	}
}

func BenchmarkMixStereo4(b *testing.B)  { benchmarkMixStereo(b, 4) }
func BenchmarkMixStereo8(b *testing.B)  { benchmarkMixStereo(b, 8) }
func BenchmarkMixStereo12(b *testing.B) { benchmarkMixStereo(b, 12) }
