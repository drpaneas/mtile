# Legacy audio v1 performance measurements

> Preserved benchmark evidence for the superseded software-synth and Direct
> Sound design. Native GBA audio v2 establishes new performance matrices.

## Status

The benchmark harness has exact ARM7TDMI measurements from mGBA 0.10.3. The
emulator mixer, refill, synth-only, synth-plus-PCM, stereo-routing, and
graphics-coexistence gates are complete. Final hardware sign-off still
requires the prepared ROMs to run on a physical GBA. Host numbers remain
regression signals only and are not GBA budgets.

## Mixer benchmark ROM

Build from `runtime/`:

```sh
make audiobench-gba
```

`runtime/cmd/audiobench` generates one 256-sample signed 8-bit mono block for
each workload:

| Index | Workload |
| ---: | --- |
| 0 | Silence |
| 1 | 1 triangle oscillator |
| 2 | 4 triangle oscillators |
| 3 | 8 triangle oscillators |
| 4 | 12 triangle oscillators |
| 5 | 1 looping PCM voice |
| 6 | 4 looping PCM voices |
| 7 | 4 oscillators + 1 PCM voice |
| 8 | 4 oscillators + 4 PCM voices |
| 9 | 1 SFX channel, no note effect |
| 10 | 1 SFX channel with slide |
| 11 | 1 SFX channel with vibrato |
| 12 | 1 SFX channel with drop |
| 13 | 1 SFX channel with fade-in |
| 14 | 1 SFX channel with fade-out |
| 15 | 1 SFX channel with fast arpeggio |
| 16 | 1 SFX channel with slow arpeggio |
| 17 | 4 SFX channels, no note effect |
| 18 | 4 SFX channels with slide |
| 19 | 4 SFX channels with vibrato |
| 20 | 4 SFX channels with drop |
| 21 | 4 SFX channels with fade-in |
| 22 | 4 SFX channels with fade-out |
| 23 | 4 SFX channels with fast arpeggio |
| 24 | 4 SFX channels with slow arpeggio |
| 25 | Full eight-command rewind ending with 4 slide channels |
| 26 | Full eight-command rewind ending with 4 vibrato channels |
| 27 | Full eight-command rewind ending with 4 drop channels |
| 28 | Full eight-command rewind ending with 4 fade-in channels |
| 29 | Full eight-command rewind ending with 4 fade-out channels |
| 30 | Full eight-command rewind ending with 4 fast-arpeggio channels |
| 31 | Full eight-command rewind ending with 4 slow-arpeggio channels |
| 32 | 4 music-owned channels at unity global gain |
| 33 | 4 music-owned channels during a 1-second `music()` fade-in |
| 34 | 4 triangle channels with `noiz` |
| 35 | 4 triangle channels with `buzz` |
| 36 | 4 triangle channels with level-2 `detune` |
| 37 | 1 triangle channel with level-1 `reverb` |
| 38 | 4 triangle channels with level-1 `reverb` |
| 39 | 4 triangle channels with level-2 `reverb` |
| 40 | 1 triangle channel with level-1 `dampen` |
| 41 | 4 triangle channels with level-1 `dampen` |
| 42 | 4 triangle channels with level-2 `dampen` |
| 43 | 4 triangle channels with every filter at its maximum setting |

Cases 0..24 and 32..43 keep Direct Sound stopped because Timer 0 + Timer 1 form
the exact 32-bit operation counter. Cases 25..31 start the real four-page stream and use
the independent Timer 2 + Timer 3 counter around checkpoint restore, eight
commands, and one page regeneration. Each case is warmed once, then timed once.
Results appear on screen as cycles per block and integer cycles per sample.
Each workload uses the smallest supported 4/6/8/12 voice profile that can hold
it. Active voice state is packed, so the hot loop scales with active voices
while returned voice IDs remain stable.

Debugger telemetry:

```text
benchStatus = 0xA0B00001  measuring
benchStatus = 0xA0B00002  complete
benchStatus = 0xDEADA0B0  setup failure
benchCount                  valid result count
benchFailure                failing case/stage/detail, zero on success
benchCycles[benchCount]     cycles per 256-sample block
```

From the repository root, the repeatable mGBA/GDB extractor rebuilds both
targets, resolves the telemetry addresses from the map, waits for completion,
and prints all counters:

```sh
scripts/audio/extract_audiobench.sh
```

Set `MGBA_BIN` or `GDB_BIN` when those executables are not at their macOS
defaults. The same ELF/map symbols remain suitable for a hardware debugger.

## Converting a block result to a game-tick budget

For a measured 256-sample cost `C` and sample rate `R`, estimated mixer cost
per 30 Hz game tick is:

```text
samples per tick = R / 30
cycles per tick  = C * R / (256 * 30)
tick percentage  = cycles per tick / 561,792 * 100
```

