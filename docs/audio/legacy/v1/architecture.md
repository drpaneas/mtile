# Legacy Mtile audio v1 architecture decisions

> Historical PICO-compatible Direct Sound design. Superseded by
> `../../architecture.md`; retained as engineering evidence.

## Status

The interrupt-assisted buffering, mono mixer, SFX, music, native PCM, and
editor paths pass their host and mGBA gates at 16,384 Hz. Synth-only,
synth-plus-PCM, and the experimental two-voice stereo profile pass continuity
and recovery matrices with zero unforced underruns. The measured scene limits
are fixed below; audible and real-hardware measurements remain the final
production sign-off gate.

## ADR-0001: hardware resource ownership

Audio-enabled ROMs use this initial ownership policy:

| Resource | Owner | Decision |
| --- | --- | --- |
| DMA 1 | Direct Sound FIFO A | Reserved for mono audio. |
| DMA 2 | Direct Sound FIFO B | Used by the experimental stereo backend; unused in mono. |
| DMA 3 | Mode 4 presentation/fills | Existing graphics behavior is unchanged. |
| Timer 0 | Direct Sound sample clock | Reserved while audio is active. |
| Timer 1 | Audio block cadence | Provisionally cascades from Timer 0 and requests one IRQ per 256-sample stream block. |
| Timer 2 + Timer 3 | 30 Hz pacing and tick telemetry | Existing pacing and `stat(1)`/`stat(2)` remain available. |

Audio correctness has priority over fine-grained phase and GC profiling. The
current phase profiler uses Timer 0 + Timer 1 as a 32-bit counter, so it must
be disabled while production audio owns Timer 0. Dedicated cycle benchmark
ROMs remain audio-free unless they explicitly measure audio with a different
counter strategy.

This decision preserves DMA 3 for the renderer and Timer 2 + Timer 3 for the
stable 30 Hz cadence already used by generated games.

## ADR-0002: Milestone 0 fixed-word probe

`runtime/hw.StartFIFOAProbe` is a deliberately narrow hardware probe, not the
production streaming backend. It configures:

- Direct Sound A at full volume, routed to both speakers.
- FIFO A reset and one manual priming word.
- Timer 0 at a named candidate sample clock.
- DMA 1 in FIFO special-start, repeat, 32-bit, fixed-destination mode.
- A fixed DMA source in IWRAM or EWRAM.

FIFO requests transfer four 32-bit words regardless of the programmed DMA
count or transfer-width bit. A fixed source therefore repeats one packed
four-sample PCM word indefinitely and isolates register/routing behavior from
buffer-wrap and refill behavior. It can prove tone, active silence,
stop/reset, and restart, but it cannot prove a production refill architecture
or underrun recovery.

DMA 1 and DMA 2 cannot read Game Pak ROM. Probe and production DMA sources
must live in internal RAM.

## ADR-0003: incrementing-source scheduling probe

The second probe tests per-tick scheduling without claiming it as the final
buffer architecture:

- DMA 1 increments through an 8,192-sample EWRAM array.
- Timer 1 cascades from Timer 0 and counts consumed samples.
- The first 4,096 samples are the logical stream period.
- The second 4,096 samples duplicate the first and act as a guard region.
- The 30 Hz loop polls Timer 1 and restarts DMA after the logical period.

At 16.384 kHz, one 30 Hz tick consumes about 546 samples. The duplicate guard
therefore tolerates several delayed ticks before DMA can read out of bounds.
This probe reveals whether stop/reset/restart creates audible discontinuities
and whether a realistic visual loop services the boundary in time. It does not
yet prove underrun detection or a continuously refilled mixer buffer.

## ADR-0004: provisional interrupt-assisted mono stream

`runtime/audio.MonoStream` is the first production-candidate refill design. It
is deliberately provisional until audible and real-hardware tests pass.

- Timer 0 remains the exact 16.384 kHz sample clock.
- Timer 1 cascades from Timer 0 and requests an IRQ every 256 samples
  (15.625 ms, 64 handoffs/second).
- Four contiguous 256-sample pages provide 62.5 ms of queue coverage.
- Normal code generates complete pages; the IRQ never calls the mixer.
- The IRQ only selects the next published page, redirects DMA 1, updates
  bounded telemetry, and clears the consumed page.
- Consumed pages are cleared so a missed refill yields signed PCM zero rather
  than stale audio.
