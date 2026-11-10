-- Level records contain original NES object IDs and spawn positions.
local slots = {}
local items = {}
local fireballs = {}
-- 1 active, 2 culled until its home leaves the spawn window, 3 defeated.
local spawned = {}
local slot_count = 5
local item_count = 2
local fireball_count = 4
local last_player_bottom = 0

function actor_object_kind(id)
  if id == 114 or id == 115 or id == 116 then return 1 end
  if id >= 108 and id <= 111 then return 2 end
  if id == 65 then return 4 end
  if id == 160 or id == 164 or id == 166 then return 3 end
  return 0
end

function actors_reset()
  spawned = {}
  for index = 1, slot_count do
    slots[index] = {kind = 0, id = 0, x = 0, y = 0, base_y = 0, vx = 0, vy = 0,
                    age = 0, timer = 0, home = 0, visible = false, crushed = 0,
                    knocked = false, shell_grace = 0, plant_phase = 0, plant_height = 0}
  end
  for index = 1, item_count do
    items[index] = {kind = 0, x = 0, y = 0, vx = 0, vy = 0, age = 0}
  end
  for index = 1, fireball_count do
    fireballs[index] = {active = false, x = 0, y = 0, vx = 0, vy = 0, age = 0}
  end
  last_player_bottom = player_y + player_height
end

function actor_activate_objects(scroll)
  for index = 1, #course_objects do
    local data = course_objects[index]
    local x = data[2]
    local in_spawn_window = x >= scroll - 24 and x < scroll + 248
    if spawned[index] == 2 and not in_spawn_window then spawned[index] = nil end
    if spawned[index] == nil and in_spawn_window then
      local kind = actor_object_kind(data[1])
      if kind == 0 then
        spawned[index] = 3
      else
        for slot_index = 1, slot_count do
          local slot = slots[slot_index]
          if slot.kind == 0 then
            slot.kind = kind
            slot.id = data[1]
            slot.x = x
            if kind == 3 then slot.x = slot.x + 8 end
            slot.y = data[3]
            slot.base_y = slot.y
            slot.vx = -8 / 16
            if player_x > x then slot.vx = 8 / 16 end
            slot.vy = 0
            slot.age = 0
            slot.timer = 0
            slot.home = index
            slot.visible = kind ~= 3
            slot.crushed = 0
            slot.knocked = false
            slot.shell_grace = 0
            slot.plant_phase = 0
            slot.plant_height = 0
            spawned[index] = 1
            break
          end
        end
      end
    end
  end
end

function actor_land(actor)
  local old_bottom = flr(actor.y) + 16
  local was_grounded = actor.vy == 0
  actor.vy = min(actor.vy + 3 / 16, 4)
  local next_y = actor.y + actor.vy
  local bottom = flr(next_y) + 16
  if actor.vy >= 0 then
    local floor = level_floor(flr(actor.x) + 7, bottom, old_bottom, was_grounded)
    if floor ~= nil then
      actor.y = floor - 16
      actor.vy = 0
      return
    end
  end
  actor.y = next_y
end

function actor_walk_actor(actor, shell, patrol)
  local next_x = actor.x + actor.vx
  if actor.vx ~= 0 then
    local edge = flr(next_x) + 2
    if actor.vx > 0 then edge = flr(next_x) + 13 end
    local middle_y = flr(actor.y) + 10
    local wall = level_solid(edge, middle_y)
    if shell then
      level_shell_hit(edge, middle_y)
      if wall then sfx(2) end
    end
    local turn_at_edge = false
    if patrol and actor.vy == 0 then
      local below = flr(actor.y) + 16
      turn_at_edge = level_floor(edge, below, below, true) == nil
    end
    if wall or turn_at_edge then
      actor.vx = -actor.vx
    else
      actor.x = next_x
    end
  end
  actor_land(actor)
end

function actor_plant_fire(actor)
  for index = 1, fireball_count do
    local ball = fireballs[index]
    if not ball.active then
      ball.active = true
      ball.x = actor.x + 3
      ball.y = actor.y
      ball.vx = 11 / 16
      ball.vy = 11 / 16
      if abs(player_x - actor.x) >= 80 then
        ball.vx = 14 / 16
        ball.vy = 5 / 16
      end
      if player_x < actor.x then ball.vx = -ball.vx end
      if player_y < actor.y then ball.vy = -ball.vy end
      ball.age = 0
      return
    end
  end
