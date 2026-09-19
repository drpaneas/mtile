# Discord findings: experiment plan and decision log

Completed 2026-09-19: all sixteen experiment areas have an emulator-backed
decision. Scope: all findings in the
[initial hardware review](discord-gbadev-hardware-notes-2026-09-19.md) and
[2020–2025 history review](discord-gbadev-history-2020-2025.md), with overlapping
findings combined below.

## Experiment loop

1. Preserve the current working tree, including existing user edits, as the
   baseline. Work on independent copies so candidates cannot contaminate one
   another's measurements.
2. State the hypothesis, accepted inputs, expected benefit, and failure cases.
3. Capture the baseline with the same compiler, emulator, assets, input trace,
   warm-up, and sample counts as the candidate.
4. Make the smallest executable prototype, or exercise the existing safeguard
   when the finding describes behavior already implemented.
5. Run focused correctness tests, native ARM/GBA measurements where applicable,
   and pixel/cadence/audio comparisons. Record cold-start and steady-state costs,
   ROM/RAM/VRAM changes, and worst-case behavior, not only an average speedup.
6. **KEEP** only when the measured advantage outweighs complexity, memory cost,
   and regressions. **DROP** a failed or unhelpful production change, retaining
   its diagnostic evidence. **RETAIN EXISTING** when the current implementation
   already passes the experiment. Scope is emulator execution/debugging; no
   physical-device pass is inferred.
7. After an accepted integration, rerun its focused proof and the required
   repository gates. Continue until every row has an evidence-backed disposition.

Correctness fixes may be worth keeping without a speedup: a demonstrated memory
corruption or lost audio tick outweighs a small quantified setup cost. For pure
performance changes, target at least 2% whole-workload improvement or a clear
resource/bottleneck reduction, with no unexplained p95/cadence regression.
Existing stricter AUTO gates remain authoritative. Never trade away exact Lua
semantics or pixel parity to make a benchmark pass.

No public API is added merely to demonstrate a hardware capability. Low-level
feature prototypes can be rejected after their capability, cost, and semantic
limitations have been measured. On 2026-09-19 the user explicitly selected
emulator execution and debugging as the final validation scope. Physical power/listening observations are
therefore excluded, and emulator results are not presented as physical proof.

## Recommended order and coverage

| ID | Finding / experiment | Required evidence | Status / decision |
|---|---|---|---|
| E01 | Unaligned fill/copy safety | Native offsets 0–3 with sentinel bytes; aligned hot-path cost; host parity | KEEP: safe fills/copies integrated; native and host guards pass |
| E02 | Long DMA versus audio service | Independent elapsed-time counter, varied DMA phase, lost/late service counts; chunked DMA / CPU-copy comparison | KEEP: 4,096-word DMA chunks; no lost overflows in phase probes |
| E03 | DMA startup, counts, access widths, BIOS copy contracts | Linked instruction spacing; DMA count boundaries; compatible aligned CpuFastSet copy cost | RETAIN count/width guards; DROP extra NOPs and CpuFastSet |
| E04 | Sleep during idle time | Baseline/candidate frames and cadence, IRQ/audio coexistence, actual halted execution; power claims kept separate | DROP: Mode 4 freeze despite reduced busy waiting |
| E05 | 4bpp and palette-zero semantics | Native 8bpp/4bpp pixel comparison, whole-OBJ banks, flips/remaps, cold/steady costs and tile bytes | DROP runtime 4bpp packing; RETAIN palette-zero guards |
| E06 | Shared animated BG tiles / metatiles | Same image from sprite overlays vs shared mutations; uploads, OAM, cycles; independent-phase counterexample | KEEP shared-animation example; DROP blanket metatile conversion |
| E07 | Resident tiles/HUD and VBlank admission | Existing dirty/no-dirty and first-scan workloads; compare avoidable reuploads; no partial-frame presentation | KEEP resident uploads and narrow resource guard; DROP staging prototype |
| E08 | Memory placement, stacks, waitstates | IWRAM/ROM or literal-load measurement, linked size/stack reservations; preserve standard EWRAM timing | RETAIN IWRAM/timing; correct WAITCNT comment |
| E09 | Exact arithmetic | Existing ARM edge/ABI tests and cycle benchmark; signed rounding and Lua modulo tests | RETAIN exact helpers; 3,012,900 cases pass |
| E10 | OAM count and per-line limits | Boundary fixtures, offscreen wrap tests, parity/cycle evidence; guard 128 entries and scanline budget | RETAIN guards; native 38-OBJ/1,216-cycle boundary passes |
| E11 | Compression size/granularity | Expanded-buffer sentinel proof; compressed vs raw bytes and decode cost; malformed/capacity rejection | DROP default compression; keep bounded research checker |
| E12 | OBJ blending and window limitations | Native overlapping OBJ/BG fixture versus software oracle; prove unsupported compositing; cost/benefit decision | DROP generalized OBJ alpha; keep tested blend/window limitation |
| E13 | HBlank effects / live table ownership | Coherent vs changed/rearmed table fixture, scanline offset and source rewind, native image/timing evidence | DROP production integration; keep tested double-buffer/rearm pattern |
| E14 | Simulation/display/audio clocks | Exact existing time contract, real frame-period calculation, long-run clock comparison; no silent API change | RETAIN nominal simulation clock |
| E15 | ARM constants / code-generation choices | Same operation and return/ABI with literal vs immediate sequences, ROM/IWRAM placement and cycles | RETAIN current LLVM sequence; native value/ABI/cycle proof passes |
| E16 | Emulator signoff and compatibility | Build/hash ROM bundle; emulator audio-case analysis, PSG/debugger checks and native regression | PASS emulator scope: rebuilt bundle, 42 audio cases and native debugging |