- A 304-sample wrap guard contains the following logical audio. If rendering
  delays the IRQ, DMA can continue reading valid samples.
- The IRQ reads Timer 1 progress and redirects DMA after samples already
  consumed and prefetched. It does not flush FIFO A or replay the block prefix.
- At progress 4 or 5 within a FIFO burst, mGBA 0.10.3 can enter the Timer 1
  handler on either side of DMA's internal source-latch update. The target
  handoff waits at most two sample clocks for progress 6 before disabling DMA,
  then applies the progress-derived burst offset. A 30.123-second internal
  capture of the full generated Zelda example has no jumps above 0.15 after
  this synchronization; the pre-fix capture reached 0.369141.
- An unpublished immediate-next page is not mixed after sample 32 of the
  current block. This leaves the slowest measured 16.384 kHz mixer profile
  enough time to publish or cleanly fall back to silence.

The queue owns 1,936 bytes of EWRAM in the TinyGo target map. Its Timer 1 entry
stub and handoff path occupy 596 bytes of IWRAM code; the separate mixer is
1,328 bytes of IWRAM state. Only the measured 16.384 kHz rate is accepted by this
candidate. Other named clocks remain hardware probes until their refill
deadlines are measured.

`runtime/cmd/audiostream` exercises this path with four synth voices while the
normal Mode 4 draw, DMA 3 presentation, and 30 Hz pacing loop continue. The A
button pauses/resumes production to force the documented silence/recovery path;
Start rebuilds the stream and Select stops it.

`runtime/cmd/audiocoexist` adds the complete graphics matrix. Its current mGBA
run keeps four music channels under a global fade throughout all scenes. All
seven unforced scenes completed with zero underruns; forced starvation emitted
silence, restored all three queued pages, and then completed 68 handoffs with
no further underrun. The maximum observed IRQ latency was 74 samples, within
the 304-sample guard. Continuity passes; empty, sparse-map, and complete
GBA-class frames fit one 30 Hz work budget. Clear/HUD, 100-sprite, dense-map,
and complete PICO-class scenes remain over budget. These are explicit support
limits, not continuity failures. Exact cycles are recorded in
[performance.md](performance.md).

## Version-1 PICO-style SFX path

Milestone 2's first end-to-end slice uses one shared implementation from source
asset to target PCM:

```text
assets/audio/sfx.json
  -> strict host validation and pitch/volume compilation
  -> generated audio.SFXDefinition/audio.SFXStep ROM arrays
  -> Context.SFXV
  -> four-channel audio.SFXPlayer
  -> shared Mixer
  -> MonoStream/FIFO A
```

The authored schema has explicit version 1, stable IDs 0..63, one to 32 steps,
PICO pitch 0..63, volume 0..7, speed 1..255, optional half-open loop bounds,
and exact waveform/effect names. Triangle, tilted saw, saw, square, fixed 25%
pulse, organ, deterministic noise, and phaser are implemented. `none`,
tick-accurate `slide`,
quarter-tone `vibrato`, duration-scaled `drop`, and sample-continuous
`fade-in`/`fade-out`, plus pitch-only `arpeggio-fast`/`arpeggio-slow`, are
accepted; unsupported effects fail host validation before TinyGo.

PICO-8 exports one speed unit as exactly 183 samples at 22,050 Hz. `SFXPlayer`
uses the reduced `499712/3675` target-sample ratio, splitting mixer output at
alternating 136/137-sample boundaries inside a 256-sample page without
floating-point target math or cumulative drift. Four logical channels use
fixed state, deterministic inactive-first/oldest automatic selection, explicit
restart/stop, and mixer voice ownership generations so a stolen voice is not
stopped accidentally. SFX definitions and compiled steps remain in ROM.

Compatible built-in note cells and explicit same-channel `sfx()` retriggers
update the owned oscillator in place, preserving PICO-8's per-channel phase.
This avoids the quiet speed-locked pulse produced by restarting phase zero at
every cell. A measured 48-sample edge ramp handles volume-only changes;
starts, stops, fade-in cells, and incompatible waveform/filter replacements
retain the independent release-tail/attack overlap.

