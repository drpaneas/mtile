# Mtile four-channel Game Boy audio plan

## Status

**PSG-only hardware scope locked. The cartridge runtime and single-screen
desktop Music Lab are implemented. Physical GBA and the remaining Launchkey
transport/hotplug/latency sign-off remain external release gates.**

Audio v2 uses only the four Game Boy sound channels built into the Game Boy
Advance. Direct Sound A/B, FIFO streaming, PCM samples, and software mixing are
explicitly deferred. They are neither hidden advanced features nor release
gates for this version.

The superseded software mixer and its PICO conformance tooling have been
removed. Audio v1 assets are rejected before compilation or editor writes.

### Implementation checkpoint — 2026-08-28

| Area | State |
| --- | --- |
| CH1–CH4 register layer and single-owner shared state | Implemented and host-tested |
| Timer 0 256 Hz commit clock | Implemented and TinyGo-tested |
| Bounded 96-PPQN scheduler and ownership | Implemented and host-tested |
| Strict v2 assets, compiler, and Lua APIs | Implemented; v1/PCM fail closed |
| Unified MUSIC/SFX composition + arrangement surface | Implemented; same composer, no audio pages |
| Keyboard performance and automatic tracker recording | Implemented and host-tested |
| Launchkey Mini 25 MK4 zero-config integration | Implemented; physical CoreMIDI discovery, outbound OLED/LED handshake, keys, pad, and encoder pass; final transport/hotplug/latency run pending |
| Native examples and editor round trip | Implemented |
| GB Studio factory preset register/macro fidelity | 52/52 materialize and audition; exact 72-note hUGE periods and all 14 CH4 NR43 sequences golden-tested |
| 42-case mGBA truth captures and graphics coexistence soak | Passing |
| Physical GBA listening/capture matrix | Pending external hardware run |

MIDI performance is desktop input only. MIDI is never a cartridge asset and
never adds runtime voices. Every recorded gesture is materialized as ordinary
CH1–CH4 events before a cartridge is built.

## Product promise

The GBA hardware generates all four voices. Mtile writes a small, bounded set
of registers when notes start, change, or stop. It does not synthesize every
sample on the ARM7 CPU, refill an audio FIFO, or run a PCM mix loop.

- Authentic Game Boy sound on a GBA.
- Four visible, fixed-purpose channels.
- Low and predictable target CPU use.
- No audio DMA, sample ROM/EWRAM, or stream underruns.
- One editor surface that reads like a musical instrument, not a hardware lab.

## Hardware channels

| Channel | Friendly role | Real controls |
| --- | --- | --- |
| CH1 | Pulse + sweep; lead | frequency, trigger, length, 12.5/25/50/75% duty, hardware envelope, sweep, route |
| CH2 | Pulse; harmony/bass | frequency, trigger, length, duty, hardware envelope, route |
| CH3 | Custom wave; bass/timbre | frequency, trigger, length, 32/64-sample 4-bit Wave RAM, bank, output level, route |
| CH4 | Noise; drums/effects | trigger, length, 7/15-bit LFSR, divisor, shift, hardware envelope, route |

These are not interchangeable software tracks. Only CH1 has sweep, only CH3
has Wave RAM, and CH4 is noise rather than a pitched piano channel. CH1, CH2,
and CH4 have Game Boy envelopes, not ADSR. CH3 has no hardware envelope. The
editor must not invent filters, reverb, arbitrary oscillators, PCM, or FIFO
routing.

CH3 pitch is dimension-aware: because the frequency register clocks Wave RAM
samples, a 64-sample waveform uses a different period from a 32-sample
waveform for the same note. Asset validation, target playback, desktop preview,
live preview, and `gba_note()` must all use the concrete sound dimension.

## Vocabulary

