# Mtile performance guide

This is the "Effective Mtile" for frame budgets: what the hardware gives
you, what each operation costs in measured ARM7TDMI cycles, and which
patterns keep a real game at 60 Hz. Every number below was measured with
`runtime/cmd/benchspike` on mGBA (cycle-accurate ARM7TDMI timing) using the
GBA's own TM0/TM1 hardware timers, on a 128x128 sheet with realistic
half-transparent art. Re-measure after runtime changes with:

Unless a section explicitly says otherwise, graphics numbers in this guide
describe the Mode 4 software framebuffer backend. The compile-time Mode 0
backend accepts the statically proven tile/BG/OBJ subset, and AUTO selects it
only when both its semantic and benchmark-backed cost proofs succeed. The exact API contract and expansion gates are in the
[render-mode compatibility guide](render-modes.md) and
[engineering plan](render-mode-plan.md).

## Paired static-map proof

`cmd/renderbench` built `examples/testio` twice with identical source/assets,
TinyGo 0.41.1, `-gc=none -opt=2`, and the pinned headless mGBA core. After 20
warm-up frames it compared and sampled 120 synchronized 30 Hz ticks.

| Metric | Forced Mode 4 | Forced Mode 0 |
| --- | ---: | ---: |
| LCD pixel differences | 0 | 0 |
| Median / p95 total cycles | 544,417 / 544,417 | 38,409 / 38,409 |
| Median draw cycles | 465,759 | 576 |
| Median commit cycles | 76,924 | 36,341 |
| Cold tile + map upload cycles | n/a | 1,967,744 |
| Missed frames | 0 | 0 |
| ROM bytes (debug build) | 25,980 | 25,528 |
| EWRAM free | 60,600 | 94,640 |

Mode 0 avoids redrawing 600 8x8 cells and copying a 38,400-byte framebuffer on
every tick; its immutable tiles and screen map remain resident in VRAM. That is
why this deliberately static case wins by an unusually large margin. The table
does not predict mutation-heavy, scrolling, sprite, text, or primitive-heavy
games. Those supported shapes have separate parity fixtures; their cost must be
measured with `cmd/renderbench` for the actual project. The 1,967,744-cycle
(~117 ms) cold tile/map upload is excluded from the steady-state median and
occurs during forced blank, before BG0 is enabled.

The complete `tile-v1` matrix is checked in under
[`docs/benchmarks/render-modes`](benchmarks/render-modes/README.md). Camera,
two-layer, composite OBJ/text/palette, 16 `sset`, and 16 `mset` workloads all
passed parity, memory, p95, missed-frame, and ≥20% median-speed gates. Runtime
`fset` remained pixel-exact and 15.67% faster than Mode 4, but deliberately
failed the conservative 20% AUTO gate; `tile-v1` therefore selects Mode 4 for
that source shape.

```sh
cd runtime
make benchspike-gba
mgba benchspike.gba     # the table prints on screen
```

Host-side regression benchmarks (relative cost, allocation checking) live in
`runtime/bench_test.go`:

```sh
cd runtime
go test -bench=. -benchmem
```

Every benchmark must report `0 allocs/op`. The runtime never allocates on
the Go heap after `Init`; a nonzero allocation count is a bug.

## The budget

One display frame is **280,896 cycles** (1232 cycles x 228 scanlines,
59.73 Hz). The generated loop runs `_update`/`_draw` at **30 Hz** - once
every two display frames, PICO-8's native rate - so one game tick owns
**561,792 cycles**. Fixed per-tick costs already spent on your behalf:

| Cost | Cycles | % tick |
| --- | ---: | ---: |
| DMA blit of the framebuffer to VRAM | ~77,000 | 14% |
| Input poll, tick, palette, flip | ~1,000 | <1% |

The blit targets the hidden page (which cannot tear), so it runs as soon as
`_draw` finishes rather than waiting for VBlank. In practice, treat
**~480,000 cycles as the ceiling for `_update` plus `_draw` combined**.

