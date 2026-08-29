# gb_apu and blip_buf attribution

Mtile's desktop audio monitor in `internal/gbapu` is a pure-Go adaptation of
the four Game Boy PSG channels implemented by
[`ITotalJustice/gb_apu`](https://github.com/ITotalJustice/gb_apu). The upstream
project is Copyright (c) 2024 TotalJustice and distributed under the MIT
License. Its license is retained in `docs/third-party/gb-apu-LICENSE.txt`.

The adaptation is deliberately limited to the GBA's inherited CH1-CH4 PSG
hardware. It does not include Direct Sound, FIFO A/B, or a general-purpose Game
Boy emulator. Mtile supplies its own Go-facing sound definitions and scheduling
while preserving the hardware clocks, channel state machines, frame sequencer,
length counters, envelopes, sweep, wave RAM playback, and noise LFSR behavior.

The band-limited resampler in `internal/gbapu/blip.go` is a Go port of Shay
Green's `blip_buf` and retains its full MIT notice in that source file. No C
code, cgo library, or runtime download is required by Mtile.

The renderer is covered by focused Go tests for channel timing and behavior,
allocation-free callbacks, output routing, phase-preserving retriggers, and
click/live equivalence across every bundled factory preset. These tests are
Mtile tests; they are not a claim that the Go adaptation has yet run every
upstream Blargg or Numism ROM suite.
