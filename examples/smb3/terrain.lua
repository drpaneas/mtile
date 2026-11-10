-- The importer keeps the original 16 px collision map separate from the 8 px
-- display tiles. A map edit changes both views of a block in the same update.
local changes = {}
local changes_count = 0
local changed_tiles = {}

function terrain_hex_digit(byte)
  if byte >= 97 then return byte - 87 end
  return byte - 48
end

function terrain_cell_key(cx, cy)
  return cy * course_columns + cx + 1
end

function level_metatile(cx, cy)
  if cx < 0 or cx >= course_columns or cy < 0 or cy >= course_height then return 0 end
  local replacement = changed_tiles[terrain_cell_key(cx, cy)]
  if replacement ~= nil then return replacement end
  local row = course_rows[cy + 1]
  local index = cx * 2 + 1
  return 16 * terrain_hex_digit(ord(row, index)) + terrain_hex_digit(ord(row, index + 1))
end

function terrain_tile_at(x, y)
  return level_metatile(flr(x / 16), flr(y / 16))
end

function level_solid(x, y)
  if x < 0 or x >= course_columns * 16 then return true end
  if y < 0 or y >= course_height * 16 then return false end
  return band(course_tile_flags[terrain_tile_at(x, y) + 1], 1) ~= 0
end

function level_floor(x, y, previous_y, snap)
  local tile = terrain_tile_at(x, y)
  local flags = course_tile_flags[tile + 1]
  local top = flr(y / 16) * 16
  if band(flags, 1) ~= 0 then return top end
  local slope = course_slopes[tile + 1]
  if slope ~= nil then
    local surface = top + slope[x % 16 + 1]
    if y >= surface and previous_y <= surface then return surface end
    if snap and abs(previous_y - surface) <= 4 then return surface end
    return nil
  end
  if band(flags, 2) ~= 0 and previous_y <= top then return top end
  return nil
end

function terrain_draw_cell(cx, cy, id)
  local tiles = course_tile_art[id + 1]
  local x = cx * 2
  local y = cy * 2 + course_map_row
  mset(0, x, y, tiles[1])
  mset(0, x + 1, y, tiles[2])
  mset(0, x, y + 1, tiles[3])
  mset(0, x + 1, y + 1, tiles[4])
end

function terrain_change_cell(cx, cy, old_tile, new_tile)
  local key = terrain_cell_key(cx, cy)
  if changed_tiles[key] == nil then
    changes_count = changes_count + 1
    changes[changes_count] = {cx, cy, old_tile}
  end
  changed_tiles[key] = new_tile
  terrain_draw_cell(cx, cy, new_tile)
end

function level_restore_tiles()
  for index = 1, changes_count do
    local cell = changes[index]
    terrain_draw_cell(cell[1], cell[2], cell[3])
    changed_tiles[terrain_cell_key(cell[1], cell[2])] = nil
    changes[index] = nil
  end
  changes_count = 0
end

function level_bump(x, y)
  local cx = flr(x / 16)
  local cy = flr(y / 16)
  local id = level_metatile(cx, cy)
  local action = course_block_actions[id + 1]
  if action == nil then return end
  if action == 1 and player_power == 0 then sfx(2) return end
  local replacement = course_block_replacements[id + 1]
  if replacement == nil then return end
  terrain_change_cell(cx, cy, id, replacement)
  if action == 2 then
    game_add_coin()
  elseif action == 3 or action == 4 then
    actor_spawn_powerup(cx * 16, cy * 16, action == 4)
  else
    game_add_score(50)
  end
end

function level_collect_coin(x, y)
  local cx = flr(x / 16)
  local cy = flr(y / 16)
  local id = level_metatile(cx, cy)
  if band(course_tile_flags[id + 1], 8) == 0 then return end
  local replacement = course_block_replacements[id + 1]
  if replacement == nil then return end
  terrain_change_cell(cx, cy, id, replacement)
  game_add_coin()
end
