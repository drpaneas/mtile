# Render-mode compatibility

This document is the renderer contract for Mtile's public Lua API. It answers
which calls can preserve their current semantics in a GBA **Mode 0**
tile renderer and which calls require the existing **Mode 4** bitmap renderer.
The phased implementation, semantic gates, and paired benchmark methodology are
defined in the [Mode 0 / Mode 4 engineering plan](render-mode-plan.md).

> **Current status:** Fresh projects request `auto`; older projects without a
> renderer setting retain Mode 4 as their compatibility default. Mode 0 is a
> public, compile-time option for the statically proven subset below. `auto`
> selects Mode 0 only when both that semantic proof and the versioned cost
> proof pass; otherwise it falls back exactly to Mode 4 with a source-positioned
> diagnostic. A ROM always links one backend.

The executable source of truth is
`internal/transpiler/render_modes.go`. A coverage test requires every public
builtin to be classified here and in code.

## Compatibility classes

| Class | Meaning |
| --- | --- |
| Renderer-independent | The call does not read or mutate graphics state, so it does not affect renderer selection. |
| Both | The exact API semantics can be implemented by either backend for every valid call shape. |
| Mode 0 conditional | Mode 0 is possible only when static analysis proves the restrictions in this document. Otherwise AUTO must select Mode 4. |
| Mode 4 only | Exact behavior requires arbitrary raster writes or readback of the composed software framebuffer. Any reachable call forces Mode 4. |

"Both" describes API semantics implemented by both linked backends. "Mode 0
conditional" still requires the exact call shape and resource proof below.

All public builtin arities are measured after Lua expression-list adjustment.
Only a bare final call expands to its declared result count; `(call())` is one
value, and a final zero-result builtin contributes no argument while still
executing. Mode 4 accepts every otherwise valid adjusted list. For a Mode 0
conditional drawing builtin, an expanded multi/zero-result argument list is
currently rejected with a source-positioned shape diagnostic because its
resource proof requires explicit argument expressions; adding parentheses or
spelling the arguments explicitly removes that ambiguity without changing the
runtime API.

## Implemented Mode 0 subset

`mtile build -renderer mode0 project-dir` succeeds only when the analyzer can
prove all of these conditions:

- the sheet is 8x8-aligned, and the logical map dimensions are positive whenever
  `map()` is called (its authored planes may all be transparent);
- the bundled source contains no Mode 4-only raster/readback builtin;
- `_draw` has exactly one unconditional direct `cls([color])`, then zero to
  four unconditional direct `map(...)` statements. The `cls` color is omitted
  or statically lowers through the runtime's signed 16:16 conversion to an
  integer from 0 to 255; otherwise Mode 0 could not preserve the Mode 4
  no-op semantics of an invalid color. No return may appear before `cls`,
  including in a nested branch or loop, so the clear dominates every `_draw`
  return path;
- `map()` is the single automatic form and draws every non-empty authored
  plane. Every call with arguments starts with a unique static BG0..BG3 ID
  present in the asset; its remaining zero-to-seven arguments retain the
  source/destination/size/mask order and defaults. Explicit calls appear in
  ascending back-to-front order and cannot be mixed with `map()`;
- every map viewport has a static width no greater than 64 cells and a static
  height no greater than 32 cells; map origin, destination, layer mask, camera,
  and the logical map size may be dynamic/large;
- all `spr`, supported `sspr`, and `print` calls run from `_draw`, including
  through user helpers, `if` branches, and numeric `for` loops with a proven
  finite trip count. Generic `for`, `while`, and `repeat` loops that reach
  `spr`/`sspr`/`print` cannot prove an object count, so forced Mode 0 fails
  with `M0_OBJ_BUDGET` ("unproven object count") and AUTO falls back to Mode 4.
  They are not required to be top-level statements in `_draw`. `cls` and `map`
  remain unconditional direct `_draw` statements. `spr`/`sspr` may follow any
  materialized explicit map
  (including when the object call lives in a helper invoked after that map)
  and thereby occupy its exact object-depth lane; an object behind every
  materialized BG is unrepresentable. `print` remains after every map. After
  the deterministic packing plan they use at most 128 OAM entries and at most
  1,226 normal-OBJ rendering cycles on every scanline. Literal Y positions are
  first quantized exactly like the runtime's signed 16:16 numbers, then compose
  as circular 8-bit hardware intervals; independently dynamic Y positions and
  object groups separated by a `_draw` camera change are conservatively assumed
  to coincide. This analysis follows user helper calls, so a helper-reachable
  camera rebase cannot bypass the scanline proof. Static text X positions
  include the runtime's six-pixel wrapping; dynamic text X/camera phases use a
  conservative coincident-glyph bound;