Why 30 Hz and not 60: the measured table below shows a full-screen `map()`
is ~832k cycles and 100 sprites are ~210k. At 60 Hz those numbers made
whole game genres impossible; at 30 Hz a map redraw plus a normal sprite
load plus HUD text fits with headroom. The GBA's LCD still scans at
59.73 Hz - each rendered image is simply shown for exactly two scans,
which is how most commercial GBA games shipped. A hardware timer
(TM2/TM3) paces the flip so a fast tick can never land one frame early:
the cadence is a metronomic 2 frames per flip, which the eye reads as
steady 30 FPS, not jitter. Jitter only happens when cadence *alternates*
(1-2-1-2); the pacer makes that impossible by construction.

The frame profiler (`Context.SetProfiling`) reports exact per-phase costs
on real hardware; `MissedFrames` counts ticks that overran the two-frame
slot.

### What a tick buys you: 30 Hz vs 60 Hz

| | 60 Hz loop | 30 Hz loop (current) |
| --- | ---: | ---: |
| Budget per game tick | 280,896 cycles | **561,792 cycles** |
| Fixed blit share | 27% | **14%** |
| Full 16x16 room `map()` redraw, dense art | 126% - did not fit | **63% - fits** |
| Complete Celeste-class tick (cls + room + 30 sprites + 24 particles + HUD) | ~123% - did not fit | **61% + 14% blit = 75% - fits** |
| Aligned 8x8 sprites after `cls` | ~70-95 | **~180** |
| Input latency | up to ~17 ms | up to ~33 ms |

The last row is the honest trade-off: one extra display frame of input
latency, the same cost PICO-8 itself and most shipped GBA titles accepted.
Everything else is a doubling. The panel still scans at 59.73 Hz; each
image is simply shown for exactly two scans, paced by a hardware timer so
the cadence cannot alternate (alternation, not the rate, is what reads as
jitter).

## Measured costs per call

| Operation | Cycles | % tick (30 Hz) | Notes |
| --- | ---: | ---: | --- |
| `cls(c)` | 67,542 | 12% | DMA3 fill; cost is fixed |
| `rectfill` full screen | 67,723 | 12% | full-width spans use the same DMA fill |
| `rectfill` 32x32 | ~2,500 | <1% | word-store span loop |
| `map` 30x20, 50% transparent art | 832,381 | 148% | still cannot run every tick |
| `map` 30x20, fully opaque art | 763,901 | 136% | packed words help but cells dominate |
| `spr` 8x8 x100, aligned | 207,698 | 37% | ~2,077 per sprite |
| `spr` 8x8 x100, odd x | 225,874 | 40% | ~2,259 per sprite |
| `spr` 8x8 x100, flipped | 296,186 | 53% | ~2,962 per sprite |
| `sspr` 8x8 -> 16x16 x10 | 136,882 | 24% | ~13,700 per stretch |
| `print` 40-char row | 36,138 | 6% | ~900 per glyph |
| `line` full diagonal | 44,903 | 8% | ~187 per pixel |
| `circfill` r=16 | 22,445 | 4% | span-based |
| `sin`+`cos` x100 | 34,291 | 6% | ~343 per pair, LUT |
| framebuffer DMA blit | 77,093 | 14% | paid every tick |
| `map` 16x16 dense | 354,046 | 63% | a full PICO-8 room, worst-case art |
| `map` 16x16 half-empty | 184,446 | 33% | realistic platformer density |
| `map` 16x16 with layer mask | 101,054 | 18% | masked passes reject cells cheaply |
| `map` 30x16 half-empty | 344,718 | 61% | wide scroller band |
| **complete PICO-8 tick** | **344,785** | **61%** | cls + 16x16 room + 30 sprites + 24 particles + HUD |
| **complete GBA tick** | **352,077** | **62%** | cls + 17% 30x20 room + 1 sprite + HUD ([`mapdemo`](../examples/mapdemo)) |

The last row is the go/no-go proof for PICO-8-class ports: a full game tick
- clear, whole-room redraw, 30 sprites (some flipped), 24 particle
rectfills, HUD text - plus the 14% blit totals ~75% of the tick, leaving
~140k cycles for game logic. Lua value operations cost tens of cycles, so
logic is effectively never the bottleneck; pixels are.

### What these numbers mean in practice

- **A PICO-8-sized room (16x16 cells) redraws fully every tick** - even
  with worst-case dense art (63%). This is the workload that makes
  room-based platformer ports straightforward: no dirty-rectangle
  bookkeeping needed.
