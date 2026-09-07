# Three parallax speeds, four backgrounds, and hardware objects

Open this editable example from the repository root:

```sh
go run ./cmd/mtile edit examples/parallax-rectangles
```

Build/run in the editor with Cmd/Ctrl+R. Left/right moves the small placeholder
actor. This is a rendering demonstration, not another Celeste port; the
original Celeste project is unchanged.

The example uses the existing public `map`, `camera`, `spr`, and `print` APIs
and the ordinary `mode0` renderer. It needs no experimental tile-canvas flag,
game-name checks, reserved Celeste colors, or new Lua builtins.

| Logical plane | Content | Motion per game update |
| --- | --- | --- |
| BG0 | Far rectangles | 1 pixel left |
| BG1 | Middle rectangles | 2 pixels left |
| BG2 | Near rectangles | 4 pixels left |
| BG3 | Foreground room and platforms | Stationary |
| OBJs | Actor and HUD glyphs | Actor follows input |

Each rectangle is authored once from repeated solid tiles in Map Lab. The
background hardware then displays those tiles. Every `map()` pass captures
its own current camera. The runtime changes BG scroll registers for sub-tile
movement; when the visible source window crosses a tile boundary, it updates
the corresponding hardware tilemap. It does not redraw the rectangles into a
software pixel buffer each frame.

The planes are 60x20 cells: the right half duplicates the left half. A
31-column source window can therefore cross the 240-pixel motif boundary
without a seam. The moving map passes intentionally cover all 20 rows and
their tile-boundary updates can coincide. The performance test does not hide
that worst case with staggered camera phases or reduced vertical bounds.

Mtile's logical BG0-to-BG3 order describes back-to-front painter order. The
renderer assigns hardware priority values 3, 2, 1, 0 to implement that order;
a hardware background number alone does not determine depth. The actor and
text are drawn after all map passes, so their OBJs appear in front.

## What this changes compared with Celeste's background

Celeste currently has 17 rectangles, each with its own random speed and with
a new random vertical position when it wraps. Three scroll registers cannot
represent 17 independent trajectories exactly. Grouping the art into three
fixed motifs is a deliberate visual adaptation. It keeps parallax motion,
but it is not a pixel-identical replacement for those original clouds.

This example demonstrates the reusable grouped approach without changing
Celeste's art, gameplay, or physics. Choosing the grouped style for Celeste
would be a separate game-design change.

## Verification and measured cost

The paired native mGBA benchmark uses exactly the same project in Mode 4 and
Mode 0. On 2026-09-06, all 260 captured game updates matched pixel for pixel,
including all three wrap periods. Host tests also check four-plane/OBJ/text
renderer selection and the duplicated source motif.

| Renderer/runtime | Median cycles | 95th percentile cycles | Missed game updates during capture |
| --- | ---: | ---: | ---: |
| Mode 4 reference | 566,179 | 575,391 | 190 |
| Mode 0 before map-loop optimization | 66,191 | 900,512 | 71 |
| Mode 0 after map-loop optimization | 66,111 | 231,345 | 0 |

The generic Mode 0 map-loop optimization removes repeated source-plane and
storage checks from each cell, uses clipped row traversal, and clears the
aligned tilemap shadow with DMA. It changes mtile itself and benefits other
eligible games. No example coordinates or palette colors enter that code.
The identical full-height example and identical Mode 4 ROM were retained for
the before/after measurement. The 95th percentile dropped by 74.3%; the final
median is 88.3% below the Mode 4 reference. These are measurements of this
small scene, not a forecast for the complete Celeste port.

The normal game loop has a two-LCD-frame budget of 561,792 cycles. Startup
uploads are separate from these steady-state samples. Mode 0 uses 20,560 fewer
EWRAM bytes in this fixture. The benchmark's broader promotion gate requires
at least 24 KiB saved and no missed updates in either renderer, so this
evidence intentionally uses the pixel-parity gate rather than claiming that
the full promotion gate passes.

Reproduce the uninstrumented pixel comparison and timing:

```sh
go run ./cmd/renderbench \
  -project examples/parallax-rectangles \
  -samples 260 -gate parity \
  -out /tmp/mtile-parallax-rectangles
```

The separate native diagnostic reads actual DISPCNT, BG priority registers,
and OAM after presentation. GBA scroll registers are write-only, so the pixel
comparison verifies the visible scroll result instead of reading them back.
The probe exists only in a temporary generated build, never in the editable
example:

```sh
MTILE_PARALLAX_DIAGNOSTIC=1 go test ./cmd/renderbench \
  -tags=runtimeperfdiagnostic \
  -run '^TestParallaxRectanglesHardwareDiagnostic$' \
  -count=1 -timeout=5m -v
```

Regenerate the small original assets, or edit them directly in Sprite/Map Lab:

```sh
python3 examples/parallax-rectangles/gen_assets.py
```
