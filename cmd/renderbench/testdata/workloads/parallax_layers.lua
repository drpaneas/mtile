frame=0

function _init()
end

function _update()
  frame=(frame+1)%120
end

function _draw()
  cls(225)

  local sky_cam=(flr(frame/8)+5)%240
  local mountains_cam=(flr(frame/4)+6)%240
  local hills_cam=flr(frame/2)%240
  local ground_cam=(frame+3)%240

  camera(sky_cam,0)
  map(0,flr(sky_cam/8),0,0,0,31,12,0)

  camera(mountains_cam,0)
  map(1,flr(mountains_cam/8),9,0,0,31,11,0)

  camera(hills_cam,0)
  map(2,flr(hills_cam/8),13,0,0,31,7,0)

  camera(ground_cam,0)
  map(3,flr(ground_cam/8),17,0,0,31,3,0)

  camera(0,0)
end
