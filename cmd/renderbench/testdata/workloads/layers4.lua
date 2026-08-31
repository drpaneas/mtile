function _init()
end

function _update()
end

function _draw()
  cls(225)
  -- Four authored logical planes are composited back-to-front at their natural
  -- world rows. The first argument is the stable BG identity.
  map(0,0,0,0,0,31,12,0)
  map(1,0,9,0,0,31,11,0)
  map(2,0,13,0,0,31,7,0)
  map(3,0,17,0,0,31,3,0)
end