- `spr` uses a source ID whose complete integer set is statically proven:
  an integer literal, a scalar assigned from an explicit finite integer set,
  a helper/local/parameter/field expression built from those values, or a
  public numeric builtin (`flr`, `ceil`, `min`, `max`, `mid`, `abs`, `sin`,
  `cos`, `rnd`, and arithmetic, including `%` ranges). Array indexes of a
  numeric table literal and parameters narrowed by a returning comparison
  guard (`if n < 1 or n > 5 then return`) are included. Width and height are
  proven integers from 1 to 32: either literals or the same finite integer
  sets as IDs. The object budget uses the worst proven width and height, so a
  call that may be 1x1 or 2x2 counts as four OAM entries. x/y and flips may
  be dynamic;
- `sspr` uses a static, in-bounds, 8x8-aligned source rectangle and performs no
  scaling; destination position and flips may be dynamic;
- `print` uses the four-argument form, a static printable-ASCII string or a
  proven decimal number, and a static 1..255 color for non-empty text (an empty
  string
  may use 0); OBJ palette index 0 is always transparent, whereas Mode 4 draws
  color 0 opaquely, so non-empty color-0 text is not eligible. A number is
  proven when every evaluation is a decimal `formatNumber` value: numeric
  literals, arithmetic, `#`, numeric builtins, globals whose assignments are
  only those forms (so `deaths = deaths + 1` counts), helper parameters
  passed those values, and fields/indexes the numeric analysis already
  enumerates. Any variable, boolean, table field, or unproven call is not
  eligible, even if a later runtime value happens to be numeric. Numbers use the
  same decimal `formatNumber` layout as `tostr` (digits, minus, and a fractional
  dot) without allocating a Lua string. Concatenation (`"x"..n`), `tostr`,
  `chr`, `sub`, control codes, and non-number/non-string values are not eligible.
  x/y may be dynamic or non-numeric, in which case the
  shared cursor position and implicit-newline update exactly match Mode 4.
  Text reserves OBJ tiles 480..511 and is limited to 32 unique `(glyph,color)`
  pairs, not 32 character occurrences. Numeric prints intern the charset
  `-.0123456789` at that color and budget 12 OAM entries, covering the widest
  decimal `formatNumber` form. The transpiler sorts and rasterizes
  those unique pairs into a ROM atlas from every `_draw` helper the walk reaches,
  which is uploaded once while the LCD is
  force-blanked. Build also stamps those reserved sheet cells from the atlas so
  leftover authored pixels cannot remain. The sprite editor locks IDs 480..511
  whenever the sheet contains them. Every visible occurrence still consumes one OAM entry and its
  true eight-cycle scanline cost, so the shared 128-entry/1,226-cycle proof
  remains unchanged. Repeated characters may therefore exceed 32 occurrences
  when the occurrence budgets fit; 33 unique pairs fail forced Mode 0 or make
  AUTO fall back with `M0_OBJ_BUDGET`. Other object source IDs must remain below
  480 when non-empty text is present. Empty static strings require no OBJ/text backend;
- `clip` is only `clip()` or `clip(0,0,240,160)`;
- `pal`/`palt` run outside `_draw`, including through every helper reachable
  from `_draw`; palette reset, layer reset, draw/display remaps, and the default
  transparency no-op forms are supported, but arbitrary transparent keys are
  not;
- user code does not call or retain `_init`, `_update`, or `_draw` as a value;
  the generated lifecycle is the only invocation path, which preserves the
  proven render-frame boundary and resource-call bounds; and
- `sset` is a direct unconditional statement in `_init` or `_update`; `_init`
  mutations commit while force-blanked and `_update` permits at most 16 calls
  so the visible dirty-tile upload is bounded to VBlank; and
- the BG tile budget fits: zero/one/two/three/four map layers allow at most
  1024/960/896/832/768 sheet cells respectively.

