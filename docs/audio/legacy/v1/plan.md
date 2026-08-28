# Legacy Mtile audio v1 engineering plan

> Historical snapshot. This PICO-8-compatible Direct Sound design was
> superseded by the native GBA audio v2 plan in `../../plan.md`. It remains
> available as engineering evidence and is not the current product contract.

## Status

**Version-1 audio is implemented through host, target-build, mGBA, the complete
870-case synthetic PICO-8 gate, and the supplied 229-cart PICO-8 music corpus;
production sign-off now requires only the physical-GBA run below.**

Milestone 0 real-hardware probes remain open. The interrupt-assisted stream and
Milestone 1 shared-mixer foundation are complete through host and mGBA gates.
The 16,384 Hz/four-channel
candidate now passes the full mGBA graphics matrix with zero unforced
underruns, including forced-silence recovery; real-hardware and audible gates
remain open. Milestone 2 now has a version-1 SFX schema, strict host validation,
sample-clocked four-channel sequencer, real `sfx()` lowering, generated TinyGo
integration, bounded 15.6..31.25 ms full-queue command rewind, and
`examples/audio-sfx`. The editor can now author, validate, undo, save, and
preview SFX through the shared target mixer. PICO-8 0.2.6b and mGBA 0.10.3
internal-audio captures now pass all 512 instrument/pitch cases, all 11
note-effect cases, all 92 filter cases, and all 255 speed values: 870/870
external cases pass. The album corpus adds 229 carts, 289 distinct music entry
points, and 153 cart/reference pairs: every entry imports and exports, the
median effective chroma/timbre similarities are 0.9757/0.9677, and all
engine-comparable references clear 0.90 after multipart mastering and one
mismatched supplied MP3 are classified. Slide, vibrato, drop, sample-continuous
fades, and both arpeggio modes complete the measured PICO note-effect set.
PICO's measured 64-sample attack/release edges are represented as 48-sample
target-clock ramps. Compatible cells and explicit channel retriggers preserve
oscillator phase; release/attack overlap is used only when a source must be
replaced. Generated synth projects reserve eight mixer slots for four logical
notes plus four release tails; sample projects reserve twelve for those tails
and native PCM overlap. A 30.123-second internal mGBA capture
of the imported Zelda Fortune Teller example has zero sample jumps above 0.15
(maximum 0.09375), after the Timer-1 handoff was made deterministic across
mGBA's progress-4/5 FIFO-DMA race. Preserving channel phase also removes the
audible speed-locked background pulse and improves the reference comparison's
chroma/timbre scores from 0.9733/0.9559 to 0.9918/0.9702. Physical hardware
remains the authority for the pending device sign-off.
Version-1 music assets now compile a stable four-channel pattern
graph; real `music()` calls start, transition, loop, stop, and share the
checkpointed command path with manual SFX. `examples/audio-music` links on the
TinyGo target. A deterministic mGBA internal-audio capture now cross-correlates
the fast/slow pitch contours and aligned-group transition against local PICO-8
references. The native Music Lab now presents instrument choices, a horizontal
four-track arrangement, piano roll, clickable keyboard, song tempo, and target
preview in one screen while continuing to author the stable-ID pattern graph
underneath. Sample-continuous `music()` fade-in/fade-out now shares the fixed
command and checkpoint path. Exact mGBA extraction measures its four-channel
fade at 44,276 cycles per page versus a 33,808-cycle no-fade control. The
music-active graphics matrix has zero unforced underruns and fallbacks; three
workloads fit one 30 Hz work budget, while four scene classes remain over
budget. Milestone 4 now has a strict version-1 sample
manifest, integer PCM WAV converter, stable/deduplicated signed 8-bit ROM
records, allocation-free `sample()` playback on the checkpointed producer,
loop/volume/priority semantics, a drag-and-drop Sample editor with source and
target previews, build-size telemetry, and `examples/audio-samples`. The
example links as a 77,208-byte ROM with 7,998 audio-asset bytes under the
twelve-slot transient pool and actively demonstrates the measured two-synth-plus-PCM
safety profile. Its four-synth-plus-PCM cartridge matrix now has zero
unforced mGBA underruns, retains one PCM voice through every checkpoint, and
recovers three queued pages before 68 clean post-starvation handoffs. Music
packing, a block word mixer, and active-only checkpoints cut its empty-scene
cost from 177.4% to 99.3%. The full profile fits two 30 Hz scenes. Two synth
voices plus PCM fit five common scenes, including complete PICO- and GBA-class
ticks; three synth voices fit four because its PICO case is now 100.8%. The
two-voice profile remains the safety boundary. The 100-sprite and dense-map
stresses remain outside that boundary; real-hardware listening remains open.
Audio Studio diagnostics now persist a strict version-1 settings document and edit a
project-wide `0..256` master gain used by previews and generated cartridges;
output, rate, and derived voice profile reject unmeasured values. Host fuzz
targets cover WAV boundaries, JSON validation/round trips, loop conversion,
byte/word mixer agreement, and music transitions. Milestone 5 now has
allocation-free shared left/right mixing, fixed-point linear pan, FIFO-word
packing, centered mono-byte compatibility, and an experimental synchronized
four-page target stream using FIFO A/DMA1 for left and FIFO B/DMA2 for right.
An isolated ARM7TDMI benchmark
measures two hard-panned fading voices at 29.8% of one page, four distributed
fading voices at 52.8%, and a centered 4+1 synth/PCM mix at 58.8%. The first
mGBA stream run completed 623 paired handoffs with zero underruns and three
queued pairs. A subsequent
graphics/recovery matrix gives the provisional two-voice profile zero
unforced underruns, exactly five forced misses, and 68 clean recovery handoffs;
four common scenes fit 30 Hz while the 100-sprite, dense-map, and PICO-class
scenes remain over budget. The four-voice stereo graphics profile is rejected.
Project-level stereo and pan controls are explicitly deferred from version 1,
not left as a mono-release blocker. A deterministic mGBA
internal-callback WAV now proves channel routing against the shared Go
reference: both own-channel correlations are 1.0, both swapped correlations
are 0, and two independent captures have the same SHA-256.
The heavy stereo scenes are rejected and stereo hardware/UI sign-off is
explicitly deferred beyond version 1.
Provisional resource decisions, candidate timer clocks, probe results, and the
hardware sign-off checklist are recorded in
[architecture.md](architecture.md). Exact target mixer measurements are in
[performance.md](performance.md), and the emulator/reference comparison is in
[conformance.md](conformance.md). The requirement-by-requirement audit is in
[completion.md](completion.md), and the exact external run record is in
[hardware-signoff.md](hardware-signoff.md).