The startup/count/video-width items share E03; palette-zero shares E05; map
animation/metatiles share E06; ROM prefetch/IWRAM stacks/EWRAM compatibility
share E08. Each source finding is therefore represented, including items
already protected by current code.

## Baseline and isolation

- Starting source snapshot: `/private/tmp/mtile-discord-experiments-ofgo5qwq/baseline`.
- Initial status, binary diff, and per-file SHA-256 manifest are stored beside it.
- Toolchain found: TinyGo 0.42.0, LLVM 22.1.4, Go 1.27.1.
- Native emulator: existing pinned `build/player/mtile-player` and
  `build/player/mgba/libmgba-core.a`.
- Existing user modifications are baseline inputs, not changes to revert.
- Independent DMA, sleep, and 4bpp prototypes run in sibling source copies.
  Shared emulator binaries/libraries are read-only inputs.

## Required integration gates

```sh
go test ./internal/transpiler ./internal/build ./cmd/mtile
cd runtime && go test ./...
```

Also run the affected native diagnostics and renderer parity/benchmark cases.
Any public Lua API semantic change must satisfy the full renderer contract in
`AGENTS.md`. This work adds no Lua API and preserves the existing builtin
classifications. The narrowed resource eligibility has source-position tests,
updated restrictions, native rejection/fallback fixtures, and timing evidence.

## Results, in recommended order

All sixteen areas have an explicit decision. The required root and runtime
suites pass on the integrated code. A read-only review found a missing audio
coverage assertion in the new native AUTO test; it was corrected and the
native checks passed again with audible output and all four channels required.
Compact reports are in [benchmarks/discord-hardware](benchmarks/discord-hardware/).
Full ROMs, WAVs, images, traces, snapshots and rejected candidates remain at
`/private/tmp/mtile-discord-experiments-ofgo5qwq`.

### E01 — Keep safe unaligned fill/copy behavior

The old eight-word fill corrupted 33/34/35 bytes at destination offsets 1/2/3,
including guards outside the slice. The byte-store RAM fallback eliminates
that corruption. The final combined fixture costs 186 cycles versus 153 for
an unaligned fill; aligned eight-word calls measure 215 versus 202, with the
same aligned store-multiple body. Correct memory writes outweigh this small
setup/layout cost.

The DMA byte-slice helper now assembles CPU words for an unaligned source;
both DMA-copy APIs reject a misaligned destination before writing. Twenty
native source/destination alignment cases pass, versus eighteen failures
before those guards. The alignment addition costs 14 cycles for a generic
aligned eight-word copy and 40 for a generic 9,600-word call. Host helpers
match the target contract. See [alignment evidence](benchmarks/discord-hardware/copy-alignment.md)
and [final native regression](benchmarks/discord-hardware/final-dma.json).

### E02 — Keep bounded DMA stalls