end

function actor_move_plant(actor)
  local fire = actor.id == 164 or actor.id == 166
  local wait = 48
  if fire then wait = 96 end
  local full_height = 25
  if actor.id == 166 then full_height = 33 end
  if actor.plant_phase == 0 then
    if actor.timer > 0 then actor.timer = actor.timer - 1 end
    if actor.timer == 0 and abs(player_x - actor.x) >= 27 then actor.plant_phase = 1 end
  elseif actor.plant_phase == 1 then
    actor.plant_height = actor.plant_height + 1
    if actor.plant_height == full_height then
      actor.plant_phase = 2
      actor.timer = wait
    end
  elseif actor.plant_phase == 2 then
    actor.timer = actor.timer - 1
    if fire and actor.timer == 40 then actor_plant_fire(actor) end
    if actor.timer == 0 then actor.plant_phase = 3 end
  else
    actor.plant_height = actor.plant_height - 1
    if actor.plant_height == 0 then
      actor.plant_phase = 0
      actor.timer = wait
    end
  end
  actor.visible = actor.plant_height > 0
  actor.y = actor.base_y - actor.plant_height
end

function actor_player_touches(x, y, width, height)
  return player_x + 12 >= x and player_x + 3 < x + width
      and player_y + player_height - 1 >= y and player_y + player_top < y + height
end

function actor_hit_top(actor)
  local top = flr(actor.y)
  if actor.kind == 2 and actor.timer == 0 then return top - 7 end
  return top
end

function actor_defeat(actor, knock, direction)
  spawned[actor.home] = 3
  actor.crushed = 20
  actor.knocked = knock
  if knock then
    actor.crushed = 32
    actor.vx = direction
    actor.vy = -3
  end
end

function actor_shell_contacts(shell)
  local sx = flr(shell.x)
  local sy = flr(shell.y)
  for index = 1, slot_count do
    local target = slots[index]
    if target ~= shell and target.kind > 0 and target.kind < 4 and target.visible and target.crushed == 0 then
      local tx = flr(target.x)
      local ty = actor_hit_top(target)
      local bottom = flr(target.y) + 16
      if target.kind == 3 then bottom = target.base_y end
      if sx + 13 >= tx + 2 and sx + 2 < tx + 14 and sy + 14 >= ty and sy + 2 < bottom then
        local direction = sgn(shell.vx)
        if target.kind == 2 and target.timer == 2 then actor_defeat(shell, true, -direction) end
        actor_defeat(target, true, direction)
        game_add_score(100)
        sfx(4)
        if shell.crushed ~= 0 then return end
      end
    end
  end
end

function actor_actor_contact(actor)
  if not actor.visible or actor.crushed > 0 then return end
  local x = flr(actor.x)
  local y = actor_hit_top(actor)
  local bottom = flr(actor.y) + 16
  if actor.kind == 4 then
    if actor_player_touches(x, y, 16, 16) then game_clear(flr(actor.age / 8) % 3 + 1) end
    return
  end
  if actor.kind == 3 then bottom = actor.base_y end
  if player_tail_ticks == 9 or player_tail_ticks == 12 then
    local tail = player_x + 17
    local direction = 1
    if player_left then
      tail = player_x - 10
      direction = -1
    end
    if tail + 10 > x and tail < x + 16 and player_y + 31 > y and player_y + 16 < bottom then
      actor_defeat(actor, true, direction)
      game_add_score(100)
      sfx(4)
      return
    end
  end
  if not actor_player_touches(x + 2, y + 2, 12, bottom - y - 2) then return end
  if actor.kind ~= 3 and player_vy >= 0 and last_player_bottom <= y + 6 then
    sfx(3)
    player_stomp()
    game_add_score(100)
    if actor.kind == 1 then
      if actor.id == 114 then actor_defeat(actor, false, 0)
      else actor.id = 114 end
    elseif actor.id == 110 or actor.id == 111 then
      actor.id = actor.id - 2
      actor.vy = 0
    else
      actor.timer = 1
      actor.vx = 0
    end
  elseif actor.kind == 2 and actor.timer == 1 then
    actor.timer = 2
    actor.shell_grace = 4
    sfx(4)
    local direction = 1
    if player_x >= x then direction = -1 end
    local speed = 48 * direction
    if player_vx * direction >= 0 then speed = speed + flr(player_vx / 2) end
    actor.vx = speed / 16
    actor.vy = 0
  elseif actor.shell_grace == 0 then
    player_damage()
  end
