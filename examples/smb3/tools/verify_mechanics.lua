-- Run from the example directory after importing the ROM data.
flr, abs, min, max = math.floor, math.abs, math.min, math.max
function mid(a, b, c) return max(a, min(b, c)) end
function band(a, b) return a & b end
function ord(text, index) return string.byte(text, index) end
function btn() return false end
function pause() end
function sfx() end

local background = {}
function mset(layer, x, y, tile)
  assert(layer == 0, "expected a terrain tile edit")
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

local function camera_y()
  local i = 1
  while true do
    local name, value = debug.getupvalue(smb3_camera_follow, i)
    assert(name, "no vertical camera state")
    if name == "cam_y" then return value end
    i = i + 1
  end
end

local ground_y = player_y
local initial_camera = camera_y()
player_grounded = false
player_y = ground_y - 112
smb3_camera_follow()
assert(camera_y() < initial_camera, "a high jump scrolls up")
player_y = ground_y - 16
smb3_camera_follow()
player_grounded = true
player_y = ground_y
smb3_camera_follow()
assert(camera_y() == initial_camera, "landing restores the ground framing")
assert(player_y + player_height - camera_y() == 118, "floor appears above HUD")

print("PASS: spent blocks, shell hits and coins, item release, ground framing")