`camera`, `sset`, both `mget`/`mset` arities, and `fset` are supported. Large
logical maps use a deterministic 64x32 hardware window; mutations patch or invalidate only the
affected visible tile/map state and off-screen cells never alias a wrapped
screen-block entry. Layered mutation patches only hardware passes backed by
the same logical source BG. An `mset`/`fset` after a map pass leaves that already
rasterized frame unchanged and rematerializes the affected layer on the next
pass, matching Mode 4 call-order semantics. Each map pass owns a regular 8bpp
BG layer, objects/text use OAM and OBJ VRAM, source color 0 remains transparent, and
`cls` supplies the backdrop. With no `map()` calls, no regular BG is enabled;
backdrop-only and OBJ/text scenes remain valid Mode 0 frames. The Mode 4
framebuffer and page-copy code are excluded by build tag. Unsupported forced
Mode 0 source fails before TinyGo
with a source position and stable `M0_*` reason code; it is never approximated.

### Finite-map caching and hardware scrolling

An admitted `map` rectangle can now remain in the existing 64x32 screen shadow
and VRAM across camera moves, including whole-tile moves. This is a runtime
optimization of the same finite map semantics; it adds no public API, changes
no renderer classification, and does not expand AUTO eligibility. Give `map`
the stable source rectangle and move `camera` to benefit from hardware scroll
registers without rebuilding a sliding source window.

Caching requires a full-screen clip and a viewport that cannot see a repeated
hardware copy of the finite rectangle. For source origin `(cx,cy)`, destination
`(x,y)`, size `(w,h)`, and camera `(camX,camY)`, define
`left = camX-x-cx*8` and `top = camY-y-cy*8`. The exact conditions are
`w*8-512 <= left`, `left+240 <= 512`, `h*8-256 <= top`, and
`top+160 <= 256`. The rectangle still obeys the existing 64x32 admission limit.
Outside these bounds the existing viewport materializer is used automatically;
the renderer does not wrap a finite Lua map or approximate its clipping.

The cache uses the existing screen storage and one state flag per BG. Source,
size, flags and cache/materializer transitions invalidate it as needed. `mset`
and `fset` also update affected cached off-screen cells, since those cells can
later scroll into view; cells outside the cached rectangle cannot alias it.
Mutations after a map pass retain the existing draw-order snapshot semantics.
Resident and streaming logical maps use the same visible result.

Independent finite-map pixel fixtures cover wrap boundaries, negative origins,
four and sparse planes, clipping, masks, mutation and streaming. Native examples
and before/after CPU/upload measurements are recorded in
[the general optimization report](mode0-general-optimizations-2026-09-06.md).

A bounded runtime `spr(id, ...)` variable must be initialized by an
unconditional top-level or direct `_init` assignment. Every assignment to that
same scalar must be either an integer literal or an immutable top-level integer
constant, so the compiler can enumerate the complete set before linking:

```lua
PLAYER_IDLE = 1
PLAYER_RUN_A = 32
PLAYER_RUN_B = 35
player_sprite = PLAYER_IDLE

function _update()
  if btn(0) then
    player_sprite = PLAYER_RUN_A
  else
    player_sprite = PLAYER_RUN_B
  end
end

function _draw()
  cls()
  spr(player_sprite, 40, 60)
end
```

The set above is exactly `{1,32,35}`; IDs need not be adjacent. A `spr` id may
also be a helper, local, parameter, or field expression whose integer set the
analyzer can close: public numeric builtins (`flr`, `ceil`, `min`, `max`,
`mid`, `abs`, `sin`, `cos`, `rnd`), arithmetic including `%` ranges, numeric
array-literal indexes, returning comparison guards that close a parameter to
a finite integer range or equality set, user functions that only return those,
and field increments that stop at a compared bound. Unbounded or
unknown ids still fail. Forced Mode 0 rejects them with source-positioned
`M0_SPR_SHAPE`; AUTO uses the same diagnostic and falls back to Mode 4. Every
enumerated source rectangle must fit the sheet, OBJ IDs 0..511, the optional
text reservation, and the existing OAM/scanline budgets. This is renderer
eligibility only: it adds no animation builtin and does not change `spr()`
semantics in either renderer.

The enumerated set may include `0`, so a one-cell dynamic draw can use
`0` as its inactive value. Under Mode 0, `spr(0,x,y)` consumes no runtime OAM
entry when logical sprite 0 contains only the default-transparent source color
0. This is a semantic fast path, not a compiler-budget discount: every such
call still counts as one OAM entry and eight scanline cycles in the static
worst-case proof. If authored data or `sset()` makes any sprite-0 pixel
nonzero, tile 0 is synchronized into OBJ VRAM and the same call draws a normal
hardware object; changing the last nonzero pixel back to 0 restores the
inactive path in that frame. Map cell ID 0 remains the empty-map sentinel and
is never populated from the mutable sprite-0 OBJ tile.

