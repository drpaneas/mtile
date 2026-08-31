# Background layers and parallax

This project contains four independently authored backgrounds and draws each
one with its own camera to produce continuous parallax:

| Call | Logical plane | Content | Camera speed |
| --- | --- | --- | --- |
| `map(0, ...)` | BG0 | stars and moon | `cam_x / 8` |
| `map(1, ...)` | BG1 | distant mountains | `cam_x / 4` |
| `map(2, ...)` | BG2 | nearer hills | `cam_x / 2` |
| `map(3, ...)` | BG3 | foreground ground | `cam_x` |

All four planes are independent 60x20 arrays in one version-2 `map.json`.
The Map Editor exposes them as a horizontal BG0-BG3 toolbar below the canvas:
select a layer button to paint it and use its eye control to hide or show it in
the editor composite. The two horizontal
screens are identical copies of one 30-tile motif, which makes the source
seamlessly repeat. Each call streams only the 31 columns needed around its
camera:

```lua
map(0,flr(sky_cam/8),       0,0,0,31,12,0)
map(1,flr(mountains_cam/8), 9,0,0,31,11,0)
map(2,flr(hills_cam/8),    13,0,0,31, 7,0)
map(3,flr(ground_cam/8),   17,0,0,31, 3,0)
```

## Explore the planes in Map Lab

Open the example from the repository root:

```sh
go run ./cmd/mtile edit examples/background-layers
```

Choose **MAP**. BG0 is the active editing layer by default. The bottom toolbar
runs from BG0/back on the left to BG3/front on the right, reports `ACTIVE BG0`,
and keys 1–4 select BG0–BG3 while Map Lab has focus.

Use the eyes to leave one plane visible at a time:

- BG0 contains only the stars, moon, and small clouds.
- BG1 contains the distant purple mountains.
- BG2 contains the nearer green hills.
- BG3 contains only the foreground floor.

Turn the eyes back on from BG0 through BG3 to see the same composite produced
by a bare `map()`. Eye state is preview-only; it never deletes map cells or
changes runtime visibility.

The nonzero source Y values skip transparent rows above each silhouette while
preserving its authored screen height.

The same spritesheet remains available to both map tiles and `spr()` objects.

The assets use Sprite Lab's native 240x160 sheet (30x20 editable 8x8 slots)
and a real 256-entry palette. Only the first eleven sprites are authored; the
remaining slots and colors are intentionally available for editing.

The explicit form requires static, unique BG IDs in back-to-front order. Every
call with arguments starts with that BG ID; all remaining arguments are still
optional. A bare `map()` instead draws all authored non-empty planes using one
shared current camera. Empty planes are omitted from map storage and Mode 0
hardware allocation.

## Build

From the repository root:

```sh
go run ./cmd/mtile build examples/background-layers/
```

The project requests `mode0`. To prove that the same logical calls retain
their painter-order result in the compatibility renderer:

```sh
go run ./cmd/mtile build \
  -renderer mode4 \
  -o /tmp/background-layers-mode4.gba \
  examples/background-layers/
```

That Mode 4 override is the semantic reference. Both renderers draw the same
four logical planes in BG0-to-BG3 order; Mode 0 assigns them to its four
hardware backgrounds. The checked-in four-pass benchmark is the parity and
performance evidence for this parallax form.

Regenerate the authored assets with:

```sh
python3 examples/background-layers/gen_assets.py
```

For the complete contract and limitations, see
[`docs/graphics-model.md`](../../docs/graphics-model.md).
