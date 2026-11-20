# Renderbench workloads

These Lua files replace `testdata/projects/testio/game.lua` through `renderbench`'s
`-source` flag. They deliberately reuse the same map, sheet, palette, toolchain,
input trace, warm-up, and capture count so a paired Mode 4/Mode 0 comparison
changes only the renderer.

`callsite_clone_baseline.lua` and `callsite_clone_split.lua` are paired inputs
for the opt-in `callsitecloningdiagnostic` test. They model a function with 90%
numeric calls and 10% dynamic calls, then manually split its body into numeric
and generic variants. This measures the best-case value and ROM cost of a
future compiler cloning pass without adding that pass first.

The 600-frame Mode 4 diagnostic measured:

| Call path | Mixed median | Split median | Residual change | ROM change | EWRAM | Pixels |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Shipping direct/no-alloc | 1,123,260 | 1,123,260 | 0.00% | 0.00% | unchanged | 0 |
| Forced root-protected | 1,346,457 | 1,312,963 | -3.78% | +1.31% | unchanged | 0 |

The forced path's total median improves by 2.49%, but the shipping path is
byte-for-byte and cycle-for-cycle unchanged because TinyGo already erases the
generic ABI after inlining. That makes automatic cloning unjustified for the
current workload; the diagnostic remains as a repeatable gate if real
allocating or non-inlined mixed-call functions become common later.

Re-run it after building the headless player:

```sh
go test -tags callsitecloningdiagnostic ./cmd/renderbench \
  -run TestCallSiteCloningDiagnostic -count=1 -v
```

`numeric_array_particles.lua` compares four ordinary 64-slot Lua tables with
the compact numeric-array lowering over 512 indexed particle-state operations
per tick. The 600-frame Mode 4 result was:

| Metric | Generic tables | Numeric arrays | Change |
| --- | ---: | ---: | ---: |
| Median cycles | 3,253,949 | 542,498 | -83.33% |
| Update/overhead residual | 2,937,019 | 254,996 | -91.32% |
| Lua heap allocated | 3,200 B | 1,040 B | -67.50% |
| ROM | 63,940 B | 39,364 B | -38.44% |
| Linker EWRAM | 197,036 B | 197,036 B | unchanged |
| Missed ticks | 620 | 0 | -620 |
| Differing pixels | 0 | 0 | exact |

Re-run the default-versus-rollback proof with:

```sh
go test -tags numericarraydiagnostic ./cmd/renderbench \
  -run TestNumericArrayDiagnostic -count=1 -v
```

`celeste_constant_collision.lua` uses the first Celeste room and performs four
collision rectangles per tick. The opt-in diagnostic builds the same source
with compile-time numeric constants enabled and disabled, in both renderers:

| Renderer | Rollback median | Constant median | Rollback ROM | Constant ROM | EWRAM | Pixels |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Mode 0 | 22,792 | 21,720 | 22,824 B | 22,544 B | +16 B free | 0 |
| Mode 4 | 662,765 | 662,021 | 22,540 B | 22,596 B | +16 B free | 0 |

Mode 4's debug ROM grows by 56 bytes while its total frame cost still falls by
744 cycles. Mode 0 falls by 1,072 cycles and 280 ROM bytes. Both paths preserve
their original missed-frame counts. Re-run the exact A/B with:

```sh
go test -tags numericconstantdiagnostic ./cmd/renderbench \
  -run TestNumericConstantDiagnostic -count=1 -v
```

The same Celeste collision fixture is the paired proof for immutable-sheet
runtime specialization. The source and generated Go are identical; only the
target runtime build tag differs. Mode 4 selects the compact read-only sheet
representation. Mode 0 retains its faster generic representation and removes
only the unreachable dirty-tile scan:

| Renderer | Generic median | Specialized median | Generic ROM | Specialized ROM | Pixels |
| --- | ---: | ---: | ---: | ---: | ---: |
| Mode 4 | 711,140 | 662,021 | 24,408 B | 22,596 B | 0 |
| Mode 0 | 47,668 | 21,720 | 23,040 B | 22,544 B | 0 |

Mode 4 wins by 6.91% total cycles and 49,123 draw cycles. Mode 0 wins by 54.43%
overall: commit falls from 41,425 to 15,451 cycles and cold initialization from
3,399,010 to 3,348,082 cycles, without changing EWRAM. The earlier, broader
Mode 0 read-only representation remains rejected because it regressed the draw
path; the accepted tag performs only the independently measured upload-scan
elision. Re-run it with:

```sh
go test -tags readonlysheetdiagnostic ./cmd/renderbench \
  -run TestReadOnlySheetDiagnostic -count=1 -v
```

