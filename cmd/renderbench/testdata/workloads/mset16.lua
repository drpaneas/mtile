frame=0

function _init()
end

function _update()
  frame=(frame+1)%120
  mset(0,0,9+(frame%2))
  mset(1,0,9+(frame%2))
  mset(2,0,9+(frame%2))
  mset(3,0,9+(frame%2))
  mset(4,0,9+(frame%2))
  mset(5,0,9+(frame%2))
  mset(6,0,9+(frame%2))
  mset(7,0,9+(frame%2))
  mset(8,0,9+(frame%2))
  mset(9,0,9+(frame%2))
  mset(10,0,9+(frame%2))
  mset(11,0,9+(frame%2))
  mset(12,0,9+(frame%2))
  mset(13,0,9+(frame%2))
  mset(14,0,9+(frame%2))
  mset(15,0,9+(frame%2))
end

function _draw()
  cls(225)
  map(0,0,0,0,20,15,0)
end
