-- LUMEN / Keeper of the Tide. Original game and artwork for mTile.
-- D-pad flies, A dashes, B kindles a lantern, Start pauses.
-- See README.md for the complete rules and the asset-generation scripts.
-- State is loaded first so Develop can expose the game's live numeric values.
require("state")
require("art")
require("terrain")
require("landscape")
require("world")
require("draw")

function _init()
  reset_landscape(120)
  music(0)
end

function begin_journey()
  state = 1
  play_ticks = 0
  px = 32
  py = 88
  vx = 0
  vy = 0
  view_x = 0
  facing = 1
  carried = 0
  lit_count = 0
  health = 3
  hits = 0
  checkpoint = 0
  near_lantern = 0
  dash_ticks = 0
  dash_cooldown = 0
  invulnerable = 45
  message = 0
  message_ticks = 180
  win_age = 0
  lantern_time1 = 0
  lantern_time2 = 0
  lantern_time3 = 0
  lantern_time4 = 0
  lantern_time5 = 0
  lantern_time6 = 0
  returning = false
  restart_pending = false
  ember_mask = 0
  ember_mask2 = 0
  lantern_mask = 0
  sleep1 = 0
  sleep2 = 0
  sleep3 = 0
  sleep4 = 0
  sleep5 = 0
  sleep6 = 0
  sleep7 = 0
  sleep8 = 0
  sleep9 = 0
  sleep10 = 0
  sleep11 = 0
  sleep12 = 0
  burst_life1 = 0
  burst_life2 = 0
  burst_life3 = 0
  dawn_stage = 0
  restart_landscape()
end

function follow_camera(target)
  -- The cap keeps all parallax layers within one incoming tile per update.
  view_x += mid(-6, (target-view_x)*0.12, 6)
end

function find_lantern()
  near_lantern = 0
  for i = 1, 6 do
    if abs(px-lantern_x(i)) < 25 and abs(py-lantern_y(i)) < 25 and not lantern_lit(i) then
      near_lantern = i
    end
  end
end

function kindle()
  if near_lantern == 0 then
    message = 3
    message_ticks = 100
  elseif carried < 3 then
    message = 1
    message_ticks = 100
  else
    lantern_mask = bor(lantern_mask, shl(1, near_lantern-1))
    if near_lantern == 1 then lantern_time1 = 0 end
    if near_lantern == 2 then lantern_time2 = 0 end
    if near_lantern == 3 then lantern_time3 = 0 end
    if near_lantern == 4 then lantern_time4 = 0 end
    if near_lantern == 5 then lantern_time5 = 0 end
    if near_lantern == 6 then lantern_time6 = 0 end
    carried -= 3
    lit_count += 1
    checkpoint = near_lantern
    health = 3
    invulnerable = 60
    emit_light(lantern_x(near_lantern), lantern_y(near_lantern), 20, 12)
    sfx(1)
    message = 2
    message_ticks = 110
    if lit_count == 6 then
      state = 4
      win_age = 0
      dawn_x = mid(120, lantern_x(near_lantern), 2760)
      dawn_view = mid(0, dawn_x-120, 2640)
      sfx(4)
    end
  end
end

function step_flight(pressed_a, pressed_b)
  play_ticks = min(play_ticks+1, 32760)
  if dash_cooldown > 0 then dash_cooldown -= 1 end
  if invulnerable > 0 then invulnerable -= 1 end
  if message_ticks > 0 then message_ticks -= 1 end

  local dx = 0
  local dy = 0
  if btn(0) then dx -= 1 end
  if btn(1) then dx += 1 end
  if btn(2) then dy -= 1 end
  if btn(3) then dy += 1 end
  if dx != 0 then facing = dx end
  -- Normalize diagonal movement without a square root on the target CPU.
  if dx != 0 and dy != 0 then
    dx *= 0.7071
    dy *= 0.7071
  end
  if pressed_a and dash_cooldown == 0 then
    if dx == 0 and dy == 0 then dx = facing end
    vx = dx * 4.8
    vy = dy * 4.8
    dash_ticks = 8
    dash_cooldown = 42
    sfx(2)
  end
  local dashing = dash_ticks > 0
  if dashing then
    dash_ticks -= 1
    if age % 2 == 0 then emit_light(px, py, 1, 11) end
  else
    vx = vx * 0.8 + dx * 0.36
    vy = vy * 0.8 + dy * 0.36
  end
  -- The garden is a flight space; the water and architecture are scenery.
  px = mid(12, px+vx, 2868)
  py = mid(24, py+vy, 112)
  if px == 12 or px == 2868 then vx = 0 end
  if py == 24 or py == 112 then vy = 0 end

  local first_ember = (flr(px/480)-1)*3
  for slot = 1, 9 do
    local i = first_ember+slot
    if i >= 1 and i <= 18 and not ember_taken(i) and carried < 3 then
      local ex = ember_x(i)
      local ey = ember_y(i) + sin(age/100+i/9)*2
      if abs(px-ex) < 9 and abs(py-ey) < 9 then
        take_ember(i)
        carried += 1
        emit_light(ex, ey, 6, 12)
        sfx(0)
        message = 4
        message_ticks = 65
      end
    end
  end
  local first_shadow = flr(px/240)
  for slot = 1, 3 do
    local i = first_shadow+slot-1
    if i >= 1 and i <= 12 then
      local hx = shadow_x(i)
      local hy = shadow_y(i)
      if shadow_sleep(i) == 0 and abs(px-hx) < 10 and abs(py-hy) < 9 then
        if dashing then
          disperse_shadow(i)
          emit_light(hx, hy, 12, 19)
        elseif invulnerable == 0 then
          health -= 1
          hits += 1
          invulnerable = 75
          vx = sgn(px-hx)*3
          vy = -2
          emit_light(px, py, 8, 28)
          sfx(3)
          if health == 0 then state = 3 end
        end
      end
    end
  end
  find_lantern()
  if pressed_b and state == 1 then kindle() end
  follow_camera(mid(0, px-112, 2640))
