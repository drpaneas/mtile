# Shared background tile animation

This forced-Mode-0 example draws 128 occurrences of one background tile. Each
update changes the first two rows of tile 1 with 16 direct `sset` calls. The
renderer uploads that single 64-byte tile, and all map occurrences animate
together. The rest of the tile stays unchanged.

Run `mtile build examples/shared-tile-animation`, then open the resulting ROM.
The project uses only existing `sset`, `map`, and `cls` semantics. It does not
change AUTO selection or add a runtime optimization.

Use this pattern when every occurrence should have the same animation phase.
Independent phases need separate tile IDs; changing one shared tile cannot
animate individual map occurrences independently. A native counterexample with
half the occurrences out of phase differs by 1,024 pixels per frame.

A paired 120-frame TinyGo/mGBA trial compared this layout with 128 individual
sprite overlays. It preserved every pixel while reducing median frame work
from 52,115 to 5,368 cycles (89.7%) and OAM use from 128 entries to zero. The
tradeoffs were 1,868 additional ROM bytes, 4,176 additional EWRAM bytes, and
231,591 additional cold-start cycles, amortized in about five game ticks.
The final example omits two unused comparison variables; its own native
120-frame capture remains exact at 5,386 median cycles, 14,648 ROM bytes, and
253,012 free EWRAM bytes. Measurements characterize this workload, not every
game or physical-device battery life.

The example occupies one regular background layer. Games that already draw
this map can often avoid most of the extra map-shadow allocation measured
against a sprite-only baseline. Native scanline cost remains a separate limit
from the number of OAM entries when choosing sprite alternatives.