end

function actor_fireball_touches(ball)
  local offset = 18
  local height = 16
  if player_top < 17 then
    offset = 10
    height = 22
  end
  local x = flr(ball.x) + 6 - player_x
  local y = flr(ball.y) + 8 - player_y - offset
  return x >= 0 and x < 16 and y >= 0 and y < height
end

function actor_spawn_powerup(x, y, one_up)
  for index = 1, item_count do
    local item = items[index]
    if item.kind == 0 then
      item.kind = 1
      if one_up then item.kind = 2 end
      if not one_up and player_power > 0 then item.kind = 3 end
      item.x = x
      item.y = y
      item.vx = 1
      if x < player_x then item.vx = -1 end
      item.vy = 0
      item.age = 0
      return true
    end
  end
  return false
end

function actor_update_item(item, scroll)
  item.age = item.age + 1
  if item.x < scroll - 48 or item.x > scroll + 296 or item.y > course_height * 16 + 32 then
    item.kind = 0
    return
  end
  -- Original power-ups wait for 16 ticks and rise one pixel every three ticks.
  if item.age <= 61 then
    if item.age >= 17 and (item.age - 17) % 3 == 0 then item.y = item.y - 1 end
    if item.age == 61 then item.y = item.y - 1 end
    return
  end
  if item.kind == 3 then
    if item.age < 76 then item.y = item.y - 2
    else
      if flr(item.age / 22) % 2 == 0 then item.x = item.x + 1
      else item.x = item.x - 1 end
      item.y = item.y + 0.5
    end
  else
    actor_walk_actor(item, false, false)
  end
  if actor_player_touches(item.x, item.y, 16, 16) then
    if item.kind == 2 then game_add_life()
    else
      player_grow()
      sfx(6)
      game_add_score(1000)
    end
    item.kind = 0
  end
end

function actors_update(scroll)
  actor_activate_objects(scroll)
  for index = 1, slot_count do
    local actor = slots[index]
    if actor.kind ~= 0 then
      actor.age = actor.age + 1
      if actor.shell_grace > 0 then actor.shell_grace = actor.shell_grace - 1 end
      if actor.crushed > 0 then
        actor.crushed = actor.crushed - 1
        if actor.knocked then
          actor.x = actor.x + actor.vx
          actor.y = actor.y + actor.vy
          actor.vy = min(actor.vy + 3 / 16, 4)
        end
        if actor.crushed == 0 then actor.kind = 0 end
      elseif actor.x < scroll - 48 or actor.x > scroll + 296 or actor.y > course_height * 16 + 32 then
        spawned[actor.home] = 2
        actor.kind = 0
      elseif actor.kind == 3 then
        actor_move_plant(actor)
      elseif actor.kind ~= 4 then
        local shell = actor.kind == 2 and actor.timer == 2
        actor_walk_actor(actor, shell, actor.id == 109 and actor.timer == 0)
        if (actor.id == 110 or actor.id == 115 or actor.id == 116) and actor.vy == 0
            and actor.age % 64 == 0 then actor.vy = -3 end
      end
    end
  end
  for index = 1, slot_count do
    local actor = slots[index]
    if actor.kind == 2 and actor.timer == 2 and actor.crushed == 0 then actor_shell_contacts(actor) end
  end
  for index = 1, slot_count do
    local actor = slots[index]
    if actor.kind ~= 0 then actor_actor_contact(actor) end
  end
  for index = 1, fireball_count do
    local ball = fireballs[index]
    if ball.active then
      ball.age = ball.age + 1
      ball.x = ball.x + ball.vx
      ball.y = ball.y + ball.vy
      if ball.x < scroll - 16 or ball.x > scroll + 256 or ball.y < -16 or ball.y > course_height * 16 + 16 then
        ball.active = false
      elseif actor_fireball_touches(ball) then
        player_damage()
      end
    end
  end
  for index = 1, item_count do
    local item = items[index]
    if item.kind ~= 0 then actor_update_item(item, scroll) end
  end
  last_player_bottom = player_y + player_height
