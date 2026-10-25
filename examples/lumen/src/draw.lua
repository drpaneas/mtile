-- Lumen's hardware presentation: every visible object is an authored pose.
-- Named clips own frame order, speed, looping, and stable flip bounds.
-- All loops have fixed maximum counts so Mode 0 can prove the OBJ budget.

function draw_water(scroll)
  for i = 1, 6 do
    local x = (i*79+age/10-scroll)%240
    local y = 121+(i*11)%32
    pspr(RIPPLE, x, y)
  end
  local first = flr(scroll/480)
  for slot = 1, 3 do
    local i = first+slot-1
    if i >= 1 and i <= 6 and lantern_lit(i) then
      local x = lantern_x(i)+sin(age/100+i/8)*3
      pspr(REFLECTION, x-scroll-16, 123)
    end
  end
end

function draw_lanterns(scroll)
  local first = flr(scroll/480)
  for slot = 1, 3 do
    local i = first+slot-1
    if i >= 1 and i <= 6 then
      local lx = lantern_x(i)-scroll
      local ly = lantern_y(i)
      if lantern_lit(i) then
        anim("lantern-lit", lx-8, ly-8, false, false, age/30)
        anim("bloom-opening", lx-8, 107, false, false, lantern_elapsed(i))
        for j = 1, 2 do
          local turn = age/160+j/2
          pspr(MOTE_GOLD, lx+cos(turn)*15-3, ly+sin(turn)*11-3)
        end
      else
        pspr(LANTERN_DARK, lx-8, ly-8)
        pspr(BLOOM_BUD, lx-8, 107)
      end
    end
  end
end

function draw_embers(scroll)
  local first = (flr(scroll/480)-1)*3
  for slot = 1, 9 do
    local i = first+slot
    if i >= 1 and i <= 18 and not ember_taken(i) then
      local x = ember_x(i)-scroll
      local y = ember_y(i)+sin(age/100+i/9)*2
      anim("ember-glow", x-4, y-4, false, false, age/30+i/5)
    end
  end
end

function draw_shadows(scroll)
  local first = flr(scroll/240)
  for slot = 1, 3 do
    local i = first+slot-1
    if i >= 1 and i <= 12 and shadow_sleep(i) == 0 and state != 4 then
      anim("shadow-drift", shadow_x(i)-scroll-8, shadow_y(i)-8, false, false, age/30)
    end
  end
end

function draw_moth(x, y)
  if invulnerable == 0 or age%6 < 4 or state != 1 then
    if dash_ticks > 0 then
      anim("moth-dash", x-8, y-8, facing < 0, false, age/30)
    else
      anim("moth-flight", x-8, y-8, facing < 0, false, age/30)
    end
    for i = 1, 3 do
      if i <= carried then
        local turn = age/96+i/3
        pspr(GLINT, x+cos(turn)*11-3, y+sin(turn)*8-3)
      end
    end
  end
end

function draw_burst(x, y, life, count, shade, scroll)
  if life > 0 then
    local elapsed = 24-life
    local radius = elapsed*1.5-elapsed*elapsed*0.025
    for i = 1, 6 do
      if i <= count then
        local turn = i/count
        local bx = x-scroll+cos(turn)*radius-3
        local by = y+sin(turn)*radius-elapsed*0.2-3
        if shade == 28 then
          pspr(PARTICLE_ROSE, bx, by)
        elseif shade == 19 then
          pspr(PARTICLE_MINT, bx, by)
        elseif life > 12 then
          pspr(PARTICLE_WHITE, bx, by)
        else
          pspr(PARTICLE_GOLD, bx, by)
        end
      end
    end
  end
end

function draw_world(scroll)
  draw_water(scroll)
  for i = 1, 5 do
    local x = (i*79+age/20-scroll)%240
    local y = 32+(i*37)%80+sin(age/150+i/6)*3
    pspr(MOTE_MINT, x-3, y-3)
  end
  draw_lanterns(scroll)
  if state != 0 then
    draw_embers(scroll)
    draw_shadows(scroll)
    draw_moth(px-scroll, py)
    draw_burst(burst_x1, burst_y1, burst_life1, burst_n1, burst_color1, scroll)
    draw_burst(burst_x2, burst_y2, burst_life2, burst_n2, burst_color2, scroll)
    draw_burst(burst_x3, burst_y3, burst_life3, burst_n3, burst_color3, scroll)
  end
end

function draw_hud()
  pspr(UI_HEADER, 0, 0)
  if lit_count == 0 then
    pspr(UI_DIGIT0, 120, 5)
  elseif lit_count == 1 then
    pspr(UI_DIGIT1, 120, 5)
  elseif lit_count == 2 then
    pspr(UI_DIGIT2, 120, 5)
  elseif lit_count == 3 then
    pspr(UI_DIGIT3, 120, 5)
  elseif lit_count == 4 then
    pspr(UI_DIGIT4, 120, 5)
  elseif lit_count == 5 then
    pspr(UI_DIGIT5, 120, 5)
  else
    pspr(UI_DIGIT6, 120, 5)
  end
  for i = 1, 3 do
    if i <= health then
      pspr(UI_WING_LIT, 199+i*9, 5)
    else
      pspr(UI_WING_DARK, 199+i*9, 5)
    end
  end
  if state != 4 then
    pspr(UI_FOOTER, 0, 144)
    for i = 1, 3 do
      if i <= carried then
        pspr(UI_EMBER_LIT, 51+i*9, 148)
      else
        pspr(UI_EMBER_DARK, 51+i*9, 148)
      end
    end
    anim("dash-recharge", 137, 151, false, false, (42-dash_cooldown)/30)
    if near_lantern > 0 then
      if carried == 3 then
        pspr(UI_LIGHT_READY, 185, 148)
        local x = flr(lantern_x(near_lantern)-view_x)
        pspr(LANTERN_HINT, x-2, lantern_y(near_lantern)-22+sin(age/45))
      else
        pspr(UI_LIGHT, 185, 148)
      end
    else
      pspr(UI_GATHER, 174, 148)
    end
    if message_ticks > 0 then
      if message == 0 then
        pspr(MESSAGE_GATHER, 63, 24)
      elseif message == 1 then
        pspr(MESSAGE_NEEDS, 39, 24)
      elseif message == 2 then
        pspr(MESSAGE_REMEMBERS, 60, 24)
      elseif message == 3 then
        pspr(MESSAGE_BRING, 48, 24)
      elseif message == 4 and carried == 3 then
        pspr(MESSAGE_FIND, 51, 24)
      end
    elseif play_ticks < 420 then
      pspr(MESSAGE_DASH, 51, 24)
    end
  end
end

function draw_title()
  pspr(WORDMARK, 56, 31)
  pspr(TITLE_SUBTITLE, 66, 69)
  pspr(UI_ORNAMENT, 104, 83)
  draw_moth(120, 96+sin(age/90)*3)
  pspr(TITLE_BEGIN, 66, 114)
  pspr(TITLE_CONTROLS, 0, 144)
end

function draw_interface()
  if state == 0 then
    draw_title()
  else
    draw_hud()
    if state == 2 then
      pspr(UI_PAUSE, 32, 47)
    elseif state == 3 then
      pspr(UI_REST, 32, 47)
    elseif state == 4 then
      if win_age > 75 then pspr(UI_DAWN, 54, 24) end
      if win_age > 120 then pspr(DAWN_RESTART, 0, 144) end
    end
  end
end