Today, Mtile builds and runs version-1 synth SFX, pattern music, and native
PCM samples through the provisional GBA stream. Production hardware sign-off
remains open.

This plan treats PICO-8 audio as an important compatibility frontend, not as
the limit of Mtile's audio engine. The underlying system should also support
native GBA capabilities such as PCM samples, stereo output, panning, additional
voices, and richer sequencing.

## Goals

1. Produce reliable audio on real GBA hardware and cycle-accurate emulators.
2. Preserve the approachable `sfx()` and `music()` workflow for PICO-style
   games and ports.
3. Avoid a permanent four-voice, synth-only, or mono limitation.
4. Provide target-accurate preview in the Mtile editor.
5. Keep the target runtime fixed-memory and allocation-free after startup.
6. Make CPU, EWRAM, and ROM costs visible before a game ships.
7. Continue playing without underruns during realistic worst-case graphics
   frames.
8. Build features incrementally, with measured go/no-go gates at each stage.

## Non-goals for the first release

The first audio release will not attempt to provide:

- A full DAW.
- A general plugin system.
- MP3, Ogg Vorbis, or other compressed streaming decoders on the GBA.
- Arbitrary user-defined DSP graphs.
- Closures or callback-based audio programming in Lua.
- A complete native tracker before SFX and basic music workflows are proven.
- Bit-identical PICO-8 PCM output. The different output rates and oscillator
  phases make semantic pitch, timbre, gain, envelope, and timing conformance
  the release gate instead.

Reference-recording conformance is part of the release gate. It compares
isolated internal PICO-8 exports with internal mGBA cartridge captures and does
not depend on microphones or system audio.

---

# Product model

Mtile should expose simple editor workflows while keeping a more general
engine underneath them:

```text
Game-facing APIs and editor modes
├── PICO-style SFX: sfx()
├── PICO-style music: music()
├── Native sample playback
└── Future native tracker/sequencer
                    ↓
Sequencers and source generators
├── Synth note sequencer
├── Music pattern sequencer
├── PCM sample player
└── Future richer instrument sequencer
                    ↓
General fixed voice pool
├── Oscillator voices
├── Noise voices
├── Wavetable voices
└── PCM sample voices
                    ↓
Fixed-point mixer
├── Mono initially, stereo-ready internally
├── Volume, priority, and future panning
└── Clamping and output conversion
                    ↓
GBA Direct Sound backend
├── FIFO A
├── FIFO B when stereo is enabled
├── Timer 0 or Timer 1 sample clock
└── DMA 1 and DMA 2
```

The PICO layer must be a client of the voice/mixer system. It must not define
the maximum capability of the mixer.

---

# GBA hardware resource plan

Audio shares scarce GBA hardware with the existing renderer and profiler. The
resource assignment must be explicit before implementation.

## Candidate production assignment

| Resource | Proposed owner | Notes |
| --- | --- | --- |
| DMA 1 | Direct Sound FIFO A | Conventional GBA audio DMA channel |
| DMA 2 | Direct Sound FIFO B | Stereo/native extension; unused in mono mode |
| DMA 3 | Framebuffer and fill operations | Existing graphics path remains unchanged |
| Timer 0 | Audio sample clock | Candidate default |
| Timer 1 | Available or audio-related | Exact role decided by hardware spike |
| Timer 2 + Timer 3 | 30 Hz frame pacing | Existing 32-bit pacing counter |
| PSG channels | Reserved initially | May be exposed later as optional native features |

## Known timer conflict

The current runtime uses:

- Timer 0 + Timer 1 as a 32-bit operation cycle counter.
- Timer 2 + Timer 3 as a 32-bit frame/pacing counter.

Direct Sound requires Timer 0 or Timer 1 as its sample clock. Production audio
therefore conflicts with the current operation profiler.

The first hardware spike must choose and document one of these policies:

1. **Recommended initial policy:** reserve Timer 0 for audio in audio-enabled
   ROMs, retain Timer 2+3 pacing, and disable fine-grained phase/GC cycle
   measurement while audio is active. Keep dedicated benchmark ROMs audio-free
   when exact operation timing is needed.
2. Investigate whether Timer 1 can provide the audio clock while Timer 0 is
   used independently for a reduced-width profiler. This is only acceptable
   if wrap handling and measurement range remain useful.
3. Redesign pacing or profiling around a different timing source. Do this only
   if measurements prove the simpler policy inadequate.

Audio correctness has priority over optional profiling. `stat(1)` and
`stat(2)` still need a documented behavior in audio-enabled builds; if exact
cycle accounting cannot coexist with the sample clock, the API must report a
useful approximation or explicitly reduced telemetry rather than silently
lying.

## Hardware initialization requirements

The backend must own and test:

- `SOUNDCNT_L`, `SOUNDCNT_H`, and `SOUNDCNT_X` setup.
- FIFO A/B reset and routing.
- Timer reload and control values.
- DMA source, destination, repeat, width, and special-start configuration.
- Master enable and volume behavior.
- Clean stop/reset without stale FIFO data.
- Emulator and hardware behavior after soft reset or failed initialization.

All register access belongs under `runtime/hw/`, with host-side test doubles
matching the existing MMIO architecture.

---

# Core runtime architecture

## Fixed voice pool

Use a compile-time bounded pool rather than heap allocation:

```go
type Voice struct {
    Active   bool
    Kind     SourceKind
    Priority uint8

    Phase uint32
    Step  uint32

    Volume uint16
    Pan    int16 // retained even if the first backend is mono

    // Source-specific state: oscillator, noise, wavetable, or PCM.
}
```

The exact layout must be measured for EWRAM size and hot-loop access. Avoid Go
interfaces, closures, maps, and dynamic allocation in target code. Prefer a
tagged union represented by fixed fields or parallel fixed arrays.

## Voice categories

The initial general mixer should be capable of representing:

- Synth oscillator
- Noise generator
- Small wavetable
- PCM sample

PICO SFX channels configure synth/noise voices. Native sample playback uses
PCM voices. Future instruments should reuse these sources rather than create a
second mixer.

## Voice count

Do not promise a voice count before measuring it. Benchmark candidate pools:

- 4 voices
- 6 voices
- 8 voices
- 12 voices, if practical

The first supported configuration should guarantee four PICO voices and leave
room for at least a small number of native effects if measurements permit it.
A compile-time or project setting may select among tested configurations, but
arbitrary values should not be accepted.

## Voice allocation and stealing

Every voice request needs deterministic policy:

1. Use an inactive voice.
2. Otherwise replace the lowest-priority eligible voice.
3. Break ties by oldest start time or documented channel order.
4. Never steal a protected voice unless the API explicitly permits it.

