# Windy Meadow: tile links

Flowers sway and lamps glow along a scrolling meadow, yet `game.lua` never
draws them. Each one was painted in Map Lab as a shared tile. Two of those
tiles are **linked**: the flower tile to the `sway` clip and the lamp tile to
the `lamp` pose. `map()` draws the linked object at every occurrence.

```sh
go run ./cmd/mtile edit examples/tile-links
```

**Controls:** left/right scroll; A picks the flower under the cursor.

## Look at the links

1. **Sprite Lab → Shared tiles:** select tile 4, the pink flower. The
   **SPRITE FLAGS · LINK** panel's header reads *TILE 4 -> CLIP sway*. Tile 5,
   the lamp, reads *TILE 5 -> POSE 3*. Linked tiles have amber outlines.
2. **Sprite Lab → Poses:** each object is painted upright on the OBJ 1 page:
   three flower frames, two tiles tall, and a lamp three tiles tall. Each pose
   was registered by selecting its column and pressing **REGISTER POSE**; the
   `sway` clip plays them in the order still, left, still, right.
3. **Map Lab:** every linked cell is outlined. Hover one to see what it draws.
4. **Develop → Build + Reload:** the flowers sway in the wind.

**Checkpoint:** paint more flower tiles in Map Lab, then rebuild. Every new
flower sways without any Lua change.

## Change one link

Select tile 4 in Shared tiles. Because it is linked, only **UNLINK** is
enabled; click it, and rebuild: the map draws the flower tile itself. Now
**LINK** and the selector are enabled. Press **>** until the selector shows
*POSE 0 left*, click **LINK**, and rebuild: every flower leans left and holds
still. Undo restores the original link.

## The gameplay code

`mget` still sees the linked tile, so the game can find a flower, and `mset`
removes it:

```lua
if mget(x, FLOWER_ROW) == FLOWER then
  mset(x, FLOWER_ROW, 0) -- replacing the tile removes its flower
  picked += 1
end
```

The project uses forced Mode 0. Linked objects become hardware objects: the
build counts the most flowers and lamps any screen can show and reserves that
many pose draws (75 of 128 object entries, including the cursor and text).
AUTO would keep this game on Mode 4 only because its `mset` is conditional.

Read the [tile links guide](../../docs/tile-links.md) for the exact rules,
Mode 0 limits, and measurements. `generate_assets.py` recreates the artwork,
map, and links.
