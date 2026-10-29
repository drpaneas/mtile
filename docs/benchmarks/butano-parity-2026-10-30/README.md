# Can mTile reach Butano performance?

Measured with mTile `660c8d5` and the same pinned Butano revision as the
[original comparison](../butano-30hz/README.md). **The shipping TinyGo backend
can reach that Butano baseline on these physics workloads with suitable Lua
authoring and code placement.** When both engines receive IWRAM tuning, the
current Lua compiler still has a substantial gap. An isolated native-Go control
shows further backend headroom, but requires compiler work to obtain safely
from Lua. These conclusions concern the tested workloads, not every game.

No production compiler/runtime code changed during this investigation. The
original global-array source retains its previously published timings.

## Physics using existing mTile features

Cycles are whole measured update/draw/commit intervals, including the engines'
different framework costs. These are not isolated inner-kernel timings.

| Workload | mTile, original global-array Lua | Butano baseline | mTile, tuned Lua/project |
| --- | ---: | ---: | ---: |
| 32 entities | 116,528 | 11,896 | **10,239** |
| 64 entities | 232,304 | 17,758 | **17,747** |
| 128 entities | 463,856 | 28,985 | **22,540** |
| 64 entities with drag | 234,710 | 24,832 | **19,925** |

The 64-entity result is a practical tie: its 11-cycle difference is below the
original comparison's 489-cycle stopwatch-control threshold. The other tuned
Lua cases use 14%, 22% and 20% fewer cycles than the Butano baseline.

This requires changes to the game source and project settings:

1. Declare private, fixed numeric state arrays at chunk scope with `local`,
   enabling the existing compact numeric-array optimizer and borrowed views.
2. Move the update body to a normal `simulate` helper and select it with
   `"iwramFunctions": ["simulate"]` in `project.json`. Lifecycle callbacks
   remain in the generated shell. The runtime copies the helper to IWRAM.
3. For 128 entities, store the same x/y/vx/vy values in two contiguous blocks
   of 64 records. Each block contains 256 numbers, within the existing compact
   array limit. Each loop borrows one array rather than four independent views.

The force/gravity/clamp/drag equations, iteration order and checksums remain
the same. No trajectory or checksum is precomputed. Every recorded frame and
state checksum matches the independent original oracle. Lua and project files
for every variant are retained under [variants/](variants/).

At 128 entities, four separate numeric arrays plus IWRAM measure 32,787 cycles;
two split x/y arrays measure 30,288, two interleaved arrays 29,519, and the two
64-record blocks 22,540. This isolates a material layout/register-pressure
opportunity beyond binding declarations alone. The exact instruction-level
cause of every layout difference has not been separately proven.

Tuned Lua IWRAM usage is 13,004–13,856 bytes, including the runtime's reserved
stacks. The 128-entity block layout uses 13,856 bytes out of 32 KiB minus the
96-byte interrupt reserve. This is an explicit limited-memory tradeoff.

## Apply comparable placement tuning to Butano

The stock comparison uses Butano's normal `-O2` Thumb build. For a second
comparison, the same C++ physics loop was extracted into a noinline helper
with `BN_CODE_IWRAM`, retaining the original arrays and arithmetic. The ARM
variant additionally uses GCC's `target("arm")` function attribute. Timing
hooks, assets, warm-up and checksum verification remain unchanged.

| Workload | Butano Thumb helper in IWRAM | Butano ARM helper in IWRAM | Tuned mTile Lua | mTile / Butano ARM cost |
| --- | ---: | ---: | ---: | ---: |
| 32 entities | 9,649 | **8,343** | 10,239 | 1.23× |
| 64 entities | 12,785 | **10,071** | 17,747 | 1.76× |
| 128 entities | 18,956 | **13,527** | 22,540 | 1.67× |
| 64 entities with drag | 18,120 | **10,848** | 19,925 | 1.84× |

Thus matching the old baseline does not establish parity with an equally
tuned native implementation. Both sides sustain 29.864 Hz in every case.
The original Butano ROMs/results are preserved separately from these controls.

## Native-Go diagnostic: backend headroom

For 128 entities, a hand-edited Go kernel uses the same update equations and
four static `[128]Number` buffers, with code in IWRAM. Buffer bounds are known
to the compiler. Its checksum accumulates positive whole integers, equivalent
on this fixture's clamped ranges, before converting to the published Q16.16
value. The original heap arrays initialize the buffers once before warm-up.

