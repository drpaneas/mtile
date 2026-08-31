-- Deterministic function-heavy 2D-physics workload for compiler A/B tests.
-- The numeric helpers execute about 280 calls per 30 Hz simulation tick.

local body_count = 12
local bodies = {}
local gravity = 0.045
local drag = 0.998
local restitution = 0.88
local collision_count = 0
local checksum = 0

function square(value)
  return value * value
end

function length_squared(x, y)
  return square(x) + square(y)
end

function reciprocal_manhattan(x, y)
  return 1 / (abs(x) + abs(y) + 0.001)
end

function dot2(ax, ay, bx, by)
  return ax * bx + ay * by
end

function weighted_state(x, y, vx, vy)
  return x * 3 + y * 5 + vx * 7 + vy * 11
end

function _init()
  for i = 1, body_count do
    local column = (i - 1) % 4
    local row = flr((i - 1) / 4)
    add(bodies, {
      x = 24 + column * 60,
      y = 20 + row * 31,
      vx = ((i * 7) % 7 - 3) * 0.21,
      vy = ((i * 11) % 9 - 4) * 0.17,
      radius = 3 + (i % 3) * 0.5,
      inv_mass = 0.55 + (i % 5) * 0.11,
      color = 8 + (i % 7)
    })
  end
end

function integrate_body(body)
  body.vy = (body.vy + gravity) * drag
  body.vx = body.vx * drag
  body.x = body.x + body.vx
  body.y = body.y + body.vy

  if body.x - body.radius < 3 then
    body.x = 3 + body.radius
    body.vx = abs(body.vx) * restitution
  elseif body.x + body.radius > 236 then
    body.x = 236 - body.radius
    body.vx = -abs(body.vx) * restitution
  end

  if body.y - body.radius < 12 then
    body.y = 12 + body.radius
    body.vy = abs(body.vy) * restitution
  elseif body.y + body.radius > 156 then
    body.y = 156 - body.radius
    body.vy = -abs(body.vy) * restitution
  end
end

function solve_pair(a, b)
  local dx = b.x - a.x
  local dy = b.y - a.y
  local minimum = a.radius + b.radius
  local distance_squared = length_squared(dx, dy)

  if distance_squared > 0 and distance_squared < square(minimum) then
    local inverse_length = reciprocal_manhattan(dx, dy)
    local nx = dx * inverse_length
    local ny = dy * inverse_length
    local relative_x = b.vx - a.vx
    local relative_y = b.vy - a.vy
    local approach = dot2(relative_x, relative_y, nx, ny)

    if approach < 0 then
      local impulse = -(1 + restitution) * approach / (a.inv_mass + b.inv_mass)
      local impulse_x = impulse * nx
      local impulse_y = impulse * ny
      a.vx = a.vx - impulse_x * a.inv_mass
      a.vy = a.vy - impulse_y * a.inv_mass
      b.vx = b.vx + impulse_x * b.inv_mass
      b.vy = b.vy + impulse_y * b.inv_mass
    end

    local correction = (square(minimum) - distance_squared) * 0.006
    a.x = a.x - nx * correction
    a.y = a.y - ny * correction
    b.x = b.x + nx * correction
    b.y = b.y + ny * correction
    collision_count = collision_count + 1
  end
end

function physics_step()
  for i = 1, body_count do
    integrate_body(bodies[i])
  end

  for i = 1, body_count - 1 do
    local a = bodies[i]
    for j = i + 1, body_count do
      solve_pair(a, bodies[j])
    end
  end

  checksum = 0
  for i = 1, body_count do
    local body = bodies[i]
    checksum = checksum + weighted_state(body.x, body.y, body.vx, body.vy)
  end
end

function _update()
  physics_step()
end

function _draw()
  cls(1)
  rect(2, 11, 237, 157, 6)
  for i = 1, body_count do
    local body = bodies[i]
    rectfill(
      flr(body.x - body.radius),
      flr(body.y - body.radius),
      flr(body.x + body.radius),
      flr(body.y + body.radius),
      body.color
    )
  end
  print("pairs:66 calls:~280 hits:" .. tostr(collision_count) .. " sum:" .. tostr(flr(checksum)), 4, 2, 7)
end
