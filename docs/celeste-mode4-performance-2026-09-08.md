# Celeste Mode 4 performance, September 8, 2026

The original Celeste route uses **56.49% fewer median frame cycles** after this
pass: **1,274,373.5 → 554,455.5 cycles**, fitting the 561,792-cycle budget at the
median. It is **not locked to 30 fps**: 64 of the 162 action ticks still exceed
that budget, with a p95 of 645,332 cycles. The longer 593-tick replay retains
identical pixels and watched game state, including four deaths and respawns.

These changes optimize Mtile's compiler and runtime. They require no game-name
checks, tile IDs, project configuration changes, or edits to Celeste's Lua/assets.
Rebuilding a game automatically uses the applicable optimizations. Numeric
inference and iteration improvements apply to both renderers; software tile
blitting and map caching apply to Mode 4.

The baseline is the reviewed working tree after the separate
[division optimization pass](division-performance-2026-09-08.md). This comparison
uses the original `examples/celeste`, whose AUTO policy selects Mode 4. It does
not use the separate `research/celeste-mode0` port. Measurements are GBA CPU
cycles from TinyGo ROMs executed by the pinned mGBA core; they are not host
Activity Monitor CPU percentages. The 30 Hz work budget is 561,792 cycles.

## Measured results

[Raw paired cycles, source/tool fingerprints and hash witnesses](benchmarks/celeste-mode4/2026-09-08.json)
retain the complete measurement. Both sides use identical instrumented Lua and
assets with inputs supplied once per logical update; probe/profiling overhead
is included. Percentiles use nearest rank. The first 162 action ticks follow
the existing completion route. Start restarts the game, then 400 further updates
exercise movement, jumping, dashing, four deaths and respawns. No heap errors or
audio overflow occurred, and both ROMs produced audible samples.

| Metric | Before | Optimized |
| --- | ---: | ---: |
| Action median frame cycles | 1,274,373.5 | 554,455.5 |
| Action p95 frame cycles | 1,446,798 | 645,332 |
| Action maximum frame cycles | 1,482,053 | 673,655 |
| Action ticks above budget | 148 / 162 | 64 / 162 |
| All 593 ticks: median cycles | 1,238,192 | 513,561 |
| All ticks above budget | 575 / 593 | 83 / 593 |
| All ticks: total cycles | 719,603,711 | 301,041,873 |
| Debug replay ROM bytes | 393,816 | 378,248 |
| IWRAM used / 32,672 bytes | 29,592 | 30,572 |
| EWRAM free bytes | 53,608 | 26,016 |
| First published tick, LCD frame | 30 | 118 |

Total work across the extended replay falls by 58.17%. Cache preparation adds
startup work: the first published tick moves from roughly 0.5 to 2.0 seconds.
It runs before gameplay and is never repeated by an ordinary draw call. The
optimized debug cart retains 2,100 bytes of IWRAM headroom. Uniform span storage
recovers 15,104 bytes of EWRAM compared with the intermediate copied-pixel cache.

The uninstrumented release ROM is 361,648 bytes with 26,128 bytes of free EWRAM.
A separate 600-LCD-frame boot/input check produced 244 distinct images and 481
audible frames without audio overflow. The original project files are unchanged.

The cost gate is a heuristic. In the independent fragmented map fixture,
caching was within 1% of uncached median time (465,172 versus 460,647 cycles),
with virtually identical p95 (777,487 versus 777,507). Earlier unconditional
caching and draw-time rebuilds regressed that fixture, so they were rejected.
No percentage improvement is promised for an arbitrary game.

## Changes

The compiler retains proven numeric fields through its final inference pass,
including aliases used to read and write those fields. Confirmation removes
invalid facts to a fixed point, and resets dependent function signatures.
It preserves the existing documented restrictions on changing the types of
numeric locals and stable record fields.

For `all()` loops, the compiler computes the dense-prefix length once when it
proves that the body and every called function leave array slots unchanged.
Indexed writes, `add`/`del`/`deli`, callbacks and unknown/recursive calls keep the
live length check. Named-field updates, indexed reads and allocation can retain
the faster loop; the iterator table stays rooted and elements are still fetched
after each possible GC. This removes repeated linear scans of particle lists.

The build omits the old small-map cache's IWRAM helpers when generation emits
no storage for that cache. An aligned ARM store-multiple fill also reduces
full-frame clears by about 5,500 cycles.

Mode 4 map traversal now runs in IWRAM. Compiler-generated, validated immutable
sheet metadata identifies solid tiles. Adjacent equal map IDs can fill one run
without repeatedly decoding every pixel. Transparency is checked on the source
color before palette remapping; an opaque color mapped to zero remains opaque.
Mixed full 8×8 cells use a native ARM copier with exact zero transparency and
arbitrary destination alignment. Partial cells and other draw states retain the
general renderer.

Packed map caches extend acceleration beyond the old small, single-plane cache.
Each eligible logical plane has bounded pixel/span storage. Uniform spans store
a color directly in their eight-byte record; mixed spans retain their original
source pixels. The native copier fills the uniform spans without rereading pixel
payload, saving both memory and EWRAM bus traffic. The generator uses
actual assets to estimate capacity, requires an average opaque run of at least
32 pixels, and uses target type sizes to reserve stack, Lua heap semispaces and
8 KiB of additional EWRAM headroom. Arrays become zero-length if they do not fit.
The cache supports immutable sheets, resident maps up to 64×32 cells, identity
draw palettes, source-zero transparency and unfiltered map calls. Other cases
use live cell rendering. AUTO/Mode 0 eligibility and Lua API semantics are unchanged.

Cache preparation runs after `_init`. Later map changes invalidate only their
plane; rendering immediately uses live cells without rebuilding the whole map
inside a frame. This avoids the multi-million-cycle cold rebuild spikes found
in mutation tests. Each plane is tested independently for run fragmentation:
short spans can be slower than the native cell renderer. Initialization edits
have a bounded capacity allowance and fall back if they exceed it.

Packed storage has explicit word alignment. The general sprite blitter also
checks actual source/destination addresses before using ARM word loads, fixing
a real unaligned-sheet rendering bug. Host tests alone did not reveal this;
the target regression fails on the original implementation at all three
unaligned source offsets. The existing raster copy loop also avoids forming
one-past-allocation pointers under Go's strict `checkptr` checks.

## Validation and reproduction

The target map fixture compares cached and uncached map calls against individual
sprite calls across moving cameras, clips, palette/transparency changes and map
mutations. A separate target fixture compares both map and sprite paths against
an independent per-pixel oracle at all four source alignments. The native batch
copier also has an instruction-level bounds and ABI checker.

```sh
go test ./internal/transpiler ./internal/build ./cmd/mtile
(cd runtime && go test ./...)
(cd runtime && go test -tags=mtile_mode0 ./...)
go test ./cmd/renderbench -tags=runtimeperfdiagnostic \
  -run '^Test(DivisionTarget|MapCellTarget|SheetAlignmentTarget)Diagnostic$' -count=1 -v
python3 scripts/map-packed-check/check.py --output-dir /tmp/mtile-map-packed-check
```

Build the local headless player first, or set `MTILE_DIVISION_PLAYER` to its
executable. ARM diagnostic tests run in CI after the player/TinyGo build.
