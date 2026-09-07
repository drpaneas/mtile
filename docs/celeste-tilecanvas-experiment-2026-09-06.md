# Celeste tile-canvas experiment, 2026-09-06

**Historical implementation and measurements, superseded by
[the parallax/OBJ-canvas renderer](celeste-parallax-2026-09-06.md).**
The runtime, palette reservations, unchanged-game assumptions, and command
behavior described below belong to the earlier snapshot. The current opt-in
uses four backgrounds and a generic sparse OBJ canvas; the actual Celeste
project now contains authored grouped clouds. Retained old artifacts remain
useful for comparisons, but rebuilding current source does not reproduce this
historical renderer.

## Scope and reproduction

The private `mtile_tilecanvas` runtime tag retains the current Celeste Lua,
physics, graphics and audio. It changes presentation to three Mode 0 BGs:
8bpp foreground pixels, 4bpp hardware terrain, and 4bpp rear pixels. It does
not enable hardware OBJs yet. Drawing actors into the foreground canvas
preserves their painter order relative to hair, smoke, particles and UI.

This is an engineering experiment, not a new `project.json` renderer value.
Public Mode 0/AUTO builtin classifications and rejection rules remain in force.
The paired harness adds the tag with a temporary TinyGo wrapper. The editor
also has an explicit private opt-in. Neither changes the production project,
Lua builtin registry, public renderer eligibility or build defaults.

To edit the original source and preview this experiment, run from the repository
root:

```sh
go run ./cmd/mtile edit -experimental-tilecanvas examples/celeste
```

Then build/run in the editor with Cmd/Ctrl+R. The flag applies only to this
editor process and selected project; opening another project does not inherit
the experiment. The build log names the experiment and the inspector reports
the actual Mode 0 presentation. The ordinary command without the flag retains
the project's usual renderer policy.

The editor connection was verified through the same native test: its playable
build uses the scoped `build.Service` policy directly, with no TinyGo wrapper.
It produces a byte-for-byte identical ROM to the saved playable artifact and
passes the original input/audio/heap smoke check. The rerun also retained all
280 exact paired frames. Evidence is in
`$TMPDIR/mtile-celeste-tilecanvas-1811722697` and the saved
`build/experiments/tilecanvas/editor-verification.txt` summary.

From the repository root, with TinyGo and the native player available:

```sh
MTILE_TILECANVAS_DIAGNOSTIC=1 go test ./cmd/renderbench \
  -tags=runtimeperfdiagnostic -run '^TestCelesteTileCanvasDiagnostic$' \
  -count=1 -timeout=15m -v
```

The logged evidence directory retains the frozen runtime/project, generated
Go, both ROMs, per-update traces, screenshots and mismatch images. Both builds
use the same deterministic input script and current compiler optimizations.
The baseline retains the existing Mode 4 map cache; the candidate disables
that allocation because terrain is drawn by the GBA background hardware.

## How the renderer works

`cls(0)` starts the rear pixel surface. Clouds/platforms draw directly into
4bpp tile storage: an eight-pixel row is one 32-bit word, with nibble masks for
partial rows. At the room's single `map(1)` call, the runtime preserves the
camera for hardware terrain and starts a transparent foreground surface.
The existing raster functions then draw entities, player/hair, particles and
completion UI. The foreground is converted into 8bpp tile storage.

Pixel surfaces are staged in EWRAM. Conservative occupied-tile row masks cover
each drawing operation. Foreground conversion considers the union of previous
and current coverage, so moved/disappeared pixels are erased. Clears also touch
only previously occupied regions. Dirty runs share a DMA transfer; small clean
gaps are merged when copying the preserved pixels costs less than another DMA
setup. Run descriptors are prepared before VBlank. No VRAM byte writes are used.

The foreground keeps full 8bpp indices. Rear colors use reserved palette bank 4
(cloud 206 and platform 11/14), and compact terrain uses bank 5. Physical index 255
duplicates logical black 0, allowing the completion panel to be opaque while
zero-valued holes remain transparent. Draw-time hair remapping is baked into
pixels before `pal()` resets. These reservations are specific to this scene.

