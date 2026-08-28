-- Milestone 3 pattern music and SFX interaction example.

function _init()
  music(0)
end

function _update()
  if btnp(4) then music(0, 500) end
  if btnp(5) then music(-1, 500) end
  if btnp(2) then sfx(5, 1) end
  if btnp(3) then sfx(5) end
end

function _draw()
  cls(1)
  print("MTILE MUSIC", 8, 16, 15)
  print("INTRO -> A <-> B", 8, 40, 18)
  print("A: FADE IN SONG", 8, 72, 18)
  print("B: FADE OUT MUSIC", 8, 88, 18)
  print("LEFT: INTERRUPT CH 1", 8, 104, 18)
  print("RIGHT: AUTO SFX CH", 8, 120, 18)
  print("4 CHANNEL / 16 KHZ MONO", 8, 144, 6)
end
