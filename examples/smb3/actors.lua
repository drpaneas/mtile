-- Level records contain original NES object IDs and spawn positions.
local slots = {}
local spawned = {}
local slot_count = 5
local item_slot = {kind = 0, x = 0, y = 0, vx = 0, vy = 0, age = 0}
local last_player_bottom = 0

function actor_object_kind(id)
  if id == 114 or id == 115 or id == 116 then return 1 end
  if id >= 108 and id <= 111 then return 2 end
  if id == 65 then return 4 end
  -- Piranha variants in the Plains object groups.
  if id == 160 or id == 164 or id == 166 then return 3 end
  return 0
end

function actors_reset()
  spawned = {}
  for index = 1, slot_count do
    slots[index] = {kind = 0, id = 0, x = 0, y = 0, base_y = 0, vx = 0, vy = 0,
                    age = 0, timer = 0, home = 0, visible = false, crushed = 0}
  end
  item_slot.kind = 0
  last_player_bottom = player_y + player_height
end

function actor_activate_objects(scroll)
  for index = 1, #course_objects do
    local data = course_objects[index]
    local x = data[2]
    if not spawned[index] and x >= scroll - 24 and x < scroll + 248 then
      local kind = actor_object_kind(data[1])
      if kind == 0 then
        spawned[index] = true
      else
        for slot_index = 1, slot_count do
          local slot = slots[slot_index]
          if slot.kind == 0 then
            slot.kind = kind
            slot.id = data[1]
            slot.x = x
            slot.y = data[3]
            slot.base_y = slot.y
            slot.vx = -12 / 16
            slot.vy = 0
            slot.age = 0
            slot.timer = 0
            slot.home = index
            slot.visible = true
            slot.crushed = 0
            spawned[index] = true
            break
          end
        end
      end
    end
  end
end

function actor_walk_actor(actor)
  local next_x = actor.x + actor.vx
  local edge = next_x + 2
  if actor.vx > 0 then edge = next_x + 13 end
  local turn_at_edge = false
  if actor.id == 109 and actor.timer == 0 and actor.vy == 0 then
    local below = flr(actor.y) + 16
    turn_at_edge = level_floor(edge, below, below) == nil
  end
  if level_solid(edge, actor.y + 11) or turn_at_edge then
    actor.vx = -actor.vx
  else
    actor.x = next_x
  end
  local old_bottom = flr(actor.y) + 15
  actor.vy = min(actor.vy + 3 / 16, 4)
  local next_y = actor.y + actor.vy
  local bottom = flr(next_y) + 15
  if actor.vy >= 0 then
    local floor = level_floor(actor.x + 7, bottom, old_bottom)
    if floor ~= nil then
      actor.y = floor - 16
      actor.vy = 0
      return
    end
  end
  actor.y = next_y
end

function actor_move_plant(actor)
  local phase = actor.age % 128
  local height = 0
  if phase >= 32 and phase < 52 then height = phase - 32 end
  if phase >= 52 and phase < 90 then height = 20 end
  if phase >= 90 and phase < 110 then height = 110 - phase end
  if height == 0 and abs(player_x - actor.x) < 24 then actor.age = 30 end
  actor.visible = height > 0
  actor.y = actor.base_y - height
end

function actor_player_touches(x, y, width, height)
  return player_x + 12 >= x and player_x + 3 < x + width
      and player_y + player_height - 1 >= y and player_y + player_top < y + height
end

function actor_actor_contact(actor)
  if not actor.visible or actor.crushed > 0 then return end
  local x = flr(actor.x)
  local y = flr(actor.y)
  if actor.kind == 2 and actor.timer == 0 then y = y - 7 end
  if actor.kind == 4 then
    if actor_player_touches(x, y, 16, 16) then game_clear(flr(actor.age / 8) % 3 + 1) end
    return
  end
  if player_tail_ticks == 9 or player_tail_ticks == 12 then
    local tail = player_x + 17
    if player_left then tail = player_x - 10 end
    if tail + 10 > x and tail < x + 16 and player_y + 31 > y and player_y + 16 < y + 16 then
      actor.crushed = 18
      game_add_score(100)
      sfx(4)
      return
    end
  end
  if actor.kind == 3 and not actor_player_touches(x + 2, y, 12, min(20, actor.base_y - actor.y)) then
    return
  end
  if actor.kind ~= 3 and not actor_player_touches(x + 2, y + 2, 12, 14) then return end
  if actor.kind ~= 3 and player_vy >= 0 and last_player_bottom <= y + 6 then
    sfx(3)
    player_stomp()
    game_add_score(100)
    if actor.kind == 1 then
      if actor.id == 114 then actor.crushed = 20
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
    sfx(4)
    if player_x < x then actor.vx = 3 else actor.vx = -3 end
    actor.x = actor.x + 4 * sgn(actor.vx)
  else
    player_damage()
  end
