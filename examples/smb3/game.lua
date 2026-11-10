-- Super Mario Bros. 3 native mTile project.
-- Original art and course data are prepared from the user's ROM before build.
require("course_data")
require("terrain")
require("player")
require("actors")

local course = 1
local stage = 0
local tick = 0
local cam_x = 0
local cam_y = 0
local score = 0
local coins = 0
local lives = 4
local time_left = 300
local timer_phase = 0
local clear_delay = 0
local start_held = false
local menu_jump_held = false
local map_node = 1
local unlocked_course = 1
local map_anim = 0
local card = 0
local hud_lives = -1
local hud_score = -1
local hud_coins = -1
local hud_timer = -1
local screen_fade = 0
local reveal_updates = 0

function game_add_score(amount)
  score = min(30000, score + amount)
end

function game_add_life()
  lives = min(99, lives + 1)
  sfx(7)
end

function game_add_coin()
  coins = coins + 1
  sfx(1)
  game_add_score(100)
  if coins == 100 then
    coins = 0
    game_add_life()
  end
end

function game_clear(award)
  if stage ~= 2 then return end
  stage = 4
  clear_delay = 0
  card = award
  pause(false)
  music(3)
end

function smb3_select_course(index)
  level_restore_tiles()
  course = index
  local data = courses[index]
  course_rows = data.rows
  course_columns = data.columns
  course_height = data.height
  course_objects = data.objects
  course_tile_flags = data.flags
  course_tile_art = data.art
  course_block_actions = data.actions
  course_block_replacements = data.replacements
  course_slopes = data.slopes
  course_map_row = data.map_row
  player_reset(data.spawn_x, data.spawn_y)
  actors_reset()
  tick = 0
  time_left = data.timer
  timer_phase = 0
  cam_x = 0
  cam_y = smb3_camera_ground_y()
end

function _init()
  -- Start belongs to the title, course selector and pause controls.
  pause(false)
  smb3_select_course(1)
end

-- Keep the feet ten pixels above the HUD when standing on level ground.
-- In the air the viewport follows only after Mario crosses its scroll band.
function smb3_camera_ground_y()
  return mid(0, player_y + player_height - 118, course_height * 16 - 128)
end

function smb3_camera_follow()
  local relative = player_x - cam_x
  if relative > 128 then
    cam_x = min(player_x - 128, course_columns * 16 - 240)
  elseif relative < 64 then
    cam_x = max(player_x - 64, 0)
  end
  local ground_y = smb3_camera_ground_y()
  local target_y = cam_y
  if player_grounded or cam_y < ground_y then
    target_y = ground_y
  elseif player_y - cam_y < 20 then
    target_y = max(player_y - 20, 0)
  end
  -- Show a power-up emerging near the top without putting Mario's feet
  -- behind the HUD. On level ground the normal framing has ten spare pixels.
  local reward_top = actors_reward_top(cam_x)
  if reward_top ~= nil then
    target_y = min(target_y, max(max(0, ground_y - 8), reward_top))
  end
  -- Landing on a high ledge can change the target by most of the viewport.
  -- Limit the upward change just as the original camera did.
  cam_y = cam_y + mid(-3, target_y - cam_y, 4)
end

function smb3_update_course()
  tick = tick + 1
  timer_phase = timer_phase + 1
  if timer_phase == 40 then
    timer_phase = 0
    time_left = max(0, time_left - 1)
    if time_left == 0 then player_die() end
  end
  local on_ground = player_grounded
  player_update()
  if on_ground and player_vy < -32 then sfx(0) end
  if player_y > course_height * 16 then player_die() end
  if not player_alive then
    stage = 3
    clear_delay = 0
    lives = lives - 1
    pause(false)
    music(4)
    return
  end
  smb3_camera_follow()
  level_collect_coin(player_x + 5, player_y + 24)
  level_collect_coin(player_x + 11, player_y + 28)
  actors_update(cam_x)
end

function smb3_hud_digits(field, value, previous)
  if value == previous then return value end
  local remaining = value
  for digit = field[3] - 1, 0, -1 do
    local digit_value = remaining % 10
    -- Rewriting an unchanged digit can reload the evicted HUD map chunk.
    if previous < 0 or digit_value ~= previous % 10 then
      mset(1, field[1] + digit, field[2], art_hud_digit_art[digit_value + 1])
    end
    remaining = flr(remaining / 10)
    previous = flr(previous / 10)
  end
  return value
end

function smb3_fade_to(shade)
  if shade == screen_fade then return end
  screen_fade = shade
  if shade == 0 then
    pal(1)
  else
    local colors = art_scene_fade[shade]
    for index = 0, 64 do pal(index, colors[index + 1], 1) end
  end
end

function smb3_return_after_death()
  reveal_updates = 12
  if lives <= 0 then
    lives = 4
    score = 0
    coins = 0
    card = 0
    course = 1
    unlocked_course = 1
    map_node = 1
    stage = 0
  else
    stage = 1
    music(1)
  end
end

