-- mapdemo: full-screen map() redraw every tick with camera scroll.
-- Maximum continuous 16:16 world (4095x20 cells); arrow keys move, A jumps.
-- stat(1) is last-tick CPU load (see README for HUD alloc notes).

map_w = 4095
map_h = 20
world_w = map_w * 8

px = 60
py = 136
vx = 0
vy = 0
grounded = false
cam_x = 0

function _init()
  px = 60
  py = 136
  cam_x = 0
end

function solid(x, y)
  local cx = flr(x / 8)
  local cy = flr(y / 8)
  return fget(mget(cx, cy), 0)
end

function update_camera()
  cam_x = mid(0, px - 112, world_w - 240)
end

function _update()
  local speed = 2
  vx = 0
  if btn(0) then vx = -speed end
  if btn(1) then vx = speed end

  if btn(5) and grounded then
    vy = -5
    grounded = false
  end

  vy = min(vy + 0.4, 6)

  local nx = px + vx
  if vx < 0 and not (solid(nx, py) or solid(nx, py + 7)) then
    px = nx
  elseif vx > 0 and not (solid(nx + 7, py) or solid(nx + 7, py + 7)) then
    px = nx
  end

  local ny = py + vy
  if vy > 0 then
    if solid(px, ny + 7) or solid(px + 7, ny + 7) then
      py = flr((ny + 7) / 8) * 8 - 8
      vy = 0
      grounded = true
    else
      py = ny
      grounded = false
    end
  elseif vy < 0 then
    if solid(px, ny) or solid(px + 7, ny) then
      vy = 0
    else
      py = ny
    end
  end

  px = mid(0, px, world_w - 8)
  if py > 160 then py = 136 vy = 0 end

  update_camera()
end

function _draw()
  cls(47)

  camera(cam_x, 0)
  map(flr(cam_x / 8), 0)
  spr(30, px, py)
  camera(0, 0)

  print("cpu", 4, 4, 15)
  print(flr(stat(1) * 100), 28, 4, 15)
  print("arrows move, a jumps", 4, 150, 15)
end