Candidate priority classes:

```text
UI / critical player SFX     high
ordinary gameplay SFX        medium-high
music                        medium
ambience                     low
```

PICO compatibility mode may reserve four logical channel identities while the
underlying physical voice slots remain shared.

Expose telemetry for:

- Active voices
- Peak active voices
- Voice steals
- Failed play requests

## Mixer arithmetic

Target implementation rules:

- Integer/fixed-point only.
- No per-sample divide.
- No target heap allocation.
- Phase-accumulator oscillators.
- Lookup tables where they win measured cycles.
- Wider accumulator than final signed 8-bit output.
- Explicit saturation/clamping.
- Deterministic output across host and GBA builds.

The mixer core should be target-independent Go wherever possible. Hardware
FIFO, timer, and DMA code remains in the GBA backend.

## Mono and stereo

The first hardware milestone may output mono through FIFO A, but the internal
voice model should carry panning or left/right gain from the start.

Long-term stereo path:

```text
all voices → left mix  → FIFO A
           → right mix → FIFO B
```

PICO-compatible voices default to center. Native sample voices may pan.

Do not expose game-facing pan controls until stereo timing, cost, and editor
preview are proven.

---

# Buffering and scheduling

Audio consumption is continuous; the 30 Hz game loop is not. The design must
survive a slow `_update`/`_draw` tick without an audible gap.

## Hardware spike questions

Before committing to a buffer architecture, build small ROMs to answer:

1. How FIFO special DMA behaves at block boundaries with the selected TinyGo
   and linker setup.
2. Whether source restart can be safely scheduled without a general interrupt
   dispatcher.
3. Whether a DMA-completion interrupt, timer interrupt, or a larger ring buffer
   gives the most reliable refill model.
4. How much jitter is introduced by refilling from the 30 Hz loop.
5. How mGBA behavior compares with real GBA hardware.

## Candidate architectures

Evaluate:

### A. Double buffer with DMA completion

- Mix one half while DMA consumes the other.
- Swap/refill on DMA interrupt or a reliable completion signal.
- Low latency and bounded memory.
- Requires safe interrupt integration.

### B. Larger ring buffer

- Producer writes ahead; DMA consumes continuously.
- Better tolerance for game-frame spikes.
- More EWRAM and more complex position tracking.

### C. Per-tick block scheduling

- Generate enough samples each 30 Hz tick for the next interval.
- Simplest integration.
- Only acceptable if worst-case game ticks cannot starve the FIFO.

The choice must be made from underrun measurements, not convenience alone.

## Interrupt policy

If interrupts are introduced:

- Add one small dispatcher rather than ad hoc handlers.
- Keep handlers bounded and allocation-free.
- Do not mix an entire large buffer in an interrupt handler unless measured
  safe.
- Define which state is shared between interrupt and main contexts.
- Use volatile/atomic-safe access where required.
- Never allow an interrupt to observe a half-published asset or voice state.

Prefer producing audio in normal code and using interrupts only to acknowledge,
swap, or request a buffer.

## Underrun behavior

An underrun must:

- Output silence or the safest documented fallback.
- Increment a counter.
- Avoid repeating an arbitrary stale loud sample.
- Remain recoverable without rebooting audio.

Underrun telemetry is a release-blocking metric, not optional debug trivia.

---

# Sample-rate plan

Benchmark a small set of hardware-valid rates rather than expose arbitrary
numbers. Initial candidates should include approximately:

- 16 kHz class
- 18.157 kHz class
- 22.05 kHz class
- 32.768 kHz class

For every candidate, record:

- Exact timer reload and actual hardware rate
- Mono and stereo CPU cost
- Buffer EWRAM cost
- Four synth voices
- Maximum tested mixed voices
- PCM playback quality
- Worst-case underrun behavior

The default should be the best quality that leaves robust headroom during the
existing complete PICO-class and GBA-class graphics workloads. A lower tested
rate may be selectable for games prioritizing graphics or voice count.

---

# PICO-style compatibility layer

## SFX data

A PICO-style SFX asset should support, at minimum:

- Up to 32 note steps
- Pitch
- Waveform
- Volume
- Effect
- Step speed
- Loop start/end

Implemented PICO-8 waveform set:

- Triangle
- Tilted saw
- Saw
- Square
- Pulse
- Organ
- Noise
- Phaser

Initial effect target:

- None
- Slide — implemented at PICO-8's exact 183/22,050-second SFX tick cadence
- Vibrato — implemented as a measured 16-tick quarter-tone cycle
- Drop
- Fade in
- Fade out
- Arpeggio modes — fast and slow implemented with measured four-note grouping

Implemented per-SFX filters:

- Noiz
- Buzz
- Detune 0..2
- Reverb 0..2
- Dampen 0..2

Implement effects incrementally and mark unsupported effects at build/editor
validation time. Do not silently substitute unrelated behavior.

## `sfx()` behavior

Define and test:

- Start by SFX ID
- Stop semantics
- Explicit logical PICO channel
- Automatic channel selection
- Note offset and length forms, if supported
- Restarting an already playing SFX
- Priority interaction with native voices
- Invalid IDs and exhausted voices

The final signatures should follow PICO-8 where practical while documenting
intentional differences.

## Music data

Initial music model:

- Four PICO logical channels
- Each pattern assigns an SFX per channel
- Pattern transition flags
- Loop points
- Stop behavior
- Song arrangement

## `music()` behavior

Define and test:

- Start pattern/song
- Stop and optional fade
- Pattern progression
- Loop behavior
- Interaction with manually triggered `sfx()` calls
- Reserved versus stealable music voices

PICO compatibility must not prevent the native mixer from supporting more
voices or PCM sources.

---

# Native GBA audio features

## PCM samples

The first native extension should be sample playback, not a full tracker.
Support target-ready signed 8-bit PCM with metadata for:

- Sample ID and stable name
- Source WAV path
- Target sample rate
- Gain
- Loop enabled
- Loop start/end
- Root pitch, if pitch-shifted playback is supported
- Default priority
- Default pan after stereo exists

## Sample conversion

Host build/editor tooling should support an intentionally narrow initial WAV
subset, then expand:

1. Uncompressed PCM WAV input.
2. Mono or stereo source.
3. Common 8/16/24-bit input depths as host tooling permits.
4. Conversion to target signed 8-bit PCM.
5. Target-rate resampling.
6. Optional normalization.
7. Optional leading/trailing silence trim.
8. Loop-point validation.

Keep source WAV files in the project. Generated target bytes are build output,
not the only editable copy.

