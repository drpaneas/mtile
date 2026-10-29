-- Three independent player files, inspired by Link's Awakening's file flow.
-- Slots 0..2. Key/layout: 0=present, 1=name length, 2..6=ASCII name,
-- 7=x, 8=y, 9=coins, 10=coin x, 11=successful saves. Other fields are zero.
local mode = 0 -- 0 files, 1 name, 2 play, 3 game menu, 4 wait, 5 erase confirmation
local selected = 0
local active_slot = 0
local status = 0
local file_states = {0,0,0}
local file_names = {"","",""}
local file_coins = {0,0,0}
local hero_name = ""
local name_cursor = 0 -- A..Z, then END
local menu_row = 0
local player_x = 24
local player_y = 88
local coins = 0
local coin_x = 112
local saves = 0
local after_wait = 0
local held_a = false
local held_b = false
local held_select = false
local held_start = false

function valid_record()
  if save_get(0) == 0 then return true end -- committed empty file (erase)
  if save_get(0) != 1 then return false end
  local n = save_get(1)
  if n != flr(n) or n < 1 or n > 5 then return false end
  for i=1,n do
    local code = save_get(i+1)
    if code != flr(code) or code < 65 or code > 90 then return false end
  end
  local x,y,c,cx,s = save_get(7),save_get(8),save_get(9),save_get(10),save_get(11)
  return x >= 8 and x <= 224 and y >= 64 and y <= 112 and
    c == flr(c) and c >= 0 and c <= 99 and
    (cx == 64 or cx == 112 or cx == 176) and
    s == flr(s) and s >= 0 and s <= 9999
end

function read_file(slot)
  local result = save_read("mtile-three-files-v1",slot)
  if result != 1 then return result end
  if not valid_record() then return -6 end
  if save_get(0) == 0 then return 0 end
  return 1
end

function staged_name()
  local name = ""
  for i=1,save_get(1) do name = name .. chr(save_get(i+1)) end
  return name
end

function refresh_files()
  for slot=0,2 do
    local result = read_file(slot)
    file_states[slot+1] = result
    file_names[slot+1] = ""
    file_coins[slot+1] = 0
    if result == 1 then
      file_names[slot+1] = staged_name()
      file_coins[slot+1] = save_get(9)
    end
  end
end

function restore_game()
  local result = read_file(active_slot)
  status = result
  if result == 1 then
    -- Apply only after validating every field. Failed loads preserve the run.
    hero_name = staged_name()
    player_x,player_y = save_get(7),save_get(8)
    coins,coin_x,saves = save_get(9),save_get(10),save_get(11)
  end
  return result
end