The fixed-map-layer diagnostic then keeps the same compact Celeste map and
compares logical-layer lookup with compile-time dense storage slots:

| Renderer | Logical median | Fixed-slot median | Logical ROM | Fixed-slot ROM | Pixels |
| --- | ---: | ---: | ---: | ---: | ---: |
| Mode 0 | 22,248 | 21,720 | 22,584 B | 22,544 B | 0 |
| Mode 4 | 662,505 | 662,021 | 22,764 B | 22,596 B | 0 |

Draw, commit, EWRAM, and missed-frame counts are unchanged. Dynamic layers and
streaming maps do not enter this path. Re-run with:

```sh
go test -tags fixedmaplayerdiagnostic ./cmd/renderbench \
  -run TestFixedMapLayerDiagnostic -count=1 -v
```

Finally, the static-string-capacity diagnostic compares the default 256-entry
`Context` table with the exact compiler-selected tier. This workload has no
interned Lua strings, so both renderers select the one-entry sentinel and
return 4,080 EWRAM bytes. ROM size and total/draw/commit cycles are exactly
unchanged, and all frames are pixel-identical:

```sh
go test -tags staticstringdiagnostic ./cmd/renderbench \
  -run TestStaticStringCapacityDiagnostic -count=1 -v
```

`audio_control_heapless.lua` calls the Audio v2-compatible `sfx` and `music`
controls every tick without creating Lua tables or strings. The paired
diagnostic proves that these control calls no longer keep an otherwise
unreachable Lua heap alive:

| Renderer | Heap-linked median | Heapless median | Heap-linked ROM | Heapless ROM | EWRAM returned | Pixels |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Mode 0 | 2,912 | 2,883 | 34,476 B | 32,576 B | 143,064 B | 0 |
| Mode 4 | 149,032 | 148,952 | 33,988 B | 32,100 B | 143,072 B | 0 |

Both runs retained zero missed frames. Re-run with:

```sh
go test -tags audioheapdiagnostic ./cmd/renderbench \
  -run TestAudioControlHeapDiagnostic -count=1 -v
```

`static_print_heapless.lua` isolates two literal-string `print` calls. The
optimized path calls the literal-only `PrintStaticV`: the generic wrapper and
dynamic string decoder are required only when `print` may convert a non-string
through allocating `tostr`. The exact rollback-versus-specialized capture
measured:

| Renderer | Generic median | Literal median | Generic ROM | Literal ROM | EWRAM returned | Pixels |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Mode 0 | 41,084 | 36,568 | 19,596 B | 16,676 B | 143,068 B | 0 |
| Mode 4 | 164,215 | 159,746 | 26,136 B | 17,840 B | 143,068 B | 0 |

Both paths recorded zero missed frames. Re-run with:

```sh
go test -tags staticprintheapdiagnostic ./cmd/renderbench \
  -run TestStaticPrintHeapDiagnostic -count=1 -v
```

`table_noalloc_calls.lua` executes 128 `deli`/`count` helper-call pairs per
tick while an `add` in the caller deliberately keeps the ordinary Lua heap
reachable. This isolates effect analysis rather than heap removal: deletion,
counting, and iterator inspection cannot grow the table or invoke collection,
so their user functions may bypass GC-root wrappers.

| Renderer | Root-wrapped median | Direct median | Root-wrapped ROM | Direct ROM | Pixels |
| --- | ---: | ---: | ---: | ---: | ---: |
| Mode 0 | 530,084 | 472,461 | 27,948 B | 26,796 B | 0 |
| Mode 4 | 688,174 | 632,906 | 27,976 B | 26,688 B | 0 |

Mode 0 retained zero missed frames in both runs; the deliberately over-budget
Mode 4 fixture retained its characterization count. Re-run with:

```sh
go test -tags tableeffectdiagnostic ./cmd/renderbench \
  -run TestTableEffectDiagnostic -count=1 -v
```

Example:

```sh
go run ./cmd/renderbench \
  -project testdata/projects/testio \
  -source cmd/renderbench/testdata/workloads/mset16.lua \
  -player build/player/mtile-player \
  -out /tmp/mtile-mset16
```

`fset1.lua` is a characterization workload: it is exact and keeps ample frame
headroom, but it intentionally fails the conservative 20% median-speed AUTO
gate. That measured crossover is why `tile-v1` keeps runtime `fset` projects on
Mode 4 while still permitting a forced Mode 0 build.

`zero_bg_text.lua` is the fresh-project workload: `cls()` supplies only the
hardware backdrop and `print()` uses OBJ glyphs, with no artificial `map()` or
enabled regular BG.

