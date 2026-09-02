frame=0
probe=0

function _init()
end

function _update()
  frame=(frame+1)%120
  local x=frame%30
  -- Sixteen reads model the solid/ice/spike probes of a platformer update.
  -- Every access is explicitly from logical BG1 rather than legacy BG0.
  probe=mget(1,x,9)+mget(1,(x+1)%30,9)+mget(1,(x+2)%30,9)+mget(1,(x+3)%30,9)
  probe=probe+mget(1,x,10)+mget(1,(x+1)%30,10)+mget(1,(x+2)%30,10)+mget(1,(x+3)%30,10)
  probe=probe+mget(1,x,11)+mget(1,(x+1)%30,11)+mget(1,(x+2)%30,11)+mget(1,(x+3)%30,11)
  probe=probe+mget(1,x,12)+mget(1,(x+1)%30,12)+mget(1,(x+2)%30,12)+mget(1,(x+3)%30,12)
end

function _draw()
  cls(225)
  camera(probe%8,0)
  map(0,0,0,0,0,31,20,0)
  map(1,0,9,0,0,31,11,0)
  map(2,0,13,0,0,31,7,0)
  map(3,0,17,0,0,31,3,0)
  camera(0,0)
end