function store_game()
  local count = (saves+1)%10000
  for i=0,63 do save_set(i,0) end
  save_set(0,1)
  save_set(1,#hero_name)
  for i=1,#hero_name do save_set(i+1,ord(hero_name,i)) end
  save_set(7,player_x) save_set(8,player_y) save_set(9,coins)
  save_set(10,coin_x) save_set(11,count)
  status = save_write("mtile-three-files-v1",active_slot)
  if status == 1 then saves = count end
  return status
end

function begin_wait(destination)
  after_wait = destination
  mode = 4
end

function show_files()
  refresh_files()
  mode = 0
end

function _init()
  refresh_files()
end

function _update()
  -- Actions use edges, not btnp's held-button repeat: one press = one write.
  local a,b,select,start = btn(5),btn(4),btn(7),btn(6)
  local tap_a,tap_b,tap_select = a and not held_a,b and not held_b,select and not held_select
  local tap_start = start and not held_start
  held_a,held_b,held_select,held_start = a,b,select,start
  if mode == 4 then
    if not save_pending() then
      if after_wait == 0 then show_files() else mode = 2 end
    end
  elseif mode == 0 then
    if btnp(2) then selected = (selected+2)%3 end
    if btnp(3) then selected = (selected+1)%3 end
    if tap_a or tap_start then
      active_slot = selected
      if file_states[selected+1] == 0 then
        hero_name = "" name_cursor = 0 status = 0 mode = 1
      elseif file_states[selected+1] == 1 then
        if restore_game() == 1 then mode = 2 end
      else
        status = file_states[selected+1] -- Never silently replace damaged data.
      end
    elseif tap_select and file_states[selected+1] != 0 then
      mode = 5
    end
  elseif mode == 1 then
    if btnp(0) then name_cursor = (name_cursor+26)%27 end
    if btnp(1) then name_cursor = (name_cursor+1)%27 end
    if btnp(2) then name_cursor = (name_cursor+18)%27 end
    if btnp(3) then name_cursor = (name_cursor+9)%27 end
    if tap_b then
      if #hero_name > 0 then hero_name = sub(hero_name,1,#hero_name-1) else show_files() end
    elseif tap_a or tap_start then
      if name_cursor == 26 or tap_start then
        if #hero_name > 0 then
          player_x,player_y,coins,coin_x,saves = 24,88,0,112,0
          if store_game() == 1 then begin_wait(0) end
        end
      elseif #hero_name < 5 then
        hero_name = hero_name .. chr(65+name_cursor)
      end
    end
  elseif mode == 2 then
    if btn(0) then player_x = max(8,player_x-1) end
    if btn(1) then player_x = min(224,player_x+1) end
    if btn(2) then player_y = max(64,player_y-1) end
    if btn(3) then player_y = min(112,player_y+1) end
    if abs(player_x-coin_x) < 7 and abs(player_y-88) < 7 then
      coins = min(99,coins+1)
      if coin_x == 176 then coin_x = 64 else coin_x = 176 end
    end
    if tap_select then menu_row = 0 mode = 3 status = 0 end
  elseif mode == 3 then
    if btnp(2) then menu_row = (menu_row+3)%4 end
    if btnp(3) then menu_row = (menu_row+1)%4 end
    if tap_b or tap_start then mode = 2 end
    if tap_a then
      if menu_row == 0 or menu_row == 1 then
        if store_game() == 1 then
          if menu_row == 0 then begin_wait(1) else begin_wait(0) end
        end
      elseif menu_row == 2 then
        if restore_game() == 1 then mode = 2 end
      else
        show_files() -- Explicitly leave without writing unsaved changes.
      end
    end
  elseif mode == 5 then
    if tap_b then mode = 0 end
    if tap_a then
      -- A valid empty record supersedes both older copies. Erase is atomic too.
      for i=0,63 do save_set(i,0) end
      status = save_write("mtile-three-files-v1",selected)
      if status == 1 then begin_wait(0) end
    end
  end
  -- Only gameplay owns Start for the fixed system pause. Menus keep Start.
  pause(mode == 2)
end

function _draw()
  cls(1)
  if mode == 0 then
    print("PLAYER FILES",72,12,7)
    for slot=0,2 do
      local y = 40+slot*24
      if selected == slot then print(">",24,y,10) end
      print(slot+1,42,y,6)
      if file_states[slot+1] == 1 then
        print(file_names[slot+1],66,y,7)
        print(file_coins[slot+1],174,y,10)
      elseif file_states[slot+1] == 0 then
        print("NEW GAME",66,y,6)
      else
        print("UNREADABLE",66,y,8)
      end
    end
    print("A/START OPEN  SELECT ERASE",42,120,6)
  elseif mode == 1 then
    print("NAME YOUR HERO",66,10,7)
    print(hero_name .. "_",102,28,10)
    for cell=0,26 do
      local x,y = 24+(cell%9)*22,56+flr(cell/9)*18
      if cell == name_cursor then print(">",x-6,y,10) end
      if cell == 26 then print("END",x,y,7) else print(chr(65+cell),x,y,7) end
    end
    print("A LETTER  B BACKSPACE",54,120,6)
    print("START / END TO REGISTER",48,132,6)
  elseif mode == 2 then
    print(hero_name,12,12,7)
    print("FILE",126,12,6) print(active_slot+1,156,12,7)
    print("COINS",12,32,6) print(coins,48,32,10)
    print("SAVES",126,32,6) print(saves,162,32,7)
    spr(1,player_x,player_y) spr(2,coin_x,88)
    print("SELECT  SAVE / LOAD",60,134,6)
  elseif mode == 3 then
    print(hero_name,102,12,7)
    print("SAVE AND CONTINUE",54,40,7)
    print("SAVE AND QUIT",54,58,7)
    print("LOAD LAST SAVE",54,76,7)
    print("QUIT WITHOUT SAVING",54,94,6)
    print(">",36,40+menu_row*18,10)
    print("A CHOOSE   B RETURN",60,120,6)
  elseif mode == 4 then
    print("SAVED",102,56,7)
    print("PLEASE WAIT",84,76,10)
    print("KEEP POWER ON",78,96,6)
  elseif mode == 5 then
    print("ERASE THIS FILE?",72,48,8)
    print("ONLY THIS FILE IS REPLACED",42,70,6)
    print("A ERASE   B CANCEL",66,98,7)
  end
  if status < 0 then print("SAVE / LOAD FAILED - RETRY",42,148,8) end
end
