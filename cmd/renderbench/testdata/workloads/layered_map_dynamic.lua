frame=0
layer=1
probe=0

function _init()
end

function _update()
  frame=(frame+1)%120
  if frame%2==0 then
    layer=1
  else
    layer=2
  end
  probe=mget(layer,3,9)
  mset(layer,3,9,9+(frame%2))
end

function _draw()
  cls(225)
  camera(probe%4,0)
  map(1,0,9,0,0,31,11,0)
  map(2,0,9,0,0,31,11,0)
  camera(0,0)
end
