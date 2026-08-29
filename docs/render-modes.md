# Render-mode compatibility

This document is the renderer contract for Mtile's public Lua API. It answers
which calls can preserve their current semantics in a GBA **Mode 0**
tile renderer and which calls require the existing **Mode 4** bitmap renderer.
The phased implementation, semantic gates, and paired benchmark methodology are
defined in the [Mode 0 / Mode 4 engineering plan](render-mode-plan.md).

> **Current status:** Mode 4 remains the compatibility default. Mode 0 is a
> public, compile-time opt-in for the statically proven subset below. `auto`
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

## Implemented Mode 0 subset

`mtile build -renderer mode0 project-dir` succeeds only when the analyzer can
prove all of these conditions:

- the sheet is 8x8-aligned and the logical map is non-empty;
- the bundled source contains no Mode 4-only raster/readback builtin;
- `_draw` has exactly one unconditional direct `cls([color])`, then one to four
  unconditional direct `map(...)` statements;
- every map viewport has a static width no greater than 64 cells and a static
  height no greater than 32 cells; map origin, destination, layer mask, camera,
  and the logical map size may be dynamic/large;
- all `spr`, supported `sspr`, and `print` calls are unconditional direct
  statements after every map; their worst-case combined hardware-object count
  is at most 32;
- `spr` uses a static valid source ID and static 1..32-cell width/height; x/y
  and flips may be dynamic;
- `sspr` uses a static, in-bounds, 8x8-aligned source rectangle and performs no
  scaling; destination position and flips may be dynamic;
- `print` uses the four-argument form, a static printable-ASCII string, and a
  static 0..255 color; x/y may be dynamic. Text reserves OBJ tiles 480..511,
  so other object source IDs must remain below 480 when text is present;
- `clip` is only `clip()` or `clip(0,0,240,160)`;
- `pal`/`palt` run outside `_draw`; palette reset, layer reset, draw/display
  remaps, and the default transparency no-op forms are supported, but arbitrary
  transparent keys are not; and
- `sset` is a direct unconditional statement in `_init` or `_update`; `_init`
  mutations commit while force-blanked and `_update` permits at most 16 calls
  so the visible dirty-tile upload is bounded to VBlank; and
- the BG tile budget fits: one/two/three/four map layers allow at most
  960/896/832/768 sheet cells respectively.

`camera`, `sset`, `mset`, and `fset` are supported. Large logical maps use a
deterministic 64x32 hardware window; mutations patch or invalidate only the
affected visible tile/map state. Each map pass owns a regular 8bpp BG layer,
objects/text use OAM and OBJ VRAM, source color 0 remains transparent, and
`cls` supplies the backdrop. The Mode 4 framebuffer and page-copy code are
excluded by build tag. Unsupported forced Mode 0 source fails before TinyGo
with a source position and stable `M0_*` reason code; it is never approximated.

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
| `spr` | Mode 0 conditional | Can become hardware OBJ/metasprites when tile data and palettes are resident and the frame remains within OAM, scanline, size/shape, flip, layering, and clipping limits. If those limits cannot be proven, use Mode 4. |
| `sspr` | Mode 0 conditional | Requires a backend-proven pixel-identical mapping: normally tile-aligned source rectangles and hardware-supported destination transforms. Arbitrary source crops or resampling/scaling require Mode 4. |
| `map` | Mode 0 conditional | Natural fit for regular/affine BG tilemaps when tile IDs, palette use, layer masks, camera/offset behavior, and the available BG/screen-block budget can be represented. Larger logical maps require deterministic window streaming. Multiple map passes must fit distinct hardware layers or be merged without changing order. |
| `camera` | Both | It is logical draw state. Mode 0 can apply it through BG scroll and OBJ coordinates; other reachable calls still decide whether that frame is compatible. |
| `clip` | Mode 0 conditional | Full-screen clipping is trivial. Other rectangles require the limited hardware window resources, and changing clips between draw groups must preserve call order. Unrepresentable sequences require Mode 4. |
| `pal` | Mode 0 conditional | Display-time palette remaps are native. Draw-time remaps are compatible only when palette/tile/OBJ assignments can preserve every simultaneous remap without conflicts or excess duplicated assets. |
| `palt` | Mode 0 conditional | Default source color 0 transparency maps naturally to transparent tile/OBJ index 0. Arbitrary transparent source colors require proven asset conversion/duplication with no conflicting opaque use; otherwise use Mode 4. |
| `color` | Both | Changes logical default draw/text color and does not itself rasterize. |
| `cursor` | Both | Changes logical text position/color and does not itself rasterize. |
| `print` | Mode 0 conditional | Can use a tile-font BG or OBJ glyphs when position, clipping, palette, ordering, glyph count, and layer budgets fit. Arbitrary pixel placement mixed with incompatible draw calls requires Mode 4. |