## Native game API

Do not finalize names before the mixer and asset semantics are proven.
Candidate concepts include:

- Play sample by ID
- Stop voice/channel
- Set volume
- Set pan
- Set priority
- Query playback state

Any new API must have parser, transpiler, runtime, editor, and documentation
coverage. Avoid APIs exposing FIFO, DMA, or hardware channel numbers directly.

## PSG

Do not use the four hardware PSG channels as the main PICO backend. Their
fixed roles cannot faithfully represent four interchangeable PICO voices.

Keep PSG reserved initially. Later possibilities:

- Optional native Game Boy-style API
- Cheap extra tones/noise
- Hybrid engine to reduce PCM mixer load

Expose PSG only after Direct Sound is stable and there is a clear user-facing
model. It must not complicate the basic SFX editor.

---

# Editor plan

Keep audio authoring in the existing `mtile-editor` application, not a
separate executable. Present it as one creative workspace rather than exposing
the storage/runtime architecture as top-level navigation.

Top-level workspaces:

```text
Sprites | Map | Audio Studio
                       Song | Sounds | More…
```

The editor now has three primary tabs with `1` through `3` and Tab navigation.
Audio Studio opens on Song and keeps Song and Sounds visible as its everyday
tools. WAV import and diagnostics remain available under More… because they are
occasional production tasks. The underlying asset modes and file formats stay
separate, but that implementation detail is no longer the first thing a new
composer has to understand.

Song is implemented as an integrated **Music Lab**, not a graph/table that
hands composition off to a second editor. Its visible order is the beginner's
actual job: choose an instrument, place a clip in the arrangement, write notes,
play the song. Sounds remains the advanced library for standalone effects and
low-level clip details.

## Shared editor requirements

Every audio mode needs:

- Undo/redo integrated with the existing editor history model
- Explicit save behavior
- Play/stop controls
- Keyboard shortcuts
- Current playback position
- Validation errors in the UI
- ROM/EWRAM estimates
- Estimated and measured CPU information where available
- Stable numeric asset IDs plus human-readable names

## Clips and sounds editor

### Main panels

```text
Clip library | piano roll | simple note inspector | clip preview
```

### Beginner view

- Clip name
- Tempo/note time
- Note and octave
- Instrument
- Loudness

### Advanced fields

- Stable game ID
- Clip loop start/end
- Per-note effect
- Per-SFX PICO filters: noiz, buzz, detune, reverb, dampen
- Advanced native fields later: pan, priority, gate

### Core interactions

- Click/drag note placement
- Vertical pitch editing
- Step selection
- Delete/clear
- Multi-step transpose
- Multi-step volume changes
- Play from start
- Preview selected step
- Stop
- Visible current-step cursor

### First release scope

The first editor can omit advanced waveform drawing and complex multi-select,
but exact numeric editing and target-accurate playback are required.

### Implemented Milestone 2 slice

The current Sounds mode provides a named clip library, a 32-position/64-pitch
piano roll, a beginner inspector with progressive disclosure, loop bracket,
target-PCM oscilloscope, playback cursor, add/delete controls, direct numeric
entry, undo/redo, and
validation-backed save/reload. Desktop preview compiles the in-memory asset and
runs the same `runtime/audio` mixer and sequencer used by generated ROMs at
exactly 16,384 Hz. The host adapter changes only the final sample container for
desktop output. A full looping preview is capped at four seconds so transport
playback remains finite.

## Song editor

The native editor translates the version-1 stable-ID graph into familiar
musical objects. Internal patterns are visible horizontal sections; SFX
assigned to music are colored clips; the four logical channels are Melody,
Harmony, Bass, and Drums. An empty song selects Triangle Lead and presents one
prominent Start Song action. The created clip stays inside Music Lab with its
piano roll and keyboard immediately ready. The runtime representation stays
unchanged; optional song-title metadata is editor-facing.

### Integrated workstation

- Instrument library grouped into lead, bass, and beat categories
- Four named, colored musical tracks
- Sections arranged left to right
- Colored clip block per assigned part
- Mute/solo for preview
- Next section, loop song, or end song
- Piano roll and two-octave clickable keyboard for the selected clip
- Inline octave, position-count, loudness, and clip audition controls
- Song title, song-wide tempo control, and play/stop transport
- One explicit Sound Details escape hatch for advanced synthesis fields

### Arrangement view

- Ordered horizontal section sequence with visible scroll buttons
- Insert/delete/duplicate
- Loop markers
- Current section and clip position while playing

### Interaction rule

Clicking an assigned clip edits it in place. Clicking an empty block creates a
clip from the selected instrument in place. The default composition path never
changes screen. The separate Sounds screen is progressive disclosure for sound
design, loops, effects, filters, and stable IDs—not a required step for writing
music.

## Sample editor/library

### Initial panels

```text
Sample list | waveform | metadata/conversion | source/target preview
```

### Required operations

- Import WAV
- Delete and rename
- Trim silence
- Normalize
- Select target sample rate
- Set loop start/end
- Preview source
- Preview exact converted target
- Show source size and cartridge size

Target preview must play the exact target-rate signed 8-bit data through the
same mixer math used by the ROM, then adapt only the final stream to the host
audio device.

## Audio settings/profiler mode

Show:

- Output mode: mono/stereo
- Tested sample-rate choices
- Voice-pool configuration
- Master volume
- Audio buffer EWRAM
- Audio asset ROM usage
- Estimated mixer load
- Last/maximum measured mix cycles
- Underrun count
- Peak voices
- Voice steals

Settings outside tested safe combinations should be rejected rather than
accepted with a warning.

### Implemented Milestone 5 slice

The Audio Studio diagnostics screen derives the linked production profile from
the same in-memory assets used by the build: no audio, an eight-slot/four-channel mono profile,
or a twelve-slot mono profile when native samples exist. It shows the fixed
16,384 Hz clock and editable persisted
master volume, exact packed audio ROM bytes, retained stream/player/mixer target-map
sizes, the profile-appropriate ARM7TDMI mixer baseline, the measured producer
maximum, the matching empty-scene coexistence cost, and the zero-unforced-
underrun benchmark result. A separate dual-FIFO signal rail shows the measured
two-voice stereo candidate as not selectable.

Sample-bearing projects also show the optimized full-profile empty-scene cost
as an over-budget signal and name the measured two-synth-plus-PCM fade boundary
for common 30 Hz scenes.