Slide preserves oscillator phase and linearly interpolates phase step and
volume toward the next played sequencer step once every exact rational SFX
tick. Loop wrap is a valid destination only when the complete loop is active
inside the requested offset/length slice. Vibrato uses a fixed 16-tick cosine
cycle spanning one quarter-tone peak-to-peak; at the approximately 120.4918 Hz
effect clock this is a roughly 7.53 Hz LFO. Drop linearly scales phase increment from the authored pitch
toward zero across the current step. Fade-in and fade-out use inverse
per-sample Q16 gain ramps between silence and authored volume. Arpeggios select
pitch from aligned groups of four while the current step keeps
its waveform and volume. Fast/slow cadence is four/eight ticks, halved when
SFX speed is at most eight, and its phase continues across steps, group changes,
and loop wraps. The added timeline byte fits existing channel padding, so target
state size is unchanged. Effect updates preserve oscillator phase within the
current step. Ownership-checked mixer updates verify
the voice's start generation, so a channel cannot modulate a voice after that
stable ID has been stolen and reused. The effect byte fits in the existing
eight-byte target step record.

Per-SFX `noiz`, `buzz`, `detune`, `reverb`, and `dampen` settings compile into
PICO-8's native mixed-radix filter byte. Stateless oscillator variants remain
inside `Voice`; reverb/dampen state belongs to the four logical channels so a
tail survives the final oscillator and checkpoint rewinds. Each channel owns
a fixed append-only Q6 history ring. Checkpoints retain only positions,
counters, and one-pole state, avoiding the underruns caused by copying bulk
history during a producer rewind.

Generated audio carts initialize the player before `_init`, allowing startup
SFX to be included in the initial four pages. Before each physical page, the
player stores a fixed mixer/sequencer checkpoint. After startup, valid calls
enter an eight-command fixed queue. The next service point restores the
checkpoint for the page after the immediate-next page, applies all commands in
order, regenerates that page, and discards the now-stale tail page. This leaves
two ready pages; the following service point refills the tail from the updated
timeline. The Timer 1 interrupt still performs no mixing.

Generated loops service the stream at frame start, after update, after draw,
and after presentation DMA. Full-queue command/stop latency is therefore
roughly 15.6..31.25 ms instead of approaching 62.5 ms. A depleted queue or
missing checkpoint falls back to the producer tail and increments explicit
telemetry. Queue overflow also fails closed and is observable.

The four checkpoints include complete mixer, effect-tick, music, fade-gain,
filter cursors, and timeline state. The current target map reports 19,376 bytes
of EWRAM for `SFXPlayer`, including four fixed reverb histories; the plain
stream remains 1,936 bytes and the expanded mixer is 1,328 bytes IWRAM. Build
output reports each cartridge's remaining EWRAM directly.
In mGBA, an injected A-button call moved from one pending command to one
successful rewind with no fallback or underrun, then the next service restored
three queued pages. The repeatable target benchmark measures eight commands
ending in four fading voices at 155,654 cycles (152.0 sample periods), while
either arpeggio takes 112,797 cycles. All remain within one 256-sample page and
the 304-sample guard; the worst case retains 104.0 sample periods of page
headroom. Audible behavior and controls require real-hardware sign-off.

Debugger-visible `StreamStats` records queued/mixed blocks, block boundaries,
underruns, late boundaries, maximum lateness, and fail-closed switch errors.
`SFXStats` adds pending/queued/dropped/fallback commands, successful rewinds,
and bounded music pattern/start/stop/transition/fade/invalid-call state.
Generated cartridges attach both objects to `Context`: Go callers can read one
combined `AudioStats()` snapshot, and Lua `stat(80)` through `stat(89)` expose
the release-critical underrun, queue, voice, stealing, failure, rewind,
fallback, drop, and maximum-lateness counters. Reads do not enable the optional
cycle profiler or mutate production state. Integer counters saturate at 32,767
at the Lua boundary; the Go snapshot retains their full native widths.

## Version-1 PICO-style music path

Milestone 3 reuses the SFX player and mixer rather than adding another voice
engine:

```text
assets/audio/music.json
  -> strict pattern/SFX-reference validation and duration compilation
  -> generated audio.MusicPattern ROM array
  -> Context.MusicV
  -> music state inside audio.SFXPlayer
  -> existing four logical channels, Mixer, MonoStream, and FIFO A
```

Each stable pattern ID is a `music()` entry point. Four slots contain SFX IDs
or `-1`; an explicit next ID forms the arrangement, backward links loop, and
`-1` stops. Pattern duration is the longest complete assigned SFX duration,
compiled with the same exact cumulative `183/22050`-second speed-unit clock as
the SFX sequencer. At 16,384 Hz this is the reduced rational
`499712/3675` target samples per unit, with the remainder retained across the
complete pattern duration. SFX-local loops may sound within that interval but
do not block sample-accurate transition.

