# General mtile optimizations measured with Celeste Mode 0

The optimized game is `examples/celeste-mode0`, using ordinary public Mode 0.
The compiler and runtime changes below apply by types, dimensions and rendering
state, without checking game names, source identifiers or particular sprite IDs.
The original `examples/celeste` is separate. No new Lua builtin, renderer
classification, AUTO admission exception or experimental flag was added.

```sh
go run ./cmd/mtile edit examples/celeste-mode0
```

## What changed, and how

### Interned string equality

`runtime/value.go` compares two valid static strings by their interned references.
`InternStatic` already deduplicates contents and is the sole registry writer, so
equal contents have the same reference. This avoids repeatedly reading strings
byte by byte, notably when a helper dispatches on a string argument. Mixed
static/heap strings and invalid references keep the original content comparison.
There is no new allocation or registry.

An isolated native A/B changed only this function in the frozen baseline:
route median fell from 507,083 to 431,717 cycles, updates over budget from
54 to 30, and measured cadence from 22.93 to 25.80 updates/s. All 280 images
and gameplay snapshots stayed identical. Focused tests cover deduplication,
empty strings, embedded NUL, Unicode, invalid references, mixed heap strings,
hash-table keys and GC. Small static-string capacity configurations also pass.

### Finite tilemaps retained in VRAM

`runtime/tiled_map_full.go` makes full-rectangle caching available to ordinary
Mode 0, sharing the algorithm with the earlier private experiment. It uses the
existing screen shadow, with one boolean per hardware BG rather than another
map buffer. A stable source rectangle only changes BG scroll registers as the
camera moves. Source/size/mask changes still rebuild it.

