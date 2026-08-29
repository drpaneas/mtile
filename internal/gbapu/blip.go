// Package gbapu contains the desktop monitor's pure-Go, band-limited Game Boy
// Advance PSG renderer.
package gbapu

// This file is a Go port of blip_buf by Shay Green.
//
// Copyright (c) 2003-2009 Shay Green
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

const (
	blipPreShift      = 32
	blipTimeBits      = blipPreShift + 20
	blipTimeUnit      = uint64(1) << blipTimeBits
	blipHalfWidth     = 8
	blipEndFrameExtra = 2
	blipBufferExtra   = blipHalfWidth*2 + blipEndFrameExtra
	blipPhaseBits     = 5
	blipPhaseCount    = 1 << blipPhaseBits
	blipDeltaBits     = 15
	blipDeltaUnit     = 1 << blipDeltaBits
	blipFracBits      = blipTimeBits - blipPreShift
	blipBassShift     = 9
)

// blipBuffer converts timestamped amplitude deltas from a hardware clock into
// host-rate PCM. It deliberately owns a fixed work buffer so the audio callback
// performs no allocation.
type blipBuffer struct {
	factor     uint64
	offset     uint64
	available  int
	size       int
	integrator int64
	samples    []int64
}

func newBlipBuffer(size int, clockRate, sampleRate float64) *blipBuffer {
	if size < 1 {
		size = 1
	}
	b := &blipBuffer{size: size, samples: make([]int64, size+blipBufferExtra)}
	b.setRates(clockRate, sampleRate)
	b.clear()
	return b
}

func (b *blipBuffer) setRates(clockRate, sampleRate float64) {
	if b == nil || clockRate <= 0 || sampleRate <= 0 {
		return
	}
	factor := float64(blipTimeUnit) * sampleRate / clockRate
	b.factor = uint64(factor)
	if float64(b.factor) < factor {
		b.factor++
	}
}

func (b *blipBuffer) clear() {
	if b == nil {
		return
	}
	b.offset = b.factor / 2
	b.available = 0
	b.integrator = 0
	clear(b.samples)
}

func (b *blipBuffer) clocksNeeded(sampleCount int) int {
	if b == nil || sampleCount <= 0 {
		return 0
	}
	needed := uint64(sampleCount) * blipTimeUnit
	if needed < b.offset {
		return 0
	}
	return int((needed - b.offset + b.factor - 1) / b.factor)
}

func (b *blipBuffer) endFrame(clockDuration int) {
	if b == nil || clockDuration < 0 {
		return
	}
	offset := uint64(clockDuration)*b.factor + b.offset
	b.available += int(offset >> blipTimeBits)
	b.offset = offset & (blipTimeUnit - 1)
}

func (b *blipBuffer) addDelta(clockTime, delta int) {
	if b == nil || delta == 0 || clockTime < 0 {
		return
	}
	fixed := uint32((uint64(clockTime)*b.factor + b.offset) >> blipPreShift)
	out := b.available + int(fixed>>blipFracBits)
	if out < 0 || out+15 >= len(b.samples) {
		return
	}
	phaseShift := blipFracBits - blipPhaseBits
	phase := int((fixed >> phaseShift) & (blipPhaseCount - 1))
	interpolation := int((fixed >> (phaseShift - blipDeltaBits)) & (blipDeltaUnit - 1))
	delta2 := delta * interpolation >> blipDeltaBits
	delta -= delta2

	for index := 0; index < blipHalfWidth; index++ {
		b.samples[out+index] += int64(int(blipStep[phase][index])*delta + int(blipStep[phase+1][index])*delta2)
	}
	reverse := blipPhaseCount - phase
	for index := 0; index < blipHalfWidth; index++ {
		current := blipHalfWidth - 1 - index
		b.samples[out+blipHalfWidth+index] += int64(int(blipStep[reverse][current])*delta + int(blipStep[reverse-1][current])*delta2)
	}
}

