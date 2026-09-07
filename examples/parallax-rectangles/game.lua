phase = 0
player_x = 116

function _init()
  phase = 0
  player_x = 116
end

function _update()
  phase = (phase + 1) % 240
  if btn(0) then
    player_x = max(8, player_x - 2)
  elseif btn(1) then
    player_x = min(224, player_x + 2)
  end
end

function _draw()
  cls(1)

  -- One camera per group: every rectangle in a group moves together.
  local far_x = phase
  local middle_x = (phase * 2) % 240
  local near_x = (phase * 4) % 240

  camera(far_x, 0)
  map(0, flr(far_x / 8), 0, 0, 0, 31, 20, 0)
  camera(middle_x, 0)
  map(1, flr(middle_x / 8), 0, 0, 0, 31, 20, 0)
  camera(near_x, 0)
  map(2, flr(near_x / 8), 0, 0, 0, 31, 20, 0)

  -- The fourth plane is a stationary foreground room.
  camera(0, 0)
  map(3)

  -- The normal Mode 0 renderer uses hardware OBJs for this actor and text.
  spr(5, player_x, 136)
  print("3 SPEEDS / 4 BGS / OBJ", 8, 8, 8)
  print("LEFT / RIGHT TO MOVE", 8, 18, 8)
end
