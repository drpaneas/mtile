# Mode 0 / Mode 4 engineering plan

## Status

**Milestones 0–7 and the software portion of Milestone 8 are implemented.
Fresh projects request `auto`, while older projects without a renderer setting
retain Mode 4 as their compatibility default. Forced `mode0` is fail-closed;
`auto` uses the versioned `tile-v1` proof and falls back to Mode 4.
Physical-GBA validation remains an explicit release sign-off, not a hidden
claim made from emulator results.**

This document is the completed dual-renderer engineering plan. Future graphics
API, layer, animation, editor, and asset-model work is ordered separately in
the [graphics evolution roadmap](graphics-model.md#graphics-evolution-roadmap).

The final software matrix used 120 paired headless-mGBA ticks per workload and
had zero differing pixels and zero missed frames throughout. On
`examples/testio`, Mode 0 reclaimed 34,040 EWRAM bytes and reduced median work
from 544,417 to 38,409 cycles. Camera, bounded `sset`/`mset`, multi-BG, OBJ,
text, palette, clip, and `sspr` profiles passed the material-win gate. Runtime
`fset` was exact but saved only 15.67%, below the conservative 20% AUTO gate;
that measured crossover is encoded as a Mode 4 AUTO fallback. These results
authorize only the exact `tile-v1` subset, not a claim that Mode 0 always wins.

This plan turns the compatibility contract in
[`render-modes.md`](render-modes.md) into a measured engineering decision. It
does not assume that Mode 0 is always faster. A call can be semantically valid
in Mode 0 and still be less efficient for a particular workload.

## Product question

Can Mtile compile one Lua project to either:

- the existing Mode 4 software framebuffer renderer; or
- a Mode 0 tile/BG/OBJ renderer that preserves the same supported API
  semantics and materially reduces CPU or memory cost for tile-oriented games?

The answer is accepted only when two independent gates pass:

1. **Semantic gate:** the Mode 0 result is pixel- and state-equivalent for the
   subset accepted by the Mode 0 analyzer.
2. **Efficiency gate:** paired on-target measurements show where Mode 0 is
   actually better than Mode 4, including deferred VRAM/OAM upload cost rather
   than only timing individual Lua calls.

Compatibility is necessary but not sufficient for `auto` to choose Mode 0.

## Observable acceptance

A public dual-renderer release requires all of the following:

1. The same source, assets, input trace, palette, audio profile, TinyGo version,
   and optimization flags can be built twice with a forced renderer.
2. A forced Mode 0 build either preserves documented semantics or fails at
   build time with a source location and stable reason code. It never silently
   rounds, drops, recolors, reorders, or substitutes drawing operations.
3. Mode 0 and Mode 4 LCD captures are pixel-identical at every semantic test
   checkpoint accepted by the Mode 0 analyzer.
4. Return values and mutations from `sget`, `sset`, `fget`, `fset`, `mget`, and
   `mset` are identical in both builds.
5. Paired benchmark ROMs report total tick cost, draw/build cost, commit/upload
   cost, presentation cost, missed ticks, memory footprint, and cold-start cost.
6. Mode 4 behavior and performance remain within the regression limits below.
7. Each ROM links exactly one graphics backend. A Mode 0 ROM does not retain
   the 38,400-byte software framebuffer or Mode 4 page-copy path.
8. No renderer performs a post-initialization Go-heap allocation.
9. The editor and CLI report the requested renderer, selected renderer, and the
   semantic/performance reasons for that decision.

## Explicit non-goals

- No CPU-threshold mode switch during a running scene.
- No Mode 0 emulation of `pset`, `pget`, `line`, `rect`, `rectfill`, `circ`, or
  `circfill` by hiding a software framebuffer behind the tile renderer.
- No user-facing `auto` default until forced Mode 0 has passed semantic,
  benchmark, emulator, and physical-hardware gates.
- No promise that Mode 0 wins mutation-heavy, arbitrary-scaling, or primitive-
  heavy workloads.
- No dynamic renderer interface or per-draw backend branch in production ROMs.
- No expansion to affine scene systems, effects, or additional public APIs as
  part of the first proof.

## Current baseline and reusable seams

The current implementation already provides most measurement primitives:

- `internal/build/build.go` owns project loading, transpiler options, TinyGo
  invocation, linker-map memory accounting, and the build result.
- `internal/transpiler/transpiler.go` emits the hardware initialization and
  30 Hz update/draw/present loop.
- `internal/transpiler/render_modes.go` exhaustively classifies all public
  builtins.
- `runtime.Context` owns graphics, assets, input, heap, profiling, and audio
  state; its graphics state currently includes the 38,400-byte framebuffer.
- `runtime/mode4.go` owns hidden-page copy and page flip.
- `runtime/cmd/benchspike` already measures ARM7TDMI cycles with TM0/TM1.
- `runtime.PerformanceStats` already exposes frame, update, draw, GC, blit, and
  missed-frame counters.
- The mGBA helper already supplies live LCD frames and runtime telemetry to the
  editor, so it is the natural integration seam for deterministic captures.

`benchspike` remains the historical Mode 4 microbenchmark. The separate
`cmd/renderbench` tool builds the same project twice, drives the existing
headless mGBA helper, compares final LCD pixels, consumes runtime telemetry, and
writes ROMs, PNGs, and a JSON gate report without changing those historical
results.

## Proposed architecture

### One backend per ROM

Renderer selection is compile-time:

```text
project + Lua + assets
        |
renderer analysis
        |
mode4 | mode0 | auto decision
        |
transpiler option + TinyGo build tag
        |
one linked renderer
```

The build pipeline gains a typed renderer preference and reports both requested
and selected renderer. TinyGo build tags include only the chosen graphics state
and methods. Common input, fixed-point math, heap, asset metadata, flags, map
storage, audio, and profiling remain shared.

Do not store both renderers behind a Go interface or a `Context` mode field.
That would retain both memory layouts, add branches to hot calls, and make the
comparison dishonest.

### Backend-neutral frame lifecycle

Before Mode 0 work, extract the current generated loop into a small common
lifecycle without changing its order or timing:

1. begin tick and profile;
2. update;
3. begin render frame;
4. execute `_draw` calls;
5. prepare/stage renderer output;
6. collect if needed and pace;
7. commit/present during the safe display window.

Mode 4 maps this lifecycle to framebuffer draw, hidden-page DMA copy, palette
apply, and page flip. Mode 0 maps it to EWRAM shadow state, bounded tile/map/OAM
uploads, palette/window register commit, and BG/OAM activation. The paired
benchmark times the complete lifecycle, including work deferred to step 7.

The Mode 4 refactor lands independently and must preserve golden framebuffers,
generated-loop ordering, cycle cost, and linker memory before Mode 0 code is
introduced.

### Mode 0 graphics state

The first backend needs only state required by the accepted subset:

- explicit BG character-block and screen-block ownership;
- logical-to-resident tile mapping and dirty-tile tracking;
- one or more EWRAM screen-map shadows;
- camera/scroll state;
- BG and OBJ palette state;
- an OAM shadow and deterministic allocator when sprite support begins;
- window/clip allocation only when that milestone begins; and
- counters for uploaded tile bytes, map entries, OAM entries, and commit cycles.

The logical spritesheet, map, and flags remain authoritative. VRAM is a cache,
so `sset` and `mset` update logical data first and then invalidate the relevant
resident entries.

### Analyzer output

The analyzer produces a structured report, not only a boolean:

```text
requested: mode0
selected: mode0
reason: M0_FORCED_PROVEN
```

Diagnostics have stable codes, source positions, and plain-language fixes.
The first analyzer scans every bundled call site conservatively. It does not
need call-graph reachability to ship the proof. Unprovable dynamic behavior
falls back to Mode 4 under `auto` and fails closed under forced `mode0`.

## Making “Mode 0 conditional” executable

The documentation label is a concrete analyzer rule. The accepted form may be
widened only with new parity tests and paired measurements.

| Builtin | Implemented `tile-v1` proof | Widening condition | Rejection family |
| --- | --- | --- | --- |
| `cls` | Exactly one unconditional, frame-initial clear in `_draw`. | Any accepted sequence must preserve persistent-image and call-order behavior. | `M0_CLS_NOT_FRAME_INITIAL` |
| `map` | One to four direct maps after `cls`; static viewport up to 64x32; dynamic origins/masks/camera; streamed maps and mutations. | New layouts must preserve screen-block allocation, layer order, flags, streaming, and bounded uploads. | `M0_MAP_SHAPE`, `M0_RESOURCE_LIMIT` |
| `spr` | Literal source or scalar initialized unconditionally and assigned only explicit integer literals/immutable integer constants; static size, dynamic position/flips, after `cls` and at least one materialized BG when BGs exist, within the post-packing 128-OAM/1,226-cycle scanline proof. Interleaving with later explicit maps derives Attribute-2 depth; a fully default-transparent one-cell ID 0 is skipped at runtime but remains conservatively charged. Literal Y intervals compose exactly; dynamic Y overlaps conservatively. | Table/field IDs, arithmetic ranges, dynamic size, interprocedural value inference, or a behind-all-BG lane need separate/external handling. | `M0_SPR_SHAPE`, `M0_OBJ_BUDGET` |
| `sspr` | Static aligned source rectangle, 1:1 destination size, dynamic position/flips, with the same order-derived object-depth lanes as `spr`. | Cropping/scaling needs a pixel-identical hardware transform and bounded resources. | `M0_SPR_SHAPE` |
| `clip` | Full-screen/reset form only. | Each changing clip group maps to available hardware windows without changing order. | `M0_WINDOW_BUDGET_UNPROVEN` |
| `pal` | Reset/layer reset and draw/display remaps outside `_draw`. | Mid-draw changes need conflict-free assets while preserving call order. | `M0_DRAW_SHAPE`, `M0_UNSUPPORTED_BUILTIN` |
| `palt` | Reset and default-transparency no-op forms. | Nonzero transparent keys need conflict-free converted assets and opaque-use proof. | `M0_UNSUPPORTED_BUILTIN` |
| `print` | Explicit static printable text/color, dynamic position, after maps, within 32 reserved glyph tiles and the shared 128-OAM/1,226-cycle proof. | Dynamic/control-code text needs a bounded glyph/layout plan. | `M0_PRINT_SHAPE`, `M0_OBJ_BUDGET` |

The shared `sset` API has an additional scheduling proof: calls must be direct
statements in `_init` or `_update`; initialization commits while force-blanked,
and visible updates are capped at 16 calls per tick so tile DMA stays bounded.
AUTO additionally permits at most 16 direct `mset` calls per update and only
init-time `fset`; forced Mode 0 retains exact runtime `fset` semantics.

Calls classified Both have shared logical semantics and Mode 0 invalidation or
upload paths. Unsupported conditional forms still fail with an explicit
diagnostic rather than pretending the backend is complete.

## Semantic verification

### 1. Analyzer and build tests

- Every builtin remains exhaustively classified.
- Table-driven tests cover every accepted and rejected conditional shape.
- Every rejection checks reason code, file, line, and column.
- `auto` falls back to Mode 4 when compatibility or cost is unproven.
- Forced `mode0` fails before invoking TinyGo when the program is incompatible.
- Forced `mode4` bypasses Mode 0 eligibility without changing source behavior.
- Build results and editor build events report the selected backend truthfully.
- Linker-map tests prove only one backend and one graphics-state layout linked.

### 2. Host runtime tests

- Mode 4 golden framebuffer tests remain unchanged.
- Mode 0 tests validate BG map shadows, tile residency, dirty uploads, scroll,
  palette entries, OAM ordering, window state, and mutation invalidation.
- Asset calls have a shared behavior suite run against both build tags.
- Property tests cover map edges, negative camera positions, streamed-map
  boundaries, tile eviction, and dirty-cache wraparound.
- Zero-allocation tests cover init-complete draw and commit paths.

Host shadow-state tests are necessary but not sufficient: they cannot prove
the GBA PPU's final compositing.

### 3. mGBA pixel-parity tests

Build paired ROMs from the same fixture and replay the same deterministic input
trace. Capture final LCD output, not internal palette indices, at named frames.
Compare all 240x160 pixels exactly after conversion to the same BGR555/RGBA
representation.

The parity matrix grows by milestone:

- static 30x20 and 60x20 maps;
- sparse, opaque, and color-0-transparent tiles;
- camera positions at 0, 1, 7, 8, map edges, and negative coordinates;
- flags and map-layer filtering;
- `mset` and `sset` before draw and during repeated frames;
- palette changes and transparency resets;
- overlapping BG layers and later BG/OBJ priority;
- distributed sprites and scanline-pressure rejection;
- text, clipping, flips, and each accepted `sspr` transform;
- unchanged input, timing, `stat`, GC, and four-channel PSG behavior.

Every accepted Mode 0 fixture must have zero differing pixels at every capture.
A visually similar result is a failure.

### 4. Physical-GBA sign-off

mGBA is the automated gate. Before public release, run a self-scoring cartridge
on physical GBA hardware for:

- tile/map uploads during scrolling and mutations;
- no tearing or partially updated screen blocks;
- worst accepted OAM/scanline load;
- palette/window changes;
- four-channel PSG coexistence; and
- a sustained 10-minute input/scroll/mutation soak with zero missed ticks.

## Paired benchmark design

### Method

`renderbench` builds every compatible scenario twice, forced Mode 4 and forced
Mode 0, with identical:

- generated Lua and assets;
- input/event trace;
- `-gc=none -opt=2` TinyGo flags and target JSON;
- tick rate and audio profile;
- warm-up and sample counts; and
- compiler, emulator, and runtime revisions recorded in the result.

Microbenchmarks run 64 isolated repetitions from a consistent display phase.
Scenario benchmarks warm for 120 ticks and record 900 ticks. Report minimum,
median, p95, p99, and maximum rather than a single favorable sample.

Use a benchmark-only EWRAM result buffer or fixed histogram. Do not enlarge
production `Context` merely to collect benchmark distributions.

Host `go test -bench` remains useful for allocations and relative regressions,
but renderer decisions use ARM7TDMI cycle measurements from mGBA and are
confirmed on physical hardware.

### Metrics

| Metric | Why it matters |
| --- | --- |
| Total work cycles per 30 Hz tick | Primary efficiency result; includes every renderer cost before pacing. |
| Update cycles | Control value; should be equal for paired programs. |
| Draw/build cycles | Mode 4 raster work versus Mode 0 shadow/resource construction. |
| Commit/upload cycles | Captures deferred Mode 0 VRAM/OAM work and Mode 4 framebuffer DMA. |
| Present-window cycles and scanlines | Detects VBlank pressure and tearing risk. |
| Missed ticks | Performance correctness, not merely average speed. |
| Uploaded tile/map/OAM bytes | Explains Mode 0 spikes and mutation crossover points. |
| EWRAM/IWRAM/VRAM/ROM bytes | Prevents trading CPU gains for an invalid or bloated layout. |
| Init/scene-load cycles | Separates cold upload cost from steady-state rendering. |
| GC count/max cycles | Confirms renderer choice does not hide allocation or heap regressions. |
| PSG service max/mean cycles | Confirms graphics do not starve the audio scheduler. |

Compare total work, not `map()` alone. A Mode 0 `map()` that appears cheap but
defers a large upload into VBlank has not won until the upload is included.

### Workload matrix

| Workload | Variants | Question answered |
| --- | --- | --- |
| Empty tick | no draw calls | Fixed backend/lifecycle overhead. |
| Clear + static map | 16x16 and 30x20; sparse, opaque, transparent, dense | Core tile-scene crossover against Mode 4 raster + blit. |
| Large map camera | 60x20 and streamed map; 1 px/tick; 7→8 tile boundary | Steady scroll cost and worst cache/screen-block update spike. |
| Map mutation | 1, 16, 64 `mset` cells/tick | Point where Mode 0 dirty uploads become slower than Mode 4. |
| Tile mutation | 1, 4, 16 dirty tiles/tick via `sset` | Tile upload and invalidation crossover. |
| Map layers | 1, 2, maximum accepted layers; flag masks | BG allocation and layer-composition cost. |
| Sprites | 1, 32, 64, 100 distributed; bounded ID-0 inactive slots; four interleaved BG depth lanes; separate scanline-pressure case | OAM construction cost, transparent-slot fast path, exact painter-order depth, and semantic resource rejection. |
| Text | 10 and 40 characters; static and changing | Tile-font/OBJ update crossover. |
| Palette/clip | static and per-frame accepted changes | Register/upload pressure and semantic parity. |
| `examples/testio` | exact project assets and Lua | First real-project proof. |
| GBA platformer tick | map + sprites + HUD, no Mode 4-only primitives | End-to-end game-shaped comparison. |
| PSG coexistence | platformer tick + all four channels active | Shared timing and interrupt headroom. |
| Mode 4 controls | `pset`, primitives, arbitrary `sspr` | Proves analyzer selects Mode 4; not a false paired comparison. |

Each mutation/scroll workload reports both steady frames and boundary/dirty
spikes. Averaging those together would hide the exact frames most likely to
miss the presentation window.

### Result format

Check in machine-readable results under `docs/benchmarks/render-modes/` plus a
short generated Markdown comparison. Each record includes:

- source and asset hash;
- Git revision and dirty-state flag;
- TinyGo and mGBA/hardware identity;
- renderer and analyzer reason codes;
- workload parameters and samples;
- cycle distribution and missed ticks;
- memory/linker-map totals; and
- ROM hash.

Never copy values manually from the editor CPU display into the decision table.

## Efficiency and go/no-go gates

### Mode 4 preservation gate

The neutral lifecycle refactor and all later dual-renderer work must keep:

- existing Mode 4 golden output identical;
- Mode 4 paired benchmark median and p95 within 2% of the frozen baseline;
- the existing ~77k-cycle framebuffer DMA result within measurement noise;
- no additional EWRAM graphics state; and
- zero new missed ticks or allocations.

### Map-only proof gate

Proceed beyond the first Mode 0 slice only if `examples/testio` and the static
map scenarios have:

- zero pixel differences;
- zero missed ticks after warm-up;
- at least 20% lower median total work than forced Mode 4;
- at least 10% of one 30 Hz tick (56,179 cycles) additional p95 headroom;
- at least 24 KiB net EWRAM savings versus Mode 4; and
- bounded, tear-free cold upload, reported separately from steady state.

These are minimum evidence thresholds, not promises for every compatible game.

The first `examples/testio` run passed all minimum thresholds:

| Metric | Mode 4 | Mode 0 | Gate result |
| --- | ---: | ---: | --- |
| Paired LCD frames | 120 | 120 | 0 differing pixels |
| Median total cycles | 544,417 | 38,409 | 92.94% lower |
| p95 total cycles | 544,417 | 38,409 | 523,383 cycles headroom |
| Median draw cycles | 465,759 | 576 | informational |
| Median commit cycles | 76,924 | 36,341 | post-pace Mode 0 upload is included in total cost |
| Cold tile + map upload | n/a | 1,967,744 | ~117 ms while display is force-blanked |
| EWRAM free | 60,600 | 94,640 | +34,040 bytes |
| Missed frames | 0 | 0 | pass |

Reproduce the paired emulator gate from the repository root:

```sh
go run ./cmd/renderbench \
  -project examples/testio \
  -player build/player/mtile-player
```

The command prints the temporary artifact directory containing `report.json`,
both ROMs, and both final-frame PNGs. Physical-GBA sign-off remains open; the
measured 1,967,744-cycle cold upload is reported separately from steady state and
runs while the LCD is force-blanked.

### Per-workload renderer decision

Mode 0 does not need to win every compatible workload. The benchmark matrix
defines crossover rules:

- if compatibility fails, select Mode 4;
- if Mode 0 is compatible and its workload profile has a measured material win,
  `auto` may select Mode 0;
- if measured results are within 5%, prefer Mode 4 until Mode 0 has another
  concrete benefit such as at least 24 KiB EWRAM savings;
- if static analysis cannot bound mutation, OAM, palette, layer, or upload cost,
  `auto` selects Mode 4;
- forced `mode0` may choose a semantically safe but slower backend, but the build
  report warns using benchmark-backed reason codes.

The first `auto` implementation is the small versioned `tile-v1` rule table
derived from the checked-in [benchmark profiles](benchmarks/render-modes/README.md),
not a speculative cycle simulator.

### Stop conditions

Stop the Mode 0 expansion and keep Mode 4 as the sole production backend if:

- the map-only slice cannot achieve pixel parity without retaining a software
  framebuffer;
- the map-only proof misses either CPU or EWRAM evidence thresholds;
- VBlank uploads cannot be bounded without visible tearing;
- supporting the accepted subset requires both renderers in one ROM;
- Mode 4 regresses beyond its preservation gate; or
- mGBA and physical-hardware results disagree materially on correctness or on
  which renderer wins the target workload.

Do not continue to sprites, text, palette variants, or `sspr` merely because
the initial infrastructure has already been written.

## Milestones

### 0. Freeze the Mode 4 baseline — complete for `testio`

Deliverables:

- pin toolchain/emulator metadata;
- capture current Mode 4 microbenchmarks, game-shaped scenarios, linker memory,
  `examples/testio` frames, and PSG coexistence;
- define the checked-in result schema.

Exit: results are reproducible and the same ROM hash gives stable cycle ranges.

### 1. Extract the renderer lifecycle — complete

Deliverables:

- backend-neutral generated-loop calls;
- Mode 4 adapter preserving current order;
- no Mode 0 implementation yet.

Exit: all Mode 4 semantic, allocation, memory, and 2% performance gates pass.

### 2. Add analysis and forced experimental selection — complete

Deliverables:

- typed `mode4`, experimental `mode0`, and internal `auto` preference;
- structured semantic/cost report and diagnostics;
- TinyGo tag selection through build service and transpiler options;
- CLI-only experimental forcing; editor remains Mode 4.

Exit: incompatible projects fail before TinyGo; default builds are unchanged.

### 3. Build the map-only vertical slice — first stop point complete

Deliverables:

- one 8bpp BG map, frame-initial `cls`, and default transparency;
- exact `examples/testio` Mode 0 ROM;
- Mode 0 linker/VRAM allocation validation;
- paired map benchmarks and LCD parity captures.

Exit: the `examples/testio` gate passed. Camera, streaming, mutations, sprites,
and text remain deferred; do not infer them from this result.

### 4. Map streaming and mutations — complete

Deliverables:

- large-map window streaming;
- `mget`/`mset`, `sget`/`sset`, `fget`/`fset` parity;
- dirty tile/map upload counters and crossover benchmarks;
- conservative AUTO rules for dynamic writes.

Exit: boundary frames meet p99/tick and parity gates.

### 5. Multiple BG layers — complete

Deliverables:

- map flag/layer planner;
- priority/order rules and screen-block allocation diagnostics;
- paired layer benchmarks.

Exit: every accepted allocation is exact; uncertain plans fall back to Mode 4.

### 6. OBJ sprites, then text — complete

Deliverables:

- deterministic OAM/metasprite allocator and rejection accounting;
- `spr` parity and distributed/scanline benchmark matrix;
- `print` only after sprite/OAM ownership is stable.

Exit: no accepted scene can lose an object or glyph at runtime.

### 7. Remaining conditional APIs — complete for `tile-v1`

Implement and widen one independently gated feature at a time: `clip`, `pal`,
`palt`, then the exact supported `sspr` transforms. Each addition requires new
analyzer rules, parity fixtures, resource-pressure tests, and paired benchmarks.

Exit: the public Mode 0 subset matches `render-modes.md` and all unsupported
forms have actionable diagnostics.

### 8. Public renderer selection — software complete; hardware sign-off open

Deliverables:

- documented CLI and project preference;
- editor build report showing requested/selected backend and reasons;
- versioned AUTO cost profile (`tile-v1`);
- full build-tag/TinyGo/mGBA CI matrix;
- physical-GBA sign-off (open external release gate).

Exit: forced Mode 4 remains available permanently. Fresh projects default to
`auto` based on the checked-in evidence; changing the legacy missing-field
fallback from Mode 4 remains a separate compatibility decision.

## Historical first stop point

The first implementation effort ends after Milestone 3. The complete narrow
outcome is: **the exact `examples/testio` project builds in both modes, produces
identical captured frames, and has a trustworthy paired CPU/memory report.**

That evidence authorized the completed later milestones above. It is retained
here to show the decision boundary that prevented speculative overbuilding.
