function _init()
end

function _update()
end

function _draw()
  cls(225)
  map(0,0,0,0,0,31,20,0)
  map(1,0,9,0,0,31,11,0)
  -- Current Mode 0 requires objects after every map and emits OBJ priority 0,
  -- so this sprite characterizes the current always-in-front behavior.
  spr(10,116,128)
end
