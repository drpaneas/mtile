# Legacy audio v1 troubleshooting

> Historical software-synth/mono-stream troubleshooting. Use
> `../../troubleshooting.md` for native GBA audio v2.

Mtile's current production audio path is 16,384 Hz, signed 8-bit mono through
GBA FIFO A. Synth-only projects expose four logical PICO channels and reserve
eight physical mixer slots so each channel can overlap a short release tail.
Importing any native PCM sample selects the twelve-slot profile. Stereo
authoring and the dual-FIFO backend remain experimental and cannot be enabled
from a project.

## Start with these checks

1. Build the project again and read the complete error. Invalid SFX, music, and
   sample references fail before TinyGo links the ROM.
2. Confirm the project contains the asset being called:
   `assets/audio/sfx.json`, `assets/audio/music.json`, or
   `assets/audio/samples/manifest.json` plus its WAV source.
3. Confirm the Lua ID matches the stable asset ID shown by the editor. IDs are
   not array positions and gaps are allowed. Literal missing IDs are reported
   with their Lua source position and exact audio asset file before TinyGo;
   variable/expression IDs fail closed at runtime instead.
4. Open the relevant editor tab and use its preview. If preview is also silent,
   inspect authored volume, note range, pattern assignments, and sample data.
5. If preview works but the ROM is silent, verify mGBA and the operating system
   are not muted, then display the runtime counters described below.

The editor is useful for asset problems; the cartridge counters are useful for
runtime scheduling and voice-allocation problems.

## Runtime counter overlay

Generated audio cartridges expose a bounded diagnostic snapshot through
`stat(80)` through `stat(89)`. A temporary `_draw()` overlay can print the most
important values:

```lua
print("underruns", 4, 112, 7)
print(stat(80), 76, 112, 7)
print("queued", 4, 120, 7)
print(stat(81), 76, 120, 7)
print("voices", 4, 128, 7)
print(stat(82), 76, 128, 7)
print("steals", 4, 136, 7)
print(stat(84), 76, 136, 7)
print("drops", 4, 144, 7)
print(stat(88), 76, 144, 7)
print("late", 4, 152, 7)
print(stat(89), 76, 152, 7)
```

| ID | Meaning | What an unexpected value suggests |
| ---: | --- | --- |
| 80 | Stream underruns | Normal code did not publish the next block before playback needed it. |
| 81 | Queued blocks | Zero while audio should be running indicates starvation or stopped production. |
| 82 | Active physical voices | Confirms whether a call reached the mixer. |
| 83 | Peak active voices | Shows the highest concurrency reached since startup. |
| 84 | Voice steals | A higher-priority or newer request reused a busy voice. |
| 85 | Failed starts | A request could not obtain an eligible physical voice. |
| 86 | Successful command rewinds | Normal evidence that calls were inserted into already-buffered audio. |
| 87 | Command fallbacks | A command used the producer tail because its rewind checkpoint was unavailable. |
| 88 | Dropped commands | The fixed eight-command queue filled before it could be serviced. |
| 89 | Maximum IRQ lateness, in samples | Large values point to work that delays audio page handoff. |

Graphics-only cartridges and unavailable audio state return zero. Cumulative
Lua counters saturate at 32,767 instead of wrapping negative. Go runtime tools
can read the unsaturated combined snapshot with `Context.AudioStats()`.

## Silence or a call that appears to do nothing

- `sfx()` needs a valid SFX ID. An explicit channel may replace what that
  channel was already playing; automatic selection is inactive-first, then
  oldest.
- A step with volume `0` is intentionally silent. A requested offset/length
  slice can also exclude the audible part of an SFX.
- `music()` pattern slots containing `-1` are silent. A pattern stops when its
  `next` link is `-1`; broken references are rejected at build time.
- `sample()` needs a valid compiled sample ID. Check that the imported WAV has
  audible source data and that playback volume is not zero.