- **Map-cell cost hierarchy** (each measured): an *empty* cell (sprite 0)
  is ~free - `map()` skips it before any pixel work, so a half-empty room
  costs half a dense one. A *fully opaque* tile is the fast path (packed
  32-bit copies). A *partially transparent* tile costs ~9% more (per-byte
  fallback around its color-0 holes). Level-art rules that follow: never
  fill sky/holes with opaque "background-colored" tiles - leave them
  empty and let `cls()` provide the backdrop for free. For drawn tiles,
  baking the backdrop into the art keeps the fast path, but only when the
  backdrop is actually known (a flat sky); tiles that sit over varying
  content need their transparency, and its ~9% premium is a fair price -
  don't contort art to dodge it.
- **A dense 30x20 full-GBA-screen `map()` every tick still does not fit**
  (149%). Workarounds, in preference order: sparse platformer rooms fit
  (62% for a full tick at 17% cell density - see [`mapdemo`](../examples/mapdemo));
  half-empty maps fit (61%); masked layer passes fit (18% per pass); or draw
  once per room and per tick patch only the strip a camera scroll exposed
  (~90k cycles per 64-cell patch).
- **Sprite budget by scenario** (8x8 `spr`, blit already subtracted;
  "realistic" leaves ~20% headroom for logic, particles, and HUD):

  | Frame also does... | Max aligned | Realistic |
  | --- | ---: | ---: |
  | sprites only | 233 | ~190 |
  | `cls` + sprites | 200 | ~160 |
  | `cls` + sprites, all odd-x | 184 | ~145 |
  | `cls` + sprites, all flipped | 140 | ~110 |
  | `cls` + half-empty 16x16 `map` + sprites | 112 | ~90 |
  | `cls` + dense 16x16 `map` + sprites | 30 | ~25 |

  Rule of thumb: **~200 sprites bare, ~90-110 in a real game that also
  redraws its room**. A PICO-8-class game (15-25 live objects) never
  approaches this wall. 16x16 sprites cost ~4x an 8x8, so ~50 of them
  after `cls`. Offscreen sprites reject before pixel work and are
  near-free, so drawing the whole entity list without camera culling is
  fine.
- Prefer even x positions (the packed fast path needs 4-byte alignment);
  prefer unflipped art (pre-flip in the sheet when a sprite is always
  mirrored) - flipped sprites cost ~43% more.
- `sspr` stretching costs ~6.5x an equivalent `spr`. A few per tick are
  fine; scaling every entity every tick is not.

## How the runtime spends memory

| Region | Size | Speed | What lives there |
| --- | --- | --- | --- |
| IWRAM | 32 KB | 32-bit, zero-wait | stacks (3 KB), blit/print/span cores (~5 KB), TinyGo runtime data |
| EWRAM | 256 KB | 16-bit, 2-wait | `Context` (framebuffer, palettes), mutable sheet/map/flag RAM, 2x56 KB Lua heap |
| VRAM | 96 KB | 16-bit | two Mode 4 pages |
| ROM | up to 32 MB | 16-bit + prefetch | code, assets, LUTs |

Sprite sheets are cell-compacted by default: when static analysis finds no
`sset()` call, generated ROM and EWRAM contain only non-empty 8x8 cells plus a
two-byte logical-ID index. Pixel drawing still reads EWRAM—the direct-ROM
prototype measured 100 sprites at 223,509 cycles versus 207,699 from EWRAM
and made a dense map exceed the tick budget. The final compact-EWRAM path
measured 100 sprites at 211,107 cycles (about 1.6% over the old dense layout)
and preserved the mapdemo-class full-tick result within measurement noise. Calling `sset()` anywhere opts
the project into a complete mutable EWRAM sheet. Flags remain mutable in
EWRAM because `fset()` is cheap at one byte per sprite.

The single most important architectural fact: **ARM code fetched from ROM
pays two bus accesses per instruction; the same code in IWRAM pays one
zero-wait access.** Moving the pixel cores to IWRAM (`//go:section .iwram`)
more than doubled sprite and print throughput without changing a line of
logic. IWRAM is the scarcest resource; only proven hot loops earn a place
there, and the linker map (`make benchspike-elf`, then check `.data` size)
must stay well under 32 KB.

## Runtime design rules (enforced, not aspirational)

