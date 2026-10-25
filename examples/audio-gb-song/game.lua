function _init()
  music(0)
end

function _update()
  if btnp(5) then music(0) end
  if btnp(4) then music(-1) end
end

function _draw()
  cls(1)
  print("GAME BOY ADVENTURE", 52, 30, 10)
  print("2 SECTIONS / 4 CHANNELS", 40, 62, 7)
  print("NO PCM - REAL PSG", 60, 88, 11)
  print("A PLAY   B STOP", 54, 128, 6)
end
