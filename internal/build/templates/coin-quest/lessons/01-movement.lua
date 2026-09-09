-- COIN QUEST: a first game for mTile.
-- D-pad: move. A (Z in the preview): restart.
player_x = 24
player_y = 72
speed = 2

function move_player()
  if btn(0) then player_x -= speed end
  if btn(1) then player_x += speed end
  if btn(2) then player_y -= speed end
  if btn(3) then player_y += speed end
  player_x = mid(8, player_x, 224)
  player_y = mid(32, player_y, 128)
end

function _update()
  move_player()
end

function _draw()
  cls(24)
  print("COIN QUEST", 8, 8,15)
  spr(1, player_x, player_y)
  print("ARROWS MOVE", 8, 144,15)
end