Use the exact hardware rate from `architecture.md`, not only its named rate.
Round the required block count upward when sizing synchronous buffers. Final
support decisions must include refill overhead, DMA contention, and the full
graphics coexistence matrix; the isolated mixer result is necessary but not
sufficient.

## ARM7TDMI baseline

Measured on 2026-08-23 with TinyGo 0.41.1 (`-gc=none -opt=2`) and mGBA 0.10.3.
The current 44-case harness passes its allocation check with the 1,328-byte
PICO-instrument/filter-capable mixer in IWRAM. PCM source bytes, generated
blocks, and the 19,376-byte checkpoint/filter-history player remain in EWRAM.

At the exact 16,384 Hz clock, one 256-sample block spans 262,144 CPU cycles.
The tick percentage below is therefore also the percentage of that block's
real-time audio budget.

| Workload | Profile | Cycles/block | Cycles/sample | 16,384 Hz budget |
| --- | ---: | ---: | ---: | ---: |
| Silence | 4 | 7,907 | 30.9 | 3.0% |
| 1 synth | 4 | 10,719 | 41.9 | 4.1% |
| 4 synth | 4 | 31,689 | 123.8 | 12.1% |
| 8 synth | 8 | 59,649 | 233.0 | 22.8% |
| 12 synth | 12 | 87,609 | 342.2 | 33.4% |
| 1 PCM | 4 | 69,285 | 270.6 | 26.4% |
| 4 PCM | 4 | 152,268 | 594.8 | 58.1% |
| 4 synth + 1 PCM | 6 | 217,103 | 848.1 | 82.8% |
| 4 synth + 4 PCM | 8 | 300,086 | 1,172.2 | 114.5% |

The ordinary triangle, triangle-ramp, all-music-ramp, and stateful-filtered
triangle paths select compact block mixers before entering the expanded
eight-instrument dispatcher. Stateful filters precompute their target delay
and pole, and dampen uses bounded int32 Q12 arithmetic. Checkpoints copy active
voices only and rebuild stable-ID positions. The 4+4 mixed profile remains
above real time and is not a production support boundary.

Because mixer cycles per generated sample do not change with the output clock,
the same measurements project to the candidate rates as follows:

| Workload | 16,384 Hz | 18,157 class | 22,050 class | 32,768 Hz |
| --- | ---: | ---: | ---: | ---: |
| 4 synth | 12.1% | 13.4% | 16.3% | 24.2% |
| 4 synth + 4 PCM | 114.5% | 126.9% | 154.1% | 229.0% |
| 12 synth | 33.4% | 37.0% | 44.9% | 66.8% |

Values at or above 100% cannot run in real time even before refill and game
work. Lower values still require safety headroom and the coexistence matrix;
these results do not yet select a production sample rate or advertised voice
profile.

## Measured SFX effect increments

Measured on the same mGBA/TinyGo configuration. The original effects use two
speed-4 triangle steps. Arpeggios use four speed-8 steps and are primed so the
timed page begins on their next pitch-update boundary. Fade gain continues
updating on every sample.

| Workload | Cycles/block | Cycles/sample | 16,384 Hz block budget |
| --- | ---: | ---: | ---: |
| 1 SFX channel, none | 12,110 | 47.3 | 4.6% |
| 1 SFX channel, slide | 20,960 | 81.9 | 8.0% |
| 1 SFX channel, vibrato | 16,246 | 63.5 | 6.2% |
| 1 SFX channel, drop | 19,966 | 78.0 | 7.6% |
| 1 SFX channel, fade-in | 23,534 | 91.9 | 9.0% |
| 1 SFX channel, fade-out | 23,534 | 91.9 | 9.0% |
| 1 SFX channel, fast arpeggio | 15,889 | 62.1 | 6.1% |
| 1 SFX channel, slow arpeggio | 15,889 | 62.1 | 6.1% |
| 4 SFX channels, none | 33,699 | 131.6 | 12.9% |
| 4 SFX channels, slide | 61,517 | 240.3 | 23.5% |
| 4 SFX channels, vibrato | 42,661 | 166.6 | 16.3% |
| 4 SFX channels, drop | 57,541 | 224.8 | 22.0% |
| 4 SFX channels, fade-in | 60,417 | 236.0 | 23.0% |
| 4 SFX channels, fade-out | 60,417 | 236.0 | 23.0% |
| 4 SFX channels, fast arpeggio | 41,233 | 161.1 | 15.7% |
| 4 SFX channels, slow arpeggio | 41,233 | 161.1 | 15.7% |

Slide adds 8,850 cycles for one channel and 27,818 cycles for four channels;
vibrato adds 4,136 and 8,962 cycles respectively; drop adds 7,856 and 23,842;
either fade adds 11,424 and 26,718; either arpeggio adds 3,779 and 7,534. The
global music-gain specialization keeps ownership checks out of the all-music
sample loop while mixed manual/music voices retain the general path. All
benchmark ROMs still pass the TinyGo `-gc=none` allocation check. This proves bounded generation
cost, not audible equivalence or coexistence on real hardware.

