# Legacy PICO-style SFX assets and API

> Archived compatibility documentation. Native GBA audio v2 replaces PICO
> waveforms/effects with channel-specific GBA hardware sounds and PCM samples.

## Status

This is a PICO-8-compatible SFX API backed by the provisional 16,384 Hz mono
stream. All eight instruments, all eight per-note effects, the five SFX
filters, and the exact exported speed clock are implemented in the shared
runtime/editor renderer. Automated PICO-8-to-mGBA recording gates cover these
features before the final physical-GBA sign-off.

## Project file

SFX live in `assets/audio/sfx.json`:

```json
{
  "version": 1,
  "sfx": [
    {
      "id": 0,
      "name": "jump",
      "speed": 8,
      "loop": {"start": 1, "end": 3},
      "filters": {"buzz": true, "reverb": 1, "dampen": 1},
      "steps": [
        {"pitch": 48, "waveform": "triangle", "volume": 7, "effect": "none"},
        {"pitch": 52, "waveform": "pulse", "volume": 6, "effect": "none"},
        {"pitch": 55, "waveform": "noise", "volume": 4, "effect": "none"}
      ]
    }
  ]
}
```

Version 1 limits are:

- Up to 64 effects with unique stable IDs from 0 through 63.
- One to 32 steps per effect.
- Pitch 0..63; PICO numeric pitch 33 is A at 440 Hz. PICO's displayed octave
  labels are two below scientific pitch notation.
- Volume 0..7; zero is a timed rest.
- Speed 1..255. One unit is exactly 183 samples at PICO-8's 22,050 Hz export
  rate (`8.299319... ms`, approximately `120.4918 Hz`). Mtile represents the
  same physical duration with a rational 16,384 Hz accumulator.
- Waveforms: `triangle`, `tilted-saw`, `saw`, `square`, `pulse`, `organ`,
  `noise`, and `phaser`.
- Effects: `none`, `slide`, `vibrato`, `drop`, `fade-in`, `fade-out`,
  `arpeggio-fast`, and `arpeggio-slow`.
- Optional per-SFX filters: boolean `noiz` and `buzz`, plus `detune`, `reverb`,
  and `dampen` levels 0..2.
- Optional loop `[start,end)` within the authored steps.

Unknown schema versions, unknown JSON fields, duplicate IDs/names, invalid
ranges, and unsupported effects fail before TinyGo runs. A missing file means
an empty library for compatibility with existing projects. `mtile new`
creates an explicit empty version-1 file.

## Editor workflow

Open a project, select **Audio Studio**, then **Sounds**. Press `3` to open
Audio Studio, or use Tab to cycle the three main workspaces:

```sh
go run ./cmd/mtile edit examples/audio-sfx/
```

The Sounds view is organized as a clip library, a 32-position piano roll, a
small musical inspector, and a target-accurate clip preview.

- Add, copy, or delete clips and sounds in the library. Copy creates an independent
  sound with a new game ID; existing music continues to reference the original.
- Octave labels, piano-key row shading, position numbers, and the selected note name
  keep the grid musical while preserving all 64 PICO pitches.
- Left-click or drag anywhere in the 32-step grid to place a note. Placing a
  distant note creates timed rests in between. Right-click or Delete clears
  notes without shifting the melody; `- NOTE` shortens it structurally.
- Use `Z S X D C V G B H N J M` as a chromatic keyboard. Note entry advances
  automatically. Comma/period or the visible octave buttons change octave.
- Use Left/Right to select steps and Up/Down to transpose. Shift+Left/Right
  extends a range; Ctrl/Cmd+C and Ctrl/Cmd+V copy and paste it, and Ctrl/Cmd+A
  selects the complete sound.
- The initial inspector contains only clip name, tempo/note time, note/octave,
  instrument, and loudness. Click a `-`/`+` button for small changes or click a
  numeric value to type an exact value.
- **Advanced…** reveals the game ID, loop bounds, note effect, and texture
  controls. Click the loop value to toggle looping and the instrument or effect
  value to cycle implemented choices.
- The advanced Sound Texture panel exposes `noiz`, `buzz`, `detune`, `reverb`,
  and `dampen`. Left-click advances a value and right-click reverses it. An
  all-off filter block is omitted from saved JSON.
- Click Play or press Space for the complete clip. Click Play Note or press
  Shift+Space for the selected note. Escape or Stop ends playback.
- Save with Ctrl/Cmd+S. The editor also saves when switching modes and on a
  normal quit. SFX edits participate in the existing Ctrl/Cmd+Z undo history.

All edits are validated transactionally. Invalid bounds, duplicate names or
IDs, and unsupported values are shown in the status bar without partially
changing the model. Effects that have not passed runtime and target
measurements remain unavailable rather than being approximated silently.

