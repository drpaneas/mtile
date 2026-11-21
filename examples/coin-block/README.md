# Stateful coin block

This example is the smallest current-API answer to an animated background tile
which later changes state. It deliberately adds no tile types, entities,
background API, or second map plane.

The block is one ordinary map cell. While active, one unconditional `mset()`
alternates that cell between tile IDs 4 and 5. When the player hits it, the
same cell becomes the stable used-block tile 6 and one coin appears. Other map
cells are untouched.

Controls:

- **A**: jump into the block and award its coin;
- **B**: reset the example; and
- no input: the deterministic teaching demo hits the block after three seconds,
  allowing automated Mode 4/Mode 0 captures to observe the transition.

The draw path remains immediate and renderer-neutral:

```lua
map()
spr(3,112,player_y)
spr(7,coin_x,coin_y)
```

`map()` reads the shared logical sheet as background art. The two `spr()` calls
read the same sheet as object art. The renderer does not infer a tile type from
the assets; the Lua state machine chooses the map tile ID explicitly.

The mutation is intentionally a direct statement in `_update()`:

```lua
mset(block_x,block_y,tile)
```

That call shape is within the measured `tile-v1` AUTO profile. AUTO can select
Mode 0, where only the affected visible map entry is patched, while Mode 4
redraws the framebuffer with the same logical cell value.

## Run and edit

From the folder containing the [downloaded examples](../README.md#run-an-example), run:

```sh
mtile edit examples/coin-block/
mtile build examples/coin-block/
```

Regenerate the deterministic assets with:

```sh
python3 examples/coin-block/gen_assets.py
```

## Verify and benchmark (source checkout)

This optional benchmark uses Go and the native player helper.

The paired renderer benchmark uses the zero-input auto-demo, so it includes
the active animation, the one-time transition, and the stable used state:

```sh
go run ./cmd/renderbench \
  -project examples/coin-block \
  -player build/player/mtile-player \
  -warmup 20 -samples 120
```

Exact pixels remain the semantic gate. Cycle, missed-frame, cold-upload, ROM,
and EWRAM results decide whether the Mode 0 trade-off remains worthwhile; they
must not be inferred from the fact that the example merely builds.
