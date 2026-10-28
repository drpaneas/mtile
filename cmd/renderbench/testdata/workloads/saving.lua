-- All field and status operations run on ARM against cartridge SRAM.
-- This source is paired with examples/save-game assets by renderbench.
local tick = 0
local errors = 0
function _update()
  tick = (tick + 1) % 100
  if tick % 8 == 1 then
    for i = 0, 63 do
      if not save_set(i, i - 31.5) then errors += 1 end
    end
    if save_set(-1, 9) or save_set(64, 9) or save_set(0.5, 9) or
       save_set(false, 9) or save_set(0, false) then errors += 1 end
    if save_get(-1) != 0 or save_get(64) != 0 or save_get(false) != 0 then errors += 1 end
    if save_write(("mtile-save-bench-v1")) != 1 then errors += 1 end
    for i = 0, 63 do save_set(i, 0) end
    if save_read("mtile-save-bench-v1") != 1 then errors += 1 end
    for i = 0, 63 do
      if save_get(i) != i - 31.5 then errors += 1 end
    end
    if save_read("different-key") != -3 or save_get(63) != 31.5 then errors += 1 end
  end
end
function _draw()
  cls(1)
  spr(1, 24 + tick, 80)
  if errors == 0 then
    print("SAVE OK", 8, 8, 7)
  else
    print("SAVE ERROR", 8, 8, 7)
  end
end
