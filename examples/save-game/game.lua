-- Field layout: 0=x, 1=y, 2=coins, 3=coin x, 4=successful saves.
-- Keep this key and layout together. A changed layout needs a new key.
local player_x = 24
local player_y = 80
local coins = 0
local coin_x = 112
local saves = 0
local status = 0

function new_run()
  player_x = 24
  player_y = 80
  coins = 0
  coin_x = 112
  saves = 0
  status = 3 -- Only RAM changed. The cartridge save is still available.
end

function restore_game()
  status = save_read("mtile-save-game-v1")
  if status == 1 then
    local x = save_get(0)
    local y = save_get(1)
    local n = save_get(2)
    local cx = save_get(3)
    local count = save_get(4)
    -- CRC checks bytes; the game must also check its own field meanings.
    if x < 8 or x > 224 or y < 64 or y > 112 or n < 0 or n > 99 or
       (cx != 64 and cx != 112 and cx != 176) or count < 0 or count > 9999 then
      status = -5
    else
      player_x = x
      player_y = y
      coins = flr(save_get(2))
      coin_x = cx
      saves = flr(save_get(4))
    end
  end
  return status
end

function store_game()
  local next_save = (saves + 1) % 10000
  save_set(0, player_x)
  save_set(1, player_y)
  save_set(2, coins)
  save_set(3, coin_x)
  save_set(4, next_save)
  status = save_write("mtile-save-game-v1")
  local result = status
  if status == 1 then
    saves = (saves + 1) % 10000
    status = 4
  end
  return result
end

function _init()
  pause_menu("store_game", "restore_game")
  restore_game()
end

function _update()
  if btn(0) then player_x = max(8, player_x - 1) end
  if btn(1) then player_x = min(224, player_x + 1) end
  if btn(2) then player_y = max(64, player_y - 1) end
  if btn(3) then player_y = min(112, player_y + 1) end
  if abs(player_x - coin_x) < 7 and abs(player_y - 88) < 7 then
    coins = min(99, coins + 1)
    if coin_x == 176 then coin_x = 64 else coin_x = 176 end
  end
  if btnp(7) then new_run() end
end

function _draw()
  cls(1)
  spr(1, player_x, player_y)
  spr(2, coin_x, 88)
  print("SAVE GAME", 8, 8, 7)
  print("START MENU  SELECT NEW", 8, 24, 7)
  print("MOVE TO COLLECT", 8, 40, 7)
  print("COINS", 8, 56, 7)
  print(flr(coins), 44, 56, 7)
  print("SAVES", 104, 56, 7)
  print(flr(saves), 140, 56, 7)
  if status == 4 then
    print("SAVED", 8, 128, 7)
  elseif status == 1 then
    print("LOADED", 8, 128, 7)
  elseif status == 0 then
    print("NO SAVE YET", 8, 128, 7)
  elseif status == 3 then
    print("NEW RUN - MENU TO SAVE", 8, 128, 7)
  elseif status == -1 then
    print("SAVE ERROR - MENU RETRIES", 8, 128, 7)
  elseif status == -2 then
    print("DAMAGED SAVE", 8, 128, 7)
  else
    print("BAD DATA", 8, 128, 7)
  end
end
