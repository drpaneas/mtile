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

### Cartridge saves

`pause` is **renderer-independent** configuration. It takes one boolean
and returns no values; non-booleans are ignored. It changes input routing at
the next engine poll without drawing, mutating game graphics or allocating.
Pause starts **disabled**. While disabled, Start reaches `btn(6)`/`btnp(6)`
normally. While enabled, a new Start press freezes the game beneath the fixed
**P A U S E D** message; another resumes. Start held across enabling must be
released first. Disabling an active pause schedules exact restoration in the
normal presentation window before updates resume. There are no Mode 0-specific
API restrictions or fallback conditions; literal and dynamic booleans have
identical semantics in both renderers and AUTO.

There are no system menu actions or progress callbacks. The former `pause_menu`
API has been removed. Game-defined menus call the save API directly.

The system sprite preserves frozen BG/framebuffer pixels and restores borrowed
VRAM/OAM/palette/register state on resume. Its font, spacing and behavior are
fixed. Mode 0 reserves one OAM entry, 64 OBJ scanline cycles, the last 2 KiB of
OBJ tiles and physical OBJ palette indices 241..245. Game limits remain 127
OAM entries and 1,162 worst-line cycles, including games with no save support
or with pause disabled. Resources stay reserved so enabling at runtime is safe.
See [pausing](pause-menu.md) and [paired ARM evidence](benchmarks/render-modes/pause-only-v2.md).

The system OBJ must not recolor frozen game objects. Mode 0 rejects an 8bpp
sprite source sheet containing palette indices 241..245, a print color in
that range, or an `sset` color that is nonliteral or in that range while
sheet/pose objects are drawn. `M0_PAUSE_PALETTE` identifies this failure at
the offending call; AUTO falls back to Mode 4. The current proof scans the
whole relevant source sheet, including currently unused cells. Packed 4bpp
pose variants instead reserve one of the sixteen palette banks for the UI,
leaving fifteen game banks. BG colors and `pal`/`palset` mappings are unrestricted
by this reservation: physical OBJ indices, rather than their mapped RGB
values, determine conflicts. Mode 4 has no game hardware OBJs and has no
corresponding source, palette, OAM or scanline restrictions.

`btn` and `btnp` remain renderer-independent. Start is reserved by the generated
loop only while system pause is enabled or closing. While disabled it is normal
game input, allowing file selection or naming menus to own it. Game updates,
drawing, `t`/`time`, and button-repeat ticks stop only during an active pause.
Resume suppresses held menu keys until release. Background music continues.
These rules are identical in both renderers and AUTO; manual Go Context input
polling without `PollPauseMenu` is unaffected.

`save_get`, `save_set`, `save_read`, `save_write`, and `save_pending` are **renderer-independent**:
they only access a numeric staging buffer and byte-addressed cartridge save
memory. They never read or mutate graphics state. Both renderers preserve all
64 signed 16:16 fields, error results, record ordering, and recovery behavior.
Slots 0..31 each have independent recovery copies; omitted slots retain the
original slot-zero layout. Dynamic slot expressions run once; invalid types,
fractions or ranges return -5 without changing staging or SRAM. `save_pending`
is a boolean five-second power-off reminder advanced by engine polls, including
while paused; it never blocks resume and does not acknowledge SD completion.
There are no Mode 0-specific restrictions or fallback conditions. In every
renderer, read/write keys must be literal strings of 1..32 bytes; invalid keys
fail compilation with source-positioned `SAVE_KEY`. Field indices and values
may be dynamic, including invalid types with the documented zero/false results.
Saving does not relax the rest of a scene's AUTO proof. Calls are synchronous;
write on explicit player actions rather than every update. See the dedicated
[saving guide](saving.md) and [parity/cost evidence](benchmarks/render-modes/saving-slots-v2.md).

