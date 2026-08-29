# Render-mode compatibility

This document is the renderer contract for Mtile's public Lua API. It answers
which calls can preserve their current semantics in a future GBA **Mode 0**
tile renderer and which calls require the existing **Mode 4** bitmap renderer.
The phased implementation, semantic gates, and paired benchmark methodology are
defined in the [Mode 0 / Mode 4 engineering plan](render-mode-plan.md).

> **Current status:** currently generated cartridges always use Mode 4. The Mode 0 backend
> and automatic renderer selection are not implemented yet. The classifications
> below are design constraints for that work, not a claim that Mode 0 builds are
> already available.

The executable source of truth is
`internal/transpiler/render_modes.go`. A coverage test requires every public
builtin to be classified here and in code.

## Compatibility classes

| Class | Meaning for a future build |
| --- | --- |
| Renderer-independent | The call does not read or mutate graphics state, so it does not affect renderer selection. |
| Both | The exact API semantics can be implemented by either backend for every valid call shape. |
| Mode 0 conditional | Mode 0 is possible only when static analysis proves the restrictions in this document. Otherwise AUTO must select Mode 4. |
| Mode 4 only | Exact behavior requires arbitrary raster writes or readback of the composed software framebuffer. Any reachable call forces Mode 4. |

"Both" and "Mode 0 conditional" describe compatibility, not an existing Mode 0
implementation.

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
| `sset` | Both | Update the mutable logical sheet and invalidate or upload every affected BG/OBJ tile before it is next drawn. |
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

## Renderer selection policy

The safe future policy is build-time selection:

1. `mode4` always selects the existing software framebuffer backend.
2. `mode0` succeeds only if every reachable graphics call is Both or satisfies
   its Mode 0 conditions. Otherwise the build fails with the call site and the
   violated condition.
3. `auto` selects Mode 4 if any reachable call is Mode 4 only or any conditional
   call cannot be proven safe; otherwise it selects Mode 0.

CPU telemetry must not switch modes in the middle of a scene. Mode 0 and Mode 4
assign VRAM, palettes, BGs, OAM, and persistent image state differently, so a
threshold-based live switch would lose or reinterpret state. An explicit scene
transition may switch backends only through a controlled blanking/reinitializing
step with the required assets uploaded again. Telemetry may recommend a renderer
for the next build; it must not mutate the current one.

## `examples/testio`

The current example is a strong future Mode 0 candidate: its `_draw` starts with
`cls(225)` and then makes one `map()` call, with no arbitrary pixel primitives or
`pget`. Under `auto`, it should select Mode 0 once the tilemap backend proves the
map, palette, and streaming allocation constraints. Today it still builds and
runs in Mode 4.