end

function actor_spawn_powerup(x, y, one_up)
  if item_slot.kind ~= 0 then return end
  item_slot.kind = 1
  if one_up then item_slot.kind = 2 end
  if not one_up and player_power > 0 then item_slot.kind = 3 end
  item_slot.x = x
  item_slot.y = y
  item_slot.vx = 1
  item_slot.vy = 0
  item_slot.age = 0
end

function actors_update(scroll)
  actor_activate_objects(scroll)
  for index = 1, slot_count do
    local actor = slots[index]
    if actor.kind ~= 0 then
      actor.age = actor.age + 1
      if actor.crushed > 0 then
        actor.crushed = actor.crushed - 1
        if actor.crushed == 0 then actor.kind = 0 end
      elseif actor.x < scroll - 48 or actor.x > scroll + 296 then
        spawned[actor.home] = false
        actor.kind = 0
      elseif actor.kind == 3 then
        actor_move_plant(actor)
      elseif actor.kind ~= 4 then
        actor_walk_actor(actor)
        if (actor.id == 110 or actor.id == 115 or actor.id == 116) and actor.vy == 0
            and actor.age % 64 == 0 then actor.vy = -3 end
      end
      if actor.kind ~= 0 then
        actor_actor_contact(actor)
        if actor.y > course_height * 16 + 32 then actor.kind = 0 end
      end
    end
  end
  if item_slot.kind ~= 0 then
    item_slot.age = item_slot.age + 1
    if item_slot.age <= 16 then
      item_slot.y = item_slot.y - 1
    else
      if item_slot.kind == 3 then
        if item_slot.age < 31 then item_slot.y = item_slot.y - 2
        else
          if flr(item_slot.age / 22) % 2 == 0 then item_slot.x = item_slot.x + 1
          else item_slot.x = item_slot.x - 1 end
          item_slot.y = item_slot.y + 0.5
        end
      else
        actor_walk_actor(item_slot)
      end
      if actor_player_touches(item_slot.x, item_slot.y, 16, 16) then
        if item_slot.kind == 2 then game_add_life()
        else
          player_grow()
          sfx(6)
          game_add_score(1000)
        end
        item_slot.kind = 0
      elseif item_slot.y > course_height * 16 + 32 then
        item_slot.kind = 0
      end
    end
  end
  last_player_bottom = player_y + player_height
end

function actors_draw()
  for index = 1, 5 do
    local actor = slots[index]
    if actor.kind ~= 0 and actor.visible then
      local pose = art_goomba_1
      local x = flr(actor.x)
      local y = flr(actor.y)
      local flip = actor.vx > 0
      local alternate = flr(actor.age / 8) % 2 == 1
      if actor.kind == 1 then
        if actor.crushed > 0 then pose = art_goomba_flat
        elseif actor.id == 114 then
          if alternate then pose = art_goomba_2 end
        elseif alternate then pose = art_para_goomba_2
        else pose = art_para_goomba_1 end
      elseif actor.kind == 2 then
        if actor.timer > 0 then
          if actor.id == 109 then pose = art_red_koopa_shell
          else pose = art_koopa_shell end
        else
          y = y - 8
          if actor.id == 109 then
            if alternate then pose = art_red_koopa_2 else pose = art_red_koopa_1 end
          elseif actor.id == 110 then
            if alternate then pose = art_para_koopa_2 else pose = art_para_koopa_1 end
          elseif alternate then pose = art_koopa_2
          else pose = art_koopa_1 end
        end
      elseif actor.kind == 3 then
        if actor.id == 166 then
          if alternate then pose = art_venus_open else pose = art_venus end
        elseif actor.id == 164 then
          if alternate then pose = art_green_fire_open else pose = art_green_fire end
        elseif alternate then pose = art_piranha_open
        else pose = art_piranha end
        flip = player_x > actor.x
      elseif actor.kind == 4 then
        local card = flr(actor.age / 8) % 3
        pose = art_goal
        if card == 1 then pose = art_goal_flower end
        if card == 2 then pose = art_goal_star end
        flip = false
      end
      pspr(pose, x, y, flip)
    end
  end
  if item_slot.kind ~= 0 then
    local pose = art_mushroom
    if item_slot.kind == 2 then pose = art_one_up end
    if item_slot.kind == 3 then pose = art_leaf end
    pspr(pose, flr(item_slot.x), flr(item_slot.y))
  end
end
