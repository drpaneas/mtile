# Legacy audio v1 settings and diagnostics

> Historical mono profile. Native GBA audio v2 uses the active settings
> specification in `../../settings.md`.

The editor's **Audio Studio > More… > Diagnostics** screen edits the
project-wide master gain and reports the measured production profile selected
by the current assets. It is intentionally outside the normal composing path.

## Settings file

New projects contain `assets/audio/settings.json`:

```json
{
  "version": 1,
  "output": "mono",
  "sampleRate": 16384,
  "voiceProfile": "auto",
  "masterVolume": 256
}
```

`masterVolume` is an integer from `0` (mute) through `256` (unity). In the
Diagnostics, Left/Right changes it by 16, Shift+Left/Right changes it by 1,
Home mutes, End restores unity, and the `-`/`+` buttons follow the same step.
Saving persists the setting; undo and redo include it. SFX, music, sample, and
target previews all use the in-memory value, and generated cartridges apply it
before playback begins.

The output, sample rate, and voice profile fields are versioned and persisted,
but remain locked to the measured values. Unknown fields, future versions,
stereo output, unmeasured rates/profiles, and master values outside `0..256`
fail validation before TinyGo runs. Legacy projects without the file load as
the defaults above and are not rewritten merely by opening or saving them.

## Production profile

The build derives the voice pool from authored assets:

| Project audio | Output | Rate | Voice pool |
| --- | --- | ---: | ---: |
| No SFX, music, or samples | runtime dead-stripped | — | — |
| SFX and/or music | mono FIFO A | 16,384 Hz | 8 |
| Any native PCM sample | mono FIFO A | 16,384 Hz | 12 |

Four logical PICO channels preserve oscillator phase across compatible note
cells and can each overlap one 48-sample release tail during starts, stops, or
incompatible replacements. The eight-slot synth pool removes edge clicks
without increasing the authored channel count. Sample projects retain those
transient slots plus PCM overlap in the fixed twelve-slot mixer. These are allocation limits, not CPU
support promises. The measured common-scene safety boundary remains two
simultaneous synth voices plus one PCM voice; see
[performance.md](performance.md#measured-pcm-concurrency-boundary).

## Project costs

Diagnostics recompiles the editor's in-memory sounds, music, and sample assets
with the build compilers. Its ROM figure uses the target record layout: 8 bytes
per SFX definition, 8 per step, 12 per music pattern, 24 per sample definition,
plus the exact signed 8-bit PCM bytes.

Retained target-map state is shown separately:

- Mono stream queue and telemetry: 1,936 bytes EWRAM.
- SFX/music/sample player, four checkpoints, and four fixed filter histories:
  19,376 bytes EWRAM.
- Mixer: 1,328 bytes IWRAM.

When no audio is linked, those fixed runtime costs display as absent rather
than being charged to the project.

## Benchmark versus live telemetry

The measured section uses checked-in 2026-08-23 ARM7TDMI/mGBA baselines:

- Four synth voices: 31,689 cycles per 256 samples (12.1%).
- Four synth voices plus one PCM voice: 217,103 cycles (82.8%).
- Maximum measured note-effect command-rewind producer case: 155,654 cycles (59.4%).
- Graphics matrix: zero unforced underruns; synth-only fits 3/7 scene budgets,
  full four-synth-plus-PCM fits 2/7, and the two-synth-plus-PCM safety boundary
  fits 5/7.

These are profile baselines, not observations of the current game. Last-run
mix cycles, underruns, peak voices, and voice steals stay `—` until a cartridge
telemetry import path is implemented. Cartridge counters are available to game
code through `stat(80..89)` and to Go tooling through `Context.AudioStats()`;
the editor does not claim those values were imported.

## Stereo candidate

The lower signal rail shows the experimental two-voice dual-FIFO profile:
78,168 cycles per block (29.8%), 3,264 bytes EWRAM for `StereoStream`, and
exact mGBA left/right capture correlation. It is visibly marked **not
selectable**. Importing a stereo WAV, opening the dashboard, or saving the
project cannot enable stereo; real-hardware listening and user-facing pan
controls remain post-version-1 gates.