The waveform and desktop playback are generated in memory by the shared
`runtime/audio` sequencer and mixer at exactly 16,384 Hz. The host adapter only
converts the resulting signed 8-bit mono samples to the desktop output
container. Non-looping previews include and trim the complete reverb/dampen
tail. A looping full-effect preview is capped at four seconds; ROM loops are
not capped.

## Slide effect

`"effect": "slide"` follows the current note toward the next note's pitch and
volume, matching the direction documented by the
[official PICO-8 manual](https://www.lexaloffle.com/dl/docs/pico-8_manual.html#SFX_Editor).
Mtile updates the linear interpolation once per exact exported SFX tick.
The oscillator phase is preserved, so slide does not retrigger the waveform at
each tick.

The destination is the next step that would actually play. A loop-ending step
therefore slides toward the loop start when that loop is active. A final step,
or a step previewed through an offset/length slice that excludes its successor,
has no destination and treats slide as a no-op.

The compact target step remains eight bytes because the effect occupies its
former padding byte. In the current ARM7 benchmark, one sliding channel
generates a 256-sample block in 20,960 cycles versus 12,110 without a note
effect. Four sliding channels take 61,517 cycles versus 33,699 without a note
effect, or 23.5% of the 262,144-cycle block budget at 16,384 Hz.

## Vibrato effect

`"effect": "vibrato"` rapidly varies pitch within one quarter-tone, matching
the range documented by the
[official PICO-8 manual](https://www.lexaloffle.com/dl/docs/pico-8_manual.html#SFX_Editor).
Mtile uses a fixed 16-tick cycle with extrema at plus/minus 25 cents. At the
exact 120.4918 Hz SFX clock this is approximately a 7.53 Hz LFO. The first
value is the upper
extreme, and the cycle restarts for each vibrato step.

Only the mixer phase increment changes. Volume and oscillator phase are
preserved, and no next note is required. The target uses a 16-byte ROM table
and bounded 32-bit fixed-point arithmetic; no per-channel state or target-step
bytes were added. One vibrato channel takes 16,246 cycles per page and four
take 42,661 cycles, 16.3% of the page budget.

## Drop effect

`"effect": "drop"` linearly lowers oscillator frequency toward zero over the
current step. This duration-scaled curve matches the local PICO-8 reference
export: at effect tick `t` of a speed-`s` step, the phase increment is
approximately `base * (s-t) / s`. A speed-4 step therefore uses 100%, 75%,
50%, and 25% of its authored frequency before the sequencer advances.

Drop needs no following note and does not change volume. Updates occur once
per exact rational-clock tick while preserving oscillator phase. The target uses
one bounded 32-bit division per update and adds no per-channel state or step
bytes. One drop channel takes 19,966 cycles per page and four take 57,541
cycles, 22.0% of the page budget.

## Fade effects

`"effect": "fade-in"` raises volume linearly from silence to the authored
level over the complete current step. Local PICO-8 reference PCM shows a
continuous amplitude ramp rather than a 120 Hz staircase, so Mtile updates
the gain on every generated sample. The first sample is silent and the last is
one increment below full scale; the following step begins normally.

`"effect": "fade-out"` uses the inverse curve: the first sample has the full
authored volume and gain reaches silence at the note tail. Both curves are
duration-scaled, need no adjacent note, and leave pitch and oscillator phase
unchanged.

The mixer stores ramp volume and delta in Q16 fixed point. Plain voices stay on
the original hot path, while a separate bounded ramp path runs only while at
least one fade is active. Ownership generations prevent stale SFX channels
from attaching a ramp to a stolen voice. The eight-byte target step is
unchanged. Current whole-engine target measurements are kept in
[`performance.md`](performance.md).

## Arpeggio effects

`"effect": "arpeggio-fast"` and `"effect": "arpeggio-slow"` select pitch
from aligned four-step groups: steps 0..3, 4..7, and so on. Only pitch comes
from the selected group member. Volume and waveform continue to come from the
tracker step that is currently playing. This matches local PICO-8 0.2.6b
reference recordings and the grouping described by the
[official PICO-8 manual](https://www.lexaloffle.com/dl/docs/pico-8_manual.html#SFX_Editor).

Fast advances every four SFX ticks and slow every eight. As in PICO-8, an SFX
speed of eight or less halves those cadences to two and four ticks. Arpeggio
phase starts with the first group member at playback start and then continues
across ordinary step boundaries, aligned group changes, and loop wraps. A
sliced `sfx()` call still reads pitches from the aligned group in the complete
definition. If a compact definition ends with an incomplete group, a missing
source member keeps the current step pitch.

The cadence uses shifts and masks only; updates preserve oscillator phase and
change neither mixer layout nor the eight-byte target step. One fast or slow
channel takes 15,889 cycles per measured page and four take 41,233 cycles,
15.7% of the page budget. The full eight-command rewind takes 112,797 cycles,
or 43.0% of a page and 36.2% of the 304-sample guard.

The generated effect fixture records both PICO-8 and mGBA directly. All 11
transition/envelope cases pass, including fast/slow arpeggios at speeds 18 and
8. See [`conformance.md`](conformance.md) for the method and measurements.

## SFX filters

Filters belong to the complete SFX, matching PICO-8's editor controls:

- `noiz` selects PICO-8's pitch-clocked sample-and-hold noise mode.
- `buzz` enables the measured waveform-specific buzz variant.
- `detune` 1 or 2 mixes a second oscillator at the captured offset; organ and
  phaser retain their instrument-specific component relationships.
- `reverb` 1 or 2 uses the captured 366/732-sample PICO delays, converted to
  272/544 samples at 16,384 Hz, with captured 33/64 and 1/2 feedback.
- `dampen` 1 or 2 uses the sample-rate-converted one-pole recurrence after
  reverb.

Stateful reverb/dampen histories belong to the four logical SFX channels, so
tails survive the final note and producer checkpoint rewinds. Their histories
are fixed EWRAM rings and are never copied into checkpoints or allocated at
runtime. The compact generated definition stores PICO-8's native mixed-radix
filter byte with only the editor-mode bit removed.

## Lua API

```lua
sfx(id [, channel [, offset [, length]]])
```

- `id` selects the stable numeric asset ID.
- `channel` defaults to `-1`: first inactive channel, then the oldest of four.
- Explicit channels are 0..3. Restarting a channel stops its current owned
  voice and starts from the requested step.
- `offset` defaults to 0.
- `length` defaults to 0, meaning all remaining steps. A positive length plays
  exactly that many steps.
- `sfx(-1, channel)` stops one channel.
- `sfx(-1)` stops all channels.
- Invalid IDs, channels, offsets, and lengths fail closed and increment bounded
  runtime telemetry.

When a project links any audio assets, a literal missing ID such as `sfx(12)`
is rejected before TinyGo with the Lua source position and
`assets/audio/sfx.json` path. `sfx(-1)` remains the stop operation. Variables
and expressions are intentionally checked at runtime because their values are
not statically known. Audio-less compatibility ports may retain `sfx()` calls;
they fail closed without linking the audio shell.

A configured loop runs only when its complete loop range lies inside the
selected offset/length slice.

## Timing and current limits

Step boundaries are sample-accurate. PICO-8 exports exactly 183 samples at
22,050 Hz per speed unit. The target ratio reduces to `499712/3675` samples;
the sequencer therefore alternates 136- and 137-sample ticks with a retained
remainder instead of rounding every note. `audio.SFXSamplesForUnits` exposes
the exact cumulative boundary to host preview and music compilation. The FIFO
word path has a bounded byte bridge for the one word crossing an unaligned
boundary, while ordinary spans retain the fast four-sample packer.

The stream keeps up to 62.5 ms of audio queued for graphics tolerance. An SFX
or stop requested after startup enters a fixed eight-command queue. At the next
service point, the player restores the mixer/sequencer checkpoint for the page
after the immediate-next page and regenerates it. The current and
immediate-next pages remain untouched, so normal command latency is roughly
15.6..31.25 ms while two ready safety pages remain available. The discarded
tail page is regenerated at the following service point.

If all eight command slots are occupied, another call fails closed and
increments `DroppedCommands`. If the stream is already depleted or its target
checkpoint is unavailable, commands apply at the producer tail instead;
`FallbackCommands` exposes this exceptional, higher-latency path. `_init`
calls do not enter the queue because they run before initial stream prefill.

Games can inspect the same production counters without a debugger:

```lua
underruns = stat(80)
queued = stat(81)
voices = stat(82)
peak_voices = stat(83)
steals = stat(84)
failed_starts = stat(85)
rewinds = stat(86)
fallbacks = stat(87)
dropped = stat(88)
max_late_samples = stat(89)
```

Graphics-only games and unavailable audio state return zero. The Lua values
saturate at 32,767; Go tooling can use `Context.AudioStats()` for the complete
`uint32` stream/mixer/sequencer counters.

Generated loops service audio at frame start, after update, after draw, and
after the framebuffer copy. This bounds both update-time and draw-time calls
without mixing in the Timer 1 interrupt. On the measured TinyGo/mGBA target,
the maximum fixed queue workload (eight commands ending with four fading
voices) regenerated one page in 155,654 cycles, equivalent to 152.0 sample
periods and within both the 256-sample page and 304-sample guard. Fast and slow
arpeggios each take 112,797 cycles. Audible and real-hardware latency still
require sign-off.

See [`examples/audio-sfx`](../../../../examples/audio-sfx) for all eight waveforms,
all seven note effects, automatic/explicit channels, looping, channel stop,
and stop-all. Music ownership and transition behavior are documented in
[`music.md`](music.md).