1. **Zero Go-heap allocations after Init.** `make smoke-gba` fails the
   build if TinyGo reports any runtime allocation. Lua tables and strings
   live in a fixed 56 KB semispace pair with a Cheney collector.
2. **No divides in per-pixel or per-cell loops.** ARM7TDMI has no divide
   instruction; a software divide costs hundreds of cycles. `spr`/`map`
   convert sprite ids to cells with a fixed-point reciprocal multiply.
3. **Word writes wherever alignment allows.** The framebuffer fill, packed
   sprite rows, and the VRAM blit all move 32 bits per store. VRAM itself
   cannot take 8-bit writes at all, which is why presentation is DMA of
   whole words.
4. **Fixed costs go to hardware.** `cls` and full-width `rectfill` are DMA3
   fills; the frame blit is DMA3 with the whole VDraw window to run in.
5. **Bounds checks stay out of pixel loops.** The blit cores take raw
   pointers after their callers establish bounds from the cell clamps. The
   clamps are tested; the cores are covered by the golden-framebuffer test.

## Good practice / bad practice for game code

**Do:**

- **Call `cls()` every tick without guilt.** It is a single fixed-cost
  DMA3 hardware fill: 67,542 cycles, 12% of the tick, no matter what was
  on screen. It is not a 38,400-pixel software loop. The framebuffer is
  persistent (nothing implicitly clears it), so the real choice is
  `cls` + redraw everything (simple, 12%) versus no `cls` + patch only
  what changed (fastest, but you own the bookkeeping). Start with `cls`;
  drop it only if the profiler says you must.
- Keep per-frame table work on the array part (`t[i]` with `i` in
  1..cap). Array access is a direct indexed read; hash access costs a
  probe sequence with 12-byte compares.
- Static field syntax (`object.x`) is monomorphic by default: its first
  execution validates the table and resolves the hash slot, then that access
  site assumes the value remains a table with the same field layout. This
  removes repeated type/hash/key checks from record-like game state. Replacing
  the value with another type, adding/removing fields in a way that changes its
  layout, or using different record shapes at the same function access site is
  unsupported; use indexed access (`object["x"]`) when the shape is dynamic.
- First-observed numeric globals, locals, numeric-for counters, arithmetic
  expressions, and numeric record fields use native 16:16 `rt.Number` storage
  and operations. Reassigning one of those bindings or fields to a different
  Lua type is unsupported. Keep genuinely polymorphic values behind bracket
  indexing or separate bindings. The paired 16-body physics fixture in
  [`examples/physics-stress`](../examples/physics-stress) measured a 20.14%
  reduction in update/overhead cycles with zero differing pixels across 600
  Mode 4 frames.
- Fixed, non-escaping global numeric array literals use compact 4-byte Number
  elements instead of 12-byte Value slots. Direct indexed reads/writes and
  `#array` stay native; an escape, shadow, hash field, or explicit non-number
  assignment keeps the generic table. The 64-particle, 512-access/tick
  diagnostic measured 750,595 -> 553,263 median cycles (-26.29%), 3,200 ->
  1,040 bytes of Lua heap allocation (-67.50%), and 83,348 -> 72,484 ROM bytes
  (-13.03%), with unchanged linker EWRAM and zero differing pixels across 600
  Mode 4 frames. Element types are permanent under the same monomorphic
  contract as numeric bindings.
- Direct user functions receive native `rt.Number` parameters only when every
  static call site supplies a proven number. Return positions become native
  independently when every control-flow path supplies a number in that slot;
  mixed, nil-padded, falling-through, or unknown positions stay as `rt.Value`.
  The paired 12-body,
  ~280-call/tick fixture in
  [`examples/function-physics-stress`](../examples/function-physics-stress)
  measured 875,310 -> 649,440 median cycles (-25.80%) and a 39.50% reduction
  in update/overhead residual, with zero differing pixels across 600 Mode 4
  frames. This deliberately over-budget workload amplifies call overhead; the
  percentage is not a promise for ordinary games.