Explicit `map(bg,...)` calls also define exact object-depth lanes without a new
Lua argument. Mode 4 already paints in statement order. Mode 0 counts only the
non-empty planes that receive dense hardware BG slots; after the `k`th such map
out of `N`, `spr()`/`sspr()` uses OBJ priority `N-k`. Because an OBJ wins a tie
with a BG of the same priority, the object is in front of every map already
drawn and behind every later map:

| Four-BG statement position | OBJ priority | Result |
| --- | ---: | --- |
| after BG0 | 3 | in front of BG0, behind BG1-BG3 |
| after BG1 | 2 | in front of BG0-BG1, behind BG2-BG3 |
| after BG2 | 1 | in front of BG0-BG2, behind BG3 |
| after BG3 | 0 | in front of every BG |

Empty logical planes create no phantom lane. `map()` in its automatic form
materializes every non-empty plane as one indivisible operation, so objects can
only follow it in the front lane. GBA OBJ priority has no value below all four
BG priorities; a `spr`/`sspr` before the first materialized map therefore fails
forced Mode 0 with source-positioned `M0_SPR_SHAPE`, while AUTO falls back to
Mode 4. Multiple objects in one lane retain their ordinary painter order.
`print()` is deliberately still front-only in this slice.

Within that existing proof, immutable `spr(...,2,2)`/`spr(...,4,4)` calls and
exact, unscaled 16x16/32x32 `sspr` rectangles use one square hardware OBJ
instead of four/sixteen 8x8 objects. The link-time optimization reserves OBJ
tiles 448-479 as a 32-cell mixed cache: a unique 16x16 source consumes four
contiguous cells and a unique 32x32 source consumes sixteen. First use uploads
the rectangle in row-major hardware order and later frames reuse it. It is
enabled only when there is no `sset` call and every statically referenced
sprite cell is below 448. Any bounded runtime source variable currently
disables this whole-cart packing optimization and retains the ordinary
metasprite path, whose allocation is independent of the selected frame. If the
cache cannot fit another rectangle, that draw
retains its ordinary four/sixteen-object metasprite path. OBJ text keeps tiles
480-511. The analyzer simulates the same first-use/cache-capacity order as the
runtime: a packed rectangle costs one OAM entry, while a fallback costs all of
its cells. Packing does not reduce scanline rendering time: normal OBJ cost is
still its pixel width, so both a packed 32x32 and one visible row of its
metasprite fallback cost 32 cycles.

The 1,226-cycle ceiling follows the GBA's normal 308-dot line budget with the
H-Blank Interval Free bit disabled: `308*4-6`. X position never discounts the
proof because horizontally undisplayed OBJs can still consume evaluation work.
The 8-bit Y model includes hardware wrapping and a shared dynamic camera shift.
Any source that exceeds 128 OAM entries or the scanline-cycle ceiling fails
forced Mode 0 with `M0_OBJ_BUDGET`; AUTO falls back to Mode 4 with the same
source-positioned diagnostic.

BG identity has two unambiguous forms. `map()` automatically draws all
authored non-empty planes in logical BG0-to-BG3 order. `map(bg, ...)` selects
one logical plane; every argument after `bg` is optional. BG0 is back and BG3
is front. Empty logical gaps are removed from storage and Mode 0 allocation:
if only BG1 and BG3 contain cells, they occupy two dense hardware BG slots
while retaining their logical IDs and painter order. A legacy `mset(x,y,id)`
keeps initially empty BG0 resident; `mset(bg,x,y,id)` keeps a literal target
resident, while a dynamic target conservatively retains every logical plane.

Mixing `map()` with explicit calls, duplicating a plane, using a
dynamic/missing ID, reversing order, or nesting/conditioning a map call is an
invalid cross-renderer API program and fails with `MAP_EXPLICIT_CONTRACT`
rather than falling back. The final optional argument remains the sprite-flag
mask and defaults to zero. Existing positional calls migrate by prefixing BG0
or another intended plane: `map(mx,my,...)` becomes `map(0,mx,my,...)`.

## Graphics API

