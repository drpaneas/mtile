-- Left/right selects walking and facing; release to return to idle.
facing_left = false
walking = false

function _update()
  walking = btn(0) or btn(1)
  if btn(0) then
    facing_left = true
  elseif btn(1) then
    facing_left = false
  end
end

function _draw()
  cls()
  if walking then
    anim("walk", 100, 60, facing_left)
  else
    anim("idle", 100, 60, facing_left)
  end
end
