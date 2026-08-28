package performance

import "time"

const (
	PPQN             = 96
	DefaultPartTicks = PPQN * 4
	TicksPerRow      = PPQN / 4
)

type MusicalPosition struct {
	Pattern int
	Row     int
	Tick    int
}

type Quantize uint8

const (
	QuantizeOff Quantize = iota
	QuantizeQuarter
	QuantizeQuarterTriplet
	QuantizeEighth
	QuantizeEighthTriplet
	QuantizeSixteenth
	QuantizeSixteenthTriplet
	QuantizeThirtySecond
	QuantizeSixtyFourth
)

func (q Quantize) String() string {
	switch q {
	case QuantizeQuarter:
		return "1/4"
	case QuantizeQuarterTriplet:
		return "1/4T"
	case QuantizeEighth:
		return "1/8"
	case QuantizeEighthTriplet:
		return "1/8T"
	case QuantizeSixteenthTriplet:
		return "1/16T"
	case QuantizeThirtySecond:
		return "1/32"
	case QuantizeSixtyFourth:
		return "1/64"
	case QuantizeOff:
		return "OFF"
	default:
		return "1/16"
	}
}

func (q Quantize) Ticks() int64 {
	switch q {
	case QuantizeQuarter:
		return PPQN
	case QuantizeQuarterTriplet:
		return PPQN * 2 / 3
	case QuantizeEighth:
		return PPQN / 2
	case QuantizeEighthTriplet:
		return PPQN / 3
	case QuantizeSixteenth:
		return PPQN / 4
	case QuantizeSixteenthTriplet:
		return PPQN / 6
	case QuantizeThirtySecond:
		return PPQN / 8
	case QuantizeSixtyFourth:
		return PPQN / 16
	default:
		return 1
	}
}

// Swingable reports whether alternating cells of this grid may be delayed.
// Triplet grids already carry their own uneven subdivision and are therefore
// intentionally excluded from swing.
func (q Quantize) Swingable() bool {
	return q == QuantizeQuarter || q == QuantizeEighth || q == QuantizeSixteenth ||
		q == QuantizeThirtySecond || q == QuantizeSixtyFourth
}

// TrackerClock is the single conversion authority for transport, metronome,
// recording, quantization, and cursors. Integer nanosecond arithmetic avoids
// floating-point drift.
type TrackerClock struct {
	Tempo       int
	PartTicks   int
	Origin      time.Duration
	CountInBars int
}

func NewTrackerClock(tempo, partTicks int) TrackerClock {
	if tempo < 20 || tempo > 400 {
		tempo = 120
	}
	if partTicks <= 0 {
		partTicks = DefaultPartTicks
	}
	return TrackerClock{Tempo: tempo, PartTicks: partTicks}
}

func (c TrackerClock) TickAt(at time.Duration) int64 {
	elapsed := at - c.Origin
	if elapsed <= 0 {
		return 0
	}
	return (elapsed.Nanoseconds()*int64(c.Tempo*PPQN) + int64(time.Minute)/2) / int64(time.Minute)
}

func (c TrackerClock) TimeAtTick(tick int64) time.Duration {
	if tick <= 0 {
		return c.Origin
	}
	nanos := (tick*int64(time.Minute) + int64(c.Tempo*PPQN)/2) / int64(c.Tempo*PPQN)
	return c.Origin + time.Duration(nanos)
}

func (c TrackerClock) PositionAt(at time.Duration) MusicalPosition {
	tick := c.TickAt(at)
	partTicks := int64(c.PartTicks)
	return MusicalPosition{
		Pattern: int(tick / partTicks),
		Row:     int((tick % partTicks) / TicksPerRow),
		Tick:    int(tick % TicksPerRow),
	}
}

func (c TrackerClock) QuantizeTick(tick int64, grid Quantize) int64 {
	return c.QuantizeTickSwing(tick, grid, 50)
}

// QuantizeTickSwing snaps to an alternating long/short grid. A value of 50 is
// straight timing; 75 places the off-beat three quarters through each pair.
// Comparing nearby boundaries, instead of first rounding to a straight cell,
// keeps quantization stable on both sides of a delayed off-beat.
func (c TrackerClock) QuantizeTickSwing(tick int64, grid Quantize, swing int) int64 {
	if grid == QuantizeOff || tick <= 0 {
		return max(0, tick)
	}
	step := grid.Ticks()
	if !grid.Swingable() {
		return ((tick + step/2) / step) * step
	}
	swing = min(75, max(50, swing))
	pair := step * 2
	offbeat := (pair*int64(swing) + 50) / 100
	pairIndex := tick / pair
	best := int64(0)
	bestDistance := tick
	for index := pairIndex - 1; index <= pairIndex+1; index++ {
		for _, boundary := range [...]int64{index * pair, index*pair + offbeat} {
			if boundary < 0 {
				continue
			}
			distance := boundary - tick
			if distance < 0 {
				distance = -distance
			}
			if distance < bestDistance || distance == bestDistance && boundary < best {
				best, bestDistance = boundary, distance
			}
		}
	}
	return best
}

func (c TrackerClock) CountInDuration() time.Duration {
	if c.CountInBars <= 0 {
		return 0
	}
	return c.TimeAtTick(int64(c.CountInBars*c.PartTicks)) - c.Origin
}