| Builtin | Class | Reason and Mode 0 condition |
| --- | --- | --- |
| `cls` | Mode 0 conditional | A frame-initial clear can reset the backdrop and assigned BG/OAM layers. Multiple clears, a clear after drawing, or reliance on the persistent framebuffer requires Mode 4. |
| `pset` | Mode 4 only | Writes one arbitrary pixel into the current composed image. Mode 0 tile data is not a screen-sized writable pixel buffer. |
| `pget` | Mode 4 only | Reads the result of earlier draw calls from the composed framebuffer. Tile/OBJ VRAM does not contain that composited screen image. |
| `line` | Mode 4 only | Produces arbitrary pixel coverage rather than reusable tile cells. |
| `rect` | Mode 4 only | Its inclusive, camera-aware, clipped raster semantics require arbitrary pixel writes. |
| `rectfill` | Mode 4 only | Arbitrary coordinates, sizes, overlap, and clipping require bitmap rasterization. |
| `circ` | Mode 4 only | Midpoint-circle pixels cannot generally be represented as a fixed tile or OBJ allocation. |
| `circfill` | Mode 4 only | Arbitrary span-filled circles require bitmap rasterization. |
| `spr` | Mode 0 conditional | A literal source or scalar assigned only from an explicitly enumerable integer frame set can become hardware OBJ/metasprites when every candidate fits resident tiles plus OAM, scanline, size/shape, flip, layering, clipping, and the order-derived BG depth lane. Width and height use the worst proven 1..32 values in the object budget. An enumerated one-cell ID 0 is an OAM-free runtime inactive slot only while its logical source tile is fully default-transparent; the static proof still budgets it conservatively. Arithmetic/table/function-derived or otherwise unproven IDs use Mode 4. |
| `sspr` | Mode 0 conditional | Requires a backend-proven pixel-identical mapping: normally tile-aligned source rectangles and hardware-supported destination transforms. Like `spr`, an admitted call after a materialized explicit map derives its OBJ depth from painter order. Arbitrary source crops or resampling/scaling require Mode 4. |
| `map` | Mode 0 conditional | `map()` draws every authored non-empty plane. Any 1..8-argument call uses its first argument as logical BG0..BG3 and keeps the remaining optional source/destination/size/mask semantics. Larger logical maps and compact multi-plane maps use deterministic layer-aware window streaming; the selected dense BG/screen-block and tile budgets must fit. Retained compact maps select a link-time compact runtime in both renderers; large maps select the fixed streaming runtime in Mode 0, while Mode 4 keeps the semantically identical generic path because target benchmarks reject the fixed variant there. |
| `camera` | Both | It is logical draw state. Mode 0 can apply it through BG scroll and OBJ coordinates; other reachable calls still decide whether that frame is compatible. |
| `clip` | Mode 0 conditional | Full-screen clipping is trivial. Other rectangles require the limited hardware window resources, and changing clips between draw groups must preserve call order. Unrepresentable sequences require Mode 4. |
| `pal` | Mode 0 conditional | Display-time palette remaps are native. Draw-time remaps are compatible only when palette/tile/OBJ assignments can preserve every simultaneous remap without conflicts or excess duplicated assets. |
| `palt` | Mode 0 conditional | Default source color 0 transparency maps naturally to transparent tile/OBJ index 0. Arbitrary transparent source colors require proven asset conversion/duplication with no conflicting opaque use; otherwise use Mode 4. |
| `color` | Both | Changes logical default draw/text color and does not itself rasterize. |
| `cursor` | Both | Changes logical text position/color and does not itself rasterize. |
| `print` | Mode 0 conditional | Can use OBJ glyphs when position, clipping, palette, ordering, occurrence OAM/scanline budgets, and the 32-unique-`(glyph,color)` atlas fit. Literal ASCII strings and proven decimal numbers (the same `formatNumber` layout as `tostr`, without a Lua string) intern glyph tiles at build time and upload them once into OBJ 480-511; the sprite editor locks those IDs and build overwrites leftover sheet pixels. The number proof is assignment and expression based, not "any variable". Concat, `tostr`/`chr`/`sub`, booleans, unproven fields, and control codes are not eligible. Every visible occurrence still owns one OAM entry. Numeric prints budget 12 entries. Non-empty text requires color 1..255 because hardware OBJ palette index 0 is transparent while Mode 4 color 0 is opaque; an empty string may use color 0. The current proof keeps text after every map in the front OBJ lane; interleaved/background-relative text or arbitrary pixel placement requires Mode 4. |

The classification is intentionally semantic, not merely based on whether a
similar picture could be produced. A Mode 0 backend must not silently round
coordinates, discard sprites, change overlap order, substitute a different
palette mapping, or return an invented value from `pget`.

## Asset API

These calls are compatible with both backends. A Mode 0 implementation still
has synchronization work to do because the logical cartridge data and the
visible VRAM tile/map copies are separate.

