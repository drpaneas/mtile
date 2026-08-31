# mapdemo

A single-background asset-compatibility example. Its `map.json` is still the
legacy one-plane shape, which is treated as logical BG0 without an asset
migration. `game.lua` uses `map(0, x, y)`: compared with the former positional
API, only the leading BG argument was added and the other optional arguments
retain their order and defaults.

A realistic full-screen `map()` demo: a **4095x20** world (**136 complete
240px screens plus 15 tiles**) redrawn from scratch every tick, with
`camera()` scroll as you move right, plus a playable sprite with gravity,
jumping, and `fget`-based tile collision, plus HUD text. The three authored
screens repeat across the world.

4095 tiles is the maximum useful width of one continuous world with the
current numeric model. Lua numbers are signed 16:16 fixed point, whose largest
positive integer is 32767; 4095 tiles occupy 32760 pixels. A 4096th tile or a
137th complete 240px screen would require coordinates outside that range.
This is a **coordinate limit, not a map-storage limit**. Separate rooms can
reuse local 0..239 coordinates and collectively be much larger.

The on-screen number is `flr(stat(1)*100)`: last tick's real cost. Do not
build it with `".."` concat—that allocates every frame, triggers heap GC, and
makes a latched maximum misleading.

## Measured size and memory

Built with TinyGo 0.41.1, this maximum-width demo produces approximately:

- **195 KB ROM**
- **31,460 bytes EWRAM free**
- **81,900 map cells / 163,800 map bytes in ROM**
- The 240x160 project sheet can expose all **600 8x8 IDs** without a
  sheet-sized EWRAM copy as long as game code does not call `sset`.

The map crossed the 16,384-cell streaming threshold, so increasing its length
no longer consumes EWRAM. The runtime keeps four 32x32 chunks in an 8 KB map
cache and a fixed 32 KB sparse edit overlay. Only visible chunks are copied
from ROM.

The absolute map-data ceiling is therefore normally cartridge ROM: the GBA
has 32 MiB of cartridge space and each map cell takes two bytes. With this
demo's roughly 37 KB of non-map ROM, a single theoretical map could contain
about **16.75 million cells**, or roughly **27,930 native 30x20 screens** if
all other content stayed unchanged. In practice TinyGo currently compiles
very large literal arrays slowly: a 500-screen test built successfully
(~622 KB ROM), while a 1000-screen test hit TinyGo's three-minute interpreter
timeout. Use multiple maps/rooms or improve asset embedding before treating
the cartridge ceiling as a practical single-map target.

Because mapdemo never calls `sset`, its sheet is compacted by 8x8 cell: only
the eight non-empty cells consume pixel storage in ROM/EWRAM, while a
1,200-byte index preserves IDs 0..599. A game that does call `sset` opts into
a complete mutable EWRAM sheet; that is an intentional cost for runtime pixel
editing rather than a cost imposed on every game.

## Rendering cost

Map *storage size* does not determine frame cost. `map()` visits only the
visible 30x20 cells, plus one trailing column/row while sub-tile scrolling.
The cost depends on visible tile density and transparency.

Measured on ARM7TDMI timing: approximately **62% CPU** on the original
single-screen room (352,077 of 561,792 cycles). Large maps take the streamed
cache path; `map()` resolves each visible 32-cell chunk once and then reads
its cells directly, so steady-state work still scales with the visible window,
not the complete 4095-cell width. Re-measure the exact percentage after
runtime changes rather than assuming the old 62% figure.

The sheet demonstrates the three tile cost classes from
[docs/performance.md](../../docs/performance.md):

- **Empty cells** (~82% of this map) cost almost nothing; `cls(47)` supplies
  the sky.
- **Opaque tiles** use the packed fast path.
- **Transparent tiles** pay the per-byte transparency cost.

## Regenerate and build

```sh
python3 examples/mapdemo/gen_map.py
go run ./cmd/mtile build examples/mapdemo/
```

Run `examples/mapdemo/game.gba` in mGBA. Arrow keys move and A (X with mGBA's
default keyboard mapping) jumps. Walking through all 136 screens takes a long
time; increasing the movement speed in `game.lua` is useful for traversal
testing.
