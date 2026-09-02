function _update()
end

function _draw()
  cls(0)
  spr(32, 16, 16, 2, 2, false, false)
  spr(32, 48, 16, 2, 2, true, false)
  spr(32, 80, 16, 2, 2, false, true)
  spr(32, 112, 16, 2, 2, true, true)
  spr(32, 16, 48, 2, 2, false, false)
  spr(32, 48, 48, 2, 2, true, false)
  spr(32, 80, 48, 2, 2, false, true)
  spr(32, 112, 48, 2, 2, true, true)
end
