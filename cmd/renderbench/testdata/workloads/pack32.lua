frame=0
flip_x=false
flip_y=false

function _update()
  frame=(frame+1)%120
  flip_x=frame%60>=30
  flip_y=frame>=60
end

function _draw()
  cls(0)
  spr(32,16,16,4,4,flip_x,flip_y)
  spr(36,72,16,4,4,not flip_x,flip_y)
end