Benchmark values and live cartridge telemetry are deliberately separated.
Last-run mix cycles, underruns, peak voices, and steals remain blank because
the editor has no target telemetry import path; it never presents benchmark
zeroes as measurements of the current game. The strict version-1
`settings.json` persists mono output, the measured rate, automatic profile
selection, and a `0..256` master gain. Only master gain is currently editable;
unsupported output/rate/profile values fail before TinyGo instead of appearing
to work.

## Native tracker direction

Do not build a full native tracker in the initial milestones. Revisit only
after users can ship games with:

- Synth SFX
- Four-channel music
- PCM samples
- Stereo/native voices

A tracker is justified if real projects need sample instruments, longer
patterns, more simultaneous musical voices, and richer per-note control.

### Polyend Tracker Mini as a design reference

The Polyend Tracker Mini is a strong information-architecture reference for a
future advanced Mtile tracker. Borrow its principles, not its exact visual
design, terminology, iconography, or hardware-button constraints:

- Vertical row-based patterns
- A small number of focused views
- Clear playhead and selection state
- Context-sensitive parameter editing
- Pattern/song sequencing instead of a DAW-style freeform timeline
- Integrated instrument and sample workflows
- Compact presentation that reveals advanced controls gradually

Mtile is a desktop editor, so it should add desktop-native interaction where
that improves speed and clarity:

- Mouse selection and dragging
- Computer-keyboard note entry
- Rectangular copy/paste
- Undo/redo
- Multi-row edits and transpose operations
- Direct numeric entry
- Tooltips and visible shortcut hints
- A larger contextual inspector
- Direct navigation from a pattern event to its instrument, SFX, or sample

The intended direction is therefore **Tracker Mini-inspired workflow with
desktop-native interaction**, not a clone.

### Proposed tracker layout

Keep the pattern grid visually dominant, with arrangement on the left and an
exact event inspector on the right:

```text
┌────────────────────────────────────────────────────────────────────┐
│ TRACKER   Song 03   Pattern 06   120 BPM   Row 14/64   ▶ Playing │
├──────────────┬─────────────────────────────────────┬───────────────┤
│ ARRANGEMENT  │ TRACK 1   TRACK 2   TRACK 3   TRACK 4│ EVENT         │
│              │ N  I V FX  N  I V FX  N  I V FX ... │               │
│ 00 Intro     │ C4 01 7 --  C3 04 5 --  -- -- - --  │ Note: C4      │
│ 01 Verse A   │ -- -- - --  -- -- - --  -- -- - --  │ Inst: 01      │
│>02 Chorus    │ E4 01 7 V2  G3 04 5 --  C4 07 4 --  │ Volume: 7     │
│ 03 Bridge    │ -- -- - --  -- -- - --  -- -- - --  │ FX: Vibrato 2 │
│              │>G4 01 6 --  D3 04 5 --  -- -- - --  │               │
│ [+] [Copy]   │ -- -- - --  -- -- - --  -- -- - --  │ [Instrument]  │
│ [Delete]     │ A4 01 6 --  F3 04 5 --  D4 07 4 --  │ [Sample]      │
├──────────────┴─────────────────────────────────────┴───────────────┤
│ Space Play/Stop  Enter Note  Del Clear  Cmd/Ctrl+C/V  Tab Column  │
└────────────────────────────────────────────────────────────────────┘
```

The compact default event representation should be approximately:

```text
NOTE  INSTRUMENT  VOLUME  EFFECT
C#4   03          6       V2
```

Do not show every possible parameter column for every track at all times. That
becomes unreadable as voice count grows. Selecting an event should expose its
complete fields in the inspector; an optional expanded-track mode can reveal
more effect columns when needed.

### Focused tracker views

A future tracker should use focused modes rather than one overloaded screen:

1. **Pattern** — primary note/event grid.
2. **Song** — arrangement, loop points, and transitions.
3. **Instrument** — waveform, envelope, tuning, and synth parameters.
4. **Sample** — waveform, trim, loop points, gain, and root note.
5. **Mixer** — volume, pan, mute, solo, and priority.
6. **Perform** — optional pattern launching/muting, considered much later.

The first tracker release would not need every view. Pattern, Song,
Instrument, Sample, and a minimal Mixer are the likely useful core.

### Simple mode versus advanced mode

Keep two levels of authoring:

```text
Simple:    SFX | Music
Advanced:  Tracker
```

The simple SFX/Music editors serve PICO-style ports, beginners, game jams, and
small games. The tracker serves projects needing more tracks, samples,
per-event volume/pan, richer arrangements, and native effects.

Do not turn the simple editor into a tracker with most columns hidden. It
should remain a genuinely simpler source format and workflow.

Both authoring paths should compile into the same lower-level sequencer and
voice/mixer commands:

```text
PICO SFX/music assets ─┐
                       ├→ compiled event/voice commands → general mixer
Native tracker assets ─┘
```

The authored schemas may remain distinct. Forcing simple and advanced editors
to mutate one complicated source schema would leak tracker complexity into the
beginner workflow.

Provide a future one-way **Convert to Tracker Song** operation:

```text
PICO-style song → native tracker song
```

It need not convert back because tracker edits may use voices, effects,
samples, panning, or timing that the simple format cannot represent.

### Tracker implementation prerequisites

Do not finalize tracker columns or effect codes until benchmarks establish:

- Supported voice profiles
- Sample rates
- Timing resolution
- Instrument/source types
- Pattern length limits
- Effect command set
- Stereo and panning support
- Voice priority/stealing behavior
- PCM sample capabilities

Building the tracker first would risk designing UI for features the GBA cannot
sustain, or freezing an event model before the audio engine is understood.
The implementation order remains hardware → mixer → SFX → simple music → PCM
samples → stereo/native extensions → tracker.

---

# Target-accurate preview

The editor must not implement a separate approximate synthesizer.

Recommended package split:

```text
shared audio core
├── asset types and validation
├── oscillator/noise generation
├── sequencers and effects
├── mixer math
└── target PCM conversion

GBA backend
├── MMIO
├── FIFO
├── timers
├── DMA
└── interrupt/refill integration

editor backend
├── desktop audio output
├── UI transport controls
└── final host-rate adaptation
```

The shared core must compile under normal Go and TinyGo. Keep editor-specific
objects, file decoding, and host audio dependencies out of the runtime module.

Host playback procedure:

1. Generate samples at the exact configured GBA target rate.
2. Use the same voice, effect, and mixer arithmetic as the target.
3. Resample only the final output to the desktop device rate if necessary.

Golden tests should compare generated sample bytes, not subjective playback.

---

# Asset and project format

Proposed source layout:

```text
assets/
└── audio/
    ├── settings.json
    ├── sfx.json
    ├── music.json
    └── samples/
        ├── manifest.json
        ├── explosion.wav
        └── voice-hey.wav
```