Case 32 is the four-channel unity-gain music control: 33,808 cycles per block,
132.1 cycles/sample, and 12.9% of the 16,384 Hz page budget. Case 33 activates
a one-second global fade-in and costs 44,276 cycles, 173.0 cycles/sample, and
16.9% of the page. The fade therefore adds 10,468 cycles, or 4.0% of a page.

The same ROM measures the complete fixed command queue while the real stream
owns Timer 0/1:

| Rewind workload | Cycles | Sample periods | 256-sample page budget | 304-sample guard |
| --- | ---: | ---: | ---: | ---: |
| Eight commands, four final slide channels | 124,055 | 121.1 | 47.3% | 39.9% |
| Eight commands, four final vibrato channels | 113,409 | 110.8 | 43.3% | 36.4% |
| Eight commands, four final drop channels | 121,669 | 118.8 | 46.4% | 39.1% |
| Eight commands, four final fade-in channels | 155,654 | 152.0 | 59.4% | 50.0% |
| Eight commands, four final fade-out channels | 155,654 | 152.0 | 59.4% | 50.0% |
| Eight commands, four final fast-arpeggio channels | 112,797 | 110.2 | 43.0% | 36.2% |
| Eight commands, four final slow-arpeggio channels | 112,797 | 110.2 | 43.0% | 36.2% |

All restore the page-after-next checkpoint, apply all commands, regenerate
the target page, preserve the current and immediate-next pages, and leave the
tail for the following service point. Profile-bounded checkpoints and
active-only voice copies keep the worst note-fade rewind within one page at
59.4% and within the 304-sample guard at 50.0%, leaving 106,490 page cycles
before interrupt latency.

## Measured PICO filter workloads

Cases 34..43 use the same four-channel triangle source as the SFX control and
exercise each PICO filter independently plus the maximum mixed-radix filter
byte. `noiz` is inert on triangle, matching the runtime's instrument-specific
noise semantics; its row is therefore also a dispatcher-regression check.

| Workload | Cycles/block | Cycles/sample | 16,384 Hz block budget |
| --- | ---: | ---: | ---: |
| 4 channels, `noiz` | 33,699 | 131.6 | 12.9% |
| 4 channels, `buzz` | 78,117 | 305.1 | 29.8% |
| 4 channels, level-2 `detune` | 99,609 | 389.1 | 38.0% |
| 1 channel, level-1 `reverb` | 58,624 | 229.0 | 22.4% |
| 4 channels, level-1 `reverb` | 203,015 | 793.0 | 77.4% |
| 4 channels, level-2 `reverb` | 174,471 | 681.5 | 66.6% |
| 1 channel, level-1 `dampen` | 46,472 | 181.5 | 17.7% |
| 4 channels, level-1 `dampen` | 154,407 | 603.2 | 58.9% |
| 4 channels, level-2 `dampen` | 154,407 | 603.2 | 58.9% |
| 4 channels, all filters at level 2 | 257,263 | 1,005.0 | 98.1% |

Every steady filter workload fits one page. The all-filter stress leaves only
4,881 cycles (1.9%) before stream/refill and game work, so it is a conformance
capability rather than a recommended graphics workload. The eight-command
rewind table intentionally remains the note-effect queue gate; combining a
full queue burst with four maximum stateful filters is outside that measured
low-latency command boundary.

## Interrupt-assisted stream telemetry

`runtime/cmd/audiostream` links the four-page `MonoStream` candidate with four
triangle voices, Mode 4 drawing, DMA 3 presentation, and the existing 30 Hz
pacing loop. Build it from `runtime/` with:

```sh
make audiostream-gba
```

The target map reports:

| Resource | Cost |
| --- | ---: |
| Stream queue, wrap guard, silence page, producer pointer, and telemetry state | 1,936 bytes EWRAM |
| Mixer state | 1,328 bytes IWRAM |
| Timer 1 entry stub + block handoff | 596 bytes IWRAM code |
| Logical block | 256 samples / 15.625 ms |
| Four-page queue coverage | 1,024 samples / 62.5 ms |

mGBA 0.10.3 completed 129 normal block handoffs with zero underruns and three
queued blocks. Rendering delayed 19 IRQs; the maximum observed entry latency
was 51 samples. The contiguous guard and prefetch-aware DMA redirect handled
those late entries without resetting FIFO A.

A debugger-controlled refill stall drained the queue and incremented underrun
telemetry while cleared pages supplied silence. Re-enabling service restored
three queued blocks. After recovery, 64 further handoffs added no underruns.
The final forced-test snapshot was:

```text
boundaries=73  underruns=4  late=14  maxLateSamples=67
mixedBlocks=71  queuedBlocks=3  serviceEnabled=1
```

The fourth underrun occurred between reenabling service and the next normal
service point; the count remained stable after the queue repopulated. These
debugger results prove bounded scheduling and telemetry in the emulator, but
they cannot prove audible continuity.

## Physical-GBA soak handoff

`runtime/cmd/audiohardware` is an allocation-checked, self-scoring ten-minute
harness with one ROM for each production profile. The default build keeps one
looping sample active with two synth voices; the `standard` build runs four
synth voices and no PCM. Both alternate deferred music fade-out/fade-in
commands, present a moving Mode 4 frame through DMA3, and fail closed on any
unforced underrun, stream switch failure, wrong sample count, failed start,
command fallback, or command drop. Build them with:

```sh
cd runtime && make audiohardware-gba audiohardwarestandard-gba
```

The screen reports `PASS` after 18,000 successful 30 Hz ticks. The short mGBA
regression extractor:

```sh
scripts/audio/extract_audiohardware.sh
scripts/audio/extract_audiohardwarestandard.sh
```

asserted the following early-run snapshots:

```text
ticks=239  boundaries=511  underruns=0  queued=3
rewinds=2  fallbacks=0  drops=0  activePCM=1  maxLateSamples=74
ticks=239  boundaries=630  underruns=0  queued=3
rewinds=2  fallbacks=0  drops=0  activePCM=0  maxLateSamples=74
```

Both full mGBA 0.10.3 soaks reached their ROMs' own PASS states:

```text
sample:   ticks=18000  boundaries=38646  underruns=0  queued=3  rewinds=120
switchFailures=0  fallbacks=0  drops=0  activePCM=1  maxLateSamples=75
standard: ticks=18000  boundaries=48634  underruns=0  queued=3  rewinds=120
switchFailures=0  fallbacks=0  drops=0  activePCM=0  maxLateSamples=75
both:     musicStarts=61  musicStops=60  fadeChecks=60  pendingCommands=0
```

Commands that arrive in the second half of the current page remain pending
until a handoff/refill cycle exposes an early phase with at least two ready
future pages. Rebuilding the page after next from that state preserves the
current and immediate pages without unnecessarily waiting for a third ready
page. This keeps the measured 246.1-sample regeneration away from the page
boundary while preserving command liveness. The repeatable command is
`scripts/audio/soak_audiohardware_mgba.sh`; the four-synth counterpart is
`scripts/audio/soak_audiohardwarestandard_mgba.sh`. These emulator passes
validate the ROMs and production paths, but they are deliberately not recorded
as hardware sign-off until both visible ten-minute PASS and listening checks
are completed on a physical GBA.

## Graphics coexistence matrix

`runtime/cmd/audiocoexist` runs four music-owned triangle voices at 16,384 Hz
through the SFX sequencer and interrupt-assisted stream while executing the
eight workloads required by the engineering plan. A live deferred command
restarts a 30-second global fade once before measurement, ensuring the ramp is
still active across the complete matrix. Build the debugger-friendly target
from `runtime/` with:

```sh
make audiocoexist-elf
```

The end-to-end mGBA extraction is also repeatable from the repository root:

```sh
scripts/audio/extract_audiocoexist.sh
```

Each unforced case runs for 16 complete 30 Hz ticks. Maximum work cycles cover
normal-context audio service, the named draw workload, and the production DMA
3 framebuffer blit. The pacing wait and VBlank palette/flip are excluded.
Timer 1 audio IRQs remain active during measurement; Timer 2 + Timer 3 provide
the cycle counter. `Max service` isolates the slowest single `Service` call and
records how many 256-sample pages it generated. `Max late` is the cumulative
IRQ-latency high-water mark.

Measured on 2026-08-23 with the same TinyGo and mGBA versions as the mixer
baseline:

| Index | Workload | Max work | 30 Hz tick | Max service | Pages | Boundaries | Late | Max late | Fade left | Underruns |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 0 | Empty update/draw + blit | 440,135 | 78.3% | 353,973 | 3 | 34 | 9 | 68 samples | 482,560 samples | 0 |
| 1 | `cls` + HUD | 622,142 | 110.7% | 238,624 | 2 | 36 | 15 | 70 samples | 473,344 samples | 0 |
| 2 | 100 sprites | 763,401 | 135.9% | 238,815 | 2 | 48 | 8 | 70 samples | 461,056 samples | 0 |
| 3 | Sparse 30x20 mapdemo room | 543,849 | 96.8% | 464,589 | 4 | 34 | 7 | 70 samples | 452,352 samples | 0 |
| 4 | Dense 16x16 PICO room | 1,028,727 | 183.1% | 353,934 | 3 | 66 | 10 | 74 samples | 435,200 samples | 0 |
| 5 | Complete PICO-class tick | 710,523 | 126.5% | 236,856 | 2 | 48 | 17 | 74 samples | 423,168 samples | 0 |
| 6 | Complete GBA-class tick | 528,495 | 94.1% | 238,654 | 2 | 34 | 16 | 74 samples | 414,464 samples | 0 |
| 7 | Intentional 96 ms+ starvation | 1,666,221 | 296.6% | 0 | 0 | 9 | 2 | 74 samples | 416,256 samples | 5 |