function _update()
  local start = btn(6)
  local jump = btn(5)
  local start_pressed = start and not start_held
  local jump_pressed = jump and not menu_jump_held
  start_held = start
  menu_jump_held = jump

  if reveal_updates > 0 then
    reveal_updates = reveal_updates - 1
    smb3_fade_to(flr((reveal_updates + 2) / 3))
    return
  end

  if stage == 0 then
    if start_pressed or jump_pressed then
      stage = 1
      music(1)
    end
  elseif stage == 1 then
    map_anim = map_anim + 1
    local previous_node = map_node
    if map_node == 1 and btnp(1) then map_node = 2
    elseif map_node == 2 and btnp(0) then map_node = 1
    elseif map_node == 2 and btnp(2) then map_node = 3
    elseif map_node == 3 and btnp(3) then map_node = 2
    elseif map_node >= 3 and map_node < 5 and unlocked_course >= 2 and btnp(1) then
      map_node = map_node + 1
    elseif map_node > 3 and btnp(0) then
      map_node = map_node - 1
    end
    if previous_node ~= map_node then sfx(8) end
    if (jump_pressed or start_pressed) and (map_node == 3 or map_node == 5) then
      local index = 1
      if map_node == 5 then index = 2 end
      smb3_select_course(index)
      stage = 2
      pause(true)
      if course == 1 then music(0) else music(2) end
    end
  elseif stage == 2 then
    -- Mode 0 presents every second GBA scan. Preserve two physics updates
    -- with the same hardware keypad sample between presentations.
    smb3_update_course()
    if stage == 2 then smb3_update_course() end
  elseif stage == 3 then
    player_update()
    player_update()
    clear_delay = clear_delay + 1
    local fade_out = clear_delay - (art_death_jingle_updates - 12)
    if fade_out > 0 then smb3_fade_to(min(4, flr((fade_out + 2) / 3))) end
    -- The imported song advances on the audio timer. Two extra game updates
    -- cover the phase between starting the song and its first audio tick.
    if clear_delay >= art_death_jingle_updates + 2 then smb3_return_after_death() end
  elseif stage == 4 then
    clear_delay = clear_delay + 1
    if time_left > 0 and clear_delay > 20 then
      game_add_score(50)
      time_left = time_left - 1
    end
    if clear_delay > 40 and (jump_pressed or start_pressed or time_left == 0) then
      if course < #courses then
        unlocked_course = 2
        map_node = 3
        stage = 1
        local row = art_world_map_row + 1
        mset(0, 7, row, art_map_clear[1])
        mset(0, 8, row, art_map_clear[2])
        mset(0, 7, row + 1, art_map_clear[3])
        mset(0, 8, row + 1, art_map_clear[4])
        music(1)
      else
        stage = 5
      end
    end
  elseif stage == 5 then
    if start_pressed then
      map_node = 1
      stage = 1
      music(1)
    end
  end
  if stage == 2 or stage == 3 or stage == 4 then
    hud_lives = smb3_hud_digits(art_hud_lives, max(0, lives), hud_lives)
    hud_score = smb3_hud_digits(art_hud_score, score, hud_score)
    hud_coins = smb3_hud_digits(art_hud_coins, coins, hud_coins)
    hud_timer = smb3_hud_digits(art_hud_timer, time_left, hud_timer)
  end
end

function smb3_draw_player()
  if player_flash > 0 and flr(player_flash / 3) % 2 == 1 then return end
  local x, y = player_x, player_y + 16
  local flip = not player_left
  local frame = player_pose % 9 + 1
  local pose = art_small[frame]
  if player_power > 0 then
    y = player_y
    pose = art_super[frame]
  end
  if player_power == 2 then
    if flip then x = x - 8 end
    pose = art_raccoon[frame]
    if player_tail_ticks > 0 then
      if player_tail_ticks >= 15 then pose = art_tail[1]
      elseif player_tail_ticks >= 11 then pose = art_tail[2]
      else pose = art_tail[3] end
    elseif player_wag_ticks > 0 then
      local wag_frame = flr(player_wag_ticks / 3) % 3 + 1
      if player_fly_ticks > 0 then pose = art_fly[wag_frame]
      else pose = art_flutter[wag_frame] end
    end
  end
  pspr(pose, x, y, flip)
end

function smb3_draw_course()
  camera(cam_x, cam_y)
  actors_draw()
  smb3_draw_player()
  camera(0, 0)
end

function _draw()
  cls(0)
  local bg_x = cam_x
  local bg_y = cam_y + course_map_row * 8
  if stage == 0 then bg_x = 0 bg_y = art_title_row * 8 end
  if stage == 1 or stage == 5 then bg_x = 0 bg_y = art_world_map_row * 8 end
  camera(bg_x, bg_y)
  -- Include one guard tile for sub-tile scrolling, without materializing
  -- the hidden rows and columns between the map origin and camera.
  map(0, flr(bg_x / 8), flr(bg_y / 8), 0, 0, 31, 21)
  local hud_y = 0
  if stage == 0 or stage == 1 or stage == 5 then hud_y = -160 end
  camera(0, hud_y)
  map(1, 0, 0, 0, 128, 30, 4)
  camera(0, 0)
  if stage == 1 then
    local position = art_map_positions[map_node]
    anim("world_map_mario", position[1], position[2], false, false, (map_anim + 0.25) / 30)
  elseif stage == 5 then
    print("WORLD 1-2 CLEAR", 76, 59, 16)
    print("START TO REPLAY", 76, 85, 16)
  elseif stage ~= 0 then
    smb3_draw_course()
    if stage == 3 then print("TRY AGAIN", 94, 34, 16) end
    if stage == 4 then
      print("COURSE CLEAR", 83, 34, 16)
      local award = art_goal
      if card == 2 then award = art_goal_flower end
      if card == 3 then award = art_goal_star end
      pspr(award, 167, 136)
    end
  end
end
