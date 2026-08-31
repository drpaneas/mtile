-- One top-level state table consumes one persistent global field. Its x, y,
-- and moved entries are table fields, not three additional globals.
local player = {x = 112, y = 72, moved = false}

function player_reset()
  player.x = 112
  player.y = 72
  player.moved = false
end

function player_update()
  player.moved = false
  if btn(0) then player.x = max(0, player.x - 1) player.moved = true end
  if btn(1) then player.x = min(232, player.x + 1) player.moved = true end
  if btn(2) then player.y = max(16, player.y - 1) player.moved = true end
  if btn(3) then player.y = min(136, player.y + 1) player.moved = true end
end

function player_moved()
  return player.moved
end

function player_draw()
  spr(1, player.x, player.y)
end
