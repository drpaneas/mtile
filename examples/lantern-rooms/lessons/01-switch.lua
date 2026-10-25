-- A chooses another authored room. The code also resets the spawn.
room = 0
player_x = 24
visited = false
returned = false

function _update()
  if btnp(5) then
    room = 1 - room
    player_x = 24
    if room == 1 then visited = true end
    if room == 0 and visited then returned = true end
  end
end

function _draw()
  cls(1)
  map(0, room * 30, 0, -room * 240, 0, 30, 20)
  pspr(0, player_x, 112)
  if room == 0 then print("MOON ROOM", 20, 8, 6)
  else print("AMBER ROOM", 20, 8, 6) end
  if returned then print("HOME AGAIN. A: SWITCH ROOM", 12, 150, 6)
  else print("A: VISIT BOTH ROOMS AND RETURN", 12, 150, 6) end
end
