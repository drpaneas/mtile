# Renderbench workloads

These Lua files replace `examples/testio/game.lua` through `renderbench`'s
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
| Shipping direct/no-alloc | 741,774 | 741,774 | 0.00% | 0.00% | unchanged | 0 |
| Forced root-protected | 968,454 | 875,721 | -13.60% | +0.43% | unchanged | 0 |

The forced path's total median improves by 9.58%, but the shipping path is
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
| Median cycles | 750,595 | 553,263 | -26.29% |
| Update/overhead residual | 424,398 | 259,724 | -38.80% |
| Lua heap allocated | 3,200 B | 1,040 B | -67.50% |
| ROM | 83,348 B | 72,484 B | -13.03% |
| Linker EWRAM | 201,864 B | 201,864 B | unchanged |
| Missed ticks | 620 | 0 | -620 |
| Differing pixels | 0 | 0 | exact |

Re-run the default-versus-rollback proof with:

```sh
go test -tags numericarraydiagnostic ./cmd/renderbench \
  -run TestNumericArrayDiagnostic -count=1 -v
```

Example:

```sh
go run ./cmd/renderbench \
  -project examples/testio \
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

`scanline38.lua` overlaps 38 cached 32x32 OBJs on the same lines, consuming
1,216 of the 1,226 available normal-OBJ rendering cycles. It is the accepted
boundary fixture paired with the unit-level 39-object/1,248-cycle rejection.

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