When no `sset` call exists, the generated compact sheet is immutable. Mode 4
selects a measured compile-time read-only sampler that removes the mutable
representation branch. Mode 0 keeps the generic sampler because the analogous
representation specialization improved cold upload but slightly regressed
steady frame time. Its narrower immutable-sheet tag instead removes the
unreachable per-frame dirty-tile scan while retaining the faster generic
sampler. These build choices do not change either API's semantics.

| Builtin | Class | Mode 0 responsibility |
| --- | --- | --- |
| `sget` | Both | Read the logical 8bpp spritesheet, not whichever compact VRAM copy happens to be resident. |
| `sset` | Both | Update the mutable logical sheet and invalidate or upload every affected BG/OBJ tile before it is next drawn. Mode 0 additionally applies the statically bounded call-placement rule above. |
| `fget` | Both | Read the logical sprite flag byte used by map-layer filtering. |
| `fset` | Both | Update flags so later `map` calls observe the new value in either backend. |
| `mget` | Both | `mget(x,y)` reads legacy BG0; `mget(bg,x,y)` reads logical BG0..BG3. Invalid layers/coordinates and sparse absent planes return 0. Both forms read the authoritative compact or streamed map plus its edit overlay, independently of renderer. Compact maps lower implicit BG0 and proven-constant layers directly to their dense storage slot; dynamic layers and streaming maps retain logical lookup. |
| `mset` | Both | `mset(x,y,id)` updates legacy BG0; `mset(bg,x,y,id)` updates logical BG0..BG3. Invalid inputs are no-ops. Mode 0 patches or invalidates only visible passes whose logical source matches the target, preserving draw-call order exactly. A literal target retains that plane in compact generated storage; a dynamic target retains all logical planes. Compact-map constant layers use their proven dense slot while retaining the logical BG for invalidation. AUTO additionally requires a literal layer and the bounded placement/count rule, while forced Mode 0 supports dynamic layers exactly. |

## Renderer-independent API

The following calls do not choose a renderer. Their normal CPU and memory
costs still count toward the frame budget.

| Area | Builtins | Notes |
| --- | --- | --- |
| Input | `btn`, `btnp` | Same GBA keypad state in either backend. |
| Math | `flr`, `ceil`, `abs`, `sgn`, `min`, `max`, `mid`, `sin`, `cos`, `atan2`, `sqrt`, `rnd`, `srand` | Fixed-point math and PRNG behavior remain unchanged. `%` uses Lua floor-modulo signs and `^` requires an integer literal exponent; both rules are renderer-independent. First-observed numeric globals, locals, loops, expressions, and numeric record fields lower to native `rt.Number` identically in both renderers and must retain that type. A once-initialized chunk-scope numeric literal, or an alias of an earlier proven constant, is propagated as its exact 16:16 value in both renderers; any write, shadow, pre-declaration read, computed initializer, ambiguous capture, or disabled proof keeps runtime storage. Direct user-function parameters do likewise only when all static call sites are numeric. Return positions specialize independently when every path supplies a number in that slot and no path falls through; a mixed tuple may therefore use both `rt.Number` and `rt.Value`. Missing, mixed, or unproven positions retain `rt.Value` in both renderers. Transitively non-allocating user calls bypass temporary GC-root wrappers identically in both renderers; any possible table/string allocation or unknown call effect retains protection. Transitive root-stack overflow and root-growing recursion fail during transpilation in either renderer. |
| Bitwise | `band`, `bor`, `bxor`, `bnot`, `shl`, `shr`, `lshr`, `rotl`, `rotr` | No graphics state. |
| Tables | `add`, `del`, `deli`, `count`, `all`, `pairs`, `foreach` | Renderer-independent. Indexed `add(table, value, index)` grows the array before shifting when its dense prefix fills the current capacity; it preserves hash fields and fails with the existing heap-capacity error when no additional array slot is available. Both renderers use this same insertion behavior. Bracket indexing remains fully dynamic except for fixed global numeric array literals proven not to escape, shadow, use hash fields, or receive an explicit non-number; those use compact native Number slots identically in both renderers and must remain numeric. A fixed-shape chunk-scope numeric record used only through its declared static fields is scalar-replaced identically in both renderers. Any alias, bracket access, rebinding, shape change, explicit non-number, or shadowing retains the ordinary table. Other static field syntax (`object.x`) resolves and validates its field slot once per access site, then assumes a stable table type/layout in both renderers; use `object["x"]` for dynamic shapes. If the complete post-lowering program cannot allocate, both renderers omit the periodic collector, target semispaces, and unreachable handle/root metadata; any unproven allocation retains the full heap unchanged. |
| Strings | `sub`, `tostr`, `tonum`, `chr`, `ord` | No graphics state. Static and heap strings compare with the same bytewise lexicographic ordering in both renderers; over-capacity static strings fail during transpilation instead of becoming nil at runtime. The exact unique-static count selects the same smallest safe `Context` capacity tier in both renderers without changing the 256-string source limit. |
| Time/telemetry | `t`, `time`, `stat` | API shape is unchanged; renderer-dependent CPU counters reported by `stat` will naturally differ. |
| Audio | `sfx`, `music`, `sample`, `gba_note`, `gba_stop` | PSG/audio scheduling is independent of the display mode. `sfx(id)` uses priority 160 when omitted, above Music Lab's default note priority 128, so a gameplay effect steals an active song channel. Pass a lower priority to yield. |
| Debug | `printh` | Does not affect display-mode selection. |

