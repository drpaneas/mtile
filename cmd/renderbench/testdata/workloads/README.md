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

The graphics-model characterization workloads lock down the explicit-background
contract and its renderer parity:

| Workload | Current contract under test |
| --- | --- |
| `layers4.lua` | Four explicit authored planes become four ordered hardware BGs. |
| `parallax_layers.lua` | Each map pass captures the camera active at its call site. |
| `map_arities_short.lua` | `map(bg, ...)` argument counts 1–4 preserve every omitted default. |
| `map_arities_long.lua` | `map(bg, ...)` argument counts 5–8 preserve the positional viewport/mask contract. |
| `map_mutation_isolation.lua` | `mset()` targets one logical map coordinate and only matching source regions are dirtied. |
| `object_order_current.lua` | Mode 0 objects follow every map and remain in front with OBJ priority 0. |

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
