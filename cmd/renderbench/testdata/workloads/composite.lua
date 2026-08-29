frame=0

function _init()
end

function _update()
  frame=(frame+1)%120
  camera(frame%8,0)
  pal()
  pal(7,12,0)
  pal(12,14,1)
  palt(0,true)
end

function _draw()
  cls(225)
  clip()
  map(0,0,0,0,20,15,0)
  sspr(72,0,16,16,96,24,16,16,frame>=60,false)
  print("MODE 0",100,8,12)
end