Logical channels carry one music-ownership bit. A manual explicit `sfx()` can
interrupt music until the next assigned transition. Silent music slots leave
manual SFX untouched, while `music(-1)` stops only channels still owned by
music. Automatic SFX selection keeps the established inactive-first/oldest
rule. Music commands share the fixed eight-command queue, and every physical
page checkpoint includes the complete current pattern and remaining-sample
clock so rewinds preserve arrangement phase.

The mixer also tags each physical voice as music-owned or manual. One global
music gain uses a quotient/remainder integer clock to move sample-continuously
between silence and unity for any accepted millisecond duration. This gain is
applied only to the music subtotal, so note-level fades compose with it while
manual SFX remain unscaled. Start fades begin a new pattern immediately at
silence; stop fades keep pattern transitions running until the gain reaches
silence, then release music-owned channels. Gain and fade clocks are included
in every rewind checkpoint.

No target heap state is introduced. The current target map uses the same
19,376-byte checkpointed/filter player and 1,328-byte IWRAM mixer for music;
project build output reports exact ROM and remaining EWRAM.

The current ARM7TDMI benchmark measures four music-owned channels at 33,808
cycles per 256 samples at unity gain and 44,276 cycles (16.9% of the page
budget) during a one-second global fade. The music-active mGBA coexistence
self-check has zero unforced underruns, one successful setup rewind, and zero
command fallbacks. Three heavy graphics stress cases remain over one 30 Hz
work budget.

## Version-1 native PCM path

Milestone 4 remains a client of the shared mixer and producer:

```text
assets/audio/samples/manifest.json + PCM WAV sources
  -> strict host decode/downmix/trim/normalize/resample/quantize
  -> deduplicated []int8 PCM ROM + audio.SampleDefinition records
  -> Context.SampleV
  -> audio.SFXPlayer command queue and page checkpoints
  -> shared twelve-slot Mixer, MonoStream, and FIFO A
```

Target PCM stays in ROM and each voice retains only a slice plus fixed cursor,
fraction, loop, gain, and priority state. Mixer checkpoints copy active voice
state, including that complete cursor, and rebuild stable-ID positions, so
sample start/stop required no parallel
timeline or target heap allocation. Sample-bearing carts initialize twelve
shared slots: four PICO channels can each overlap one release tail, while
native PCM uses the remaining capacity under the same inactive-first,
priority/age-stealing policy. `sample(-1)` scans and stops only PCM sources.

The host compiler accepts a deliberately narrow integer PCM WAV subset and
emits unity PCM; authored gain remains in the definition so runtime overrides
apply once. Loop points originate in source frames and are remapped to the
half-open converted range. Identical converted PCM sequences share a range.
`examples/audio-samples` compiles 7,998 bytes of SFX/music/sample records and
data into a 77,208-byte ROM and leaves 50,500 bytes EWRAM free. Its looping
two-channel pattern plus interactive PCM controls exercise the documented
mixed safety profile directly.

The `pcm` build of `runtime/cmd/audiocoexist` exercises the production
twelve-slot cartridge path with four fading synth channels plus one looping PCM
voice. All seven unforced mGBA graphics cases retain that exact source state
with zero underruns, sample failures, command fallbacks, or drops. Forced
starvation emits five silent pages, restores all three queued pages, and adds
68 clean recovery handoffs. Page-level music packing and a block word mixer
reduce the full profile's empty-scene cost from 177.4% to 99.3% without
changing output. Four synth voices plus PCM fit empty and sparse scenes; three
synth voices plus PCM fit four common scenes, while two fit five. The two-synth
candidate is the documented safety boundary because the three-synth PICO case
consumes 100.8% of one tick. Neither candidate includes the 100-sprite or dense-map
stresses. Exact cycles are recorded in [performance.md](performance.md).

### Shared editor preview

The headless editor model loads and saves the same version-1 SFX, music, and sample
libraries used by the build pipeline. Preview first runs the normal strict
compilers, maps their records into `runtime/audio`, and renders through the
same `Mixer`, `SFXPlayer`, and music sequencer linked into generated carts.
Step duration, pattern transitions, loop bounds, oscillator phase,
deterministic noise, music fade gain, volume, and signed 8-bit saturation
therefore have one implementation on host and target.