## Design principles

- Human-readable JSON for authored metadata.
- Stable numeric IDs for compact runtime calls.
- Human-readable names in the editor.
- Renaming must not renumber assets.
- Reordering the editor list must not renumber assets.
- WAV files remain source assets.
- Generated compact arrays belong in the temporary build workspace/ROM.
- Validation must happen before TinyGo invocation where possible.
- Asset limits and memory costs must be reported clearly.

## Version-1 `settings.json`

```json
{
  "version": 1,
  "output": "mono",
  "sampleRate": 16384,
  "voiceProfile": "auto",
  "masterVolume": 256
}
```

Only named, benchmarked profiles should be accepted initially.

## Versioning

Audio schemas need an explicit version before public release. Migrations must
be atomic and preserve stable IDs. The editor should refuse unknown future
versions rather than rewrite them destructively.

---

# Build pipeline integration

The root build pipeline should:

1. Load and validate audio settings/assets.
2. Convert WAV source assets on the host.
3. Deduplicate target sample data where worthwhile.
4. Pack SFX, music, and sample metadata into ROM arrays.
5. Emit fixed-size EWRAM state and buffers.
6. Link the ELF and validate EWRAM reserve as it does today.
7. Report audio ROM/EWRAM costs in build output.
8. Reject unsupported combinations before producing a ROM.

Example future output:

```text
built game.gba (412,832 bytes, 22,144 bytes EWRAM free)
audio: 18,157 Hz mono, 8 voices, 4,096-byte buffers, 84,120-byte assets
```

Do not generate huge per-sample Go literals if TinyGo compile time becomes a
problem. The map experiments already showed that very large literals can hit
TinyGo's interpreter timeout. Investigate binary embedding or generated
assembly/object data before sample libraries become large.

---

# Lua/transpiler integration

## Current transition

`sfx()` now lowers to the runtime when generated SFX assets are present; calls
without an attached player fail closed after evaluating their arguments.
`music()` lowers to the shared runtime and uses the same ordered argument
evaluation and bounded command queue as `sfx()`.

Maintain argument evaluation order and side effects throughout the transition.
Tests already guard no-op argument evaluation; equivalent tests must guard the
real calls.

## Required coverage for every audio API

- Parser argument validation
- Transpiler lowering
- Runtime behavior
- Invalid-ID behavior
- Asset validation
- Target allocation checks
- Generated-host harness tests
- TinyGo ROM build
- Documentation

## Runtime errors

Avoid panics. Provide bounded, observable status for:

- Invalid SFX/music/sample ID
- Voice exhaustion
- Underrun
- Unsupported asset mode
- Failed initialization

Decide which errors are build-time failures and which are runtime telemetry.
Static invalid IDs should be caught at build time when they are literal and
known; dynamic values must fail safely at runtime. **Implemented for
`sfx()`, `music()`, and `sample()`: diagnostics include the Lua source position
and exact asset file, while `-1` stop calls and audio-less compatibility calls
remain valid.**

---

# Test strategy

## Unit tests

### Shared core

- Oscillator sample sequences
- Noise determinism
- Phase stepping
- Saturation/clamping
- Volume and panning
- Voice start/stop
- Voice priority and stealing
- Loop boundaries
- Every note effect
- Music pattern transitions
- Sample looping

### Runtime hardware abstraction

- Sound register initialization
- Timer reload/control
- DMA configuration
- FIFO reset and routing
- Mono/stereo routing
- Backend stop/reset
- Host MMIO simulation

### Assets

- Valid/invalid SFX records
- Stable IDs
- Pattern references
- WAV conversion
- Loop points
- Unsupported formats
- Size overflow
- Schema version handling

### Editor models

- Undo/redo
- Save/reload
- SFX edits
- Music arrangement edits
- Sample metadata
- Conversion settings
- Stable IDs after rename/reorder

## Golden tests

Store compact golden PCM outputs for:

- Each waveform
- Each effect
- One complete SFX
- One four-channel pattern
- One mixed synth+sample scene
- Mono and stereo configurations

Host and TinyGo-compatible core code must produce the same target bytes.

## Property/fuzz tests

Use host fuzzing for:

- WAV parser boundaries and malformed/truncated chunks. **Implemented.**
- Asset JSON validation and strict round trips. **Implemented.**
- Mixer saturation and byte/word mono/stereo agreement. **Implemented.**
- SFX and converted-sample loop-point validation. **Implemented.**
- Sequencer state transitions. **Implemented.**

All targets completed local five-second campaigns; CI runs a one-second smoke
campaign per target and retains discovered corpus entries as normal regression
inputs.

## Integration tests

- Lua `sfx()` → generated Go → runtime state
- Lua `music()` → pattern progression
- Editor save → build → ROM
- Sample import → conversion → ROM
- TinyGo zero-allocation build
- Audio ROM coexisting with `mapdemo`

## Hardware/emulator smoke tests

Create dedicated ROMs:

```text
runtime/cmd/audiospike       register/DMA/FIFO proof
runtime/cmd/audiobench       cycle and underrun matrix
runtime/cmd/audiocoexist     graphics coexistence and recovery matrix
runtime/cmd/audiohardware    two self-scoring ten-minute physical-GBA profiles
runtime/cmd/audiostereocapture  deterministic stereo/reference sequence
examples/audio-sfx           simple SFX project
examples/audio-music         pattern/song project
examples/audio-samples       mixed two-synth-plus-native-PCM project
```

Use mGBA for automated memory/telemetry extraction where possible, then verify
milestones on real hardware. Emulator success alone is insufficient for final
DMA/timer sign-off.

---

# Benchmark plan

All target audio decisions require ARM7TDMI measurements.

## Mixer matrix

Measure each supported sample rate with:

- Silence
- 1 synth voice
- 4 synth voices
- Maximum candidate synth voices
- 1 PCM voice
- 4 PCM voices
- Mixed synth + PCM
- Mono
- Stereo
- Effects disabled/enabled
- Best-case and worst-case waveform paths

Record:

- Cycles per generated sample
- Cycles per buffer
- Percentage of a 30 Hz tick
- EWRAM buffer/state cost
- ROM code/table cost
- Underruns

## Coexistence matrix

Audio must run while these graphics workloads execute:

1. Empty update/draw
2. `cls` + HUD
3. 100 sprites
4. Sparse 30x20 mapdemo-class frame
5. Dense 16x16 PICO-class room
6. Complete PICO-class tick from `benchspike`
7. Complete GBA-class tick from `benchspike`
8. Intentional over-budget frame to verify graceful underrun telemetry/recovery

## Performance gates

