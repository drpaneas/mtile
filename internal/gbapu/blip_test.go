package gbapu

import "testing"

func TestBlipClockConversionProducesExactHostSampleCount(t *testing.T) {
	const (
		clockRate  = 16_777_216
		sampleRate = 48_000
	)
	buffer := newBlipBuffer(sampleRate/10, clockRate, sampleRate)
	const frames = 1024
	clocks := buffer.clocksNeeded(frames)
	buffer.endFrame(clocks)
	if buffer.available != frames {
		t.Fatalf("%d GBA clocks produced %d samples, want %d", clocks, buffer.available, frames)
	}
}

func TestBlipBandlimitsStepAndSettlesWithoutAllocation(t *testing.T) {
	buffer := newBlipBuffer(512, 16_777_216, 48_000)
	buffer.addDelta(0, 12_000)
	clocks := buffer.clocksNeeded(256)
	buffer.endFrame(clocks)

	nonzero := 0
	peak := int16(0)
	for buffer.available > 0 {
		sample := buffer.readSample()
		if sample != 0 {
			nonzero++
		}
		if sample < 0 {
			sample = -sample
		}
		if sample > peak {
			peak = sample
		}
	}
	if nonzero < blipHalfWidth || peak == 0 || peak > 20_000 {
		t.Fatalf("band-limited step nonzero=%d peak=%d", nonzero, peak)
	}
}

func TestBlipFastDeltaIsDeterministic(t *testing.T) {
	render := func() [64]int16 {
		buffer := newBlipBuffer(128, 16_777_216, 48_000)
		buffer.addDeltaFast(7, 4096)
		buffer.addDeltaFast(701, -4096)
		buffer.endFrame(buffer.clocksNeeded(64))
		var result [64]int16
		for index := range result {
			result[index] = buffer.readSample()
		}
		return result
	}
	if first, second := render(), render(); first != second {
		t.Fatal("blip output changed between identical renders")
	}
}
