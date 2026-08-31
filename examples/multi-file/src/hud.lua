-- A second top-level state table consumes the example's second global field.
local hud = {moves = 0}

function hud_reset()
  hud.moves = 0
end

function hud_update(moved)
  if moved then hud.moves += 1 end
end

function hud_draw()
  print("MULTI-FILE", 8, 8, 7)
  print("MOVES", 144, 8, 6)
  print(hud.moves, 190, 8, 7)
  print("ARROWS MOVE", 8, 150, 6)
end
