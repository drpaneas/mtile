-- The same door, with a gameplay-owned clock that can stop.
opening = false
opening_ticks = 0
paused = false

function _update()
  if btnp(6) then paused = not paused end
  if not paused then
    if btnp(5) then
      opening = true
      opening_ticks = 0
    elseif opening then
      opening_ticks = min(opening_ticks + 1, 30)
    end
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
  if paused then
    print("PAUSED  -  START TO RESUME", 52,150, 21)
  else
    print("A  OPEN    START  PAUSE", 60,150, 9)
  end
end
