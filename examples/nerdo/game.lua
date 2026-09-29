facing_left = false

function _draw()
  cls()

  if btn(0) then
    facing_left = true
  elseif btn(1) then
    facing_left = false
  end

  if btn(0) or btn(1) then
    anim("walk", 100, 60, facing_left)
  else
    anim("idle", 100, 60, facing_left)
  end
end