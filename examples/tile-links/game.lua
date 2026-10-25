-- Windy Meadow: tile links.
-- Every flower and lamp in this map was painted in Map Lab as a shared tile.
-- Those two tiles are linked in Sprite Lab, so map() draws the "sway" clip and
-- the "lamp" pose at each one instead. This file never draws them itself.

FLOWER = 4 -- the shared tile linked to the "sway" clip
FLOWER_ROW = 15
cam = 0
picked = 0

function _update()
  if btn(0) then cam = max(0, cam - 2) end
  if btn(1) then cam = min(64 * 8 - 240, cam + 2) end
  if btnp(5) then
    local x = flr((cam + 120) / 8)
    -- The map still holds the linked tile, so gameplay can find flowers.
    if mget(x, FLOWER_ROW) == FLOWER then
      -- Replacing the tile removes its flower.
      mset(x, FLOWER_ROW, 0)
      picked += 1
    end
  end
end

function _draw()
  cls(24)
  camera(cam, 0)
  map()
  camera()
  spr(3, 116, 120) -- the column A picks from
  print("picked", 4, 4, 15)
  print(picked, 44, 4, 15)
  print("arrows scroll, a picks", 4, 150, 15)
end
