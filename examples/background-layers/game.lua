cam_x=0
scroll_period=1920
player_x=116

function _init()
  cam_x=0
end

function _update()
  local step=1
  if btn(0) then
    step=-2
  elseif btn(1) then
    step=2
  end
  cam_x=(cam_x+scroll_period+step)%scroll_period
  player_x=cam_x+116
end

function _draw()
  cls(1)

  local sky_cam=(flr(cam_x/8)+5)%240
  local mountains_cam=(flr(cam_x/4)+6)%240
  local hills_cam=flr(cam_x/2)%240
  local ground_cam=(cam_x+3)%240

  -- BG0: stars and moon, moving at one eighth of world speed.
  camera(sky_cam,0)
  map(0,flr(sky_cam/8),0,0,0,31,12,0)

  -- BG1: distant mountains.
  camera(mountains_cam,0)
  map(1,flr(mountains_cam/8),9,0,0,31,11,0)

  -- BG2: nearer hills.
  camera(hills_cam,0)
  map(2,flr(hills_cam/8),13,0,0,31,7,0)

  -- BG3: foreground ground, moving at full world speed.
  camera(ground_cam,0)
  map(3,flr(ground_cam/8),17,0,0,31,3,0)

  -- Objects come after every map pass and are currently in front of all BGs.
  player_x=ground_cam+116
  spr(10,player_x,128)

  camera(0,0)
  print("left/right override auto",4,4,5)
end
