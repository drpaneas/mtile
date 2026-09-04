-- Draw the full logical map while scrolling far into mapdemo's 4095x20 world.
-- The runtime should visit only the cells intersecting the screen-space clip.
function _init()
  tick = 0
end

function _update()
  tick = (tick + 1) % 64
end

function _draw()
  cls(130)
  camera(1600 + tick, 0)
  clip(3, 5, 234, 150)
  map(0)
end
