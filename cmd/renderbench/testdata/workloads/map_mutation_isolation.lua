frame=0

function _init()
end

function _update()
  frame=(frame+1)%120
  -- Only the first map pass below contains logical cell (0,0).
  mset(0,0,9+(frame%2))
end

function _draw()
  cls(225)
  map(0,0,0,0,0,31,20,0)
  map(1,0,9,0,0,31,11,0)
end
