frame=0

function _init()
end

function _update()
  frame=(frame+1)%120
  local tile=9+(frame%2)
  mset(1,0,9,tile)
  mset(1,1,9,tile)
  mset(1,2,9,tile)
  mset(1,3,9,tile)
  mset(1,4,9,tile)
  mset(1,5,9,tile)
  mset(1,6,9,tile)
  mset(1,7,9,tile)
  mset(1,8,9,tile)
  mset(1,9,9,tile)
  mset(1,10,9,tile)
  mset(1,11,9,tile)
  mset(1,12,9,tile)
  mset(1,13,9,tile)
  mset(1,14,9,tile)
  mset(1,15,9,tile)
end

function _draw()
  cls(225)
  map(1,0,9,0,0,31,11,0)
end
