-- The entry point owns the cartridge callbacks and explicitly selects every
-- Lua source unit that belongs to the game.
require("player")
require("hud")

function _init()
  player_reset()
  hud_reset()
end

function _update()
  player_update()
  hud_update(player_moved())
end

function _draw()
  cls(15)
  map()
  player_draw()
  hud_draw()
end