end

function actor_plant_pose(actor, alternate)
  local heights = art_piranha_heights
  if actor.id == 166 then
    if alternate then heights = art_venus_open_heights else heights = art_venus_heights end
  elseif actor.id == 164 then
    if alternate then heights = art_green_fire_open_heights else heights = art_green_fire_heights end
  elseif alternate then
    heights = art_piranha_open_heights
  end
  local rows = min(8, flr(actor.plant_height / 4))
  if rows > 0 then return heights[rows] end
  return nil
end

function actors_draw()
  for index = 1, 5 do
    local actor = slots[index]
    if actor.kind ~= 0 and actor.visible then
      local pose = art_goomba_1
      local goomba_walk = false
      local x = flr(actor.x)
      local y = flr(actor.y)
      local flip = actor.vx > 0
      local alternate = flr(actor.age / 8) % 2 == 1
      if actor.kind == 1 then
        if actor.crushed > 0 and not actor.knocked then pose = art_goomba_flat
        elseif actor.id == 114 then
          goomba_walk = true
        elseif alternate then pose = art_para_goomba_2
        else pose = art_para_goomba_1 end
      elseif actor.kind == 2 then
        if actor.timer > 0 then
          if actor.id == 109 then pose = art_red_koopa_shell
          else pose = art_koopa_shell end
        else
          -- The imported Koopa frames span 34 and 35 pixels respectively.
          y = y - 18
          if alternate then y = y - 1 end
          if actor.id == 109 then
            if alternate then pose = art_red_koopa_2 else pose = art_red_koopa_1 end
          elseif actor.id == 110 then
            if alternate then pose = art_para_koopa_2 else pose = art_para_koopa_1 end
          elseif alternate then pose = art_koopa_2
          else pose = art_koopa_1 end
        end
      elseif actor.kind == 3 then
        if actor.plant_phase == 2 and actor.id ~= 160 then alternate = actor.timer <= 40 and actor.timer > 28 end
        pose = actor_plant_pose(actor, alternate)
        flip = player_x > actor.x
      elseif actor.kind == 4 then
        local card = flr(actor.age / 8) % 3
        pose = art_goal
        if card == 1 then pose = art_goal_flower end
        if card == 2 then pose = art_goal_star end
        flip = false
      end
      if goomba_walk then
        anim("goomba_cycle", x, y, flip, actor.knocked, (actor.age + 0.25) / 60)
      elseif pose ~= nil then
        pspr(pose, x, y, flip, actor.knocked)
      end
    end
  end
  for index = 1, 4 do
    local ball = fireballs[index]
    if ball.active then
      local pose = art_enemy_fireball
      local frame = flr(ball.age / 4) % 4
      if frame % 2 == 1 then pose = art_enemy_fireball_alt end
      local spin = frame >= 2
      pspr(pose, flr(ball.x), flr(ball.y), (ball.vx < 0) ~= spin, spin)
    end
  end
  for index = 1, 2 do
    local item = items[index]
    if item.kind ~= 0 and item.age >= 17 then
      local pose = art_mushroom
      local heights = art_mushroom_heights
      if item.kind == 2 then
        pose = art_one_up
        heights = art_one_up_heights
      elseif item.kind == 3 then
        pose = art_leaf
        heights = art_leaf_heights
      end
      if item.age < 61 and heights ~= nil then
        local rows = flr((flr((item.age - 17) / 3) + 1) / 4)
        if rows > 0 then pose = heights[rows] else pose = nil end
      end
      if pose ~= nil then pspr(pose, flr(item.x), flr(item.y)) end
    end
  end
end