The classification is intentionally semantic, not merely based on whether a
similar picture could be produced. A Mode 0 backend must not silently round
coordinates, discard sprites, change overlap order, substitute a different
palette mapping, or return an invented value from `pget`.

## Asset API

These calls are compatible with both backends. A Mode 0 implementation still
has synchronization work to do because the logical cartridge data and the
visible VRAM tile/map copies are separate.

| Builtin | Class | Mode 0 responsibility |
| --- | --- | --- |
| `sget` | Both | Read the logical 8bpp spritesheet, not whichever compact VRAM copy happens to be resident. |
| `sset` | Both | Update the mutable logical sheet and invalidate or upload every affected BG/OBJ tile before it is next drawn. Mode 0 additionally applies the statically bounded call-placement rule above. |
| `fget` | Both | Read the logical sprite flag byte used by map-layer filtering. |
| `fset` | Both | Update flags so later `map` calls observe the new value in either backend. |
| `mget` | Both | Read the logical map, including streamed-map edits. |
| `mset` | Both | Update the logical map and invalidate or patch any visible streamed tilemap entry. |

## Renderer-independent API

The following calls do not choose a renderer. Their normal CPU and memory
costs still count toward the frame budget.

| Area | Builtins | Notes |
| --- | --- | --- |
| Input | `btn`, `btnp` | Same GBA keypad state in either backend. |
| Math | `flr`, `ceil`, `abs`, `sgn`, `min`, `max`, `mid`, `sin`, `cos`, `atan2`, `sqrt`, `rnd`, `srand` | Fixed-point math and PRNG behavior remain unchanged. |
| Bitwise | `band`, `bor`, `bxor`, `bnot`, `shl`, `shr`, `lshr`, `rotl`, `rotr` | No graphics state. |
| Tables | `add`, `del`, `deli`, `count`, `all`, `pairs`, `foreach` | Lua heap/table behavior remains unchanged. |
| Strings | `sub`, `tostr`, `tonum`, `chr`, `ord` | No graphics state. |
| Time/telemetry | `t`, `time`, `stat` | API shape is unchanged; renderer-dependent CPU counters reported by `stat` will naturally differ. |
| Audio | `sfx`, `music`, `sample`, `gba_note`, `gba_stop` | PSG/audio scheduling is independent of the display mode. |
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
`fset` during `_init` only. Nested/unbounded `mset` and runtime `fset` remain
available under forced `mode0`, but AUTO chooses Mode 4 because their cost is
not statically proven. The checked-in [paired benchmark evidence](benchmarks/render-modes/README.md)
records the crossover that drives this rule.

The project preference lives in `project.json`:

```json
{
  "activePalette": "aurora",
  "renderer": "auto"
}
```

Existing projects with no `renderer` field normalize to `mode4`. The editor's
Preferences panel changes this project setting and its Develop inspector/build
log show requested renderer, selected renderer, stable reason, and fallback.
`mtile build -renderer mode4|mode0|auto` is an explicit one-build override;
without the flag, the project preference is used.

CPU telemetry must not switch modes in the middle of a scene. Mode 0 and Mode 4
assign VRAM, palettes, BGs, OAM, and persistent image state differently, so a
threshold-based live switch would lose or reinterpret state. An explicit scene
transition may switch backends only through a controlled blanking/reinitializing
step with the required assets uploaded again. Telemetry may recommend a renderer
for the next build; it must not mutate the current one.

## `examples/testio`

`examples/testio` is the first accepted vertical slice. With TinyGo 0.41.1 and
the pinned headless mGBA helper, the paired 120-frame capture produced zero
differing LCD pixels and zero missed frames. Forced Mode 0 used 34,040 fewer
EWRAM bytes and measured 38,409 median total cycles versus 544,417 for forced
Mode 4. These results apply to this static `cls()+map()` workload; they do not
justify accepting a call shape that the analyzer cannot prove. The CI renderer
job now builds Mode 4, forced Mode 0, and AUTO, then runs a frame-synchronized
paired mGBA parity/memory/cycle gate and uploads its report.
