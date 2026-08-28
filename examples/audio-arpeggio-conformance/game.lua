-- Deterministic PICO/Mtile arpeggio conformance cartridge.
frame = 0

function _init()
  frame = 0
end

function _update()
  frame = frame + 1
  if frame == 15 then
    sfx(0, 0)
  elseif frame == 45 then
    sfx(1, 0)
  elseif frame == 75 then
    sfx(2, 0)
  elseif frame == 120 then
    sfx(3, 0)
  elseif frame == 165 then
    sfx(-1)
  elseif frame == 180 then
    frame = 0
  end
end

function _draw()
  cls(1)
  print("ARPEGGIO CONFORMANCE", 8, 16, 15)
  print("0.5S: FAST", 8, 48, 18)
  print("1.5S: SLOW", 8, 64, 18)
  print("2.5S: FAST GROUPS", 8, 80, 18)
  print("4.0S: SLOW GROUPS", 8, 96, 18)
  print("CYCLE: 6 SECONDS", 8, 128, 18)
end
