-- One authored map; one static sailor. Walk to the striped harbour marker.
x = 88
left = false

function _update()
  if btn(0) then x = x - 1 left = true end
  if btn(1) then x = x + 1 left = false end
  x = mid(0, x, 224)
end

function _draw()
  cls(1)
  map(1)
  spr(1, x, 98, 2, 4, left)
  print("TIDEWALK", 8, 5, 31)
  print("LEFT / RIGHT  -  ONE MAP", 8, 154, 31)
end