The baseline 9,600-word DMA takes 76,929 cycles, longer than the audio timer's
65,536-cycle period. At phase 57,344 the independent cascade counter sees two
overflows while only one interrupt is delivered. Chunks of 4,096 words preserve
both interrupts across all sixteen phase/handler-body combinations tested.
The final combined copy takes 77,013 cycles, about 0.11% more, with exact bytes.

A separate 1,800-display-frame PSG workload improves from 7,533 to 7,689 serviced
ticks (+2.07%) and 295 to 304 events, with no frame skips, queue errors or
rejections and the same 9,590-cycle maximum IRQ. This also demonstrates why
its old self-test alone was insufficient: both builds said PASS despite the
baseline losing ticks. The independent overflow counter is the regression
check. See [DMA analysis](benchmarks/discord-hardware/dma.md).

### E03 — Retain existing contracts; drop speculative replacements

Negative, zero, oversized, small and chunk-boundary DMA counts preserve
sentinels. Zero never reaches the hardware count register. The transferred
video data uses word/halfword accesses. Extra startup NOPs show no correctness
benefit and add 24 cycles to the tested full copy, so they are dropped.

CpuFastSet reproduces the compatible aligned framebuffer bytes but costs
85,426 modeled cycles in the final fixture versus DMA's 77,013. It is dropped;
no general memcpy replacement or unaligned/tail support is claimed. These
BIOS timings use mGBA's HLE implementation. The stricter BIOS block contract
remains a constraint, not a new production path.

### E04 — Drop idle sleeping

The prototype raises halted-state sampling from 0% to about 66% in an idle
probe and 11.67% in the dense PSG probe. All 228 scanline-entry cases and 120
pacing cases pass. Pong Mode 4 and background layers Mode 0 also preserve
pixels/cadence over 180 matched ticks.

However, background layers Mode 4 stops publishing after its first frame,
including a 5,000-display-frame run. Debugging observes BIOS Halt with IME zero;
the exact root cause is not isolated. That failure outweighs the idle proxy,
so production polling remains. No battery saving is claimed. The rejected
patch, runner and [evidence](benchmarks/discord-hardware/idle-video/summary.json)
are retained.

### E05 — Drop runtime 4bpp packing

A restricted immutable low-index prototype preserves 120-frame native parity,
flips, transparent source zero and logical draw/display palette remaps. Tile
traffic halves from 32,704 to 16,352 bytes, but cold upload grows from 1,163,932
to 1,814,950 cycles (+55.9%). Steady work changes only 30,745→30,713 (-0.10%),
ROM grows 112 bytes and EWRAM is unchanged.

Drop that implementation. This does not rule out a future build-time packed
representation or palette-bank allocator; neither is silently admitted to
AUTO. Keep the existing palette-zero/opaque-color/exhaustion safeguards.
See [tile trials](benchmarks/discord-hardware/tiles.md).

### E06 — Keep shared animation; drop blanket metatile conversion

The [shared-tile example](../examples/shared-tile-animation/README.md) replaces
128 identical-phase sprite overlays with one shared mutable BG tile. Median
work falls 52,115→5,368 cycles (89.7%); OAM use falls 128→0. All 120 synchronized
frames match, including the first. Costs are 1,868 ROM bytes, 4,176 EWRAM bytes
and 231,591 cold cycles, amortized in roughly five updates. A required
independent-phase counterexample differs by 1,024 pixels per frame: separate
animation phases require separate tile IDs. No automatic rewrite is added.

The distinct 2×2 metatile decoder passes 100 repetitions of both repeated and
diverse maps, preserving all indices and canaries. Repeated payload shrinks
2,048→272 bytes, but diverse payload grows to 2,304. Expansion costs
6,042→27,530 cycles (4.56×) and still needs 2 KiB expanded RAM. Drop it as a
blanket format replacement; retain the reproducible storage experiment.

### E07 — Keep residency and a narrow commit resource guard

The same clean workload costs at most 2,410 commit cycles with resident maps
versus 35,470 when all four maps are needlessly reuploaded, with identical
pixels. Keep the existing dirty flags.

Four dirty maps, sixteen tile mutations, palette changes, 128 objects and
dense PSG exposed 19/57 unsafe warm commits, maximum 100,599 cycles, against
an 83,776-cycle VBlank. Moving palette calculations before admission lowers
the maximum to 93,211 but still fails 6/57 window checks and uses 512 extra
bytes. Drop that incomplete fix, even though its sampled pixels match.

