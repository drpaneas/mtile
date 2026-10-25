-- The long garden lives in immutable generated tile-layout lookups. Only a 32-column
-- neighborhood is copied into each compact map plane, twice side by side.
-- Up to 31 visible columns can cross the ring seam inside a cached 64-cell map.
-- This keeps all four hardware backgrounds on the compact Mode 0 map path.

landscape_ready = false
terrain_bank = 0
landscape_spare_ready = true
landscape_spare_column = 0
landscape_far_tile = 0
landscape_garden_tile = 0
landscape_front_tile = 0

function landscape_far_column(world_col, bank)
  local slot = world_col%32
  for row = 0, 19 do
    local tile = 0
    if world_col >= 0 and world_col < TERRAIN_FAR_COLS then tile = terrain_far_at(world_col, row) end
    mset(1, slot, row+bank*20, tile)
    mset(1, slot+32, row+bank*20, tile)
  end
end

function landscape_garden_column(world_col, bank)
  local slot = world_col%32
  for row = 0, 19 do
    local tile = 0
    if world_col >= 0 and world_col < TERRAIN_GARDEN_COLS then tile = terrain_garden_at(world_col, row) end
    mset(2, slot, row+bank*20, tile)
    mset(2, slot+32, row+bank*20, tile)
  end
end

function landscape_front_column(world_col, bank)
  local slot = world_col%32
  for row = 0, 19 do
    local tile = 0
    if world_col >= 0 and world_col < TERRAIN_FRONT_COLS then tile = terrain_front_at(world_col, row) end
    mset(3, slot, row+bank*20, tile)
    mset(3, slot+32, row+bank*20, tile)
  end
end

-- Whole fills handle initialization and explicit scene jumps. Each layer
-- keeps literal-layer mset calls, with no Lua tables or allocation involved.
function landscape_fill_far(first)
  for row = 0, 19 do
    for col = 0, 31 do
      local world_col = first+col
      local tile = 0
      if world_col >= 0 and world_col < TERRAIN_FAR_COLS then
        tile = terrain_far_at(world_col, row)
      end
      local slot = world_col%32
      mset(1, slot, row+terrain_bank*20, tile)
      mset(1, slot+32, row+terrain_bank*20, tile)
    end
  end
end

function landscape_fill_garden(first)
  for row = 0, 19 do
    for col = 0, 31 do
      local world_col = first+col
      local tile = 0
      if world_col >= 0 and world_col < TERRAIN_GARDEN_COLS then
        tile = terrain_garden_at(world_col, row)
      end
      local slot = world_col%32
      mset(2, slot, row+terrain_bank*20, tile)
      mset(2, slot+32, row+terrain_bank*20, tile)
    end
  end
end

function landscape_fill_front(first)
  for row = 0, 19 do
    for col = 0, 31 do
      local world_col = first+col
      local tile = 0
      if world_col >= 0 and world_col < TERRAIN_FRONT_COLS then
        tile = terrain_front_at(world_col, row)
      end
      local slot = world_col%32
      mset(3, slot, row+terrain_bank*20, tile)
      mset(3, slot+32, row+terrain_bank*20, tile)
    end
  end
end

function reset_landscape(scene_x)
  landscape_far_tile = flr(flr(scene_x/4)/8)
  landscape_garden_tile = flr(flr(scene_x)/8)
  landscape_front_tile = flr(flr(scene_x*1.125)/8)
  landscape_fill_far(landscape_far_tile)
  landscape_fill_garden(landscape_garden_tile)
  landscape_fill_front(landscape_front_tile)
  landscape_ready = true
end

-- The unused bank is prepared a column at a time while the journey runs.
-- Restart can then select the opening garden without copying a whole map
-- during a visible frame. Both banks start ready in the authored map.
function prewarm_landscape()
  if not landscape_spare_ready then
    local bank = 1-terrain_bank
    landscape_far_column(landscape_spare_column, bank)
    landscape_garden_column(landscape_spare_column, bank)
    landscape_front_column(landscape_spare_column, bank)
    landscape_spare_column += 1
    if landscape_spare_column == 32 then landscape_spare_ready = true end
  end
end

function restart_landscape()
  -- Normal play always gives prewarming its 32 ticks: losing three wings
  -- takes longer, and the dawn invitation waits 120 ticks. The fallback
  -- also makes direct calls before that point preserve the exact scenery.
  if not landscape_spare_ready then
    for col = landscape_spare_column, 31 do
      landscape_far_column(col, 1-terrain_bank)
      landscape_garden_column(col, 1-terrain_bank)
      landscape_front_column(col, 1-terrain_bank)
    end
  end
  terrain_bank = 1-terrain_bank
  landscape_far_tile = 0
  landscape_garden_tile = 0
  landscape_front_tile = 0
  landscape_ready = true
  landscape_spare_ready = false
  landscape_spare_column = 0
end

function prepare_landscape(scene_x)
  if not landscape_ready then
    reset_landscape(scene_x)
    return
  end

  local far_tile = flr(flr(scene_x/4)/8)
  local garden_tile = flr(flr(scene_x)/8)
  local front_tile = flr(flr(scene_x*1.125)/8)
  local far_delta = far_tile-landscape_far_tile
  local garden_delta = garden_tile-landscape_garden_tile
  local front_delta = front_tile-landscape_front_tile

  if abs(far_delta) >= 32 then
    landscape_fill_far(far_tile)
  else
    for step = 1, abs(far_delta) do
      if far_delta > 0 then
        landscape_far_column(landscape_far_tile+30+step, terrain_bank)
      else
        landscape_far_column(landscape_far_tile-step, terrain_bank)
      end
    end
  end
  if abs(garden_delta) >= 32 then
    landscape_fill_garden(garden_tile)
  else
    for step = 1, abs(garden_delta) do
      if garden_delta > 0 then
        landscape_garden_column(landscape_garden_tile+30+step, terrain_bank)
      else
        landscape_garden_column(landscape_garden_tile-step, terrain_bank)
      end
    end
  end
  if abs(front_delta) >= 32 then
    landscape_fill_front(front_tile)
  else
    for step = 1, abs(front_delta) do
      if front_delta > 0 then
        landscape_front_column(landscape_front_tile+30+step, terrain_bank)
      else
        landscape_front_column(landscape_front_tile-step, terrain_bank)
      end
    end
  end

  landscape_far_tile = far_tile
  landscape_garden_tile = garden_tile
  landscape_front_tile = front_tile
  prewarm_landscape()
end
