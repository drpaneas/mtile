frame=0

function _init()
end

function _update()
  frame=(frame+1)%120
  fset(9,0,frame%2==0)
end

function _draw()
  cls(225)
  map(0,0,0,0,0,20,15,1)
end
