-- Data-oriented particle workload for the compact numeric-array diagnostic.
-- Four fixed arrays hold 64 particles. One physics step performs about 512
-- indexed numeric reads/writes per 30 Hz simulation tick.

local particle_count = 64
local substeps = 1
local gravity = 0.012
local drag = 0.997
local checksum = 0

local px = {
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0
}

local py = {
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0
}

local vx = {
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0
}

local vy = {
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0,
  0, 0, 0, 0, 0, 0, 0, 0
}

function _init()
  for i = 1, particle_count do
    local column = (i - 1) % 16
    local row = flr((i - 1) / 16)
    px[i] = 10 + column * 14
    py[i] = 22 + row * 32
    vx[i] = ((i * 7) % 11 - 5) * 0.031
    vy[i] = ((i * 13) % 9 - 4) * 0.027
  end
end

function _update()
  checksum = 0
  for substep = 1, substeps do
    for i = 1, particle_count do
      local x = px[i]
      local y = py[i]
      local dx = vx[i] * drag
      local dy = (vy[i] + gravity) * drag

      x = x + dx
      y = y + dy
      if x < 4 then
        x = 4
        dx = abs(dx)
      elseif x > 235 then
        x = 235
        dx = -abs(dx)
      end
      if y < 14 then
        y = 14
        dy = abs(dy)
      elseif y > 155 then
        y = 155
        dy = -abs(dy)
      end

      px[i], py[i], vx[i], vy[i] = x, y, dx, dy
      checksum = checksum + x * 3 + y * 5 + dx * 7 + dy * 11 + substep
    end
  end
end

function _draw()
  cls(1)
  rect(2, 11, 237, 157, 6)
  for i = 1, particle_count do
    pset(flr(px[i]), flr(py[i]), 8 + (i % 7))
  end
  print("64 particles / 512 array ops", 4, 2, 7)
end
