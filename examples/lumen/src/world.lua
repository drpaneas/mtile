-- Twelve 240px rooms form six gardens, each with three embers and a lantern.
-- Geometry stays numeric; two nine-bit masks fit the 16.16 number format.
function ember_x(i)
  if i == 1 then return 42 end
  if i == 2 then return 210 end
  if i == 3 then return 346 end
  if i == 4 then return 538 end
  if i == 5 then return 681 end
  if i == 6 then return 836 end
  if i == 7 then return 1010 end
  if i == 8 then return 1144 end
  if i == 9 then return 1303 end
  if i == 10 then return 1508 end
  if i == 11 then return 1656 end
  if i == 12 then return 1805 end
  if i == 13 then return 1964 end
  if i == 14 then return 2113 end
  if i == 15 then return 2270 end
  if i == 16 then return 2462 end
  if i == 17 then return 2605 end
  return 2774
end

function ember_y(i)
  if i == 1 then return 72 end
  if i == 2 then return 40 end
  if i == 3 then return 94 end
  if i == 4 then return 46 end
  if i == 5 then return 98 end
  if i == 6 then return 32 end
  if i == 7 then return 100 end
  if i == 8 then return 48 end
  if i == 9 then return 83 end
  if i == 10 then return 36 end
  if i == 11 then return 90 end
  if i == 12 then return 52 end
  if i == 13 then return 85 end
  if i == 14 then return 30 end
  if i == 15 then return 104 end
  if i == 16 then return 53 end
  if i == 17 then return 99 end
  return 41
end

function ember_taken(i)
  if i <= 9 then return band(ember_mask, shl(1, i-1)) != 0 end
  return band(ember_mask2, shl(1, i-10)) != 0
end

function take_ember(i)
  if i <= 9 then
    ember_mask = bor(ember_mask, shl(1, i-1))
  else
    ember_mask2 = bor(ember_mask2, shl(1, i-10))
  end
end

function lantern_x(i)
  return (i-1)*480+416
end

function lantern_y(i)
  if i == 1 then return 87 end
  if i == 2 then return 72 end
  if i == 3 then return 91 end
  if i == 4 then return 64 end
  if i == 5 then return 88 end
  return 80
end

function lantern_lit(i)
  return band(lantern_mask, shl(1, i-1)) != 0
end

function shadow_x(i)
  local home = (i-1)*240+130
  if i%2 == 0 then home += 7 end
  return home+sin(age/180+(i-1)/3)*22
end

function shadow_y(i)
  local slot = (i-1)%6
  local home = 65
  if slot == 1 then home = 55 end
  if slot == 2 then home = 92 end
  if slot == 3 then home = 45 end
  if slot == 4 then home = 76 end
  if slot == 5 then home = 59 end
  return home+cos(age/150+(i-1)/3)*12
end

function shadow_sleep(i)
  if i == 1 then return sleep1 end
  if i == 2 then return sleep2 end
  if i == 3 then return sleep3 end
  if i == 4 then return sleep4 end
  if i == 5 then return sleep5 end
  if i == 6 then return sleep6 end
  if i == 7 then return sleep7 end
  if i == 8 then return sleep8 end
  if i == 9 then return sleep9 end
  if i == 10 then return sleep10 end
  if i == 11 then return sleep11 end
  return sleep12
end

function disperse_shadow(i)
  if i == 1 then sleep1 = 150 end
  if i == 2 then sleep2 = 150 end
  if i == 3 then sleep3 = 150 end
  if i == 4 then sleep4 = 150 end
  if i == 5 then sleep5 = 150 end
  if i == 6 then sleep6 = 150 end
  if i == 7 then sleep7 = 150 end
  if i == 8 then sleep8 = 150 end
  if i == 9 then sleep9 = 150 end
  if i == 10 then sleep10 = 150 end
  if i == 11 then sleep11 = 150 end
  if i == 12 then sleep12 = 150 end
end

-- Three reusable radial bursts are evaluated analytically while drawing.
-- This retains a light particle effect without an allocated particle table.
function emit_light(x, y, count, shade)
  if burst_cursor == 1 then
    burst_x1 = x
    burst_y1 = y
    burst_n1 = min(6, count)
    burst_color1 = shade
    burst_life1 = 24
  elseif burst_cursor == 2 then
    burst_x2 = x
    burst_y2 = y
    burst_n2 = min(6, count)
    burst_color2 = shade
    burst_life2 = 24
  else
    burst_x3 = x
    burst_y3 = y
    burst_n3 = min(6, count)
    burst_color3 = shade
    burst_life3 = 24
  end
  burst_cursor = burst_cursor%3+1
end

function step_ambience()
  sleep1 = max(0, sleep1-1)
  sleep2 = max(0, sleep2-1)
  sleep3 = max(0, sleep3-1)
  sleep4 = max(0, sleep4-1)
  sleep5 = max(0, sleep5-1)
  sleep6 = max(0, sleep6-1)
  sleep7 = max(0, sleep7-1)
  sleep8 = max(0, sleep8-1)
  sleep9 = max(0, sleep9-1)
  sleep10 = max(0, sleep10-1)
  sleep11 = max(0, sleep11-1)
  sleep12 = max(0, sleep12-1)
  burst_life1 = max(0, burst_life1-1)
  burst_life2 = max(0, burst_life2-1)
  burst_life3 = max(0, burst_life3-1)
  if lantern_lit(1) then lantern_time1 = min(60, lantern_time1+1) end
  if lantern_lit(2) then lantern_time2 = min(60, lantern_time2+1) end
  if lantern_lit(3) then lantern_time3 = min(60, lantern_time3+1) end
  if lantern_lit(4) then lantern_time4 = min(60, lantern_time4+1) end
  if lantern_lit(5) then lantern_time5 = min(60, lantern_time5+1) end
  if lantern_lit(6) then lantern_time6 = min(60, lantern_time6+1) end
end

function lantern_elapsed(i)
  if i == 1 then return lantern_time1/30 end
  if i == 2 then return lantern_time2/30 end
  if i == 3 then return lantern_time3/30 end
  if i == 4 then return lantern_time4/30 end
  if i == 5 then return lantern_time5/30 end
  return lantern_time6/30
end