### Scene requirements

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
- all `spr`, supported `sspr`, `pspr`, `anim`, and `print` calls run from `_draw`, including
  through user helpers (including helpers used inside expressions), `if`
  branches, and numeric `for` loops with a proven finite trip count. Generic
  `for`, `while`, and `repeat` loops that reach
  `spr`/`sspr`/`pspr`/`anim`/`print` cannot prove an object count, so forced Mode 0 fails
  with `M0_OBJ_BUDGET` ("unproven object count") and AUTO falls back to Mode 4.
  They are not required to be top-level statements in `_draw`. `cls` and `map`
  remain unconditional direct `_draw` statements. `spr`/`sspr`/`pspr`/`anim` may follow any
  materialized explicit map
  (including when the object call lives in a helper invoked after that map)
  and thereby occupy its exact object-depth lane; an object behind every
  materialized BG is unrepresentable. `pspr` uses compiled pose layouts and
  disjoint streamed tile windows alongside ordinary sheet objects. Precise
  pose-ID proofs narrow the budget; otherwise all authored poses are considered.
  Mutually exclusive `if`/`elseif`/`else` paths retain every pose call site and
  text glyph for allocation. OAM counts and each hardware scanline use
  independent maxima across alternatives, then add the surrounding draws;
  the branch using most objects need not have the highest scanline cost.
  Nested helpers and branches follow the same rule. Ordinary sprite packing
  is discounted in a branched scene only when every possible packed source
  fits the shared cache; otherwise the proof charges its per-cell fallback.
  Empty or out-of-range poses are no-ops. Up to 127 bounded pose invocations
  may share a frame, subject to the actual hardware budgets. Streamed windows
  use up to 480 OBJ tiles, reduced to 448 when text is linked or 416 when the
  ordinary-sprite packing cache is linked; identity-mapped `spr`/`sspr` source
  tiles are reserved before those windows. Background `sset` edits preserve
  separate-bank poses and update shared-sheet poses on the next draw. `print` remains after every map. After
  the deterministic packing plan they use at most 127 OAM entries and at most
  1,162 game OBJ rendering cycles on every scanline. Literal Y positions are
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
  numeric table literal, indexes into a nested named numeric array field of an
  immutable table (`idle.fr1[1]`), and parameters narrowed by a returning comparison
  guard (`if n < 1 or n > 5 then return`) are included. Width and height are
  proven integers from 1 to 32: either literals or the same finite integer
  sets as IDs. The object budget uses the worst proven width and height, so a
  call that may be 1x1 or 2x2 counts as four OAM entries. x/y and flips may
  be dynamic;
