-- A starts the door's saved nonlooping clip from its first pose.
opening = false
opening_ticks = 0

function _update()
  if btnp(5) then
    opening = true
    opening_ticks = 0
  elseif opening then
    opening_ticks = min(opening_ticks + 1, 30)
  end
end

function _draw()
  cls(1)
  map()
  if opening then
    anim("open", 96, 56, false, false, opening_ticks / 30)
  else
    pspr(0, 96, 56)
  end
  print("MOON DOOR", 98, 5, 10)
  print("A  OPEN / START AGAIN", 65,150, 9)
end