Exact percentages will be set after the first spike, but release criteria are:

- Zero target Go heap allocations.
- Zero underruns during every documented supported graphics workload.
- Bounded mix-buffer generation time with safety headroom.
- No corruption or visible graphics regression from DMA/timer sharing.
- No unreported voice stealing.
- Stable playback cadence during 30 Hz frame pacing.

Do not advertise a voice count/sample rate combination that only works in an
empty game loop.

---

# Documentation plan

Before public audio release, add:

```text
docs/audio/
├── plan.md                 this roadmap
├── architecture.md         final implemented design/resource ownership
├── sfx.md                  SFX API and editor guide
├── music.md                music API and editor guide
├── samples.md              import/conversion/native sample API
├── performance.md          measured configurations and budgets
└── troubleshooting.md      silence, clipping, underruns, emulator controls
```

Update:

- Root README feature/API lists
- Project layout
- Performance guide
- Editor controls
- Example READMEs
- Contribution requirements for audio hot paths

The planned documentation set now exists, including the symptom-driven
[`troubleshooting.md`](troubleshooting.md) guide and its runtime
`stat(80..89)` diagnostic workflow. Hardware sign-off caveats remain explicit
rather than presenting the provisional stream as released.
CI now links all three shipped audio examples, runs the audio fuzz smoke targets and mono mixer/PCM/rewind
and graphics-coexistence ROMs through TinyGo's target allocation report, and
checks a generated all-audio package under the production `-gc=none -opt=2`
policy. Contributor instructions name the same audio hot-path gates.

---

# Milestones and acceptance gates

## Milestone 0 — Architecture decision records and hardware probes

### Work

- Document timer/DMA ownership. **Implemented.**
- Build FIFO A tone and silence ROMs. **Implemented.**
- Verify exact timer rates. **Implemented in mGBA; physical check pending.**
- Test DMA repeat/block behavior. **Implemented in mGBA; physical check pending.**
- Test stop/reset/restart. **Implemented; physical listening pending.**
- Compare mGBA and real hardware. **Real hardware pending.**
- Decide buffer/refill architecture. **Four-page interrupt-assisted stream selected.**
- Decide initial mono sample rate. **16,384 Hz selected.**
- Build self-scoring ten-minute production-profile ROMs. **Implemented as the
  sample and `standard` variants of `runtime/cmd/audiohardware`.**

### Gate

A repeatable, allocation-free ROM plays uninterrupted PCM for at least ten
minutes, reports zero underruns, and does not interfere with Mode 4 DMA3
presentation. The short mGBA extractions and full 18,000-tick soaks are
complete. The sample-profile run reached 38,646 handoffs and the four-synth run
reached 48,634. Both completed 120 command rewinds and all 60 verified fade
cycles with three queued pages, zero pending commands, a 75-sample maximum
lateness, and no underrun, stream-switch failure, fallback, or drop. The sample
build retained one PCM voice; the standard build retained zero. Both visible
PASS states and listening checks still must be observed on a physical GBA.

## Milestone 1 — Shared mono mixer

### Work

- Fixed voice pool.
- Oscillator, noise, wavetable, and PCM source kinds.
- Fixed-point phase and volume.
- Priority and stealing.
- Mono target output.
- Host golden tests.
- Runtime telemetry.

The production stream, mixer, and sequencer telemetry is available as one Go
`Context.AudioStats()` snapshot. Generated games attach the stream
automatically, and Lua `stat(80..89)` exposes bounded underrun, queue, voice,
steal, failure, rewind, fallback, drop, and maximum-lateness counters without
enabling the cycle profiler.

### Gate

Mixer passes golden tests, benchmark matrix, zero-allocation gate, and
worst-case graphics coexistence at one documented sample-rate/voice profile.

## Milestone 2 — SFX assets, runtime API, and editor

The functional authoring path is implemented. This milestone remains
experimental until the provisional stream clears audible and real-hardware
gates.

### Work

- Versioned SFX schema.
- Initial waveforms and note sequencing.
- `sfx()` implementation.
- SFX editor mode.
- Target-accurate preview.
- `examples/audio-sfx`.

### Gate

A user can author, preview, save, build, and play a multi-note SFX without
manual generated-code edits. Invalid assets fail before TinyGo. Runtime remains
allocation-free and underrun-free under documented workloads.

## Milestone 3 — PICO effects and music

All eight instruments, slide, vibrato, drop, fade-in, fade-out, fast/slow
arpeggios, all five filters, and the exact exported speed clock are implemented.
The complete 870-case instrument/pitch, note-effect, filter, and 1..255 speed
recording gate passes. The supplied corpus audit also exports all 229 carts and
all 289 distinct graph roots. Its 153 paired references include an explicitly
reconstructed interactive Slipways arrangement; 150 clear 0.90 on both
effective metrics directly, while the remaining three have stronger direct or
global evidence proving one source mismatch and two multipart-locator
artifacts. The music
asset/runtime path is implemented;
its editor, exact host transport, mute/solo preview, and sample-continuous
call-site fade behavior are implemented. The music-fade benchmark and
music-active graphics coexistence telemetry are now extracted. Continuity
passes in mGBA. The optimized all-music gain path and typed profile-bounded
checkpoints bring three of seven unforced graphics workloads under one 30 Hz
work budget; four workloads remain over budget.

### Work

- Effects in measured increments. **Implemented; 11/11 external cases pass.**
- Exhaustive instrument/pitch, effect, filter, and speed recording harnesses.
  **Implemented; all 870/870 external cases pass.**
- Full supplied-cart importer/exporter/comparator audit. **Implemented; all
  229 carts and 289 distinct entries export, with 153 references compared.**
- Versioned music/pattern schema. **Implemented (v1 pattern graph).**
- Four logical PICO channels. **Implemented.**
- `music()` implementation. **Implemented, including fade-in/fade-out.**
- Integrated Music Lab with instruments, horizontal four-track arrangement,
  inline piano roll/keyboard, title, tempo, and playback. **Implemented over
  the native stable-ID graph.**
- Mute/solo host preview. **Implemented as unsaved transport state.**
- `examples/audio-music`. **Implemented.**

### Gate

Complete songs transition and loop correctly; SFX/music channel interaction is
documented and tested; editor and target output agree at the PCM-byte level.

## Milestone 4 — Native PCM samples

The version-1 implementation path is complete. Its runtime/converter/editor
tests, direct mixed-voice mixer benchmark, and cartridge-level mGBA continuity
matrix pass. The optimized four-synth-plus-PCM profile fits two of seven 30 Hz
matrix scenes. Three and two synth voices plus PCM fit five common scenes but
not the 100-sprite or dense-map stresses. Two synth voices plus PCM is the
documented safety boundary; real-hardware uninterrupted-playback sign-off
remains open.