The 64 KiB BG allocation is:

| Region | Byte range | Notes |
| --- | --- | --- |
| Foreground | 0–38,463 | 600 8bpp tiles and one blank tile |
| Rear | 38,464–57,695 | 600 4bpp tiles and one blank tile |
| Terrain | 57,696–58,335 | 19 compact tiles and one blank tile |
| Free alignment space | 58,336–59,391 | 1,056 bytes |
| Screenblocks | 59,392–65,535 | Three 32×32 maps |

Terrain screen entries are initialized directly while the display is blank.
Terrain pixels borrow the not-yet-used foreground backing as temporary DMA
storage, then restore it to zero. This avoids a permanent map shadow and avoids
TinyGo allocating an escaping local DMA buffer under `-gc=none`.

Foreground/rear BG scroll stays zero because their pixels already include
camera clipping. Terrain scroll is captured at the map call, so completion's
later `camera()` reset cannot move the terrain incorrectly. A frozen draw
preserves all committed surfaces and scroll.

There is no second pair of tile canvases in VRAM. A conservative byte/run cost
check happens before visible writes. The current VBlank is used only when its
remaining complete scanlines cover that bound; otherwise presentation waits
for the next VBlank. Target measurements also check the actual upload window.
Commit work is included through `accountPostPaceWork`, rather than being
omitted from the frame-cycle comparison.

The admission estimate is `28000 + 2 * bytes + 192 * DMA_runs` cycles. The
largest measured fixed residual was 24,736 cycles, including a whole scanline
of measurement rounding; the constant adds another 3,264 cycles. This is a
calibrated bound for the tested scene and generated IWRAM loops, not a universal
GBA timing guarantee. A late-commit guard remains active. The maximum accepted
estimate in this trace was 83,744 of the 83,776 cycles in a complete VBlank.

The important speed improvements are sparse clears/conversion, drawing rear
pixels directly into tile storage, coalescing DMA runs before VBlank, and
putting hot packing/upload loops in IWRAM. Palette reset also avoids redundant
256-entry scans; the required transparency scan handles opaque-black encoding
in the same pass. Merely converting two full software surfaces into tiles was
slower than the existing Mode 4 renderer.

## Bounds that remain deliberate

- One 30×20 immutable map plane, one default-sized map call, full map clip,
  identity map palette/key-zero transparency, and camera offsets within 8 pixels.
- At most 19 nonzero terrain IDs and 15 terrain colors.
- Rear raster colors 0/206/11/14; foreground reserves banks 4/5 and index 255.
- At most 64 prepared DMA runs per layer, after merging cheap gaps. A frame
  whose estimated upload exceeds VBlank is rejected before visible writes.
- Rear `sspr()` is explicitly rejected; the current scene does not call it.
- The current room has no active authored platform/entity markers. Host
  fixtures cover rear platform colors, but do not establish arbitrary rooms.
- No general composed `pget()` implementation or arbitrary palette/memory API
  support. This is why the experiment cannot be admitted to public AUTO.

## Verification

Host tests independently decode committed GBA screenblocks, tile data and
palettes, comparing every pixel through camera offsets, overlapping layers,
opaque black, palette reset, disappearance and freeze. Packing tests cover
aligned/unaligned buffers, all tile edges, dirty-bit accumulation, sparse/dense
equivalence, palette edits, zero allocations and pointer checks. Additional
direct-rear tests cover 1,024 partial span cases, overlaps, text and 192 sprite
configurations. Occupancy hooks have 80 isolated drawing cases, so unrelated
overlaps cannot conceal an untracked fast-path write.

The native harness follows the unchanged 162-update completion route, waits
on the real completion screen, presses Start, then triggers a centered death
burst with the game's existing helper and observes normal respawn. It compares
logical gameplay state and whole-frame RGB, records drawing/upload/total work,
checks heap/audio errors, and reads actual DISPCNT after presentation.

## Native results