## What a Mode 0 backend must own

A correct tile renderer is more than changing `DISPCNT`:

1. Allocate character blocks, screen blocks, BG layers, palettes, and OAM
   without overlap.
2. Keep the logical spritesheet and map authoritative while maintaining
   visible VRAM caches for `sset` and `mset`.
3. Convert large maps into a deterministic streamed window when they exceed
   one hardware tilemap layout.
4. Preserve camera, clip, palette, transparency, overlap, and draw-order
   semantics or reject the build with a source location and a reason.
5. Validate worst-case OBJ and scanline use. Runtime overflow must never turn
   into silently missing sprites.
6. Keep audio, input, heap, update cadence, and fixed-point behavior identical.

Mode 0 removes the full 38,400-byte framebuffer clear/blit path for compatible
tile scenes and lets the PPU compose BGs and OBJs. It does not make Lua, the
TinyGo runtime, garbage collection, audio scheduling, or game logic free.

The normal Mode 0 presenter starts visible VRAM/OAM commits at the first
VBlank scanline (160), waiting for a fresh window if the outer frame pacer
returns later. This reserves all 67 remaining complete scanlines; it does
not make an oversized commit fit. `LastBlitWindow` records the actual writes,
excluding the wait. Native fixtures must still demonstrate that the commit
finishes before active display resumes.

BG and OBJ palette uploads share the palette dirty flag. Moving sprites
alone does not rewrite the unchanged OBJ palette. Palette changes made by
`_init` are committed in `FinalizeInitHardware`, while the LCD remains
force-blanked, before the first visible map/OAM commit. Later remaps and
resets update both banks at the next presentation, including frames with
no visible objects. These are general lifecycle/cost fixes; no public Lua
classification or AUTO admission rules change.

## Renderer selection and project policy

Selection is build-time:

1. `mode4` always selects the software framebuffer backend (`M4_FORCED`).
2. `mode0` selects Mode 0 only after the proof above succeeds
   (`M0_FORCED_PROVEN`); otherwise the build fails at the violating call.
3. `auto` selects Mode 0 when the semantic and cost proofs succeed
   (`M0_AUTO_ELIGIBLE`). A semantic failure selects Mode 4 with
   `M4_AUTO_MODE0_INELIGIBLE`; a semantically exact but cost-unproven shape uses
   `M4_AUTO_MODE0_COST_UNPROVEN`. Both include the exact source-positioned
   fallback diagnostic.

AUTO profile `tile-v1` accepts the semantic subset above plus only these
mutation shapes: the already bounded direct `sset` calls, at most 16 direct
`mset` calls per `_update` (any number of direct initialization calls), and
`fset` during `_init` only. Four-argument `mset` calls additionally use a
numeric-literal layer. Nested/unbounded or dynamic-layer `mset` and runtime
`fset` remain available under forced `mode0`, but AUTO chooses Mode 4 because
their widest cost is not statically proven. `mget(bg,x,y)` adds no renderer
upload and has no extra AUTO restriction. The checked-in
[paired benchmark evidence](benchmarks/render-modes/README.md) records the
accepted shapes and crossover decisions.

The project preference lives in `project.json`:

```json
{
  "activePalette": "aurora",
  "renderer": "auto"
}
```

`mtile new` writes `renderer: "auto"`. Existing projects with no `renderer`
field normalize to `mode4`, preserving their established framebuffer behavior.
The editor's Preferences panel changes this project setting and its Develop
inspector/build log show requested renderer, selected renderer, stable reason,
and fallback.
`mtile build -renderer mode4|mode0|auto` is an explicit one-build override;
without the flag, the project preference is used.