- Multi-return specialization preserves Lua expansion while allowing mixed Go
  tuples such as `(rt.Number, rt.Value, rt.Number)`. In the dedicated 240-call
  fixture at
  [`examples/multi-return-physics-stress`](../examples/multi-return-physics-stress),
  TinyGo's normal inliner erased the tagged/native distinction completely:
  both shipping-path ROMs measured 642,296 median cycles and 43,996 bytes. With
  the same root-protected out-of-line path in both builds, native tuple results
  measured 813,269 -> 792,039 median cycles (-2.61%), 539,625 -> 520,425
  update/overhead cycles (-3.56%), and 50,012 -> 49,456 ROM bytes (-1.11%).
  Both comparisons had zero differing pixels across 600 Mode 4 frames and
  unchanged EWRAM. The out-of-line result applies to calls that cannot be
  inlined or must retain root protection; it is not an extra gain on helpers
  the compiler already collapses completely.
- Calls to transitively proven non-allocating functions bypass the generated
  temporary-root push/pop wrapper. Allocation through table creation/growth,
  string creation/concatenation, an unknown builtin, or an unproven recursive
  cycle keeps the protected path. A dedicated workload with one live table
  root and 384 pure numeric calls per tick in
  [`examples/noalloc-call-stress`](../examples/noalloc-call-stress) measured
  334,028 -> 234,519 median cycles (-29.79%) and 112,219 -> 13,873
  update/overhead cycles (-87.64%), with zero differing pixels across 600 Mode
  4 frames. Its one cold-start missed tick fell to zero. This fixture isolates
  wrapper overhead and deliberately overrepresents tiny helper calls.
- Reuse tables. `local pool = {}` at `_init`, indices recycled, beats
  `add`/`del` churn: `#t` and `add` both walk the array border, and the
  collector's pause grows with live data.
- Use `flr()` results cached in locals when the same coordinate feeds
  several draws; every Lua arithmetic op is a function call at -opt=2.
- Profile on hardware early with `SetProfiling` + `PerformanceStats`;
  `MaxUpdate`/`MaxDraw` catch the worst frame, not the average.

**Avoid:**

- Full-screen `map()` per frame (see above; it cannot fit).
- `sspr` scaling on many entities per frame.
- Flipping sprites at draw time when the art could carry both facings.
- String building in `_update`/`_draw`. Concat allocates from the Lua
  heap every call and drives collector pauses; `print` of a static string
  allocates nothing.
- Deep `del(t, value)` on long arrays: it is a linear search plus a
  shift. `deli(t, i)` with a known index halves that; swap-with-last and
  pop is O(1).

## Known limits (clear expectations)

| Limit | Value | Where enforced |
| --- | --- | --- |
| Update rate | 30 Hz (two 59.73 Hz scans per tick) | generated loop |
| Sprites on screen per tick (cls + draw) | ~180 8x8 | budget math above |
| Full-screen map redraws per tick | room transitions only | budget math above |
| Lua heap | 56 KB live data | `runtime/heap.go` |
| Table array part | 256 entries | `maxArrayCap` |
| Table hash part | 128 entries | `maxHashCap` |
| String length | 4,096 bytes | `maxStringBytes` |
| Interned static strings | 256 | `maxStaticStrings` |
| Locals rooted across calls | 512 | `maxLocals` |
| Map streaming threshold | >16,384 cells streams from ROM | `mapcache.go` |
| Streaming map edits (`mset`) | 3,072 distinct cells | `MapEditFillLimit` |
| EWRAM for assets + heap + Context | 256 KB total | linker + `ValidateHeapBudget` |
| Continuous positive world coordinate | 32,767 px (4,095 whole 8px tiles) | signed 16:16 `Number` |
| Cartridge ROM | 32 MiB total | GBA address space + `ValidateROM` |

When a limit fails, it fails closed and observably: heap exhaustion returns
`HeapOOM` (nil result in Lua) and preview builds show `HEAP OOM` in the editor,
map-edit exhaustion sets `LastMapError`, and the build refuses ROMs whose
static assets cannot fit EWRAM.

## Measuring your own game

```lua
-- game.lua: watch the real per-phase costs on screen
function _draw()
  cls(0)
  -- ... your drawing ...
end
```

Build with profiling by calling `SetProfiling(true)` from generated code
(exposed to Lua as a debug builtin in a future release), or run the ROM in
mGBA and read `PerformanceStats` fields with the debugger. `MissedFrames`
counts frames whose update+draw exceeded a full display interval - the
number that must stay at zero before shipping on hardware.