### Work

- WAV importer/converter. **Implemented (PCM 8/16/24-bit mono/stereo).**
- Sample manifest. **Implemented (strict version 1).**
- Sample library/editor mode. **Implemented (drag-and-drop WAV import).**
- Native sample playback API. **Implemented as `sample()`.**
- Looping, volume, and priority. **Implemented.**
- Source versus target preview. **Implemented with target mixer reuse.**
- `examples/audio-samples`. **Implemented.**

### Gate

Imported samples play at the documented rate without underruns, target size is
shown before build, and mixed synth+sample workloads pass the benchmark gate.

## Milestone 5 — Stereo and panning

The mixer and experimental dual-FIFO stream slices are implemented and
host/target-emulator measured. Two hard-panned, continuously fading music
voices pass the complete mGBA graphics, forced-starvation, and recovery matrix.
Four distributed fading voices consume 52.8% of a page in isolation but fail
the full graphics matrix, so that profile is rejected. Release decision:
project-level stereo and pan are deferred from version 1. The generated mono
stream and game-facing APIs remain unchanged; the low-level stereo experiment
stays covered for a post-v1 hardware/user-interface decision.

### Work

- FIFO B and DMA2 backend. **Implemented experimentally with synchronized Timer 1 handoffs.**
- Left/right mix buffers. **Implemented in the shared mixer and four-page `StereoStream`.**
- Voice panning. **Implemented in the low-level mixer and persistent SFX-channel configuration with a fixed-point linear law; Lua/editor exposure deferred from v1.**
- Internal stereo capture. **Implemented with mGBA's `mAVStream` callback and exact shared-profile correlation.**
- Stereo sample conversion/preview policy. **Implemented for version 1: builds use a deterministic integer L/R average and mono ROM, while source preview preserves separate channels for waveform display and host audition.**
- Updated settings and profiler UI. **Implemented with editable persisted master gain, locked measured output/rate/profile, exact project costs, separated benchmark/live telemetry, and a visibly locked stereo candidate.**

### Gate

Stereo passes hardware tests, remains within documented CPU/EWRAM budgets, and
mono projects do not pay unnecessary stereo cost. **Not a version-1 release
gate; the feature is disabled and dead-stripped from mono projects.**

## Milestone 6 — Native tracker go/no-go

Version-1 decision: **no-go / defer**. There is no shipped-project evidence
that the implemented SFX, pattern-music, and sample-library workflows are
insufficient, so adding a second sequencing schema now would create product
and compatibility risk without a concrete requirement. Reopen this milestone
only after serious-project feedback identifies missing event or instrument
semantics.

### Work

Collect feedback from shipped or serious projects. Determine whether SFX,
patterns, and samples are insufficient. If the decision is to proceed, turn
the Tracker Mini-inspired direction above into tested wireframes and keyboard/
mouse interaction prototypes before committing an asset schema.

Prototype at least:

- Pattern grid with compact and expanded track views
- Arrangement and event inspector panels
- Computer-keyboard note entry
- Rectangular selection and copy/paste
- Instrument/sample navigation
- Conversion of a simple PICO-style song into tracker data

### Gate

Proceed only with concrete requirements for richer instruments, longer
patterns, additional musical voices, and per-note controls. User-test the
wireframes and prove that the event model fits benchmarked engine features.
Otherwise invest in stability, import tooling, effects, and performance
instead.

---

# Risks and mitigations

## Timer/profiler conflict

**Risk:** audio sample clock consumes a timer pair currently assumed by
profiling.

**Mitigation:** make ownership explicit; prioritize audio correctness; provide
documented reduced telemetry or benchmark-only profiler builds.

## DMA contention

**Risk:** FIFO DMA and framebuffer DMA interfere or produce timing surprises.

**Mitigation:** reserve DMA1/2 for audio and DMA3 for graphics; run combined
on-target workloads; monitor presentation and underruns.

## TinyGo code generation

**Risk:** mixer constructs allocate or compile inefficiently under `-gc=none`.

**Mitigation:** fixed arrays, tagged state, no interfaces/closures, allocation
checker, linker-map review, target disassembly and cycle measurements.

## Audio preview mismatch

**Risk:** editor sounds different from ROM.

**Mitigation:** shared core and exact target-byte golden tests; resample only
after target mix generation.

## PCM asset size

**Risk:** samples consume cartridge ROM quickly and huge literals make TinyGo
builds time out.

**Mitigation:** show size in editor, use samples selectively, investigate
binary embedding before scaling libraries, and reject cartridge overflow in
preflight.

## Mixer CPU growth

**Risk:** adding native features silently reduces graphics capacity.

**Mitigation:** named tested voice profiles, visible estimates/telemetry,
combined benchmark gates, no arbitrary settings.

## Editor scope explosion

**Risk:** trying to build SFX, tracker, sampler, and DAW features together
prevents shipping anything robust.

**Mitigation:** ship in order: hardware → mixer → SFX → music → samples →
stereo; defer native tracker pending demonstrated demand.

## Compatibility becoming the architecture

**Risk:** PICO's four-channel model leaks into the core and blocks native
features.

**Mitigation:** keep PICO channels in the compatibility sequencer; use a
general voice pool and mixer beneath it.

---

# Definition of done for the whole audio initiative

The initiative is complete when:

1. `sfx()` and `music()` are real, documented, and useful.
2. PCM samples and native voices can coexist with PICO-style audio.
3. Stereo/panning are available in a measured supported profile, if hardware
   benchmarks approve them.
4. SFX, music, samples, and settings are editable in `mtile-editor`.
5. Editor preview matches target PCM generation.
6. Runtime audio is fixed-memory and allocation-free.
7. Underruns, voice steals, CPU cost, ROM cost, and EWRAM cost are observable.
8. Audio survives documented worst-case graphics workloads.
9. Hardware resource ownership is documented and tested.
10. Example projects demonstrate SFX, music, samples, and mixed playback.
11. CI covers host logic, asset conversion, generated code, TinyGo linking,
    and allocation checks.
12. Internal PICO-8 and mGBA recordings pass the complete documented
    instrument/pitch, effect, filter, and speed conformance gate.
13. At least one real GBA hardware pass confirms every production profile.

The final product should feel simple for a PICO-style game while remaining a
general GBA audio system underneath:

```lua
-- Simple compatibility workflow
sfx(3)
music(0)

-- Native features can grow alongside it, not replace it
-- (exact API names intentionally deferred until the engine is proven)
```