The paired mGBA run compared 280 logical updates with **zero differing pixels**
and identical gameplay watches. Coverage includes initial spawn, all 162 inputs
of the existing completion route, dash freeze, camera shake, the opaque
completion panel, Start/restart, eight visible death particles and normal
respawn. The death fixture positions the player centrally and calls the real
`kill_player()` helper; it is renderer/lifecycle coverage, not another collision
or original-Celeste physics proof.

The following measurements cover only the same 162-update completion route,
with audio and debug instrumentation enabled in both builds. Percentiles use
the renderbench nearest-rank convention. The CPU allowance is 561,792 cycles
per game update, corresponding to two GBA LCD frames (approximately 30 Hz).

| Measurement | Current Mode 4 | Private tile canvas |
| --- | ---: | ---: |
| Total work p50 | 587,900 cycles | 542,226 cycles |
| Total work p95 | 743,270 cycles | 680,315 cycles |
| Total work maximum | 819,260 cycles | 734,737 cycles |
| Updates exceeding CPU allowance | 111 / 162 | 58 / 162 |
| LCD frames for the route | 440 | 401 |
| Observed update cadence | 21.99 Hz | 24.13 Hz |

This reduces median work by 7.8% and improves actual update cadence by 9.7%.
It **does not achieve stable 30 Hz**. The remaining cost includes software
foreground drawing, packing and game update work. Hardware sprites are a
possible subsequent experiment, with painter-order parity still required.
The gain is not universal: completion-screen median work rises from 625,660
to 637,552 cycles because its large opaque panel increases canvas preparation.
Initial terrain construction and Start reload also remain expensive one-time
updates; the full phase breakdown is saved with the evidence.

Every actual candidate upload started at or after scanline 160 and finished
by scanline 215, inside VBlank. The largest payload was 22,400 bytes and the
largest combined DMA run count was 63. Actual DISPCNT was `0x0700`, confirming
Mode 0 with the three BGs enabled. `LastBlit` in the traces includes preparation
as well as commit; it is not the duration of the VBlank upload alone.

Host verification passed for the default runtime, public Mode 0 runtime and
focused private tile-canvas fixtures with `checkptr=2`. The required transpiler,
build and CLI suites also passed. These are native emulator measurements;
physical GBA testing remains outstanding.

The separate playable ROM uses the original Lua and normal joypad input, with
no route/probe injection. After 170 LCD frames of startup, it passed a further
120 LCD frames of input smoke testing with active sound and no heap or audio
overflow errors. It occupies 379,256 ROM bytes, 31,180 / 32,672 usable IWRAM
bytes, and leaves 4,432 EWRAM bytes beyond its static allocations, including
the reserved 56 KiB Lua heap. This is tight memory headroom; the experiment
should not become a general backend without further memory work.

| Linked comparison build | ROM bytes | IWRAM used | Application EWRAM free |
| --- | ---: | ---: | ---: |
| Instrumented Mode 4 | 392,044 | 31,548 | 16,264 |
| Instrumented tile canvas | 395,048 | 31,160 | 4,312 |

The candidate consumes 11,952 more EWRAM bytes despite removing the Mode 4
map cache. Both builds retain the same 56 KiB Lua heap reservation.

## Saved artifacts

The final test passed using the evidence directory
`$TMPDIR/mtile-celeste-tilecanvas-967503687`. Selected artifacts are copied to
the ignored `build/experiments/tilecanvas` directory in this checkout:

- [Playable ROM](../build/experiments/tilecanvas/celeste-tilecanvas-playable.gba)
  uses the original game source and normal controls.
- `mode4.gba` and `tilecanvas.gba` are the deterministic comparison ROMs.
- `mode4-traces.json` and `tilecanvas-traces.json` retain per-update state,
  cycle counts, framebuffer hashes, scanline windows and upload statistics.
- Matching screenshots cover spawn, route, freeze, shake, completion, restart,
  death and respawn. `playable-smoke-test.png` comes from the separate ROM.
- `manifest.json` records artifact hashes and the frozen runtime/Lua hashes.
- [Full measurements](../build/experiments/tilecanvas/RESULTS.md) include phase
  timing and linked memory; `verification-result.txt` records the final PASS.

These generated files are local evidence, not committed source. The opt-in
test above regenerates them in a fresh logged temporary directory.