The Ebitengine adapter runs the shared core at the exact 16,384 Hz target rate
and converts each signed mono byte to stereo signed 16-bit output only after
rendering. Editor windowing, transport state, and host audio dependencies stay
outside the runtime module. Music mute/solo is implemented by clearing host
preview assignments after timing compilation, so it cannot mutate cartridge
data or shorten the arrangement clock. Host golden tests compare target PCM
bytes and step/sample mapping directly. Sample source preview exposes the
decoded/downmixed source separately; target preview runs converted ROM bytes,
authored gain, and loop behavior through the shared mixer.

## Shared mixer foundation

The first hardware-independent Milestone 1 slice lives in `runtime/audio` and
is linked into `audiospike` so TinyGo checks the real target code path. It
provides:

- One fixed 12-slot state array with selectable 4, 6, 8, or 12 active voices.
- Packed active state for contiguous hot-loop reads, with stable voice IDs
  maintained by fixed lookup tables.
- Square, fixed 25% pulse, triangle, saw, deterministic noise, 32-sample
  wavetable, and signed 8-bit PCM source kinds.
- Full-cycle fixed-point oscillator phase and 16.16 PCM stepping.
- PCM intro/loop ranges.
- Integer volume, sample-continuous Q16 gain ramps, and fixed-point pan state.
  Plain mono voices bypass the ramp hot path.
- Deterministic inactive-first allocation, priority stealing, protected
  voices, and oldest-start tie breaking.
- Explicit signed 8-bit saturation.
- Packed FIFO-word output without an intermediate conversion buffer.
- Active-only checkpoints that preserve stable IDs without copying inactive
  slots or retaining stale PCM slice pointers.
- Active/peak voices, steals, and failed-start telemetry.

The complete 12-slot PICO-instrument/filter-capable `Mixer` occupies 1,328
bytes in the TinyGo ARM target map.
The current production candidate places that hot state in IWRAM: target
measurements save 6–29% for active workloads compared with the retained EWRAM
baseline. The 4/6/8 profiles reduce work but not state size; compile-time state specialization
can be considered only if later memory measurements justify the extra generated
variants. Host golden and allocation tests cover every source category,
looping, saturation, stealing, stable IDs, and packed byte order. Benchmark
entry points cover synth, PCM, and mixed workloads through 12 voices; exact
results are recorded in [performance.md](performance.md).

### Provisional stereo/pan slice

Milestone 5 now has a hardware-independent stereo path in the same mixer. Pan
uses a bounded linear law from -256 (hard left), through 0 (unity gain in both
channels), to +256 (hard right). `MixStereo` emits separate signed 8-bit
streams and `MixStereoWords` packs the same samples for paired FIFO A/FIFO B
pages. Centered output is byte-for-byte equal to `MixMono`, including PCM end
handling, note-volume ramps, and global music fades. Both stereo methods are
allocation-free and independently saturate the two channels.

The mono methods, production `MonoStream`, and generated game path are
unchanged. A separate TinyGo ROM measures 119,608 cycles for four centered
stereo voices, 131,384 for four distributed-panned voices, 78,168 for two
hard-panned fading voices, 138,298 for four distributed fading voices, and
154,181 for four centered synth voices plus one hard-right PCM voice. Eight
panned voices consume 96.0% of a page and twelve exceed real time. The two-voice
fading case consumes 29.8%; the four-voice fading case consumes 52.8% before
stream and graphics work. Mono-only ROMs dead-strip the stereo loops and second
page ring.

Pan can be changed through generation-checked mixer ownership or configured
persistently per SFX channel so a setting survives sequencer voice restarts.
These are low-level controls only; Lua assets and editor settings do not expose
them.

The experimental `StereoStream` wires those words to FIFO A/DMA1 left and FIFO
B/DMA2 right. It publishes and redirects two four-page rings as one unit,
applies one Timer 1 progress-derived burst offset to both DMA sources, and
silences both sides together on underrun. Its SFX/music/sample producer uses
the existing four checkpoints and command-rewind rules. One normal-context
service call is capped at two page pairs to bound renderer-adjacent latency.
The stream occupies 3,264 bytes EWRAM, exactly 1,328 bytes more than
`MonoStream`; the increment is the second ring plus wrap guard. Host tests
cover paired continuation guards, rewind, handoff, underrun recovery,
exclusive mono/stereo ownership, and zero allocations.