- Voice priority matters. If `stat(84)` or `stat(85)` rises, reduce concurrency,
  lower the priority of expendable samples, or avoid restarting effects every
  update tick.
- Calls made during `_init()` are supported: generated games attach the player
  before `_init()` and include startup audio in the initial stream pages.

## Dropouts, clicks, or intermittent silence

First watch `stat(80)`, `stat(81)`, and `stat(89)`. An increasing underrun count
with a depleted queue is a scheduling problem, not a missing asset. The stream
fails closed to signed PCM silence and can recover without rebooting audio.

The current four-synth mono profile fits empty, sparse-map, and complete
GBA-class scenes. Clear/HUD, 100-sprite, dense-map, and complete PICO-class
scenes remain over one 30 Hz work budget. With native PCM, keep ordinary
concurrency near the documented two-synth-plus-one-PCM safety boundary; the
twelve-slot pool is an allocation capacity, not a promise that a full active
mix fits every graphics scene. If a game resembles an over-budget scene:

- avoid rebuilding the same dense screen when nothing changed;
- reduce large per-frame sprite and map workloads;
- avoid bursts of more than eight audio commands between service points;
- inspect `stat(87)` and `stat(88)` for command-pressure problems;
- reproduce with a smaller scene to separate rendering pressure from the
  authored sound.

`stat(1)` and `stat(2)` still report frame/update timing, but they do not replace
the dedicated audio counters.

## Clipping or harsh output

The mixer sums voices and saturates at signed 8-bit output. Several loud voices
can therefore flatten peaks even though no counter reports an underrun. Lower
SFX step volumes, sample playback volume, or the number of simultaneous loud
voices. You can also lower the project master gain in **Audio Studio > More… >
Diagnostics**; the persisted `0..256` value affects editor target previews and
the cartridge.

Some brightness is intrinsic to simple pulse/saw waveforms and the 16,384 Hz
target rate. Compare the editor's exact target preview before treating it as a
runtime defect. Noise steps and high notes are intentionally rougher than a
desktop synthesizer.

## Editor preview differs from the cartridge

SFX and Music previews use the same sequencer and mixer as the target. The
Sample editor has two distinct auditions:

- **Source** plays the decoded WAV, preserving its original left/right
  channels for inspection.
- **Target** plays the converted signed 8-bit, 16,384 Hz mono data that is
  packed into the ROM.

A stereo source is deterministically averaged to mono for the version-1 target.
Opening or previewing it does not enable stereo output. Looping, conversion,
and volume should be judged with Target preview.

**Audio Studio > More… > Diagnostics** edits master gain and reports checked-in
target measurements and current project asset sizes. Its live-counter fields remain blank because
the editor does not yet import telemetry from a running cartridge; use the
`stat()` overlay for live values.

## Emulator and recording checks

- Click the mGBA window before testing input-triggered sounds. Its default GBA
  A/B mapping is X/Z.
- Check both mGBA's mute/volume state and the operating-system output device.
- Other desktop audio, such as YouTube Music, does not alter Mtile's samples,
  but it can mask what you hear through the same speakers or headphones.
- Mtile's reproducible mGBA capture scripts read mGBA's internal
  `mAVStream` callback. They do not open a microphone or record YouTube/system
  playback, so captured WAVs can be compared with the generated Go reference.

Emulator success is not final hardware sign-off. The current stream, audible
quality, and stereo candidate still require the real-GBA listening and
DMA/timer gates listed in [architecture.md](architecture.md).

## Reporting a reproducible audio bug

Include the smallest project that reproduces it, the generated ROM, Mtile,
TinyGo, and emulator versions, whether editor Target preview matches, and the
values of `stat(80)` through `stat(89)` before and after the problem. For a
visual/editor regression, include the project path and a screenshot of the
affected tab. Do not use an operating-system microphone recording as the only
audio artifact when an internal mGBA capture is available.
