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

coin_x = 112
coin_y = 72
score = 0

function collect_coin()
  -- Both sprites occupy 8x8 boxes. Touching edges do not overlap.
  if abs(player_x - coin_x) < 8 and abs(player_y - coin_y) < 8 then
    score += 1
    -- A predictable route makes the game easy to learn and test.
    coin_x = 36 + (score * 53) % 160
    coin_y = 40 + (score % 2) * 64
  end
end

function _update()
  move_player()
  collect_coin()
end

function _draw()
  cls(24)
  print("COIN QUEST", 8, 8,15)
  print("COINS", 144, 8,15)
  print(score, 200, 8,15)
  spr(2, coin_x, coin_y)
  spr(1, player_x, player_y)
  print("ARROWS MOVE", 8, 144,15)
end