- `sspr` uses a static, in-bounds, 8x8-aligned source rectangle and performs no
  scaling. The rectangle is a proven singleton integer for `sx`/`sy`/`sw`/`sh`
  (and `dw`/`dh` when present): an integer literal, or the same finite-number
  proof as `spr` IDs restricted to one value, including indexes into a numeric
  table literal (`idle={0,8,40,80}` then `sspr(idle[1],idle[2],idle[3],idle[4],x,y)`,
  or a nested named numeric array (`idle={fr1={0,8,40,80}}` then `idle.fr1[1]`).
  A set of more than one source value, an unproven index, or a mutated table is
  not eligible. Destination position and flips may be dynamic;
- `print` uses the four-argument form, a static printable-ASCII string or a
  proven decimal number, and a static 1..255 color for non-empty text (an empty
  string
  may use 0); OBJ palette index 0 is always transparent, whereas Mode 4 draws
  color 0 opaquely, so non-empty color-0 text is not eligible. A number is
  proven when every evaluation is a decimal `formatNumber` value: numeric
  literals, arithmetic, `#`, numeric builtins, globals whose assignments are
  only those forms (so `deaths = deaths + 1` counts), helper parameters
  passed those values, and fields/indexes the numeric analysis already
  enumerates. An unproven variable, boolean, table field, or call is not eligible, even if
  a later runtime value happens to be numeric. Numbers use the
  same decimal `formatNumber` layout as `tostr` (digits, minus, and a fractional
  dot) without allocating a Lua string. Concatenation (`"x"..n`), `tostr`,
  `chr`, `sub`, control codes, and non-number/non-string values are not eligible.
  x/y may be dynamic or non-numeric, in which case the
  shared cursor position and implicit-newline update exactly match Mode 4.
  Text reserves physical OBJ tiles 448..479. A unique `(glyph,color)` pair
  uses one 8×8 tile if bitmap rows 8–9 are blank, otherwise two tiles in one
  8×16 OBJ. The bank holds 32 compact pairs, 16 tall pairs, or a fitting mixture;
  this is separate from the character occurrence budget. Numeric prints intern the charset
  `-.0123456789` at that color and budget 12 OAM entries, covering the widest
  decimal `formatNumber` form. The transpiler sorts and rasterizes
  those unique pairs into a ROM atlas from every `_draw` helper the walk reaches,
  which is uploaded once while the LCD is
  force-blanked. Build also stamps those reserved sheet cells from the atlas so
  leftover authored pixels cannot remain. Authored sheet metadata keeps IDs 480..511, independently of physical OBJ addresses. The sprite editor locks IDs 480..511
  whenever the sheet contains them. Every visible occurrence still consumes one OAM entry and its
  true eight-cycle scanline cost, so the shared 127-entry/1,162-cycle game proof includes the system reservation. Repeated characters may therefore exceed 32 occurrences
  when the occurrence budgets fit; 33 unique pairs fail forced Mode 0 or make
  AUTO fall back with `M0_OBJ_BUDGET`. Other object source IDs must remain below
  448 when non-empty text is present. Empty static strings require no OBJ/text backend;
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
- when native audio is linked and four BGs are materialized, no `sset` or
  `pal` call is reachable from `_update` or `_draw`, including through helpers.
  This combination has no proven VBlank commit bound: forced Mode 0 rejects it
  at the mutation call with `M0_RESOURCE_LIMIT`, and AUTO falls back to Mode 4.
  Initialization-only mutations remain supported while force-blanked. Unused
  authored audio does not trigger this restriction, nor do four authored
  planes when fewer than four are drawn; and
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

The optional OBJ animation bank is stored in ROM. Its 512×320-pixel editor
pages describe artwork locations independently of VRAM allocation and global
pose IDs. PAGE+ adds another page until the 65,536 source tile positions are
used; after 25 full pages, the final page is 512×192 pixels. Named clips in
`assets/animation.json` store ordered pose IDs, FPS and loop behavior. `anim`
selects a pose from those immutable tables
and follows exactly the `pspr` rendering path. Builds store each exactly equal
nonblank 8×8 pixel pattern once and skip blank patterns, preserving logical
source IDs. This saves ROM; it does not relax active-pose tile, OAM, scanline,
or transfer budgets. Normally an entire animation library stays in ROM while
only its current visible poses occupy OBJ VRAM.

If the existing streaming-window or upload proof fails, Mode 0 can instead
keep an immutable reachable pose library resident in OBJ VRAM. This alternative
requires no `sset` anywhere in the program and complete source pixel data at
build time. It preserves each hardware object's exact contiguous tile pixels,
layout, authored flips, and painter order, sharing equal pixel runs across
poses and draw sites. The resulting bank must fit after every reachable
legacy sprite tile and below the text and ordinary-object packing reservations
(at most 480 8bpp-equivalent tiles in total). Unreachable poses do not consume this bank;
unknown pose IDs or clip names still conservatively include every possible
pose. The bank uploads once while the LCD is force-blanked, so animation and
motion subsequently change OAM without visible pose uploads or EWRAM staging.
Already accepted streaming scenes keep their existing allocation. A library
that cannot fit retains the original streaming proof and its stable,
source-positioned rejection or AUTO fallback. Residency does not relax finite
draw counts, OAM, scanline, layering, palette, or native-audio restrictions.
See [resident-pose evidence](benchmarks/render-modes/pose-resident-v1.md).

Literal `pspr` pose names and `anim` clip names resolve to indices at build
time. `pspr("hero_idle", x, y)` is lowered to the identical numeric call before
renderer selection, including its subset budget, flips, variants, and argument
evaluation. Names are case-sensitive, nonempty string literals (parentheses
are allowed); generated symbols and dynamic string lookup are not supported.
Missing names fail every renderer with source-positioned `PSPR_UNKNOWN_POSE`
and a nearby spelling suggestion when available. Names survive other poses'
deletion/reordering; renaming or deleting the referenced name fails the build.
Numeric calls retain signed 16:16 conversion and invalid-ID no-ops. A proven
out-of-range numeric literal emits a nonfatal `PSPR_ID_OUT_OF_RANGE` warning;
dynamic numeric IDs keep their behavior, and stale in-range IDs cannot be
detected. No names or lookup tables are added to pose runtime records. See
[named-pose parity and cost evidence](benchmarks/render-modes/pose-names-v1.md). A whole-program
allocation proof lets pose/clip-only games omit Lua heap storage and root
registration, including games that choose between static clip names in global
variables. Programs that create strings or tables retain their heap and GC
roots. Immutable OBJ pixels are word-aligned in ROM so pose staging can use
aligned ARM copies; unchanged poses retain their staged tiles. These are
implementation optimizations and do not relax the scene proof or change
playback semantics. See the [CPU and memory audit](benchmarks/render-modes/obj-optimization-v1.md).

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

A `map` call without a size covers the clip. Mode 4 counts every cell hidden
before the clip as part of that rectangle; once the camera is far enough into
a wide or tall map for that count to exceed 64x32, Mode 0 starts the
rectangle at the first visible cell instead. Cells keep their positions, so
the image is identical.

### Linked tiles

A shared tile linked to a pose or clip in `assets/tilelinks.json` never draws
as a map tile. Each `map` call draws the linked objects of its visible cells
after its tiles, in row-major order, exactly like `pspr`/`anim` calls at that
point; the map's flag mask filters linked marker IDs using their current
`fget` flags too. An omitted width or height includes off-screen anchors
whose poses reach into the clip; a positive size still limits the explicit
source rectangle. The map still stores the tile for `mget`, `fget`, and
`mset`. See the [tile links guide](tile-links.md) for the full semantics.

Mode 0 hides linked tiles from every BG shadow (viewport, full-rectangle
cache, and `mset` patches) and draws the objects through the pose machinery.
The compiler adds one pose call site to each `map` call, repeated once for the
most linked cells that call can show at once: the largest count in any window
of the call's cell size (or the screen widened by the objects' reach) over
each drawn plane. Positions are treated as unknown and flips as absent, so the
OAM and scanline proofs assume every object coincides. Those sites share the
ordinary 127-entry, 1,162-cycle, and OBJ VRAM budgets.