All seven unforced workloads completed with zero audio underruns and no stream
switch failures. Every case retained four active music channels, a nonzero fade
clock, exactly one cumulative setup rewind, and zero fallback or dropped
commands. The forced case withheld service for at least six logical audio
blocks, emitted cleared silence for five missed pages, and repopulated all
three queued pages. After repopulation, 32 normal ticks completed 68 more block
handoffs with zero additional underruns.

The cycle column is a blocking capacity warning, not just an audio pass. Empty,
sparse-map, and complete GBA-class cases fit one 30 Hz work budget. Clear/HUD,
100-sprite, dense-map, and complete PICO-class cases remain at 110.7%, 135.9%,
183.1%, and 126.5%. The stream stays continuous because production service
runs at frame start, after update, after draw, and after the DMA3 copy, matching
generated cartridges. Those four scene classes cannot carry a production 30
Hz promise. Measured refill bursts cost approximately 116,125..119,392 cycles
per generated page, including an active-only checkpoint.

Debugger telemetry is retained in `coexistStatus`, `coexistCount`, the
`coexist*` case arrays (including fade, rewind, and fallback state), and the
recovery words. `0xA0C00002` means the self-check passed; `0xDEADA0C0` means
setup, unforced-underrun, fade-state, or recovery failure. Emulator timing does
not replace real-hardware listening and visual verification.

## Native PCM graphics coexistence matrix

The `pcm` build of `runtime/cmd/audiocoexist` repeats the identical matrix with
the generated-sample production profile: a twelve-slot mono mixer containing
four continuously fading music-owned triangle voices and one looping signed
8-bit PCM voice. Build and extract it with:

```sh
cd runtime && make audiopcmcoexist-gba audiopcmcoexist-elf
cd .. && scripts/audio/extract_audiopcmcoexist.sh
```

Measured on 2026-08-23 with TinyGo 0.41.1 (`-gc=none -opt=2`) and mGBA 0.10.3:

| Index | Workload | Max work | 30 Hz tick | Max service | Pages | Boundaries | Late | Max late | Fade left | Underruns |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 0 | Empty update/draw + blit | 558,127 | 99.3% | 367,999 | 3 | 34 | 10 | 74 samples | 482,560 samples | 0 |
| 1 | `cls` + HUD | 637,319 | 113.4% | 368,020 | 3 | 36 | 15 | 74 samples | 473,344 samples | 0 |
| 2 | 100 sprites | 778,572 | 138.6% | 251,700 | 2 | 48 | 10 | 74 samples | 461,056 samples | 0 |
| 3 | Sparse 30x20 mapdemo room | 559,020 | 99.5% | 479,778 | 4 | 34 | 7 | 74 samples | 452,352 samples | 0 |
| 4 | Dense 16x16 PICO room | 1,073,573 | 191.1% | 368,020 | 3 | 68 | 11 | 74 samples | 434,944 samples | 0 |
| 5 | Complete PICO-class tick | 725,708 | 129.2% | 249,619 | 2 | 45 | 15 | 74 samples | 423,424 samples | 0 |
| 6 | Complete GBA-class tick | 644,166 | 114.7% | 251,539 | 2 | 35 | 14 | 74 samples | 414,464 samples | 0 |
| 7 | Intentional 96 ms+ starvation | 1,693,570 | 301.5% | 0 | 0 | 9 | 2 | 74 samples | 413,184 samples | 5 |

The self-check finished with status `0xA0C00002`. All seven unforced cases had
zero underruns and stream switch failures. Every checkpoint retained four
music channels, the live fade, exactly one PCM voice, exactly one PCM start,
one cumulative setup rewind, and zero sample failures, invalid calls, command
fallbacks, or drops. Forced starvation emitted five silent misses, restored all
three queued pages, then completed 68 more handoffs with no new underrun.

This closes the native PCM cartridge-level mGBA continuity/recovery gate, but
not a broad full-profile production capacity gate. Empty and sparse-map cases
fit at 99.3% and 99.5%; the other five cases exceed one 30 Hz work budget. A
page-level music packing pass, long-fade state specialization, and block word
mixer reduced the empty case from the original 177.4% result without changing
target bytes. Active-only checkpoints remove unused twelve-slot copies, while
music-owned PCM and short fade tails retain the general path.

### Measured PCM concurrency boundary