Keep a narrow compiler resource guard: linked native audio plus four
materialized BGs cannot also use per-frame `sset`/`pal`. Forced Mode 0 rejects
that shape at the original source location; AUTO falls back to exact Mode 4.
Initialization-only mutations, unused audio assets and fewer drawn planes
remain supported. The four-dirty-map/audio control without these mutations
passes 120 first-scan frames, maximum 64,540 cycles.

The final native test checks both mutable and packed rejected fixtures:
AUTO matches explicit Mode 4 over 59 first/last captures each, with audible
output, all four PSG channels and no audio errors. Clean and dirty-map Mode 0
controls pass. See [final presentation checks](benchmarks/discord-hardware/final-present.log).

### E08 — Retain IWRAM placement and standard waitstates

Moving existing table helpers back to ROM worsens the numeric-hash workload.
Current IWRAM placement saves 18.62% in Mode 0 and 11.58% in Mode 4, with zero
differing pixels across 600 synchronized ticks each. It costs 744/760 IWRAM
bytes and 16 ROM bytes, with unchanged EWRAM and heap.

Linked fixtures reserve 1 KiB IRQ and 8 KiB user stacks; the largest inspected
IWRAM end is `0x030061a8`, below the BIOS-reserved area. This is a placement
check, not a dynamic maximum-stack proof. WAITCNT stays `0x4317`; its inaccurate
reset-default comment is corrected. No EWRAM overclock is introduced.
See [IWRAM measurements](benchmarks/discord-hardware/iwram.log) and
[linked layout](benchmarks/discord-hardware/memory-layout.json).

### E09 — Retain exact arithmetic

Existing division passes 3,012,900 edge/random/small-divisor/power-of-two/
fixed-point cases (6,025,800 ARM executions), preserving ABI and exact results,
with zero per-case cycle regressions against restoring division. Code size
remains 2,024 versus the old 532 bytes. Example unsigned `65536/60000` improves
369→13 cycles. Runtime tests preserve signed rounding and Lua modulo.
See [native division report](benchmarks/discord-hardware/division.log).

### E10 — Retain separate OAM and scanline limits

Eighty-two focused compiler/runtime/palette cases pass. The shared-animation
baseline exercises the exact 128-entry OAM boundary. The final native
`scanline38` fixture exercises 38 packed objects and 1,216 of the 1,226 allowed
scanline cycles: 120 frames match Mode 4, no Mode 0 missed frames, median/p95
34,090 cycles. Existing guards reject the over-budget case and prevent
coordinate wrapping before OAM masking. No redundant runtime budget check is
added. See [native boundary report](benchmarks/discord-hardware/final-scanline38.json).

### E11 — Drop default compression; keep the bounded research checker

Validated BIOS LZ77 reduces a redundant 16 KiB payload to 1,980 bytes, but costs
331,600 cycles versus a 47,291-cycle raw copy (7.0×). Incompressible input grows
to 18,436 bytes and costs 1,925,305 cycles (40.7×). Separate 256-byte frames
cost 7,953/30,471 cycles versus 898 raw; they reduce working storage at the
cost of weaker compression and per-frame overhead.

Whole/frame parity and canaries pass. Destinations sized from compressed
length, other undersized buffers, and seven malformed streams are rejected
before writes. Drop default integration because no measured current ROM
constraint outweighs these CPU/storage tradeoffs. Keep the bounded
[research checker](../scripts/compression-check/README.md). Payload totals
exclude decoder code and a possible frame-offset table.

### E12 — Retain tested blend/window limitations

Native positive control: a semitransparent red OBJ blends with a blue BG.
A blue OBJ underneath does not become its alpha target; with a green BG
underneath, the result blends red with green. WIN0 can mask that BG and leave
opaque red inside while the outside remains blended; it cannot turn the
underlying OBJ into a separate compositing layer.

Drop generalized hardware OBJ-alpha integration. The capability does not
preserve arbitrary software compositing semantics, and no Lua API is added.
These controls establish layer participation, using mGBA's output quantization.
See [native blend/window controls](benchmarks/discord-hardware/idle-video/video-controls.txt).

### E13 — Retain coherent HBlank technique as research

An immutable table rearmed in VBlank has zero bad pixels. Omitting source
rewind changes 38,160/38,400 pixels next frame. Overwriting the active source
produces a split red/blue frame. Updating the inactive table then swapping at
VBlank preserves both complete frames.

