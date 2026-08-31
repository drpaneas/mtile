# Map asset schema

`assets/map.json` has two supported on-disk shapes. Both use spritesheet cell
IDs; sprite ID 0 is transparent.

## Legacy single plane

The arrays in the following schema examples are abridged for readability.

Projects created before background authoring omit `version` and store BG0 in
`cells`:

```json
{
  "width": 30,
  "height": 20,
  "cells": [0, 0, 0]
}
```

The array must contain exactly `width * height` entries. Loading, viewing,
editing BG0, and saving this shape does not migrate it. It is the logical BG0
plane: `map()` draws it when it is non-empty and `map(0)` selects it directly.
`mget(x,y)` and `mset(x,y,id)` continue to address BG0.

## Version 2: four background planes

The first material editor write to BG1, BG2, or BG3 expands the map to (erasing
an already transparent extra plane remains a no-op):

```json
{
  "version": 2,
  "width": 30,
  "height": 20,
  "layers": [
    {"id": 0, "cells": [0, 0, 0]},
    {"id": 1, "cells": [0, 0, 0]},
    {"id": 2, "cells": [0, 0, 0]},
    {"id": 3, "cells": [0, 0, 0]}
  ]
}
```

Real arrays still contain exactly `width * height` entries. All four IDs are
required exactly once; file order is not semantic. BG0 is composited at the
back and BG3 at the front. All planes share dimensions so camera coordinates,
screen guides, resizing, and layer switching refer to the same world grid.

The expansion is preservation-safe:

- BG0 is copied exactly.
- BG1-BG3 start transparent.
- Undo before saving restores the complete legacy shape.
- Saving a legacy project without an extra-plane edit writes the legacy shape.
- No automatic contraction occurs, so a later save cannot silently discard
  authored BG1-BG3 data. Git/file backup is the disk-level rollback after save.

## Editor behavior

The Map Editor displays a horizontal BG0-to-BG3 toolbar below the canvas: BG0
is the back plane on the left and BG3 is the front plane on the right. Clicking
a background selects the only plane modified by paint, stamp, erase, or resize
operations. Its separate eye control changes only the editor composite; it does
not delete cells or alter runtime visibility. Every project opens with BG0
active; number keys 1–4 select BG0–BG3 while Map Lab has focus.

## Build and runtime layout

The on-disk version-2 schema always contains four planes, but the build scans
their cells and stores only non-empty planes. Their logical IDs remain BG0-BG3;
a plane mask maps those IDs to dense ROM/EWRAM storage. For example, authored
BG1 and BG3 occupy two planes of map storage, not four. A source that calls
`mset()` keeps BG0 allocated even when it starts empty, because it can become
non-empty at runtime.

Mode 0 independently allocates one hardware screen shadow per plane actually
drawn. `map()` draws every stored non-empty plane in BG0-to-BG3 order, so the
same BG1/BG3 example reserves two hardware backgrounds. `map(3)` reserves only
one hardware background while still reading logical BG3. Small maps use a
complete mutable EWRAM copy of the compact data; maps whose compact cell count
crosses the streaming threshold keep ROM authoritative and include the logical
BG ID in each cache key.

The standalone `assetsync` generator has no Lua source to analyze for a future
`mset()`, so it conservatively retains every schema plane. Normal `mtile build`
has the complete source bundle and performs the sparse packing described above.

The Lua forms are:

```lua
map()
map(bg)
map(bg, cell_x, cell_y, screen_x, screen_y, cells_w, cells_h, mask)
```

`map()` draws all authored non-empty planes. For every call with arguments,
the first argument is always the integer-literal logical BG ID 0..3. Every
following argument is optional and retains the established order and default;
for example `map(2, 10, 4)` draws BG2 starting at source cell `(10,4)`.
Migrating an old `map(cell_x, cell_y, ...)` call means prefixing its intended
BG: `map(0, cell_x, cell_y, ...)`. The final `mask` remains a spritesheet-flag
filter, not a background number.

See [Graphics model](graphics-model.md) and
[Render-mode compatibility](render-modes.md) for its ordering and renderer
contract. Stage 2 deliberately leaves `mget`/`mset` on BG0; layer-aware runtime
mutation requires a separately versioned public API decision.
