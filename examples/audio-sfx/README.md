# audio-sfx

> **Legacy audio v1 fixture.** This project remains runnable while native-GBA
> audio v2 is developed, but PICO instruments and effects are no longer the
> forward audio contract. See the
> [archived v1 documentation](../../docs/audio/legacy/v1/).

The first versioned SFX asset and runtime example. It demonstrates all eight
PICO instruments; tick-accurate pitch/volume slides, vibrato, frequency drop,
sample-continuous fades, and both arpeggio cadences; all five PICO filter
controls; automatic and explicit logical channels; a loop; channel stop; and
stop-all.

From the repository root:

```sh
go run ./cmd/mtile edit examples/audio-sfx/
go run ./cmd/mtile build examples/audio-sfx/
```

Select the SFX tab to inspect, edit, and preview all eight waveforms through the
same 16,384 Hz mixer used by the ROM. Save with Ctrl/Cmd+S before rebuilding.

Controls in mGBA:

- A: play the detuned vibrato/fade-out `coin` on an automatic channel
- B: play the dropping `noiz`/dampen `noise-hit`
- Left: play the tilted-saw `fast-arpeggio`
- Right: play the organ `slow-arpeggio`
- Up: start the sliding buzz-saw `looping-alarm` on logical channel 0
- Down: stop logical channel 0
- Start: play the reverb/dampen `phaser-sweep`
- Start+Select: stop all SFX channels
- Select: toggle the live `stat(80..89)` audio diagnostics screen

The diagnostics screen reads the production cartridge state directly. It
shows stream underruns and queue depth, current/peak voices, steals, failed
starts, successful buffered-page rewinds, command fallbacks/drops, and maximum
IRQ lateness. The counters are observational only and do not enable the cycle
profiler or change mixing. See the
[legacy audio troubleshooting guide](../../docs/audio/legacy/v1/troubleshooting.md) for
interpretation.

The current experimental slice uses exactly 16,384 Hz mono and four logical
channels. Each authored `speed` unit is exactly 183 samples at PICO-8's
22,050 Hz export rate (`8.299319... ms`), represented on target by a retained
rational accumulator. The `slide`
effect approaches the next played step's pitch and volume once per interval;
`vibrato` follows a 16-tick quarter-tone pitch cycle; `drop` linearly lowers
frequency over each step; the first startup note uses `fade-in`, and the coin's
final note uses `fade-out`. Fast and slow arpeggios select pitch from aligned
four-step groups at their measured PICO cadences. Unsupported effects fail
during asset validation.
Runtime calls use the checkpointed page-after-next queue, so
normal button-to-audio command latency is bounded to roughly 15.6..31.25 ms;
audible real-hardware sign-off is still pending.
