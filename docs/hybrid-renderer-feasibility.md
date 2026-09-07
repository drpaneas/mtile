# Hybrid rendering for Celeste on GBA

**Historical assessment, superseded by
[the implemented parallax/OBJ-canvas design](celeste-parallax-2026-09-06.md).**
The sections below describe the earlier investigation, not the current private
runtime or current Celeste asset layout.

This records the initial feasibility assessment. The subsequent private
`mtile_tilecanvas` experiment lives in `runtime/tilecanvas.go`; it is exercised
against Mode 4 by `TestCelesteTileCanvasDiagnostic` in `cmd/renderbench`.
The experiment is not eligible for public Mode 0 or AUTO selection.
See [implementation notes and measured results](celeste-tilecanvas-experiment-2026-09-06.md)
for the completed first experiment.

## Hardware modes and rendering methods

The useful target for Celeste is a Mode 0 display combining regular tile
backgrounds, hardware objects and software-rasterized tile surfaces. A tile
surface can expose pixel drawing to the game while remaining a regular tiled
background to the display hardware. Tonc provides a concrete implementation
of this technique, including pixel plotting and map construction, in its
[tiled-surface text renderer](https://gbadev.net/tonc/tte.html).

Raster mode switching is a separate technique. A scanline boundary can separate
display regions, but does not provide a Mode 4 layer composited over Mode 0
terrain on the same scanline. Changing DISPCNT changes interpretation of
already-prepared graphics memory; it does not convert the data. Interrupt
latency, register state, BG2 affine coordinates and overlapping VRAM layouts
would require a dedicated raster implementation and target verification.
See [display modes and timing](https://gbadev.net/tonc/video.html).

## Assessment of the three proposals

| Technique | Appropriate use | Celeste assessment |
| --- | --- | --- |
| HBlank scanline split | Deliberately separated horizontal regions | Effects move over the terrain throughout the screen; splitting rows does not solve that composition. |
| Mode 0 plus rasterized tile surfaces | Sparse effects, dynamic text/panels, small pixel canvases | Best first experiment. Preserve regular terrain rendering and upload only changed effect tiles. |
| Mode 4 plus hardware OBJs | A genuinely bitmap-heavy background with sprite actors/UI | A separate useful mtile backend extension, but it retains the terrain rasterization/full-frame upload costs. |

In bitmap modes, hardware sprites have 16 KiB at `0x06014000–0x06017fff`,
and use OBJ tile numbers 512–1023. This is part of the same 96 KiB VRAM,
with a smaller OBJ allocation than in tiled modes. It is sufficient to make
Mode 4 plus sprites viable, but the existing Mode 0 OBJ uploader cannot be
reused unchanged. [Tonc sprite memory layout](https://gbadev.net/tonc/regobj.html)

## What mtile currently does

- `runtime/mode4.go` sets `MODE_4 | BG2_ON`; OBJ is disabled. Both bitmap page
  slots are initialized, and the RAM framebuffer is copied to the hidden page.
- `runtime/render_state_mode4.go` sends `spr()` to the software `drawSprite`.
  Calling the Lua sprite API is therefore not evidence of hardware OBJ use.
- `runtime/mode0_obj.go` owns the hardware OAM path, currently built only for
  Mode 0 and based at `0x06010000` with a 32 KiB allocation.
- `runtime/mode0.go` already has dirty-tile tracking and VBlank tile/OAM commits.
  That infrastructure is useful, but the current `sset()` subset does not
  implement an arbitrary drawing canvas: it has a deliberately small proven
  per-update call budget.
- The public renderer contract in `docs/render-modes.md` still requires exact
  accepted behavior. No eligibility or public API classification was changed
  for this assessment.

## A tiled canvas still needs a memory and upload budget

For 8bpp tiles in row-major tile order, a canvas-local pixel maps to:

```text
tile = (y / 8) * tilesWide + (x / 8)
byte = tile * 64 + (y % 8) * 8 + (x % 8)
```

This is not a linear `y * width + x` bitmap layout. A 16×16 tile grid covers
128×128 pixels and uses 16 KiB of pixel storage at 8bpp. The visible 240×160
area needs 600 unique tiles: 38,400 bytes at 8bpp, or 19,200 at 4bpp, before
maps, other BG tiles and any second buffer. A 4bpp surface also requires a
compatible 15-visible-color palette bank; it is not a universal replacement
for the public 256-color drawing API.
[Tile and map storage](https://gbadev.net/tonc/regbg.html)

The intended implementation draws into a RAM tile surface, tracks old and new
dirty regions, and uploads bounded changed tiles during VBlank. Old pixels
must be cleared when an effect moves or disappears. Rebuilding and uploading
the complete surface every frame can lose the intended benefit.

VRAM does not accept ordinary byte writes. Single-pixel VRAM edits require
proper halfword handling; staged transfers can instead use aligned 16/32-bit
writes or DMA. A canvas also needs a deliberate VBlank/double-buffer strategy
to prevent modifying tiles while they are visible.
[Tonc on VRAM writes](https://gbadev.net/tonc/bitmaps.html)

The current map contains only 19 distinct nonzero tile IDs. That suggests
room for compact terrain plus effect storage, but mtile currently addresses
the authored sheet by source ID, so compact allocation and reserved canvas
ranges still need implementation and proof.

## Exact composition is the main semantic constraint

Celeste's `_draw` orders clouds/platforms, terrain, entities, player hair,
player body, smoke, ambient particles, death particles and completion UI.
One frontmost effect BG cannot reproduce that ordering: hair must be behind
the body, while ambient/death effects are in front, and clouds are behind
terrain. A design needs suitable separate depth planes, or it must rasterize
the affected objects into the same surface to preserve painter order.

The completion panel draws opaque logical color zero. In a hardware tiled
overlay, tile index color zero is transparent, so blindly copying those pixels
would expose the game beneath the panel. Palette allocation needs an explicit
opaque-black entry while retaining transparent holes. Per-call `pal()`,
clipping, camera/shake and dash-freeze persistence must also remain correct.
General `pget()` would need the composed image, not only the canvas contents;
that API must retain its fallback until an exact implementation exists.

## First bounded experiment

Start with unchanged hardware terrain and one front effect surface for the
ambient/death primitives. Use the existing software output as the pixel
oracle, clear both old/new affected tiles, and measure bytes uploaded plus
update/draw/commit cycles. Then extend the composition to player/body/hair,
clouds and dynamic completion text with explicit depth and palette rules.

Verify edge clipping, overlapping effects, opaque black, disappearance,
freeze, shake, completion and restart. Compare the complete native framebuffer
and the same logical input route; measure worst action frames as well as the
median. Only proven call shapes should become eligible for AUTO. If their
resources or semantics cannot be established, retain Mode 4 fallback.

This could preserve pixel effects that the earlier cosmetic Mode 0 prototype
simplified. It does not by itself prove that all frames fit 561,792 cycles:
the latest prototype's 31/162 over-budget route updates still include CPU
update/preparation work that a different display layout alone cannot remove.
