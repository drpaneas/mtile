function _init()
end

function _update()
end

function _draw()
  cls(225)
  map(0,0,0,0,0,31,20,0)
  map(1,0,9,0,0,31,11,0)
  -- An object after every map is the front lane and emits OBJ priority 0.
  -- This fixture keeps the established always-in-front compatibility case.
  spr(10,116,128)
end