The count is sound only while linked cells stay where the map placed them, so
Mode 0 rejects any `mset` whose value may be a linked tile. A value must be a
literal or a finite set that the ID analysis enumerates, and none of its
values may be linked. Expanded multi-result argument lists such as
`mset(values())` are rejected for linked maps because the syntax's last
expression need not be the written tile ID. A linked or unproven ID, or an
expanded argument list, fails forced Mode 0 with `M0_TILE_LINKS` at that
`mset`, and AUTO falls back to Mode 4. Mode 4 has no
such restriction: placing a linked tile on a cell outside the build-time index
switches that layer to scanning its visible cells.

Independent runtime pixel fixtures cover both lookup paths, live flag changes,
all mask defaults, right/bottom reach, and explicit rectangle limits in both
renderers. The [native regression records](benchmarks/tile-links/README.md#regression-call-shapes)
add synchronized pixel comparisons and cycle measurements for filtered maps
with default or explicit extents and unlinked `mset` writes.

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
same scalar must be either an integer literal, an immutable top-level integer
constant, or arithmetic the compiler can close to a finite integer set. The
enumerated form is:

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

The set above is exactly `{1,32,35}`; IDs need not be adjacent. Increment and
decrement counters are closed by abstract interpretation across complete
`_init`/`_update`/`_draw` callbacks, including Pico-8 `+=` / `-=`, wrap
assignments such as `if sprite == 9 then sprite = 1 end` or `if sprite > 8`,
modulo reduction, frozen integer bounds assigned once at top level or in
`_init`, and helpers statically inlined from those callbacks. The proof is the
values the variable can hold when `_draw` starts, so a wrap sentinel that only
exists between statements inside `_update` is not a drawn id. An unguarded
`sprite += 1` with no reset or modulus still fails. `_draw` called from
`_update` or `_init` also fails closed, because the callback boundary no longer
hides those intermediates.

A `spr` id may
also be a helper, local, parameter, or field expression whose integer set the
analyzer can close: public numeric builtins (`flr`, `ceil`, `min`, `max`,
`mid`, `abs`, `sin`, `cos`, `rnd`), arithmetic including `%` ranges, numeric
array-literal indexes, returning comparison guards that close a parameter to
a finite integer range or equality set, user functions that only return those,
and field increments that stop at a compared bound. Unbounded or
unknown ids still fail. Forced Mode 0 rejects them with source-positioned
`M0_SPR_SHAPE`; AUTO uses the same diagnostic and falls back to Mode 4. Every
enumerated source rectangle must fit the sheet, OBJ IDs 0..479, the optional
text reservation, and the existing OAM/scanline budgets. This is renderer
eligibility only and does not change `spr()` semantics in either renderer.
Named clip playback is provided separately by `anim()`.

For `pspr` and `anim`, both renderers preserve the same calculated rectangle
when flipping: each flipped axis adds `min + max` to the supplied coordinate
before the internal origin-relative mirror. Bounds are the union of local
8×8 part rectangles, including signed offsets and tile padding. `pspr` uses
its own pose; `anim` uses every pose referenced by that clip. Empty poses do
not enlarge the union. Camera and authored per-part flips still compose with
this adjustment, and unflipped drawing is unchanged. The two signed 32-bit
adjustments live in ROM metadata; there is no gameplay bounds scan or
flip-only pixel transfer. Existing manual flip position offsets must be
removed when rebuilding. This changes neither Mode 0 eligibility nor the
resource and fallback conditions listed below.

The [automatic flip verification](benchmarks/render-modes/pose-flip-v1.md)
checks both renderers against independent expected pixels across 120 game
frames and every sampled Mode 0 LCD scan. It also covers one pose shared by
clips with different bounds and confirms zero tile uploads for cached flips.

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
tiles 416-447 as a 32-cell mixed cache: a unique 16x16 source consumes four
contiguous cells and a unique 32x32 source consumes sixteen. First use uploads
the rectangle in row-major hardware order and later frames reuse it. It is
enabled only when there is no `sset` call and every statically referenced
sprite cell is below 416. Any bounded runtime source variable currently
disables this whole-cart packing optimization and retains the ordinary
metasprite path, whose allocation is independent of the selected frame. If the
cache cannot fit another rectangle, that draw
retains its ordinary four/sixteen-object metasprite path. OBJ text keeps tiles
448-479. The analyzer simulates the same first-use/cache-capacity order as the
runtime: a packed rectangle costs one OAM entry, while a fallback costs all of
its cells. Packing does not reduce scanline rendering time: normal OBJ cost is
still its pixel width, so both a packed 32x32 and one visible row of its
metasprite fallback cost 32 cycles.

The hardware has 1,226 cycles with H-Blank Interval Free disabled (`308*4-6`);
the game ceiling is 1,162 after reserving 64 for the pause overlay. X position never discounts the
proof because horizontally undisplayed OBJs can still consume evaluation work.
The 8-bit Y model includes hardware wrapping and a shared dynamic camera shift.
Any source that exceeds 127 OAM entries or the scanline-cycle ceiling fails
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
| `pspr` | Mode 0 conditional | Accepts a numeric ID or case-sensitive literal pose name. Literal names lower to identical numeric calls before renderer selection; missing names fail all renderers with source-positioned `PSPR_UNKNOWN_POSE` and a close spelling suggestion when available. Numeric out-of-range literals emit nonfatal `PSPR_ID_OUT_OF_RANGE` warnings without changing invalid-ID no-ops. Names add no runtime storage or lookup and preserve every existing restriction below. Draws an authored pose from the animation bank, or the shared sheet when no animation bank exists. Call-level flips mirror inside its build-calculated local tile bounds, preserving the bounds at the same destination. The compiler packs complete nonoverlapping unflipped tile rectangles into exact 8–64px hardware objects; overlapping or individually flipped parts retain their original cell semantics. Known IDs use a subset budget; unknown IDs conservatively use every authored pose. Bounded invocations may mix with `spr`/`sspr` and use mutable shared-sheet art. Nonoverlapping streaming windows, or the exact immutable resident bank described above, must coexist with static sheet objects, text, and any ordinary-object packing cache within OBJ VRAM. Programs using `sset` cannot use the resident alternative. Both paths must satisfy finite-draw, OAM, scanline, and applicable upload budgets. An oversized or unbounded scene fails forced Mode 0 and makes AUTO select Mode 4; the runtime never silently truncates an admitted scene. |
| `anim` | Mode 0 conditional | Selects a registered clip frame at saved FPS, then uses the pose draw path and budgets. Call-level flips use one build-calculated union of every referenced pose’s local tile bounds, not the current frame’s bounds; this fixes the mirror axis throughout playback. Omitted elapsed time follows the shared game update clock; explicit seconds clamp below zero, loop or hold the last frame according to the clip. Literal names budget that clip; dynamic names budget all registered clips. Empty/missing dynamic names draw nothing; missing literal names fail with `ANIM_UNKNOWN_CLIP`. Bounded `_draw` invocations must satisfy the same ordering, VRAM, OAM, scanline, and transfer proofs as `pspr`; otherwise forced Mode 0 fails and AUTO uses Mode 4. |
| `spr` | Mode 0 conditional | A literal source or scalar assigned only from an explicitly enumerable integer frame set can become hardware OBJ/metasprites when every candidate fits resident tiles plus OAM, scanline, size/shape, flip, layering, clipping, and the order-derived BG depth lane. Width and height use the worst proven 1..32 values in the object budget. An enumerated one-cell ID 0 is an OAM-free runtime inactive slot only while its logical source tile is fully default-transparent; the static proof still budgets it conservatively. Arithmetic, table, and helper-derived IDs are admitted when the finite-number proof above succeeds; otherwise use Mode 4. |
| `sspr` | Mode 0 conditional | Requires a backend-proven pixel-identical mapping: an 8x8-aligned in-bounds source rectangle whose `sx`/`sy`/`sw`/`sh` (and `dw`/`dh` if present) are proven singleton integers, not merely similar crops. Literals and singleton table-index/helper results from the `spr` finite-number proof count, including indexes into a nested named numeric array field (`idle.fr1[1]` when `idle.fr1` is a single immutable `{0,8,40,80}`-style table). A multi-value set or unproven index does not. Like `spr`, an admitted call after a materialized explicit map derives its OBJ depth from painter order. Arbitrary source crops, resampling, or scaling require Mode 4. |
| `map` | Mode 0 conditional | `map()` draws every authored non-empty plane. Any 1..8-argument call uses its first argument as logical BG0..BG3 and keeps the remaining optional source/destination/size/mask semantics. Larger logical maps and compact multi-plane maps use deterministic layer-aware window streaming; the selected dense BG/screen-block and tile budgets must fit. Retained compact maps select a link-time compact runtime in both renderers; large maps select the fixed streaming runtime in Mode 0, while Mode 4 keeps the semantically identical generic path because target benchmarks reject the fixed variant there. Linked tiles draw their poses or clips instead; see [Linked tiles](#linked-tiles). |
| `camera` | Both | It is logical draw state. Mode 0 can apply it through BG scroll and OBJ coordinates; other reachable calls still decide whether that frame is compatible. |
| `clip` | Mode 0 conditional | Only `clip()` and `clip(0,0,240,160)` are currently admitted. Every other clipping rectangle requires Mode 4; a forced Mode 0 build rejects it. |
| `pal` | Mode 0 conditional | Display-time palette remaps are native. Draw-time remaps are compatible only when palette/tile/OBJ assignments can preserve every simultaneous remap without conflicts or excess duplicated assets. |
| `palt` | Mode 0 conditional | Only default-transparency reset/no-op forms outside `_draw` are currently admitted. Arbitrary transparent source colors require Mode 4; asset conversion is not a current public fallback. |
| `color` | Both | Changes logical default draw/text color and does not itself rasterize. |
| `cursor` | Both | Changes logical text position/color and does not itself rasterize. |
| `print` | Mode 0 conditional | Can use OBJ glyphs when position, clipping, palette, ordering, occurrence OAM/scanline budgets, and the 32-physical-tile atlas (one or two tiles per unique `(glyph,color)` pair) fit. Literal ASCII strings and proven decimal numbers (the same `formatNumber` layout as `tostr`, without a Lua string) intern glyph tiles at build time and upload them once into physical OBJ 448-479; authored sheet metadata and editor locks remain at IDs 480-511. The number proof is assignment and expression based, not "any variable". Concat, `tostr`/`chr`/`sub`, booleans, unproven fields, and control codes are not eligible. Every visible occurrence still owns one OAM entry. Compact glyphs use 8×8 objects; glyphs with ink in bitmap rows 8/9 use 8×16. Ten-pixel text lines can overlap in hardware; the scanline proof counts each object's actual 8 or 16 rows. Numeric prints budget 12 entries. Non-empty text requires color 1..255 except reserved OBJ indices 241..245; hardware OBJ palette index 0 is transparent while Mode 4 color 0 is opaque; an empty string may use color 0. The current proof keeps text after every map in the front OBJ lane; interleaved/background-relative text or arbitrary pixel placement requires Mode 4. |

The classification is intentionally semantic, not merely based on whether a
similar picture could be produced. A Mode 0 backend must not silently round
coordinates, discard sprites, change overlap order, substitute a different
palette mapping, or return an invented value from `pget`.

## Asset API

These calls access the same logical cartridge data in both backends. `mset`
is Mode 0 conditional when linked tiles require an object-count proof. A
Mode 0 implementation also synchronizes the logical data with separate
visible VRAM tile/map copies.

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
| `mset` | Mode 0 conditional | `mset(x,y,id)` updates legacy BG0; `mset(bg,x,y,id)` updates logical BG0..BG3. Invalid inputs are no-ops. Mode 0 patches or invalidates only visible passes whose logical source matches the target, preserving draw-call order exactly. A literal target retains that plane in compact generated storage; a dynamic target retains all logical planes. Compact-map constant layers use their proven dense slot while retaining the logical BG for invalidation. With linked tiles, Mode 0 requires explicit arguments and a literal or enumerated finite tile-ID set containing no linked ID, because linked-object allocations come from authored map cells; unproven IDs, linked writes, or expanded multi-results fail with source-positioned `M0_TILE_LINKS`, and AUTO falls back to Mode 4. Without linked tiles, expanded arguments remain supported. AUTO additionally requires a literal layer and the bounded placement/count rule, while forced Mode 0 supports dynamic layers exactly. |

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
| Audio | `sfx`, `music`, `sample`, `gba_note`, `gba_stop` | `sample` is retained in the registry for an explicit unsupported-PCM diagnostic; Audio v2 builds reject it. The supported PSG calls are independent of the display mode. `sfx(id)` uses priority 160 when omitted, above Music Lab's default note priority 128, so a gameplay effect steals an active song channel. Pass a lower priority to yield. |
| Debug | `printh` | Compatibility no-op: arguments are evaluated but no log or file is written. Does not affect display-mode selection. |

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

Mode 0 removes Mode 4's full-page clear and software redraw for compatible
tile scenes and lets the PPU compose BGs and OBJs. It does not make Lua, the
TinyGo runtime, garbage collection, audio scheduling, or game logic free.

The normal Mode 0 presenter starts visible VRAM/OAM commits at the first
VBlank scanline (160), waiting for a fresh window if the outer frame pacer
returns later. This reserves all 67 remaining complete scanlines; it does
not make an oversized commit fit. `LastBlitWindow` records the actual writes,
excluding the wait. Native fixtures must still demonstrate that the commit
finishes before active display resumes.

Pose streaming prepares pixels and transfer bookkeeping before VBlank in an
EWRAM buffer sized to the allocated pose windows: 64 bytes per tile, up to
32 KiB for all 512 tiles. One contiguous DMA commits the changed range. Small
pose workloads do not pay for a full-size buffer, and ordinary sheet objects
do not consume this staging allocation.

The compiler also budgets worst-case pose transfers together with dirty BGs,
direct `_update` tile edits, palette changes, the ordinary-sprite packing cache,
and native-audio interrupts. A conservative estimate above the usable
82,544-cycle window rejects forced Mode 0 and selects Mode 4 under AUTO, even
when the tiles fit VRAM. Initialization edits are committed while force-blanked.
[Pose-streaming evidence](benchmarks/render-modes/pspr-stream-v1.md) covers
960 matching game frames and every intervening LCD scan, plus 240 matching
AUTO-fallback frames. Hardware-safe transfers do not guarantee a frame rate:
game logic and pose preparation must also fit the CPU frame budget.

A dense native-audio trial with four dirty maps, 16 tile mutations, palette
remaps, and 128 OBJs measured a 100,599-cycle warm commit and exposed first-scan
tearing. The full 68-line VBlank is only 83,776 cycles. Moving palette work
outside the commit reduced the maximum to 93,211 cycles but did not establish
safe admission, so that runtime prototype was dropped. A separate control with
four dirty maps and the same audio/OBJ load, but no per-frame `sset` or `pal`,
passed 120 synchronized first-scan frames with a 64,540-cycle maximum. The
compiler therefore conservatively excludes the combined mutation shape above;
it does not claim that every excluded program would tear. Audio scheduling
and the public API's renderer classifications remain unchanged.

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

For interactive engineering work, `mtile build -experimental-tilecanvas
examples/celeste` and `mtile edit -experimental-tilecanvas examples/celeste`
opt only that selected project into the experiment. They use Mode 4 code
generation with the private runtime tag and report the actual Mode 0
presentation as `experimental-tilecanvas`. This explicit opt-in is not public
Mode 0/AUTO eligibility; the private composition and resource bounds above still
apply. Omit the flag to use the normal project renderer policy.

## `testdata/projects/testio`

`testdata/projects/testio` is the first accepted vertical slice. With TinyGo 0.41.1 and
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

[Named animation evidence](benchmarks/render-modes/anim-runtime-v1.md) verifies
automatic game-clock advancement, explicit timing, looping/final-frame holds,
and mixed dynamic-name playback with zero differences across 240 game frames
and 480 LCD scans. The fixtures use the same pose resource proof as `pspr`.

### Pose color variants: `pvariant`

`pvariant(name)` selects a named replacement map from `poses.json` for subsequent
`pspr` and `anim` calls. `pvariant()` or `pvariant(0)` resets to the original
colors. Positive numeric IDs select variants in their authored order (1-based);
prefer names because deleting a variant changes numeric IDs. Unknown dynamic
names/IDs reset to original colors; unknown literal names fail compilation with
`PVARIANT_UNKNOWN`. Selection persists until changed, including between frames.
It affects neither `spr`, maps, nor the global palette. Only nonzero indices
1–255 can be replaced; transparency stays at zero. Replacement is simultaneous,
before global `pal`/`palt` processing in Mode 4.

Classification: **Mode 0 conditional**. Mode 4 builds exact recolored pose
pixels from the canonical source. Mode 0 requires immutable resident 4bpp poses,
15 visible colors per hardware object, at most 15 compatible palette banks,
and sufficient OBJ VRAM. All declared variants of every reachable canonical
pose are included in OAM, scanline and storage proofs, even when selected via
helpers or dynamic names. `spr`, `sspr`, `print`, `pal`, `palt`, and `sset` cannot
be combined with this Mode 0 variant bank: their existing 8bpp palette/mutation
semantics would conflict. Unsupported allocation fails forced Mode 0 with a
source-positioned `M0_RESOURCE_LIMIT`; AUTO falls back to Mode 4. Existing pose
projects without variants retain their existing streaming/resident policy.
Palette banks are uploaded while initializing; switching variants changes OAM
attributes and never recolors or uploads sprite pixels during a frame.

[Palette sharing and native parity evidence](benchmarks/render-modes/pose-color-variants-v1.md)
covers the accepted pose/animation variant shapes and resource savings.

### Project fonts and UTF-8 `print`

`print` remains **Mode 0 conditional**; `cursor` remains **both renderers**.
Arcade ASCII (95 glyphs) is the default when no font is selected. Greek,
German and international repertoires require an explicit project selection;
unsupported characters use `?` and only the selected font enters the ROM.
All compiled fonts use a monochrome 6×10 bitmap and fixed 6×10 cell (40×16
cells). The approved B migration changes line/newline/wrap advance from 8 to
10 pixels in both backends. Explicit cursor positions and print's return x
remain pixel coordinates; six-pixel horizontal advance and four-cell tabs
remain shared. Partial glyphs clip without changing logical positions.
Legacy version-1 4×5 documents are padded during compilation; see
[font migration](fonts.md#existing-projects-and-version-1-fonts).

Mode 4 decodes printable UTF-8 codepoints from the selected project font.
Unsupported codepoints and malformed UTF-8 use `?`. Repeat controls consume a
whole UTF-8 character; other control parameters and Lua string APIs remain
byte based. No runtime normalization, shaping or combining-mark composition
is performed.

Mode 0 admits the documented static ASCII/decimal-number subset. It compiles
the **selected** font into exactly matching OBJ glyphs. When rows 8/9 are
blank it uses one 8×8 tile; otherwise it uses two consecutive tiles in a vertical
8×16 OBJ. The omitted rows are proven transparent, never cropped ink. The cell
and logical line advance remain 6×10 for every glyph. Physical OBJ tiles
**448–479** and authored sheet IDs **480–511** remain reserved. Sorted pairs
are packed consecutively; the second tile of a tall pair immediately follows
its top tile in both OBJ VRAM and the authored aliases. Unused reserved cells
are cleared. Capacity is **32 compact or 16 tall glyph/color pairs**, or a
mixture costing at most 32 physical tiles. Each occurrence uses one OAM entry.
The resource proof counts its complete 8/16-row hardware span, including overlap
between adjacent ten-pixel lines. Numeric calls intern `-.0123456789` at their
admitted color and budget the selected font's maximum decimal glyph height.
Exceeding 32 tiles fails forced Mode 0 or falls back from AUTO with stable,
source-positioned `M0_OBJ_BUDGET`. Other color, OAM, ordering, clipping and
palette restrictions above remain mandatory. Unicode and controls fail/fall
back with `M0_PRINT_SHAPE`; a Greek/German font printing only admitted ASCII
is eligible. No merely similar glyph rasterization is accepted.

The one selected 1bpp font remains in cartridge ROM for software/system text.
The Mode 0 atlas is a derived cache, uploaded once during blanked startup;
there is no second selectable font and no steady-state glyph upload.
Every ASCII glyph in every preset is compared across the full 8×16 extent
with Mode 4 at colors 1, 7 and 255. [Production B evidence](../research/font-typography/production-b.md)
records 360 synchronized real-ROM parity frames across empty/HUD/edge scenes,
including negative origins, wrapping, bottom/right clipping and descenders,
with ARM cycle and ROM/RAM measurements.