`runtime/cmd/audiostereostream` retained three queued pairs through 623 mGBA
handoffs with zero underruns using the accepted two-hard-panned-voice profile.
Its extractor asserts running state, zero underruns, and three queued pairs.
DMA1 and DMA2 both remained enabled with control
`0xB640`, Direct Sound routing settled at `0x120C` (A left, B right), and the
maximum Timer 1 lateness was 65 samples inside the 304-sample guard. This is
emulator scheduling/register evidence, not audible proof.

The full mGBA coexistence harness passes with two continuously fading,
hard-panned music voices. All seven unforced graphics workloads have zero
underruns. Forced starvation produces exactly five silent misses, service
restores three queued pairs, and 68 more handoffs add no misses. Empty,
clear/HUD, sparse-map, and complete GBA-class scenes fit one 30 Hz tick; the
100-sprite, dense-map, and complete PICO-class stress scenes remain over budget,
matching the mono support boundary. A four-voice stereo run produced unforced
misses and exceeded the empty-scene tick budget, so it is rejected as a
production graphics profile. Internal-audio capture, real-hardware listening,
and user-facing pan controls were the remaining gates at that point.

`runtime/cmd/audiostereocapture` now closes the emulator channel-routing gate
with a spectrally distinct two-voice profile shared by the target ROM and host
reference renderer: a 512 Hz hard-left square and 768 Hz hard-right triangle.
The checked-in mGBA 0.10.3 internal `mAVStream.postAudioFrame` capture has
65,368 stereo signed 16-bit frames at 32,768 Hz. After its 3,184-frame startup
prefix, each recorded channel correlates 1.0 with its own reference and 0 with
the opposite reference. Inter-channel correlation is 0, both gain ratios are
1.0, and the settled capture has no silent frames. A second run produced the
same SHA-256. This proves the emulator's Direct Sound A-left/B-right software
routing without observing system audio. Real-hardware listening and
user-facing pan controls remain absent, so
project-level stereo stays disabled. The version-1 sample policy is now
explicit: cartridge PCM remains the deterministic integer L/R average, while
the editor alone preserves source channels for stereo waveform display and
audition.

## Candidate Timer 0 clocks

Timer 0 uses the 16,777,216 Hz system clock with the F/1 prescaler. The exact
sample rate is `16,777,216 / period` and the reload is `65,536 - period`.

| Named rate | Period | Reload | Exact hardware rate |
| --- | ---: | ---: | ---: |
| 16,384 Hz | 1,024 | `0xFC00` | 16,384 Hz |
| 18,157 Hz class | 924 | `0xFC64` | 18,157.160173... Hz |
| 22,050 Hz class | 761 | `0xFD07` | 22,046.275953... Hz |
| 32,768 Hz | 512 | `0xFE00` | 32,768 Hz |

The first probe runs at exactly 16,384 Hz. That is a probe choice, not the
production-rate decision.

## Initialization and stop contract

Start performs these operations in order:

1. Stop Timer 0 and DMA 1 and clear prior probe state.
2. Set the sound bias midpoint and enable the sound master.
3. Route Direct Sound A to left and right at full volume and reset FIFO A.
4. Prime FIFO A and program DMA 1 source/destination/control.
5. Load the Timer 0 reload and optionally start Timer 1 in cascade mode.
6. Enable DMA 1, then start Timer 0.

Stop disables Timer 0 before DMA 1, resets FIFO A, clears the DMA/timer
registers, writes a zero FIFO word, and disables the sound master. Restart
always begins with the same stop/reset sequence.

## Probe ROM

Build from `runtime/`:

```sh
make audiospike-gba
```

The ROM starts in incrementing-source stream mode with a 128 Hz triangle wave
carried at 16.384 kHz. Controls are:

- A: fixed-source 4.096 kHz tone
- B: DMA-fed silence
- L: incrementing-source 128 Hz stream
- Start: stop/reset and restart the current mode
- Select: stop and disable Direct Sound

The debugger-visible `probeStatus` word reports tone, silence, stream, stopped,
or failed initialization. `streamSamples`, `streamRestarts`, and
`streamLateRestarts` report scheduling behavior. The visual loop continues
using Mode 4 presentation through DMA 3 and 30 Hz pacing through Timer 2 +
Timer 3.

## Hardware sign-off still required

`runtime/cmd/audiohardware` is the self-scoring physical-GBA handoff harness
for both supported mono production profiles. Build both variants from
`runtime/`:

```sh
make audiohardware-gba audiohardwarestandard-gba
```

`audiohardware.gba` continuously runs the measured two-synth-plus-one-looping-
PCM safety profile. `audiohardwarestandard.gba` runs the separate four-synth
profile with no PCM voice. Both issue periodic one-second fade-out/fade-in
commands through checkpoint rewind and present a moving Mode 4 screen with
DMA3. After 18,000 successful 30 Hz ticks (ten minutes), each screen changes
from `RUNNING` to `PASS`. Any unforced underrun, stream-switch failure, wrong
PCM voice count, failed sample start, command fallback, or command drop changes
it to `FAIL`. A toggles an intentional stall and restarts the test on resume,
Start performs a full audio restart, and Select verifies stop. Debugger-visible
`hardware*` words mirror status, ticks, boundaries, underruns, queue depth,
rewinds, fallbacks, drops, and active PCM voices.

The short automated mGBA extractor:

```sh
scripts/audio/extract_audiohardware.sh
scripts/audio/extract_audiohardwarestandard.sh
```

The sample-profile extractor reached 239 ticks and 511 handoffs; the standard
extractor reached 239 ticks and 630 handoffs. Both had zero underruns, three
queued pages, two successful fade rewinds, zero fallbacks or drops, and a
74-sample maximum Timer 1 lateness. The sample build retained one PCM voice;
the standard build retained zero, as required. Both extractors assert those
invariants.

The full sample and standard soak scripts reached PASS at 18,000 ticks and
38,646 / 48,634 handoffs respectively. Each completed 120 successful rewinds,
60 verified fade-out/fade-in cycles, three queued pages, a 75-sample maximum
lateness, and zero pending commands, underruns, stream switch failures,
fallbacks, or drops. The sample build retained one PCM voice and the standard
build retained zero. Commands observed in the second half of the current page
stay pending until a handoff/refill exposes an early phase with at least two
ready future pages. Rebuilding the page after next from that state preserves
the current and immediate pages without waiting for an unnecessary third ready
page. This keeps the measured 246.1-sample regeneration away from the playback
boundary without losing command liveness. Only uninterrupted physical runs can
satisfy the hardware gate.

### Emulator observation

mGBA 0.10.3 reached the first automatic stream restart in the original
hand-built waveform probe and both measured mixer layouts:

```text
                        samples  restarts  late restarts
hand-built stream         4,233         1              0
initial mixer stream      4,296         1              0
packed/IWRAM mixer        4,315         1              0
```

The 30 Hz loop therefore serviced the 4,096-sample logical boundary 137, 200,
and 219 samples later respectively, well inside the duplicated guard. The
mixer-backed EWRAM bytes also matched the expected signed triangle sequence.
This proves Timer 1 cascade, incrementing DMA progression, polling, source
restart, and target mixer output cooperate under mGBA. Audible continuity
still requires listening tests; debugger telemetry cannot prove the absence
of a click or short silence during reset.

Record each result separately for current mGBA and real GBA hardware:

- Tone starts from a cold boot.
- Active silence contains no stale tone bursts.
- Stop produces no stale loud sample.
- Repeated tone/silence/stop/restart transitions remain reliable.
- Stream restarts do not click, stall, or change pitch.
- `streamRestarts` advances and `streamLateRestarts` remains zero.
- Playback pitch matches the exact Timer 0 rate.
- DMA 3 Mode 4 presentation remains visually correct during playback.
- Both `audiohardware.gba` and `audiohardwarestandard.gba` reach their visible
  ten-minute PASS states without an audible discontinuity or visual corruption.

These probe checks remain prerequisites for comparing audible behavior with
the interrupt-assisted candidate. Do not select the final production buffering
architecture from emulator telemetry alone.

The interrupt-assisted candidate additionally requires:

- Listening for clicks or duplicated/skipped samples at normal handoffs.
- Confirming delayed handoffs remain continuous through the guard.
- Confirming forced underruns are silence and recovery does not replay stale
  samples.
- Repeating the zero-unforced-underrun graphics matrix result on real hardware.
- Running both production-profile hardware ROMs to PASS on real GBA hardware.
- Verifying the maximum Timer 1 lateness stays below the 304-sample guard.

## Hardware references

- [GBATEK: GBA sound control, FIFO, timers, and sound DMA](https://mgba-emu.github.io/gbatek/)
- [Tonc: GBA DMA control fields](https://gbadev.net/tonc/dma.html)
