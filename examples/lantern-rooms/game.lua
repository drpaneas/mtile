-- Crossing an edge chooses the next room and its opposite-edge spawn.
room = 0
player_x = 24
facing_left = false
visited = false
returned = false

function _init()
  pause(true) -- This example starts directly in gameplay.
end

function _update()
  if btn(0) then player_x -= 2 facing_left = true end
  if btn(1) then player_x += 2 facing_left = false end
  if player_x > 224 then
    if room == 0 then
      room = 1 player_x = 8 visited = true
    else player_x = 224 end
  elseif player_x < 0 then
    if room == 1 then
      room = 0 player_x = 216 returned = visited
    else player_x = 0 end
  end
end

function _draw()
  cls(1)
  map(0, room * 30, 0, -room * 240, 0, 30, 20)
  pspr("lantern_keeper", player_x, 112, facing_left)
  if room == 0 then print("MOON ROOM", 20, 8, 6)
  else print("AMBER ROOM", 20, 8, 6) end
  if returned then print("HOME AGAIN. EXPLORE AT YOUR PACE", 12, 150, 6)
  elseif room == 1 then print("LEFT: RETURN TO THE MOON ROOM", 12, 150, 6)
  else print("RIGHT: VISIT THE AMBER ROOM", 12, 150, 6) end
end
