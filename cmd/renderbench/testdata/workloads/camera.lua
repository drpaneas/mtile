frame=0

function _init()
end

function _update()
  frame=(frame+1)%120
  camera(frame%8,0)
end

function _draw()
  cls(225)
  map(0,0,0,0,0,20,15,0)
end
