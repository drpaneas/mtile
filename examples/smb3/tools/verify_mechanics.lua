-- Run from the example directory after importing the ROM data.
flr, abs, min, max = math.floor, math.abs, math.min, math.max
function mid(a, b, c) return max(a, min(b, c)) end
function band(a, b) return a & b end
function ord(text, index) return string.byte(text, index) end
local held = -1
function btn(button) return button == held end
function btnp(button) return btn(button) end
function pause() end
function sfx() end
local current_song
function music(song) current_song = song end
function pal() end

local background = {}
local hud_writes = 0
function mset(layer, x, y, tile)
  if layer == 1 then hud_writes = hud_writes + 1 return end
  assert(layer == 0, "expected a terrain or HUD tile edit")
  background[y * 512 + x] = tile
end

dofile("game.lua")
_init()

local awarded_coins = 0
local powerups = 0
local points = 0
function game_add_coin() awarded_coins = awarded_coins + 1 end
function game_add_score(amount) points = points + amount end
local item_slot_available = true
function actor_spawn_powerup()
  if not item_slot_available then return false end
  powerups = powerups + 1
  return true
end

local function find_cell(test)
  for cy = 0, course_height - 1 do
    for cx = 0, course_columns - 1 do
      local tile = level_metatile(cx, cy)
      if test(tile) then return cx, cy, tile end
    end
  end
  error("test fixture missing from World 1-1")
end

local function graphic(cx, cy)
  local address = (cy * 2 + course_map_row) * 512 + cx * 2
  return {background[address], background[address + 1],
          background[address + 512], background[address + 513]}
end

local function same_tiles(left, right)
  for i = 1, 4 do
    if left[i] ~= right[i] then return false end
  end
  return true
end

local cx, cy, active = find_cell(function(tile) return course_block_actions[tile + 1] == 2 end)
level_bump(cx * 16 + 8, cy * 16 + 15)
assert(awarded_coins == 1 and level_metatile(cx, cy) == 0x5f)
assert(same_tiles(graphic(cx, cy), course_tile_art[0x5f + 1]))
assert(not same_tiles(graphic(cx, cy), course_tile_art[active + 1]))
assert(level_solid(cx * 16 + 8, cy * 16 + 8), "a spent block remains solid")
assert(not level_shell_hit(cx * 16 + 8, cy * 16 + 8))
level_bump(cx * 16 + 8, cy * 16 + 15)
assert(awarded_coins == 1, "a spent block cannot pay again")
level_restore_tiles()
assert(level_metatile(cx, cy) == active)
assert(same_tiles(graphic(cx, cy), course_tile_art[active + 1]))

assert(level_shell_hit(cx * 16 + 8, cy * 16 + 8), "shell can hit a question block")
assert(awarded_coins == 2 and level_metatile(cx, cy) == 0x5f)

cx, cy = find_cell(function(tile) return (course_tile_flags[tile + 1] & 8) ~= 0 end)
assert(level_shell_hit(cx * 16 + 8, cy * 16 + 8), "shell can pick up a loose coin")
assert(awarded_coins == 3 and level_metatile(cx, cy) == 0x41)
assert(not level_shell_hit(cx * 16 + 8, cy * 16 + 8))

cx, cy, active = find_cell(function(tile) return course_block_actions[tile + 1] == 3 end)
item_slot_available = false
assert(not level_shell_hit(cx * 16 + 8, cy * 16 + 8))
assert(level_metatile(cx, cy) == active and powerups == 0, "keep the block if no item can spawn")
item_slot_available = true
assert(level_shell_hit(cx * 16 + 8, cy * 16 + 8) and powerups == 1)
assert(not level_shell_hit(cx * 16 + 8, cy * 16 + 8) and powerups == 1)

cx, cy = find_cell(function(tile) return course_block_actions[tile + 1] == 1 end)
assert(level_shell_hit(cx * 16 + 8, cy * 16 + 8), "shell can break a brick")
assert(points == 50 and not level_solid(cx * 16 + 8, cy * 16 + 8))

local function upvalue(fn, wanted)
  for index = 1, 100 do
    local name, value = debug.getupvalue(fn, index)
    assert(name, "missing game state " .. wanted)
    if name == wanted then return value end
  end
  error("missing game state " .. wanted)
end

local function camera_y() return upvalue(smb3_camera_follow, "cam_y") end

local ground_y = player_y
local initial_camera = camera_y()
player_grounded = false
player_y = ground_y - 112
smb3_camera_follow()
assert(camera_y() == initial_camera - 3, "airborne scroll respects the upward speed limit")

-- Landing on a high platform changes the camera target by many pixels at once.
player_grounded = true
for _ = 1, 42 do
  local previous_camera = camera_y()
  smb3_camera_follow()
  assert(previous_camera - camera_y() <= 3, "high landing cannot snap the camera up")
end
assert(camera_y() == initial_camera - 112)
player_y = ground_y
for _ = 1, 29 do
  local previous_camera = camera_y()
  smb3_camera_follow()
  assert(camera_y() - previous_camera <= 4, "downward scroll is also bounded")
end
assert(camera_y() == initial_camera, "return to ground restores the framing")
assert(player_y + player_height - camera_y() == 118, "floor appears above HUD")

hud_writes = 0
local last_time = smb3_hud_digits(art_hud_timer, 300, -1)
assert(hud_writes == 3 and last_time == 300)
last_time = smb3_hud_digits(art_hud_timer, 300, last_time)
assert(hud_writes == 3, "unchanged HUD digits must not touch the map cache")
last_time = smb3_hud_digits(art_hud_timer, 299, last_time)
assert(hud_writes == 6 and last_time == 299)
smb3_hud_digits(art_hud_timer, 298, last_time)
assert(hud_writes == 7, "only the changed HUD digit should write")

-- Open the course through its normal map controls, then reproduce a death.
for _, button in ipairs({6, 1, 2, 5}) do
  held = button
  _update()
end
assert(upvalue(_update, "stage") == 2)
player_die()
held = -1
_update()
assert(current_song == 4 and upvalue(_update, "stage") == 3)

for elapsed = 1, art_death_jingle_updates + 1 do
  -- Start must not cut the death music short.
  held = elapsed == 2 and 6 or -1
  _update()
  assert(current_song == 4 and upvalue(_update, "stage") == 3)
end
assert(upvalue(smb3_fade_to, "screen_fade") == 4, "fade reaches black before the scene changes")
_update()
assert(current_song == 1 and upvalue(_update, "stage") == 1)
for _ = 1, 12 do _update() end
assert(upvalue(smb3_fade_to, "screen_fade") == 0, "the map fades back in")

print("PASS: blocks, shell hits, bounded camera, complete death jingle and fade")
