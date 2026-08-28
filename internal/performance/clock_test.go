package performance

import (
	"testing"
	"time"
)

func TestTrackerClockRoundTripAndPosition(t *testing.T) {
	clock := NewTrackerClock(120, DefaultPartTicks)
	for tick := int64(0); tick < 10_000; tick++ {
		if got := clock.TickAt(clock.TimeAtTick(tick)); got != tick {
			t.Fatalf("tick %d round trip = %d", tick, got)
		}
	}
	position := clock.PositionAt(clock.TimeAtTick(DefaultPartTicks + 5*TicksPerRow + 7))
	if position != (MusicalPosition{Pattern: 1, Row: 5, Tick: 7}) {
		t.Fatalf("position = %#v", position)
	}
}

func TestTrackerClockDoesNotDriftAtOddTempo(t *testing.T) {
	clock := NewTrackerClock(137, DefaultPartTicks)
	duration := 24 * time.Hour
	got := clock.TickAt(duration)
	want := int64(duration) * 137 * PPQN / int64(time.Minute)
	if got != want {
		t.Fatalf("24-hour tick = %d, want %d", got, want)
	}
}

func TestQuantizeUsesRawTickEveryTime(t *testing.T) {
	clock := NewTrackerClock(120, DefaultPartTicks)
	if got := clock.QuantizeTick(17, QuantizeSixteenth); got != 24 {
		t.Fatalf("1/16 quantize = %d", got)
	}
	if got := clock.QuantizeTick(17, QuantizeThirtySecond); got != 12 {
		t.Fatalf("1/32 requantize from raw = %d", got)
	}
	if got := clock.QuantizeTick(17, QuantizeSixtyFourth); got != 18 {
		t.Fatalf("1/64 requantize from raw = %d", got)
	}
	if QuantizeSixtyFourth.Ticks() != 6 || QuantizeSixtyFourth.String() != "1/64" {
		t.Fatalf("1/64 quantize = %d ticks, %q", QuantizeSixtyFourth.Ticks(), QuantizeSixtyFourth.String())
	}
	for grid, want := range map[Quantize]int64{
		QuantizeQuarterTriplet:   64,
		QuantizeEighthTriplet:    32,
		QuantizeSixteenthTriplet: 16,
	} {
		if grid.Ticks() != want {
			t.Fatalf("%s quantize = %d ticks, want %d", grid, grid.Ticks(), want)
		}
	}
}

func TestSwingQuantizeDelaysOnlyOffbeats(t *testing.T) {
	clock := NewTrackerClock(120, DefaultPartTicks)
	for _, grid := range []Quantize{QuantizeQuarter, QuantizeEighth, QuantizeSixteenth, QuantizeThirtySecond, QuantizeSixtyFourth} {
		if !grid.Swingable() {
			t.Fatalf("straight grid %s does not accept swing", grid)
		}
	}
	for _, grid := range []Quantize{QuantizeQuarterTriplet, QuantizeEighthTriplet, QuantizeSixteenthTriplet, QuantizeOff} {
		if grid.Swingable() {
			t.Fatalf("non-straight grid %s unexpectedly accepts swing", grid)
		}
	}
	if got := clock.QuantizeTickSwing(35, QuantizeEighth, 66); got != 63 {
		t.Fatalf("swung eighth offbeat = %d, want 63", got)
	}
	if got := clock.QuantizeTickSwing(90, QuantizeEighth, 66); got != 96 {
		t.Fatalf("swung eighth downbeat = %d, want 96", got)
	}
	if got := clock.QuantizeTickSwing(35, QuantizeEighthTriplet, 66); got != 32 {
		t.Fatalf("triplet grid must ignore swing: got %d", got)
	}
	if got := clock.QuantizeTickSwing(7, QuantizeSixtyFourth, 66); got != 8 {
		t.Fatalf("swung sixty-fourth offbeat = %d, want 8", got)
	}
}

func TestPulseWaveformDuties(t *testing.T) {
	for duty, high := range map[int]int{12: 4, 25: 8, 50: 16, 75: 24} {
		wave := PulseWaveform(duty)
		for index, value := range wave {
			want := uint8(0)
			if index < high {
				want = 15
			}
			if value != want {
				t.Fatalf("duty %d sample %d = %d, want %d", duty, index, value, want)
			}
		}
	}
}