The probe schedules 160 halfword transfers (320 bytes), 2,400 DMA service
cycles, about 0.854% of a display frame, and 644 bytes for two 161-entry tables.
That cost excludes table generation and CPU setup. Drop production API
integration without a demonstrated scene benefit; retain the tested source
ownership/rearm pattern and reproducible native controls.

### E14 — Retain the simulation-clock contract

Thirty updates remain one `time()` second. At two LCD scans per update,
those sixty scans occupy approximately 1.004562 wall seconds; display rate
is 59.7275006 Hz and audio is independently hardware-timed. The nominal-time
test passes. Do not silently reinterpret simulation time as display or audio
time. See [clock evidence](benchmarks/discord-hardware/idle-video/summary.json).

### E15 — Retain LLVM's tested constant sequence

Materializing `0x040000de` through two ARM immediates takes 14 versus 19 cycles
in ROM and 5 versus 6 in IWRAM, with the same 12-byte routine footprint.
Sixteen thousand executions per path preserve the value, r1–r12, stack,
condition flags and ARM return state. The existing compiler already emits
that sequence for the tested DMA address, so no generic rewrite is warranted.
This one ARM case is not a blanket rule for arbitrary constants or Thumb code.
The [constant runner](../scripts/discord-hardware/constants/README.md) retains
the assembly, native checker and [measured report](benchmarks/discord-hardware/constants.json).

### E16 — Emulator validation complete

The final six-ROM PSG bundle was rebuilt from the accepted runtime and its
checksums/structure/allocation gates pass. All 42 reference audio cases pass
headless mGBA PCM analysis, including silence, stereo routing, trigger windows
and hardware length stops. Native CPU/DMA debugging, exact image captures,
PSG coexistence and the required Go suites provide the final acceptance proof.
The final bundled PSG ROM matches its debugger ELF byte for byte and reaches
PASS over 1,800 display frames: 7,689 service calls, 304 events, all four
channels, and zero frame skips/errors. See [final debugger telemetry](benchmarks/discord-hardware/final-coexist.json)
and the [validation record](benchmarks/discord-hardware/final-validation.json).
The bundle's physical-result template is intentionally unfilled because the
user selected emulator-only validation; it is not an outstanding task.

Final ROMs/WAVs are under `evidence/final-rom-bundle` and
`evidence/final-audio-reference` in the temporary experiment directory.
[Final audio report](benchmarks/discord-hardware/final-audio-reference.json).

## Re-running the experiments

Run from the repository root with the existing native player built. Use new
output directories; full artifacts are intentionally kept outside the source.

```sh
python3 scripts/dma-check/check.py --output-dir /tmp/mtile-dma-check
python3 scripts/division-check/check.py --output-dir /tmp/mtile-division-check
python3 scripts/compression-check/check.py --output-dir /tmp/mtile-lz77-check
python3 scripts/discord-hardware/metatiles.py --out /tmp/mtile-metatiles
python3 scripts/discord-hardware/constants/check.py --output-dir /tmp/mtile-constants
python3 scripts/discord-hardware/idle-video/run.py video --out /tmp/mtile-video
python3 scripts/discord-hardware/fixtures.py --out /tmp/mtile-tiles
MTILE_DISCORD_TRIAL_DIR=/tmp/mtile-tiles go test -tags=discordtrial ./cmd/renderbench -run TestDiscordSharedAnimationTrial -count=1 -v
```

The [tile README](../scripts/discord-hardware/README.md) describes the rejected
4bpp patch. The [idle README](../scripts/discord-hardware/idle-video/README.md)
describes the rejected sleep reproduction. Existing presentation diagnostics
accept `MTILE_MODE0_PRESENT_NATIVE=1`, `MTILE_MODE0_PRESENT_HOOK=none`, an output
directory, and a case filter; the final run uses `clean,dirty-maps,all-mutable-audio-dense,all-packed-audio-dense`.
IWRAM comparison uses `TestRuntimeTableIWRAMDiagnostic` with a private baseline
runtime where only `tableData`/`hashFind` section annotations are removed.

No rejected runtime candidate is active in the working tree. Existing user
edits were preserved. New retained behavior is limited to memory-safe hardware
helpers, bounded DMA stalls, the renderer resource guard, and the authored
shared-animation example; the WAITCNT edit corrects documentation only.