GBA tilemap wrapping must not accidentally turn the finite Lua `map` call into
an infinitely repeated map. The exact bounds and automatic return to viewport
materialization are in [the renderer contract](render-modes.md#finite-map-caching-and-hardware-scrolling).
Mutation tests include cached off-screen cells, draw-order snapshots, flag
changes, sparse planes and streaming map storage. Independent pixel fixtures
decode the resulting hardware map and compare it with a finite-map painter.

The game's only parallax authoring change is to pass its existing full 60x20
cloud rectangle to `map`, instead of moving a 31x20 source window. Camera,
cloud speeds, wrapping and images stay the same. Across the 280-update trace,
BG/tile upload frames fell from **158 to 3**. This uses the four existing
hardware backgrounds; it introduces no hidden fifth surface or bitmap buffer.

### Typed drawing calls and smaller ARM call setup

`internal/transpiler/typed_draw_calls.go` lowers proven numeric arguments of
`spr`, `camera` and `map` to existing typed runtime functions. Previously the
generated caller wrapped numbers in full Lua `Value` records, passed those
wide records, and the callee unpacked them again. The new path converts the
same 16.16 numbers to integers directly. Explicit `nil` or unknown numeric
operands retain the old wrapper; flip operands retain Lua truthiness.

Argument evaluation, multiple results and GC roots are sequenced before this
choice. Tests compare pixels and behavior with the wrapper path, including
fractional coordinates, omitted/nil arguments, side effects and non-boolean
flips. The internal compiler rollback option is for testing, not a Lua API.

In the linked ARM image, 91 `SprV` call sites became `Spr` calls and four
`MapBGAtV` sites became `MapBGAt`. One representative sprite call setup shrank
from 26 to 17 instructions, and outgoing stack arguments from 64 to 4 bytes
after link-time constant propagation. These are call-site measurements, not a
claim that every sprite operation costs the same number of cycles.

### Single-cell OBJ bounds without division

`runtime/mode0_obj.go` validates an 8x8 sprite ID against the validated total
sheet cell count. The old bounds check calculated its sheet row using division,
even though a single cell cannot cross a row. Multi-cell sprites keep the full
row/column check. This removes a real `__aeabi_uidiv` call from the hot single-cell
path without changing the OAM construction or its limits.

Tests compare the old division-based validity rule and complete OAM attributes
across rectangular sheets, row boundaries, invalid IDs, camera offsets, flips,
transparent sprite zero and the OBJ tile limit.

### Numeric lengths and a loop-variable correctness fix

The existing length helper always returns a number, but `#ordinary_table` was
not consistently inferred as numeric. The compiler now preserves that result
type, enabling numeric loops and existing arithmetic optimizations. It still
evaluates the operand once and calls the same helper, including its existing
behavior for unsupported value kinds.

This exposed an older bug: the visible variable of a numeric `for` loop could
be forced to numeric storage even when the body assigned it a string or bool.
The compiler now keeps that binding as a Lua `Value` when it is reassigned,
while maintaining a separate numeric induction value. A lexical scan respects
local shadows and nested loop bindings. Tests verify values inside the loop,
iteration counts, mutable table lengths, operand side effects and GC behavior.

### Game authoring: compact panel data and row-by-row traversal

The UI's two 204-element tables and two 10-element digit lists are now
chunk-local numeric arrays. This uses the compiler's existing compact array
storage, not a new optimization keyed to their names. `tile_renderer.lua`
explicitly requires `ui_tiles` before its functions are declared, so the
dependency-first mtile bundle gives those functions the correct local bindings.
The generator produces the same local declarations; the asset JSON is unchanged.

Panel construction and restoration now loop over rows and columns directly,
advancing one flat index. This eliminates division/remainder by 17 for every
cell while preserving the order of all 204 reads/writes. Digit layout, counter
formatting, backup lifetime and final images are unchanged. Generated Go
contains two `NewNumberArray(204)` and two `NewNumberArray(10)` allocations and
uses `NumberArrayGet/Set` in place of generic table access. Host tests reproduce
the actual shared lexical chunk and verify preallocation, counters and seven
complete save/restore cycles.

This last authoring step reduced the panel-transition maximum from 1,169,109
to 826,902 cycles and saved 3,536 bytes of allocated Lua heap. The heap's reserved
capacity and the linker's free EWRAM remain unchanged.

## Comparable native measurements

The baseline is the completed tile conversion in
`build/experiments/celeste-mode0/final`, not the old bitmap or private hybrid.
These runs use the same 162-update route, input script, assets, gameplay and
diagnostic probes. Two GBA LCD scans allow **561,792 cycles per game update**.
Cadence is measured separately from elapsed LCD scans, not inferred from median
CPU time. The completion-panel transition is included in the route maximum.

| Metric | Tile baseline | Engine + stable maps | Final, with compact UI |
| --- | ---: | ---: | ---: |
| Median cycles/update | 507,083 | 357,182 | 358,573 |
| 95th percentile | 778,763 | 477,301 | 485,052 |
| Maximum route update | 1,735,538 | 1,169,109 | 826,902 |
| Updates over 561,792 cycles | 54/162 | 1/162 | 1/162 |
| Measured updates/s | 22.93 | 28.71 | 28.88 |
| BG/tile upload frames | 158/280 | 3/280 | 3/280 |
| Free EWRAM, instrumented | 61,456 bytes | 61,456 bytes | 61,456 bytes |
| Free EWRAM, playable | 61,584 bytes | 61,584 bytes | 61,584 bytes |
| Playable ROM | 476,556 bytes | 471,904 bytes | 452,200 bytes |

The final typical route update uses 29.3% fewer cycles, and the route maximum
is 52.4% lower. This combined comparison
includes the game's stable-map call change. Only the static-string A/B above
isolates one engine change; the other improvements interact after link-time
optimization, so their percentages must not be added together.

The remaining largest route spike is the first completion-panel construction,
with no GC that tick. Separate restart and respawn phases also contain
initialization costs: final restart maximum is 2,788,669 cycles and respawn
maximum is 686,155. These phases are not hidden inside a route average.
The results do not establish a constant 30 Hz
cadence for every gameplay state or for a denser, differently authored level.

A final intrusive snapshot profile locates the remaining panel cost: about
241k cycles in map writes and 216k in checked numeric-array access, versus 26k
in digit formatting and 3k for the four map submissions. Timer overhead raises
the measured draw from 745k to 835k, so these inclusive probes are explanatory,
not another release benchmark. The map cache is already inexpensive here.
Eliminating those checks or introducing bulk mutation would require separate
lifetime/bounds/draw-order proof; this change retains their existing semantics.

## Verification beyond this game

Three independent examples were also built in ordinary Mode 0 and Mode 4.
Each comparison captured 260 synchronized frames. A separate native replay
measured Mode 0 at 260 updates in 520 LCD scans, **29.86375 updates/s**, with
zero missed frames for each example.

| Example | Different pixels | Mode 0 median / p95 cycles |
| --- | ---: | ---: |
| `parallax-rectangles` | 0 | 52,591 / 272,434 |
| `coin-block` | 0 | 29,958 / 30,022 |
| `background-layers` | 0 | 34,701 / 80,111 |

These cover scrolling multi-layer maps, partial map calls, OBJ/text and stateful
`mset` animation. Their frozen snapshot includes string equality, typed drawing,
single-cell OBJ bounds and map caching; it predates the later length inference
change, which these examples do not need. Source hashes accompany the captures.
The requested **parity gates pass**. The separate full policy gate does not:
the Mode 4 controls miss frames, and the parallax examples' approximately 20 KiB
memory savings fall below that policy's 24 KiB threshold. No gate was weakened.

For Celeste, the final run passed:

- 280 exact Mode 0/Mode 4 RGB comparisons and first-scan comparisons;
- 280 exact gameplay snapshots against the previous port;
- 272 exact non-panel images against the previous hybrid; its eight panel
  images remain the intentional UI adaptation from the initial tile conversion;
- zero late VBlank commits, effect queue errors or observed tearing;
- public proof of 103 OAM entries / 824 worst-case scanline cycles, with at most
  43 actually active OBJs in the trace;
- the real playable ROM's boot and 160 LCD frames with input, active sound,
  no heap error and no audio overflow;
- 43 PICO-8 physics scenarios / 1,166 states and six lifecycle scenarios /
  600 states, using the pre-existing, unchanged numeric tolerances.

Required Go suites passed: `internal/transpiler`, `internal/build`, `cmd/mtile`
and the default runtime. Full four-BG/OBJ/text runtime tests, focused zero/one-BG,
resident/streaming, private-backend regressions and static-string capacity tiers
also passed. All GBA cycle and image evidence here is from native **mGBA**;
physical-console testing has not been performed.

## Evidence and reproduction

Artifacts are under `build/experiments/mode0-general-optimizations/`:

- `static-equality/`: isolated before/after ROMs, traces and inclusive hotspot
  profiling. Intrusive function timers are separate from the cadence results.
- `generic/`: independent example reports, ROMs, images, source snapshot and
  independently measured cadence.
- `combined-round1/`: string/cache/OBJ/typed-call combination before lengths.
- `combined-round2/`: complete engine changes, generated code and physics oracle.
- `final/`: complete engine changes plus compact UI, playable ROM and final proof.

```sh
MTILE_CELESTE_MODE0_NATIVE=1 go test ./cmd/renderbench \
  -tags=runtimeperfdiagnostic -run '^TestCelesteMode0NativeDiagnostic$' \
  -count=1 -timeout=15m -v
python3 examples/celeste-mode0/tests/verify_all.py --gba
go test ./internal/transpiler ./internal/build ./cmd/mtile
cd runtime && go test ./...
```

The native comparison needs the built player helper and the previous hybrid
trace; the physics oracle needs local PICO-8. Per-change tests are colocated
with the new runtime/compiler files. The game generators are authoring tools
and do not run on the GBA.