CPU telemetry must not switch modes in the middle of a scene. Mode 0 and Mode 4
assign VRAM, palettes, BGs, OAM, and persistent image state differently, so a
threshold-based live switch would lose or reinterpret state. An explicit scene
transition may switch backends only through a controlled blanking/reinitializing
step with the required assets uploaded again. Telemetry may recommend a renderer
for the next build; it must not mutate the current one.

## Private tile-canvas experiment

The private `mtile_tilecanvas` engineering build tag combines up to four tiled
backgrounds with a sparse foreground canvas presented as 16×16 hardware OBJs.
The current implementation derives behavior from map geometry, pixel coverage,
and hardware/storage limits; it no longer reserves Celeste colors or admits
only one fixed room. A complete-map cache uses hardware scrolling only where
its camera/wrap bounds preserve the finite map call's exact image.

It is not a project renderer value, public Mode 0 capability, or AUTO profile,
and it changes no builtin classification above. The private path requires an
immutable sheet, ordered map calls before raster drawing, identity map palette,
key-zero transparency and full map clip. It admits one frame-initial `cls`,
up to 96 occupied 16×16 foreground blocks, and a bounded VBlank upload. Opaque
color zero requires an unused nonzero OBJ palette entry; no fixed source index
is reserved. Composed `pget` remains unsupported. Unsupported private behavior
is rejected rather than silently rendered approximately.

See [the current implementation and constraints](celeste-parallax-2026-09-06.md).
The earlier scene-specific allocation and measurements remain in the
[historical tile-canvas notes](celeste-tilecanvas-experiment-2026-09-06.md).
Public builds continue to reject/fall back for unsupported raster calls as
documented above; the experiment is not evidence for wider public eligibility.

For interactive engineering work, `mtile edit -experimental-tilecanvas
examples/celeste` opts only that editor's selected project into the experiment.
It uses Mode 4 code generation with the private runtime tag and reports the
actual Mode 0 presentation as `experimental-tilecanvas`. This explicit opt-in
is not public Mode 0/AUTO eligibility; the private composition and resource
bounds above still apply. Omit the flag to use the normal project renderer policy.

## `examples/testio`

`examples/testio` is the first accepted vertical slice. With TinyGo 0.41.1 and
the pinned headless mGBA helper, the paired 120-frame capture produced zero
differing LCD pixels and zero missed frames. Forced Mode 0 used 34,040 fewer
EWRAM bytes and measured 38,409 median total cycles versus 544,417 for forced
Mode 4. These results apply to this static `cls()+map()` workload; they do not
justify accepting a call shape that the analyzer cannot prove. The CI renderer
job now builds Mode 4, forced Mode 0, and AUTO, then runs a frame-synchronized
paired mGBA parity/memory/cycle gate and uploads its report.

The mapless `cls()+print()` fresh-project shape has its own paired 120-frame
evidence: zero differing pixels and missed frames, 35,032 more free EWRAM, and
a 49.84% lower median work cost in Mode 0. It therefore belongs to AUTO's
evidence-backed subset without requiring an artificial empty `map()` call.

The duplicate-heavy `text_intern.lua` proof draws 32 `A` glyph occurrences
from one unique `(glyph,color)` tile. Compared with the former per-occurrence
staging path, Mode 0 median work falls from 162,434 to 71,117 cycles (56.22%),
draw work falls from 94,901 to 16,062 cycles (83.08%), and EWRAM free rises by
1,984 bytes. Its paired Mode 4/Mode 0 capture has zero differing pixels and no
missed frames. The proof does not weaken object budgeting: it still reports 32
OAM entries and 256 worst-line OBJ cycles.

## `examples/coin-block`

`examples/coin-block` is the current-API stateful-background proof. One direct
`mset()` per update alternates a single active map cell, then leaves it on a
stable used tile after a deterministic player hit and one coin award. Its
paired 120-frame Mode 4/Mode 0 capture covers the transition with zero
differing pixels and zero missed frames.

Mode 0 measured 102,790 median total cycles against Mode 4's 305,554, reclaimed
30,952 EWRAM bytes, and produced a 23,024-byte ROM instead of 31,564 bytes. The
Mode 0 cold upload cost 3,273,893 cycles. This supports AUTO for the already
bounded direct-`mset` shape in a persistent tile scene; it does not remove the
cold-load trade-off or justify accepting unbounded/indirect mutations.
