frame=0

function _init()
end

function _update()
  frame=(frame+1)%120
  sset(8,0,(frame%2)*12)
  sset(16,0,(frame%2)*12)
  sset(24,0,(frame%2)*12)
  sset(32,0,(frame%2)*12)
  sset(40,0,(frame%2)*12)
  sset(48,0,(frame%2)*12)
  sset(56,0,(frame%2)*12)
  sset(64,0,(frame%2)*12)
  sset(72,0,(frame%2)*12)
  sset(80,0,(frame%2)*12)
  sset(88,0,(frame%2)*12)
  sset(96,0,(frame%2)*12)
  sset(104,0,(frame%2)*12)
  sset(112,0,(frame%2)*12)
  sset(120,0,(frame%2)*12)
  sset(128,0,(frame%2)*12)
end

function _draw()
  cls(225)
  map(0,0,0,0,0,20,15,0)
end
