function _init()
  music(0)
end

function _update()
  if btnp(4) then
    sample(0)
  end
  if btnp(5) then
    sample(1)
  end
  if btnp(2) then
    sample(0, 96, 200)
  end
  if btnp(3) then
    sample(-1)
  end
  if btnp(6) then
    music(0, 500)
  end
  if btnp(7) then
    music(-1, 500)
  end
end

function _draw()
  cls(1)
  print("NATIVE PCM + MUSIC", 8, 8, 10)
  print("A: IMPACT", 8, 28, 7)
  print("B: LOOPING ENGINE", 8, 44, 7)
  print("LEFT: QUIET HIGH PRIORITY", 8, 60, 6)
  print("RIGHT: STOP PCM VOICES", 8, 76, 8)
  print("START: FADE MUSIC IN", 8, 92, 7)
  print("SELECT: FADE MUSIC OUT", 8, 108, 8)
  print("2 SYNTH + PCM / 16 KHZ", 8, 132, 5)
  print("MEASURED SAFE MIXED PROFILE", 8, 144, 5)
end
