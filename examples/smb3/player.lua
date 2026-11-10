-- Mario's dry, ordinary movement. Positions use whole NES world pixels with
-- separate sixteenth-pixel remainders. The original collision origin stays
-- fixed across Small and Super. Small Mario's visible body starts lower.
player_x = 0
player_y = 0
player_vx = 0
player_vy = 0
player_grounded = false
player_left = false
player_alive = true
player_power = 0
player_pmeter = 0
player_pose = 0
player_height = 32
player_top = 17
player_flash = 0
player_death_ticks = 0
player_tail_ticks = 0
player_wag_ticks = 0
player_fly_ticks = 0

local raccoon_b_held = false
local x_fraction = 0
local y_fraction = 0
local wiggle = 16
local jump_held = false
local ducking = false
local animation_phase = 2
local animation_ticks = 0
local meter_ticks = 0
local death_wait = 0
local speed_jump = {0, 2, 4, 8}
local walk_delay = {7, 6, 5, 4, 3, 2, 1, 1, 1}
local small_walk = {1, 2, 1, 2}
local super_walk = {1, 2, 0, 2}

function player_reset(x, y)
  player_x = x
  player_y = y
  player_vx = 0
  player_vy = 0
  player_grounded = true
  player_left = false
  player_alive = true
  player_power = 0
  player_pmeter = 0
  player_pose = 0
  player_top = 17
  player_flash = 0
  player_death_ticks = 0
  player_tail_ticks = 0
  player_wag_ticks = 0
  player_fly_ticks = 0
  raccoon_b_held = btn(4)
  x_fraction = 0
  y_fraction = 0
  wiggle = 16 -- Observed before the first active input of the reference replay.
  jump_held = btn(5)
  ducking = false
  animation_phase = 2
  animation_ticks = 0
  meter_ticks = 0
  death_wait = 0
end

function player_grow()
  if player_alive then
    player_power = min(2, player_power + 1)
    player_top = 5
  end
end

function player_stomp()
  player_vy = -64
  player_grounded = false
  player_pose = 4
end

function player_die()
  if not player_alive then return end
  player_alive = false
  player_grounded = false
  player_power = 0
  player_top = 17
  player_flash = 0
  player_pmeter = 0
  player_tail_ticks = 0
  player_wag_ticks = 0
  player_fly_ticks = 0
  player_vx = 0
  player_vy = -64
  player_pose = 8
  player_death_ticks = 0
  death_wait = 48
end

function player_damage()
  if not player_alive or player_flash > 0 then return end
  if player_power == 0 then
    player_die()
  else
    player_power = 0
    player_top = 17
    ducking = false
    player_flash = 113
    player_tail_ticks = 0
    player_wag_ticks = 0
    player_fly_ticks = 0
  end
end

function player_move_horizontal(short)
  local total = x_fraction + player_vx
  local step = flr(total / 16)
  x_fraction = total - step * 16
  if step == 0 then return end

  local direction = 1
  local side = 14
  local upper = 14
  if short then
    side = 13
    upper = 20
  end
  if step < 0 then
    direction = -1
    side = 15 - side
  end
  for i = 1, abs(step) do
    local next_x = player_x + direction
    local probe_x = next_x + side
    if level_solid(probe_x, player_y + upper) or level_solid(probe_x, player_y + 27) then
      player_vx = 0
      return
    end
    player_x = next_x
  end
end

function player_floor_surface(x, y, previous_y, snap)
  local left_floor = level_floor(x + 4, y, previous_y, snap)
  local right_floor = level_floor(x + 11, y, previous_y, snap)
  if left_floor == nil then return right_floor end
  if right_floor == nil then return left_floor end
  return min(left_floor, right_floor)
end

function player_move_vertical(short)
  player_vy = min(64, player_vy)
  local total = y_fraction + player_vy
  local step = flr(total / 16)
  y_fraction = total - step * 16
  if player_vy < 0 then
    player_grounded = false
    local head = 6
    if short then head = 16 end
    for i = 1, -step do
      local next_y = player_y - 1
      if level_solid(player_x + 8, next_y + head) then
        player_vy = 0
        level_bump(player_x + 8, next_y + head)
        return true
      end
      player_y = next_y
    end
    return false
  end

  -- Standing support and downward sweeps share the original two foot slots.
  -- A semisolid supplies a surface only if approached from above.
  local snap = player_grounded
  player_grounded = false
  for i = 0, step do
    local next_y = player_y
    if i > 0 then next_y = next_y + 1 end
    local surface = player_floor_surface(player_x, next_y + player_height, player_y + player_height, snap)
    if surface ~= nil then
      player_y = surface - player_height
      player_vy = 0
      player_grounded = true
      return false
    end
    player_y = next_y
  end
  return false
end

function player_acceleration(main, fraction, direction)
  if direction < 0 then
    if fraction == 0 then return -main end
    return -main - 1 + flr((256 - fraction + wiggle) / 256)
  end
  return main + flr((fraction + wiggle) / 256)
end