The same twelve-slot cartridge layout was also measured with three and two
active fading synth channels plus the looping PCM voice. These targets and
extractors retain the complete matrix rather than substituting an isolated
mixer estimate:

```sh
cd runtime
make audiopcmthreecoexist-elf audiopcmtwocoexist-elf
cd ..
scripts/audio/extract_audiopcmthreecoexist.sh
scripts/audio/extract_audiopcmtwocoexist.sh
```

| Active workload | Empty | Clear/HUD | 100 sprites | Sparse map | Dense map | PICO tick | GBA tick | Cases within 30 Hz |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 4 synth + 1 PCM | 99.3% | 113.4% | 138.6% | 99.5% | 191.1% | 129.2% | 114.7% | 2/7 |
| 3 synth + 1 PCM | 69.3% | 86.6% | 127.5% | 71.2% | 176.3% | 100.8% | 87.8% | 4/7 |
| 2 synth + 1 PCM | 61.9% | 76.7% | 100.9% | 61.0% | 155.0% | 91.8% | 79.0% | 5/7 |

Every profile passed continuity, fade, sample-state, checkpoint, forced-
starvation, and recovery assertions with zero unforced underruns. Three synth
voices plus one PCM fit empty, clear/HUD, sparse-map, and complete GBA-class
scenes; their PICO-class case is 100.8%. Two synth voices plus one PCM also fit
the complete PICO-class scene with 8.2 percentage points of headroom. Neither
approves the 100-sprite or dense-map stresses, and the two-synth choice remains
provisional until the real-GBA listening gate passes.

## Generated SFX target smoke

`examples/audio-sfx` exercises authored JSON, host validation/compilation, Lua
lowering, fixed sequencer state, the mixer, and the interrupt-assisted stream
in one generated cartridge. All three shipped audio examples retain the normal
240x160 sprite sheet and 30x20 map used by editor-created projects, so the
complete cartridge and free-EWRAM totals include that graphics workspace; the
audio-record costs remain reported separately. Its retained TinyGo target map
reports:

| Resource | Cost |
| --- | ---: |
| Complete ROM | 76,872 bytes |
| EWRAM free after fixed heap reserve | 50,344 bytes |
| Four-page stream | 1,936 bytes EWRAM |
| Four-channel SFX/music player, command queue, effects, filters/histories, and four mixer checkpoints | 19,376 bytes EWRAM |
| Mixer | 1,328 bytes IWRAM |
| Seven SFX definitions | 56 bytes ROM |
| Twenty-five compiled steps | 200 bytes ROM |

The build passes TinyGo `-gc=none -opt=2` with no nonzero target allocation
reports. At the first generated-loop service point
in mGBA, `_init` had started SFX ID 0 and the prefilled stream contained
nonzero packed triangle PCM. Injecting one A-button sample produced one pending
command before the post-update service and `pending=0`, `starts=2`,
`rewinds=1`, `fallback=0` immediately after it. The rewind left exactly two
ready pages; the post-draw service restored three with zero underruns.

That injected-call inspection establishes the queue transition; the repeatable
current benchmark supplies the cycle gate. Its worst note-effect rewind is
155,654 cycles for eight commands ending with four fading channels. This is
59.4% of one 256-sample page and 50.0% of the 304-sample wrap guard. These are mGBA
execution-path and cycle results, not audible quality or real-hardware latency
sign-off.

`examples/audio-music` uses the same fixed runtime state. Its retained target
map adds three 12-byte music pattern records in ROM; the complete cartridge is
69,472 bytes and leaves 50,524 bytes EWRAM free. Pattern sequencing,
transitions, and fades are allocation-free. The earlier SFX effect/rewind
cycle figures and cases 32..33 are extracted from the current build. Four
music-owned channels cost 33,808 cycles at unity gain and 44,276 during a
one-second global fade. The music-active coexistence self-check passes without
unforced underruns; empty, sparse, and GBA-class cases fit one tick, while
clear/HUD and the three heavier graphics cases remain over budget.

`examples/audio-samples` selects the twelve-slot profile and exercises generated
PCM definitions plus real `sample()` lowering while a looping pattern occupies
two music-owned synth channels. It links as a 77,208-byte ROM, leaves 50,500
bytes EWRAM free, and reports 7,998 bytes of packed SFX/music/sample records and
deduplicated signed 8-bit PCM. This makes the shipped example exercise the
measured two-synth-plus-PCM safety boundary rather than PCM in isolation. The
full cartridge matrix above closes the emulator continuity gate; physical
hardware listening remains open for the native API path.

## Provisional stereo target baseline

`runtime/cmd/audiostereobench` isolates stereo mixer cost from the established
44-case mono/SFX ROM so linking provisional stereo code cannot perturb its
command-rewind layout. Build and extract its eight exact mGBA cases with:

```sh
scripts/audio/extract_audiostereobench.sh
```

Measured on 2026-08-23 with TinyGo 0.41.1 (`-gc=none -opt=2`) and mGBA
0.10.3. Each case emits 256 samples per channel; the mono control runs in the
same ROM and layout.

| Workload | Cycles/block | Cycles/sample pair | 16,384 Hz page budget |
| --- | ---: | ---: | ---: |
| Mono control, 4 centered synth voices | 80,712 | 315.3 | 30.8% |
| Stereo, 4 centered synth voices | 119,608 | 467.2 | 45.6% |
| Stereo, 4 distributed-panned synth voices | 131,384 | 513.2 | 50.1% |
| Stereo, 2 hard-panned fading synth voices | 78,168 | 305.3 | 29.8% |
| Stereo, 4 distributed-panned fading synth voices | 138,298 | 540.2 | 52.8% |
| Stereo, 8 distributed-panned synth voices | 251,574 | 982.7 | 96.0% |
| Stereo, 12 distributed-panned synth voices | 371,680 | 1,451.9 | 141.8% |
| Stereo, 4 centered synth + 1 hard-right PCM voice | 154,181 | 602.3 | 58.8% |

Four fixed-gain stereo synth voices and the measured 4+1 mixed case clear the
isolated page gate. The graphics profile also requires a live music fade: two
hard-panned fading voices cost 29.8%, while four distributed fading voices cost
52.8% before stream, DMA, or game work. Eight voices leave only 4.0% before
those costs, and twelve cannot run in real time. The target map retains the
existing 1,328-byte mixer state. Linked provisional stereo code contributes a
480-byte common path, 800-byte generic ramp path, 640-byte all-music ramp path,
428-byte source stepper, and 184-byte FIFO-word packer in IWRAM. Mono-only ROMs
dead-strip these paths, so they pay neither this code cost nor a second output
buffer.

These measurements approved the FIFO B/DMA2 stream prototype below and the
two-voice coexistence candidate. They do not enable stereo in projects:
real-hardware listening and user-facing policy remain open.

## Provisional stereo stream telemetry

`runtime/cmd/audiostereostream` drives the accepted two-hard-panned-voice mixer
through FIFO A/DMA1 left and FIFO B/DMA2 right. The fixed `StereoStream` owns
two synchronized page rings but one readiness mask, page cursor, Timer 1 IRQ,
and telemetry record. Build and inspect it repeatably with:

```sh
scripts/audio/extract_audiostereostream.sh
```

The TinyGo target map and 2026-08-23 mGBA 0.10.3 extraction report:

| Resource/result | Cost/value |
| --- | ---: |
| Stereo stream queue, two wrap guards, shared silence page, pointers, and telemetry | 3,264 bytes EWRAM |
| Increment over `MonoStream` | 1,328 bytes EWRAM |
| Mixer state | 1,328 bytes IWRAM |
| Paired block handoffs | 623 |
| Underruns | 0 |
| Queued/mixed page pairs at snapshot | 3 / 627 |
| Late Timer 1 entries / maximum lateness | 89 / 65 samples |
| DMA1/DMA2 control | `0xB640` / `0xB640` |
| Direct Sound routing after FIFO reset bits clear | `0x120C` (A left, B right) |

The 65-sample maximum remains inside the shared 304-sample guard. The extractor
now fails unless the stream is running with zero underruns and three queued
pairs. Identical
DMA controls plus one redirect offset establish synchronized scheduling in
mGBA; they do not establish channel separation or click-free output by ear.
The graphics coexistence and internal-capture gates are recorded below;
real-hardware listening remains.

## Provisional stereo graphics coexistence matrix

The `stereo` build of `runtime/cmd/audiocoexist` reuses the complete mono
graphics, starvation, fade, and recovery harness. It runs two music-owned
triangle voices at 16,384 Hz, hard-panned left and right, and caps each
normal-context service burst at two generated page pairs. Build and extract it
with:

```sh
cd runtime
make audiostereocoexist-elf
cd ..
scripts/audio/extract_audiostereocoexist.sh
```

Measured on 2026-08-23 with TinyGo 0.41.1 (`-gc=none -opt=2`) and mGBA
0.10.3:

| Index | Workload | Max work | 30 Hz tick | Max service | Pairs | Boundaries | Late | Max late | Fade left | Underruns |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 0 | Empty update/draw + blit | 447,624 | 79.7% | 243,017 | 2 | 34 | 8 | 60 samples | 482,560 samples | 0 |
| 1 | `cls` + HUD | 517,437 | 92.1% | 243,017 | 2 | 34 | 15 | 61 samples | 473,856 samples | 0 |
| 2 | 100 sprites | 774,604 | 137.9% | 243,017 | 2 | 50 | 9 | 65 samples | 461,056 samples | 0 |
| 3 | Sparse 30x20 mapdemo room | 548,875 | 97.7% | 243,017 | 2 | 34 | 7 | 65 samples | 452,352 samples | 0 |
| 4 | Dense 16x16 PICO room | 1,054,752 | 187.8% | 238,722 | 2 | 68 | 7 | 66 samples | 434,944 samples | 0 |
| 5 | Complete PICO-class tick | 720,707 | 128.3% | 230,790 | 2 | 46 | 14 | 73 samples | 423,168 samples | 0 |
| 6 | Complete GBA-class tick | 535,636 | 95.3% | 243,026 | 2 | 34 | 17 | 74 samples | 414,464 samples | 0 |
| 7 | Intentional 96 ms+ starvation | 1,731,796 | 308.3% | 0 | 0 | 9 | 1 | 74 samples | 413,184 samples | 5 |

The self-check status is `0xA0C00002`. All seven unforced cases preserve both
active channels, the live fade, one setup rewind, zero fallback commands, and
zero underruns. Intentional starvation produces exactly five silent paired
misses. Service restores three queued pairs; 68 subsequent handoffs add no
underruns. The largest two-pair service call is 243,026 cycles, or at most
121,513 cycles per generated pair when both are produced.

Empty, clear/HUD, sparse-map, and complete GBA-class scenes fit one 30 Hz work
budget. The 100-sprite, dense-map, and complete PICO-class stress scenes remain
at 137.9%, 187.8%, and 128.3%, matching the mono support boundary. A
four-distributed-voice fading profile was also attempted but produced
unforced underruns and exceeded the empty-scene tick budget; it is rejected
despite its 52.8% isolated mixer result. The two hard-panned voice profile is
the only provisional stereo graphics candidate.

## mGBA internal stereo capture

`runtime/cmd/audiostereocapture` runs a 512 Hz hard-left square and a 768 Hz
hard-right triangle through `StereoStream`, FIFO A/DMA1, and FIFO B/DMA2. The
target and `runtime/cmd/audiostereoreference` import one shared profile, so the
reference cannot silently drift from the ROM. The host renderer duplicates
each 16,384 Hz mixer sample at mGBA's 32,768 Hz sound-bias rate and applies the
emulator's exact Direct Sound gain.

`scripts/audio/capture_mgba_audio.sh` captures `mAVStream.postAudioFrame`
directly. It compiles the checked-in shim against mGBA 0.10.3 headers, copies
the installed executable to a temporary directory, ad-hoc signs that copy so
the capture library can load, and leaves the installed application untouched.
It never opens a microphone or operating-system audio device. Reproduce the
complete build, capture, and correlation gate with:

```sh
MGBA_SOURCE=/path/to/mgba-0.10.3 \
  scripts/audio/extract_audiostereocapture.sh
```

The checked-in
[`mgba-stereo-routing.wav`](../../captures/mgba-stereo-routing.wav) was measured on
2026-08-23. Two independent runs produced the same file:

| Result | Value |
| --- | ---: |
| Format | 65,368 stereo signed 16-bit frames at 32,768 Hz |
| SHA-256 | `a0ced77f9207289744d8ddce8b5c85e40c5fc76be8ebfef6a24a62f496320eb0` |
| Aligned stream start | 3,184 callback frames |
| Alignment score | 1.000000 |
| Recorded left vs left reference | 1.000000 |
| Recorded right vs right reference | 1.000000 |
| Recorded left vs right reference | 0.000000 |
| Recorded right vs left reference | 0.000000 |
| Recorded left vs recorded right | 0.000000 |
| Recorded/reference gain, left/right | 1.000000 / 1.000000 |
| Longest settled paired silence | 0 frames |

This closes mGBA channel separation and sample-sequence equivalence for the
provisional profile. It does not prove DAC behavior, analog noise, speaker or
headphone routing, or click-free output on a physical GBA.

## Host regression baseline

On an Apple M2 Pro, Go's 256-sample oscillator benchmark currently reports the
following medians. Stereo emits two 256-sample channels and applies per-voice
pan; these are regression timings, not ARM7TDMI projections.

| Voices | Mono time/block | Stereo time/block | Stereo/mono | Allocations |
| ---: | ---: | ---: | ---: | ---: |
| 4 | 3.17 us | 5.74 us | 1.81x | 0 |
| 8 | 5.68 us | 10.90 us | 1.92x | 0 |
| 12 | 8.32 us | 16.43 us | 1.97x | 0 |

Run it with:

```sh
cd runtime
go test ./audio -run '^$' -bench 'BenchmarkMix(Mono|Stereo)' -benchmem
```

The stereo path has zero Go allocations and preserves centered mono bytes, but
these host timings must not be used to advertise a GBA voice count. Exact
target, coexistence, and internal-capture results are above; real-hardware
listening remains required.