func (b *blipBuffer) addDeltaFast(clockTime, delta int) {
	if b == nil || delta == 0 || clockTime < 0 {
		return
	}
	fixed := uint32((uint64(clockTime)*b.factor + b.offset) >> blipPreShift)
	out := b.available + int(fixed>>blipFracBits)
	if out < 0 || out+8 >= len(b.samples) {
		return
	}
	interpolation := int((fixed >> (blipFracBits - blipDeltaBits)) & (blipDeltaUnit - 1))
	delta2 := delta * interpolation
	b.samples[out+7] += int64(delta*blipDeltaUnit - delta2)
	b.samples[out+8] += int64(delta2)
}

func (b *blipBuffer) readSample() int16 {
	if b == nil || b.available <= 0 {
		return 0
	}
	sample := b.integrator >> blipDeltaBits
	b.integrator += b.samples[0]
	if sample > 32767 {
		sample = 32767
	} else if sample < -32768 {
		sample = -32768
	}
	b.integrator -= sample << (blipDeltaBits - blipBassShift)
	b.removeSamples(1)
	return int16(sample)
}

func (b *blipBuffer) removeSamples(count int) {
	if b == nil || count <= 0 || count > b.available {
		return
	}
	remaining := b.available + blipBufferExtra - count
	copy(b.samples, b.samples[count:count+remaining])
	clear(b.samples[remaining : remaining+count])
	b.available -= count
}

// Sinc_Generator(0.9, 0.55, 4.5), copied from blip_buf.
var blipStep = [blipPhaseCount + 1][blipHalfWidth]int16{
	{43, -115, 350, -488, 1136, -914, 5861, 21022},
	{44, -118, 348, -473, 1076, -799, 5274, 21001},
	{45, -121, 344, -454, 1011, -677, 4706, 20936},
	{46, -122, 336, -431, 942, -549, 4156, 20829},
	{47, -123, 327, -404, 868, -418, 3629, 20679},
	{47, -122, 316, -375, 792, -285, 3124, 20488},
	{47, -120, 303, -344, 714, -151, 2644, 20256},
	{46, -117, 289, -310, 634, -17, 2188, 19985},
	{46, -114, 273, -275, 553, 117, 1758, 19675},
	{44, -108, 255, -237, 471, 247, 1356, 19327},
	{43, -103, 237, -199, 390, 373, 981, 18944},
	{42, -98, 218, -160, 310, 495, 633, 18527},
	{40, -91, 198, -121, 231, 611, 314, 18078},
	{38, -84, 178, -81, 153, 722, 22, 17599},
	{36, -76, 157, -43, 80, 824, -241, 17092},
	{34, -68, 135, -3, 8, 919, -476, 16558},
	{32, -61, 115, 34, -60, 1006, -683, 16001},
	{29, -52, 94, 70, -123, 1083, -862, 15422},
	{27, -44, 73, 106, -184, 1152, -1015, 14824},
	{25, -36, 53, 139, -239, 1211, -1142, 14210},
	{22, -27, 34, 170, -290, 1261, -1244, 13582},
	{20, -20, 16, 199, -335, 1301, -1322, 12942},
	{18, -12, -3, 226, -375, 1331, -1376, 12293},
	{15, -4, -19, 250, -410, 1351, -1408, 11638},
	{13, 3, -35, 272, -439, 1361, -1419, 10979},
	{11, 9, -49, 292, -464, 1362, -1410, 10319},
	{9, 16, -63, 309, -483, 1354, -1383, 9660},
	{7, 22, -75, 322, -496, 1337, -1339, 9005},
	{6, 26, -85, 333, -504, 1312, -1280, 8355},
	{4, 31, -94, 341, -507, 1278, -1205, 7713},
	{3, 35, -102, 347, -506, 1238, -1119, 7082},
	{1, 40, -110, 350, -499, 1190, -1021, 6464},
	{0, 43, -115, 350, -488, 1136, -914, 5861},
}
