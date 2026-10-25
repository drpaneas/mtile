-- Arrows stir its wings and turn its head; release to rest.
facing_left = false
fluttering = false

function _update()
  fluttering = btn(0) or btn(1)
  if btn(0) then
    facing_left = true
  elseif btn(1) then
    facing_left = false
  end
end

function _draw()
  cls(1)
  map()
  if fluttering then
    anim("flutter", 88, 48, facing_left)
  else
    anim("idle", 88, 48, facing_left)
  end
end