end

function _update()
  local pressed_a = btn(5) and not old_a
  local pressed_b = btn(4) and not old_b
  local pressed_start = btn(6) and not old_start
  old_a = btn(5)
  old_b = btn(4)
  old_start = btn(6)
  -- Every ambient period divides 28800, including the 240px scrolling loops.
  -- This keeps their phases continuous when the fixed-point clock wraps.
  if state != 2 then age = (age+1)%28800 end
  if state == 0 then
    if pressed_a or pressed_start then begin_journey() end
  elseif state == 2 then
    if pressed_start or pressed_a then state = 1 end
  elseif state == 3 then
    if pressed_a then
      if checkpoint > 0 then
        px = lantern_x(checkpoint)
        py = lantern_y(checkpoint)-18
      else
        px = 32
        py = 88
      end
      vx = 0
      vy = 0
      returning = true
      health = 3
      invulnerable = 90
      dash_ticks = 0
      dash_cooldown = 0
      state = 1
    elseif pressed_b then
      begin_journey()
    end
  elseif state == 4 then
    if restart_pending then
      begin_journey()
    else
      win_age = min(win_age+1, 300)
      vx *= 0.9
      vy *= 0.9
      px += (dawn_x-px)*0.018
      py += (89-py)*0.025+sin(age/90)*0.08
      local next_stage = min(6, flr(win_age/20))
      local restart_request = win_age > 120 and (pressed_a or pressed_start)
      -- A sky change gets its own upload frame, including out-of-order wins.
      if next_stage == dawn_stage and not restart_request then follow_camera(dawn_view) end
      -- Reset the sky one update before swapping the three terrain banks.
      -- This keeps their map uploads in separate VBlank windows.
      if restart_request then restart_pending = true end
    end
  else
    if pressed_start then
      state = 2
    elseif returning then
      local target = mid(0, px-112, 2640)
      follow_camera(target)
      if abs(target-view_x) < 2 then returning = false end
    else
      step_flight(pressed_a, pressed_b)
    end
  end
  if state != 2 then
    step_ambience()
  end
  dawn_stage = 0
  if state == 4 and not restart_pending then dawn_stage = min(6, flr(win_age/20)) end
  local scene_x = view_x
  if state == 0 then scene_x = 120 end
  prepare_landscape(scene_x)
end

function _draw()
  cls(2)
  local scene_x = view_x
  if state == 0 then scene_x = 120 end

  -- Four independent hardware backgrounds, from sky to foreground.
  -- Dawn moves through an authored sky strip; no palette writes are used.
  local sky_pixel = flr(scene_x/8)
  camera(sky_pixel%256, 0)
  map(0, 0, dawn_stage*6, 0, -dawn_stage*48, 64, 20, 0)
  local far_pixel = flr(scene_x/4)
  camera(far_pixel%256, 0)
  map(1, 0, terrain_bank*20, 0, -terrain_bank*160, 64, 20, 0)
  local garden_pixel = flr(scene_x)
  camera(garden_pixel%256, 0)
  map(2, 0, terrain_bank*20, 0, -terrain_bank*160, 64, 20, 0)

  local front_pixel = flr(scene_x*1.125)
  camera(front_pixel%256, 0)
  map(3, 0, terrain_bank*20, 0, -terrain_bank*160, 64, 20, 0)

  -- One screen-space OBJ camera preserves a precise scanline proof for the
  -- interface. World objects subtract their camera explicitly before drawing.
  camera()
  if state == 4 then pvariant("glow") else pvariant() end
  draw_world(flr(scene_x))
  pvariant()
  draw_interface()
end
