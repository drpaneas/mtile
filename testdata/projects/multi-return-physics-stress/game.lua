-- Deterministic tuple-heavy 2D physics workload for multi-return ABI A/B.
-- Twelve bodies, four substeps, and five tuple helpers per body produce
-- 240 multi-return calls per 30 Hz simulation tick.

local body_count = 12
local substeps = 4
local bodies = {}
local gravity = 0.035
local drag = 0.997
local checksum = 0
local ticks = 0

function accelerate(vx, vy, phase)
  local wind = (phase % 5 - 2) * 0.0015
  return (vx + wind) * drag, (vy + gravity) * drag
end

function rotate_velocity(vx, vy)
  return vx * 0.999 - vy * 0.003, vx * 0.003 + vy * 0.999
end

function advance_position(x, y, vx, vy)
  return x + vx, y + vy
end

function contain_axis(position, velocity, low, high)
  if position < low then
    return low, abs(velocity) * 0.91
  elseif position > high then
    return high, -abs(velocity) * 0.91
  else
    return position, velocity
  end
end

function weighted_pair(x, y, vx, vy)
  return x * 3 + vx * 7, y * 5 + vy * 11
end

function _init()
  for i = 1, body_count do
    local column = (i - 1) % 4
    local row = flr((i - 1) / 4)
    add(bodies, {
      x = 22 + column * 62,
      y = 20 + row * 42,
      vx = ((i * 7) % 9 - 4) * 0.11,
      vy = ((i * 11) % 7 - 3) * 0.09,
      color = 8 + (i % 7)
    })
  end
end

function physics_step()
  for pass = 1, substeps do
    for i = 1, body_count do
      local body = bodies[i]
      body.vx, body.vy = accelerate(body.vx, body.vy, pass + i)
      body.vx, body.vy = rotate_velocity(body.vx, body.vy)
      body.x, body.y = advance_position(body.x, body.y, body.vx, body.vy)
      body.x, body.vx = contain_axis(body.x, body.vx, 5, 234)
      body.y, body.vy = contain_axis(body.y, body.vy, 13, 155)
    end
  end

  checksum = 0
  for i = 1, body_count do
    local body = bodies[i]
    local horizontal, vertical = weighted_pair(body.x, body.y, body.vx, body.vy)
    checksum = checksum + horizontal + vertical
  end
  ticks = ticks + 1
end

function _update()
  physics_step()
end

function _draw()
  cls(1)
  rect(3, 11, 236, 157, 6)
  for i = 1, body_count do
    local body = bodies[i]
    rectfill(flr(body.x) - 2, flr(body.y) - 2, flr(body.x) + 2, flr(body.y) + 2, body.color)
  end
  print("tuple:240 tick:" .. tostr(ticks) .. " sum:" .. tostr(flr(checksum)), 4, 2, 7)
end
