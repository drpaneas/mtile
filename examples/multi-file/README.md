# Explicit multi-file Lua

This example keeps cartridge membership explicit. `game.lua` is the only
entry point and requires the two source modules it uses:

```lua
require("player")
require("hud")
```

Mtile resolves those names to `src/player.lua` and `src/hud.lua`, compiles each
file once before `game.lua`, and reports the exact source order in Develop's
Output panel. Adding an unrelated `.lua` file does not change the cartridge
until the entry point (or another required module) requires it.

The example deliberately stores state in two tables:

```lua
local player = {x = 112, y = 72, moved = false}
local hud = {moves = 0}
```

Top-level `local` state remains persistent in Mtile, so the editor reports
`GLOBALS 2 / 256`. Table fields such as `player.x` and `hud.moves` do not each
consume another global slot. Function parameters and locals inside functions
also do not consume global slots.

Run it from the repository root:

```sh
go run ./cmd/mtile edit examples/multi-file/
go run ./cmd/mtile build examples/multi-file/
```

Click the preview and use the arrow keys to move the sprite. The HUD counts
movement updates, demonstrating ordinary calls and shared state across the
explicitly connected source files.