function player_steer(direction, running, was_grounded, speed)
  local friction = 96
  if player_power ~= 0 then friction = 32 end
  local previous_direction = 1
  if player_vx < 0 then previous_direction = -1 end
  if direction == 0 then
    if was_grounded and player_vx ~= 0 then
      player_vx = player_vx + player_acceleration(-1, friction, previous_direction)
    end
    return
  end
  if direction * player_vx < 0 then
    player_vx = player_vx + 2 * direction
    return
  end
  local speed_limit = 24
  if running then
    speed_limit = 40
    if player_pmeter == 127 then speed_limit = 56 end
  end
  if speed < speed_limit then
    player_vx = player_vx + player_acceleration(0, 224, direction)
  elseif speed > speed_limit and was_grounded then
    player_vx = player_vx + player_acceleration(-1, friction, direction)
  end
end

function player_update_meter(running)
  if meter_ticks > 0 then meter_ticks = meter_ticks - 1 end
  if player_pmeter == 127 and running then
    meter_ticks = 16
  elseif meter_ticks == 0 then
    if running then
      player_pmeter = player_pmeter * 2 + 1
      meter_ticks = 8
    else
      player_pmeter = flr(player_pmeter / 2)
      if player_pmeter > 0 then meter_ticks = 24 end
    end
  end
end

function player_update_pose(speed, direction, skidding)
  animation_ticks = animation_ticks + 1
  if animation_ticks >= walk_delay[flr(speed / 8) + 1] then
    animation_ticks = 0
    animation_phase = (animation_phase + 1) % 4
  end
  if direction == 0 and speed == 0 then animation_phase = 2 end

  if ducking then
    player_pose = 7
  elseif not player_grounded then
    player_pose = 4
    if player_vy >= 0 then player_pose = 5 end
  elseif skidding then
    player_pose = 6
  elseif direction == 0 and speed == 0 then
    player_pose = 0
  elseif player_power == 0 then
    player_pose = small_walk[animation_phase + 1]
  else
    player_pose = super_walk[animation_phase + 1]
  end
end

function player_update_death()
  player_death_ticks = player_death_ticks + 1
  if death_wait > 0 then
    death_wait = death_wait - 1
    return
  end
  player_vy = min(64, player_vy + 2)
  local total = y_fraction + player_vy
  local step = flr(total / 16)
  y_fraction = total - step * 16
  player_y = player_y + step
end

function player_update()
  if not player_alive then
    player_update_death()
    return
  end
  if player_flash > 0 then player_flash = player_flash - 1 end
  wiggle = (wiggle + 112) % 256

  local left = btn(0)
  local right = btn(1)
  local running = btn(4)
  local jump = btn(5)
  local new_jump = jump and not jump_held
  jump_held = jump
  local direction = 0
  if left then direction = -1 end
  if right then direction = 1 end
  if left or right then player_left = left end

  local was_grounded = player_grounded
  if player_power == 0 then
    ducking = false
  elseif was_grounded then
    ducking = btn(3) and not btn(2) and direction == 0
  end
  local short = player_power == 0 or ducking
  player_top = 5
  if short then player_top = 17 end

  -- Apply the previous update's velocities before steering and jump impulse.
  player_vx = mid(-64, player_vx, 64)
  local speed = abs(player_vx)
  local skidding = was_grounded and speed > 1 and direction * player_vx < 0
  player_move_horizontal(short)
  local hit_ceiling = player_move_vertical(short)
  player_steer(direction, running, was_grounded, abs(player_vx))

  if new_jump and was_grounded then
    player_vy = -56 - speed_jump[min(3, flr(abs(player_vx) / 16)) + 1]
    player_grounded = false
    if player_power == 2 and player_pmeter == 127 and player_fly_ticks == 0 then
      player_fly_ticks = 128
    end
  end
  if not hit_ceiling and not player_grounded and (not was_grounded or new_jump) then
    if jump and player_vy < -32 then
      player_vy = player_vy + 1
    else
      player_vy = player_vy + 5
    end
  end
  if player_power == 2 then
    if player_fly_ticks > 0 then player_fly_ticks = player_fly_ticks - 1 end
    if player_wag_ticks > 0 then player_wag_ticks = player_wag_ticks - 1 end
    if not was_grounded and not player_grounded and new_jump then player_wag_ticks = 16 end
    if not player_grounded and player_wag_ticks > 0 and not hit_ceiling then
      if player_fly_ticks > 0 then player_vy = min(player_vy, -24)
      elseif player_vy > 16 then player_vy = 16 end
    end
    if player_tail_ticks > 0 then player_tail_ticks = player_tail_ticks - 1 end
    if running and not raccoon_b_held and not ducking and player_tail_ticks == 0 then
      player_tail_ticks = 18
    end
    if player_tail_ticks == 9 then
      local tail_x = player_x + 21
      if player_left then tail_x = player_x - 6 end
      level_bump(tail_x, player_y + 28)
    end
  end
  raccoon_b_held = running
  player_update_meter(running and was_grounded and speed >= 40)
  player_update_pose(speed, direction, skidding)
end
