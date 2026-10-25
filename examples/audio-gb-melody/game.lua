function _init()
  music(0)
end

function _update()
  if btnp(5) then music(0) end
  if btnp(4) then music(-1) end
end

function _draw()
  cls(1)
  print("GAME BOY HARDWARE MELODY", 18, 24, 10)
  print("CH1 PULSE + SWEEP", 32, 54, 9)
  print("CH2 PULSE", 32, 70, 11)
  print("CH3 WAVE", 32, 86, 12)
  print("CH4 NOISE DRUMS", 32, 102, 8)
  print("A PLAY   B STOP", 54, 132, 7)
end
