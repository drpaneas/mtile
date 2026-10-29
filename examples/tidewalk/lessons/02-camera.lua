-- Same map and sailor. The world is now two screens wide.
x = 88
left = false

function _update()
  if btn(0) then x = x - 1 left = true end
  if btn(1) then x = x + 1 left = false end
  x = mid(0, x, 464)
end

function _draw()
  cls(1)
  local scroll = mid(0, x - 112, 240)
  camera(scroll, 0)
  map(1)
  spr(1, x, 98, 2, 4, left)
  camera(0, 0)
  print("TIDEWALK", 8, 5, 31)
  print("LEFT / RIGHT  -  FIND THE LIGHTHOUSE", 8,150, 31)
end