| Diagnostic, 128 entities | Median | p95 | Maximum |
| --- | ---: | ---: | ---: |
| Native Go, buffers in EWRAM | 12,959 | 12,983 | 12,991 |
| Native Go, buffers in IWRAM | **7,839** | 7,863 | 7,871 |
| Butano ARM/IWRAM helper | 13,527 | 13,556 | 13,568 |

**These Go rows are not shipping mTile Lua results.** The kernel was edited
after transpilation and does not mirror its private state back into the old
Lua heap objects. It proves the constrained benchmark computation can run this
fast through the same TinyGo toolchain and mTile frame shell; it does not prove
general Lua table semantics or debugger visibility survive such lowering.
Before editing, rebuilding the original generated Go with the same flags was
required to produce a byte-identical ROM. Both edited controls then pass all
pixel/checksum checks, and ELF symbols confirm code/buffer placement.

This control changes bounds/type representation, aliasing opportunities,
checksum representation and optionally data placement together. Its gains
cannot be attributed solely to a single removed check.

## Scrolling: average cost improves, tail latency does not match

The original rapid-scrolling call draws a 31×21-cell rectangle whose source
origin changes every tick. An existing-API alternative draws a 64×32-cell
cached rectangle, moving its origin in 32-cell horizontal/8-cell vertical
steps. The visible viewport remains inside that rectangle on the entire route;
every captured image matches the original reference.

| Fast scrolling | Median | Mean | p95 | Maximum |
| --- | ---: | ---: | ---: | ---: |
| Butano baseline | 8,855 | 9,477 | **11,061** | **25,827** |
| Current mTile, original call | 64,272 | 64,771 | 88,901 | 117,310 |
| mTile, larger cached window | **1,446** | 12,086 | 132,324 | 158,775 |

Window reuse makes ordinary frames much cheaper and reduces mean cost 5.36×.
It concentrates work into less frequent, larger rebuilds: p95 and maximum
worsen. It therefore does not establish full scrolling parity with Butano.
Incremental row/column updates with exact finite-map and mutation semantics
remain the substantive engine opportunity.

## What is feasible to automate?

- Prove or guard fixed numeric global arrays without rebinding, alias escape,
  nonnumeric writes, observable identity changes or unsupported indices.
- Use bounded numeric loops and suitably guarded native storage/views, while
  preserving allocation-failure and unsupported-shape behavior.
- Select hot-code/data placement within the actual IWRAM budget, manually or
  with profiling. A blanket placement policy is unsuitable for larger games.
- Preserve overlapping map cells across camera shifts and update only newly
  exposed rows/columns, retaining finite edges, clipping, layers and edits.

There is no measured TinyGo barrier preventing parity on these computations.
Obtaining that performance automatically from the original generic Lua source
requires additional compiler/runtime engineering and semantic proof. Arbitrary
dynamic tables still require their general behavior.

## Evidence and reproduction

All runs use the pinned mGBA core, 60 warm-up ticks and two independent captures
of 600 ticks. Each case's repeated medians agree. There are **25,200 verified
ticks**: 12,000 Lua-authoring ticks, 9,600 Butano-placement ticks, 2,400 native-Go
diagnostic ticks and 1,200 ticks from a separate clean-directory reproduction
of the 128-entity block layout. Physical GBA confirmation is outstanding.

- [Physics comparison](physics-comparison.json), [Lua variants](mtile-authoring-results.json),
  [Butano controls](butano-tuned-results.json), [native diagnostics](native-control-results.json).
- [Verification](verification.json), [environment](environment.json), [required gates](checks.json),
  [clean reproduction](clean-reproduction.json).
- [Profiles](profiles/) attribute the separate numeric-view/placement costs;
  their useful/other instruction buckets are diagnostic, not release totals.
- [Raw evidence](evidence.tar.xz) contains captures, exact source/configuration,
  control scripts, generated Go/C++, headers and per-PC counters. ROM/ELF hashes identify the local binaries.
  [SHA-256 manifest](sha256.json) covers the saved records.

From the repository root, with the standard toolchain and headless player:

```sh
python3 scripts/butano-bench/parity.py --work /private/tmp/mtile-butano-parity
python3 scripts/butano-bench/parity.py --verify-only
```

`--only REGEX` selects a subset. The clean-directory check rebuilt
`physics_128-blocked_iwram` with this runner and reproduced the exact ROM hash
and both 22,540-cycle medians. The archived additional-control scripts retain
the host paths and commands used for the Butano/native-Go experiments.