- **Channel**: one physical PSG voice, CH1 through CH4.
- **Sound**: one reusable configuration valid for exactly one channel.
- **Clip**: a bounded note/event sequence for exactly one channel.
- **Part**: one named musical section containing up to four simultaneous clips.
- **Song**: an ordered arrangement of parts.
- **Arm**: select the physical channel or channels that live performance may
  use. Arming does not record.
- **Take**: one raw timestamped performance retained for non-destructive
  quantization before it is materialized into CH1–CH4 events.

The UI calls reusable musical units parts (Intro, Verse, Chorus). It does not
ask a beginner to understand pattern IDs, buses, FIFOs, or MIDI CC numbers.

## Runtime architecture

```text
music(), sfx(), gba_note()
            |
  bounded command queue
            |
  96-PPQN integer scheduler
            |
    256 Hz commit clock
            |
   CH1   CH2   CH3   CH4
 pulse pulse  wave  noise
            |
       GBA PSG mixer
```

- Work per commit is bounded by four channels and a fixed command queue.
- No target path performs per-sample synthesis, software mixing, FIFO refill,
  audio DMA, or post-initialization allocation.
- Shared PSG registers have one owner. CH3 Wave RAM uses a bank-safe update.
- Trigger and legato period updates are distinct operations.
- The rational clock prevents long-run drift at non-integral tempos.
- Direct calls, gameplay SFX, and song playback use deterministic ownership and
  cannot accidentally stop another owner class.

## Assets and APIs

```text
assets/audio/
├── settings.json
├── sounds.json
└── music.json
```

`sounds.json` contains only CH1–CH4 definitions. `music.json` contains typed
clips, four-lane parts, song order, and optional editor-only raw take metadata.
The compiler strips raw takes from generated cartridge data. `settings.json`
contains only PSG levels, PSG ratio, and the fixed `gameboy-psg` profile.

Beginner Lua:

```text
sfx(id [, priority])
music(id)
```

Native Lua:

```text
gba_note(channel, note, sound_id [, duration_ticks])
gba_stop(channel)
```

There is no sample, FIFO, bus, PCM-rate, or software-voice API in Audio v2.

## Unified Music Lab

The editor uses one persistent spatial model:

```text
transport + ARM/REC + performance mode
channel rail | active part piano roll | hardware inspector
             | complete song order directly below
```

There are no audio subtabs. Composition and arrangement are two scales of the
same document and remain visible at the same time.

### Channel rail

- Four fixed cards remain visible. Their colors continue through the grid,
  allocation rail, and song order.
- Selecting a channel reveals only its real controls.
- Mute/solo affect preview only.
- MONO arms one selected pitched channel; POLY2 arms CH1+CH2; POLY3 arms
  CH1+CH2+CH3; NOISE arms CH4.

### Active part composer

- The selected named part is edited directly in the main piano roll. CH4 turns
  the same area into a labeled hardware-noise drum grid.
- Visible part boundaries and names select a part without changing screens.
- Notes support click/drag, keyboard and MIDI input, right-click erase,
  Delete/Backspace, undo/redo, exact straight/triplet/dotted durations, and
  moving playback/record cursors. Shift-drag creates a rectangular multi-note
  selection; copy/paste/delete and Shift+Arrow operate on the selected notes
  without flattening their relative timing.
- The piano keyboard and octave controls expose only the native frequency
  range of the selected CH1, CH2, or CH3 hardware channel.
- CH3 includes its Wave RAM drawer. CH4 offers named hardware-noise presets,
  never audio samples.

### Song order

- A compact order strip immediately below the composer always shows the whole
  arrangement, for example `Intro -> Verse -> Chorus -> Verse -> Ending`.
- Parts can be added, renamed inline, duplicated, reused, reordered, deleted,
  and looped. Reuse adds another reference; duplicate creates an independent
  copy.
- Users never wire next pointers.

### Live performance

- Mac keyboard and MIDI controllers emit the same `PerformanceEvent` stream.
  Neither input path writes tracker data directly.
