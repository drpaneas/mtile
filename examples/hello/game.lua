-- A small native GBA cartridge.
local player_x = 112
local player_y = 72

function _init()
  player_x = 112
  player_y = 72
end

function _update()
  if btn(0) then player_x = max(0, player_x - 1) end
  if btn(1) then player_x = min(232, player_x + 1) end
  if btn(2) then player_y = max(16, player_y - 1) end
  if btn(3) then player_y = min(136, player_y + 1) end
end

function _draw()
  cls(15)
  map()
  spr(1, player_x, player_y)
  print("HELLO GBA", 8, 8, 7)
  local hint = 6
  if btn(0) or btn(1) or btn(2) or btn(3) then hint = 40 end
  print("ARROWS MOVE", 8, 150, hint)
end