`text_intern.lua` draws 32 copies of one literal glyph/color pair. The planner
must retain 32 OAM entries and 256 worst-line cycles while generated ROM and
VRAM contain one 64-byte text tile. Its paired capture is the steady-state
performance and pixel-parity proof for compile-time glyph interning.

`pack16.lua` draws eight 16x16 sprites from one immutable source rectangle and
covers all hardware flip combinations. It is the pixel-parity and steady-state
cycle fixture for the Mode 0 packed-OBJ cache.

`pack32.lua` draws two distinct 32x32 sprites, filling the cache's 32 cells,
and animates all four hardware flip combinations over 120 frames. It proves
that each draw lowers from sixteen 8x8 objects to one square OBJ while the
renderer proof retains its true 32-cycle scanline width.

`oam64.lua` draws 64 distinct 8x8 sprites in eight separated rows. It is the
pixel-parity and throughput fixture for the expanded 128-entry OAM allocator.

`scanline36.lua` overlaps 36 cached 32x32 OBJs on the same lines, consuming
1,152 of the 1,162 game cycles available after reserving 64 for the system
pause overlay. CI checks its pixels, memory, speed and Mode 0 frame delivery.
The earlier `scanline38.lua` fixture requires 1,216 cycles and now exercises
the compiler's fallback to Mode 4.

`dynamic_sprite_frames.lua` cycles sixteen draw slots through the explicit,
non-contiguous frame set `{1, 32, 35}`. The sprite ID variable is initialized
unconditionally and every later assignment names one immutable integer frame
constant. It is the pixel-parity and throughput fixture for bounded runtime
`spr(id, ...)` selection; no sprite ID is derived with arithmetic.

`inactive_sprite_slots.lua` alternates 64 one-cell draws between the bounded
set `{0, 1}`. ID 0 is the fully transparent inactive value and must consume no
runtime Mode 0 OAM entries; ID 1 restores all 64 visible objects. The compiler
still reports the conservative 64-entry/64-cycle worst case. Paired captures
must remain pixel-identical to Mode 4 in both halves of the animation.

`object_depth_lanes.lua` draws one object after each of four non-empty explicit
map passes. These positions become Mode 0 OBJ priorities 3, 2, 1, and 0 while
Mode 4 retains ordinary statement-order painting. The objects overlap real
BG0-BG3 pixels, so the paired capture proves all four occlusion lanes rather
than merely comparing non-overlapping draws.

The graphics-model characterization workloads lock down the explicit-background
contract and its renderer parity:

| Workload | Current contract under test |
| --- | --- |
| `layers4.lua` | Four explicit authored planes become four ordered hardware BGs. |
| `parallax_layers.lua` | Each map pass captures the camera active at its call site. |
| `map_arities_short.lua` | `map(bg, ...)` argument counts 1–4 preserve every omitted default. |
| `map_arities_long.lua` | `map(bg, ...)` argument counts 5–8 preserve the positional viewport/mask contract. |
| `map_mutation_isolation.lua` | `mset()` targets one logical map coordinate and only matching source regions are dirtied. |
| `layered_mget.lua` | Sixteen `mget(1,x,y)` reads per update exercise a platformer-style collision workload on BG1. |
| `layered_mset16.lua` | Sixteen literal-layer `mset(1,x,y,id)` writes exercise the widest layered mutation shape admitted by AUTO. |
| `layered_map_dynamic.lua` | Dynamic `mget(layer,...)` and `mset(layer,...)` remain exact in forced Mode 0; AUTO deliberately withholds dynamic-layer mutation pending a separate cost proof. |
| `object_order_current.lua` | Mode 0 objects follow every map and remain in front with OBJ priority 0. |
| `object_depth_lanes.lua` | Interleaved objects occupy the four exact BG-relative OBJ priority lanes. |

Their checked-in Lua source and eligibility unit test are permanent. The CI
renderer matrix additionally executes each workload through paired Mode 4 and
Mode 0 mGBA captures under the `parity` gate, so a pixel mismatch fails before
an API migration lands. The second renderer is captured at the exact runtime
frame numbers observed for the first; skipped publications fail explicitly,
and a mismatch writes both first-difference PNGs for diagnosis. Frame delivery,
memory, and cycle results remain in the report as characterization evidence but do not change the separately
evidence-backed AUTO profile. In particular, the current four-layer cold upload
records one Mode 0 startup missed frame; the stricter default `full` gate still
rejects it.

These multi-background workloads run over `examples/background-layers`, whose
versioned map contains real non-empty BG0–BG3 planes. This prevents a parity run
from accidentally proving only transparent output or relying on a legacy BG0
asset.
