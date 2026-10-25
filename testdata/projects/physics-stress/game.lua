-- Deterministic 2D-physics stress workload for compiler A/B benchmarks.
-- Sixteen record-shaped bodies produce 120 pair checks per 30 Hz tick.

local body_count = 16
local bodies = {}
local gravity = 0.045
local drag = 0.998
local restitution = 0.88
local collision_count = 0
local checksum = 0

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
  local distance_squared = dx * dx + dy * dy

  if distance_squared > 0 and distance_squared < minimum * minimum then
    local inverse_length = 1 / (abs(dx) + abs(dy) + 0.001)
    local nx = dx * inverse_length
    local ny = dy * inverse_length
    local relative_x = b.vx - a.vx
    local relative_y = b.vy - a.vy
    local approach = relative_x * nx + relative_y * ny

    if approach < 0 then
      local impulse = -(1 + restitution) * approach / (a.inv_mass + b.inv_mass)
      local impulse_x = impulse * nx
      local impulse_y = impulse * ny
      a.vx = a.vx - impulse_x * a.inv_mass
      a.vy = a.vy - impulse_y * a.inv_mass
      b.vx = b.vx + impulse_x * b.inv_mass
      b.vy = b.vy + impulse_y * b.inv_mass
    end

    local correction = (minimum * minimum - distance_squared) * 0.006
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
    checksum = checksum + body.x * 3 + body.y * 5 + body.vx * 7 + body.vy * 11
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
  print("pairs:120 hits:" .. tostr(collision_count) .. " sum:" .. tostr(flr(checksum)), 4, 2, 7)
end
