-- Experimental upper bound for compiler call-site cloning. The helper body is
-- duplicated manually: numeric calls use numeric_step while the 10% dynamic
-- calls retain generic_step. Runtime behavior matches the baseline exactly.

local body_count = 12
local substeps = 4
local bodies = {}
local impulses = {}
local checksum = 0

function numeric_step(value, velocity, bias)
  local next_value = value + velocity
  next_value = next_value * 0.997
  next_value = next_value + bias
  next_value = next_value - velocity * 0.003
  next_value = next_value + velocity * velocity * 0.0004
  return next_value
end

function generic_step(value, velocity, bias)
  local next_value = value + velocity
  next_value = next_value * 0.997
  next_value = next_value + bias
  next_value = next_value - velocity * 0.003
  next_value = next_value + velocity * velocity * 0.0004
  return next_value
end

function _init()
  for i = 1, body_count do
    local column = (i - 1) % 4
    local row = flr((i - 1) / 4)
    add(bodies, {
      x = 24 + column * 57,
      y = 24 + row * 39,
      vx = ((i * 7) % 9 - 4) * 0.035,
      vy = ((i * 11) % 7 - 3) * 0.028,
      color = 8 + (i % 7)
    })
    add(impulses, ((i * 13) % 11 - 5) * 0.017)
  end
end

function _update()
  checksum = 0
  for substep = 1, substeps do
    for i = 1, body_count do
      local body = bodies[i]
      local predicted = body.x

      for pass = 1, 9 do
        local bias = (pass - substep) * 0.0002
        predicted = numeric_step(predicted, body.vx, bias)
      end

      local impulse = generic_step(impulses[i], body.vy, 0.0001)
      body.vy = (body.vy + 0.012) * 0.996
      body.vx = body.vx + impulse * 0.00003
      body.x = predicted + impulse * 0.00002
      body.y = body.y + body.vy

      if body.x < 5 then
        body.x = 5
        body.vx = abs(body.vx)
      elseif body.x > 234 then
        body.x = 234
        body.vx = -abs(body.vx)
      end
      if body.y < 14 then
        body.y = 14
        body.vy = abs(body.vy)
      elseif body.y > 154 then
        body.y = 154
        body.vy = -abs(body.vy)
      end

      checksum = checksum + body.x * 3 + body.y * 5 + body.vx * 7 + body.vy * 11
    end
  end
end

function _draw()
  cls(1)
  rect(2, 11, 237, 157, 6)
  for i = 1, body_count do
    local body = bodies[i]
    rectfill(flr(body.x - 2), flr(body.y - 2), flr(body.x + 2), flr(body.y + 2), body.color)
  end
  print("calls:480 numeric:90% sum:" .. tostr(flr(checksum)), 4, 2, 7)
end