- MONO uses last-note priority.
- POLY2 uses CH1+CH2: highest active pitch on CH1, lowest on CH2.
- POLY3 uses CH1+CH2+CH3: high/middle/low respectively. One note uses only
  CH1; two use CH1+CH2.
- If held notes exceed capacity, newest notes win. The full held set is kept so
  an older held note returns when a newer one is released.
- The live allocation rail exposes the physical result, for example
  `G4 -> CH1 | E4 -> CH2 | C4 -> CH3`.
- In poly modes CH3 uses a generated 32-sample pulse-equivalent wave. Duties
  12.5/25/50/75% are 4/8/16/24 high nibbles followed by low nibbles.

### Recording

- ARM enables live preview. REC additionally captures raw timestamps and the
  allocator's actual channel starts, stops, and reassignments.
- One `TrackerClock` drives playback, recording, metronome, count-in, loop
  boundaries, quantization, and every cursor.
- Quantize is Off, straight 1/4 through 1/64, or quarter/eighth/sixteenth
  triplet. Straight grids support 50–66% swing. Raw timing is retained so
  re-quantization never compounds rounding.
- The piano-roll snap is independent from record quantization and preserves
  exact 96-PPQN positions. Note-length presets include straight, triplet, and
  dotted values.
- Count-in is Off/1/2 bars. Loop recording retains Take 1..N. Overdub merges;
  replace mode replaces only the recorded range.
- Internal note/parameter events are materialized into exact physical channel
  events and real sound records. No Poly Track or runtime chord event exists.
- Parameter timing is explicit: duty/wave/noise-shape controls `RETRIGGER`,
  envelope/sweep controls apply on `NEXT_NOTE`, and `IMMEDIATE` is reserved
  until a safe live-register path exists. Preview and recorded tracker output
  use the same classification.

### Launchkey Mini 25 MK4

- Detect the supported device, open its performance input/output ports, and
  configure it with no MIDI Learn, CC setup, or Novation Components preset.
- Connect enables programmer/DAW control, pad and encoder modes, channel-color
  LEDs, and OLED status. Close exits DAW mode. Disconnect sends all-notes-off
  and safely finalizes an active take.
- Keys drive the selected performance mode. Velocity and aftertouch are ignored
  because the Game Boy channels do not implement them.
- Eight drum pads trigger kick, snare, closed/open hat, two toms, click, and FX.
  Other pads choose modes and toggle metronome/count-in/loop/overdub.
- Eight endless encoders change real hardware fields only. Relative values use
  64 as the pivot. Linked changes materialize only on hardware-compatible
  reserved channels: duty on CH1–CH3, envelopes on CH1–CH2, sweep on CH1, and
  wave level on CH3. No no-op automation is recorded.
- PLAY toggles playback and RECORD toggles recording. LEDs/OLED use musical
  labels, never raw MIDI terminology.

### Real-time boundary

- MIDI callbacks and the audio reader communicate through bounded preallocated
  queues and do not wait for `ebiten.Update`.
- The audio callback owns oscillator state and does no formatting, UI work,
  blocking I/O, or per-sample allocation.
- Preferred MIDI-to-state latency is under 2 ms. Preferred audible latency is
  at most 15 ms; the supported-Mac release maximum is 20 ms.
- Preview and cartridge playback share note periods, waveform tables,
  envelopes, sweep, noise parameters, and channel semantics.
- Desktop PSG audition renders at the 48 kHz host rate, integrates pulse, Wave
  RAM, and LFSR transitions over each output sample, and applies the same
  AC-coupled high-pass behavior as a hardware emulator. It never routes native
  PSG through the legacy 16,384 Hz Direct Sound preview path.

### External design reference

GB Studio validates the one-document model: its piano roll renders pattern
blocks across the sequence and keeps order editing in the same page. Mtile
does not copy GB Studio code or format; the reference only removes unnecessary
mode boundaries.

## Engineering milestones

