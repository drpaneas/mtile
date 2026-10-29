-- All field and status operations run on ARM against cartridge SRAM.
-- This source is paired with examples/save-game assets by renderbench.
local tick = 0
local errors = 0
local checked_slots = 0
function _update()
  tick = (tick + 1) % 100
  if tick % 8 == 1 then
    local slot = flr(tick/8)%3
    if tick > 40 then slot = 31 end
    for i = 0, 63 do
      if not save_set(i, i - 31.5 + slot) then errors += 1 end
    end
    if save_set(-1, 9) or save_set(64, 9) or save_set(0.5, 9) or
       save_set(false, 9) or save_set(0, false) then errors += 1 end
    if save_get(-1) != 0 or save_get(64) != 0 or save_get(false) != 0 then errors += 1 end
    if slot == 0 then
      if save_write(("mtile-save-bench-v1")) != 1 then errors += 1 end
    elseif slot == 31 then
      if save_write("mtile-save-bench-v1",31) != 1 then errors += 1 end
    else
      if save_write("mtile-save-bench-v1",slot) != 1 then errors += 1 end
    end
    if not save_pending() then errors += 1 end
    if save_read("mtile-save-bench-v1",false) != -5 or
       save_write("mtile-save-bench-v1",32) != -5 or
       save_read("mtile-save-bench-v1",0.5) != -5 then errors += 1 end
    for i = 0, 63 do save_set(i, 0) end
    if slot == 0 then
      if save_read("mtile-save-bench-v1") != 1 then errors += 1 end
    elseif slot == 31 then
      if save_read("mtile-save-bench-v1",31) != 1 then errors += 1 end
    else
      if save_read("mtile-save-bench-v1",slot) != 1 then errors += 1 end
    end
    for i = 0, 63 do
      if save_get(i) != i - 31.5 + slot then errors += 1 end
    end
    if save_read("different-key",slot) != -3 or save_get(63) != 31.5 + slot then errors += 1 end
    if slot == 0 then checked_slots = bor(checked_slots,1)
    elseif slot == 1 then checked_slots = bor(checked_slots,2)
    elseif slot == 2 then checked_slots = bor(checked_slots,4)
    else checked_slots = bor(checked_slots,8) end
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