### 0 — PSG-only contract

- Active docs, schema, APIs, and UI expose only CH1–CH4.
- PCM/Direct Sound assets are unsupported and fail closed in v2.

### 1 — Hardware truth

- Direct-register CH1–CH4 conformance ROMs and TinyGo allocation checks.
- mGBA and physical-GBA matrix for duty, sweep, envelope, Wave RAM/level,
  noise, length, routing, and cold-start first note.

### 2 — Controller and scheduler

- Central shared-register ownership, safe CH3 writes, Timer 0 lifecycle.
- Fixed queue, rational 96-PPQN clock, note-off, ties, loops, part changes,
  deterministic song/SFX/direct ownership, and telemetry.

### 3 — Assets, compiler, APIs, preview

- Strict JSON, typed validation, compact generated tables, dead stripping,
  beginner/native Lua lowering, and clear legacy/PCM diagnostics.
- Preview and target use the same accepted hardware parameters.

### 4 — One-screen Music Lab

- Remove CREATE/SONG/GBA and render one active-part composer plus one always
  visible order strip and hardware inspector.
- Preserve mouse editing, inline rename, copy/paste, delete, undo/redo,
  moving cursor, key highlighting, section creation/reuse/duplicate/reorder.
- Support channel-aware pitch navigation, rectangular multi-note editing,
  exact tuplets/dotted notes/swing, common and custom meters, and optional
  per-part tempo overrides without another screen.
- Verify with interaction tests and rendered screenshots.

### 5 — Performance engine and recorder

- Shared keyboard/MIDI events and deterministic MONO/POLY2/POLY3/NOISE
  allocation with held-note restoration.
- Generated CH3 pulse waves, master clock, ARM/REC, count-in, metronome,
  loop takes, overdub/replace, raw timestamps, quantization, and materialization.
- Low-latency streaming PSG preview independent of the UI frame loop.

### 6 — Launchkey integration

- Generic MIDI parser/transport with fake-device tests.
- Exact-device adapter, relative encoders, pads, transport, LEDs/OLED, DAW
  lifecycle, reconnect safety, golden-byte protocol tests, and latency metrics.
- Physical controller acceptance run.

### 7 — Release verification

- Keyboard and Launchkey examples, full Go/TinyGo/race/fuzz/property tests,
  no-allocation audio gate, editor screenshots, mGBA captures, graphics soak,
  documentation audit, physical Launchkey run, and physical-GBA sign-off.

## Definition of done

1. Only CH1–CH4 appear in active assets, APIs, generated runtime, and editor.
2. Every accepted control maps to real Game Boy hardware behavior.
3. Scheduler and live audio paths are bounded, drift-free, and allocation-free
   where required.
4. Replacing or stopping one channel preserves the other three.
5. Preview, generated target state, and mGBA output agree.
6. Four-channel playback survives representative graphics load.
7. The CH1–CH4 matrix passes on mGBA and physical GBA.
8. Composition and arrangement are simultaneously visible in one Music Lab;
   no CREATE/SONG/GBA switch remains.
9. Keyboard and Launchkey produce the same allocator events, and recorded
   chords become only exact CH1–CH3 transitions.
10. Raw takes re-quantize without cumulative error; loop takes,
    overdub/replace, hotplug, and all-notes-off are deterministic.
11. Launchkey connects with no setup and controls transport plus real PSG
    parameters with correct LED/OLED feedback.
12. A beginner can create, perform, record, arrange, save, build, and hear a
    song without PCM, FIFO, PICO, MIDI CC numbers, or generic DAW concepts.
13. Straight, triplet, dotted, and swung timing survives edit, save, preview,
    compilation, and target scheduling without grid rounding.
14. Meter and part-tempo changes remain explicit arrangement metadata and take
    effect at exact part boundaries.

Direct Sound/PCM may be proposed later as a separately planned version. It is
not partially exposed in this one.
