sprite = 0

function _update()
  sprite += 1
  if sprite == 9 then sprite = 1 end
end

function _draw()
  cls(0)
  spr(sprite, 24, 24)
  spr(sprite, 56, 24)
  spr(sprite, 88, 24)
  spr(sprite, 120, 24)
  spr(sprite, 24, 56)
  spr(sprite, 56, 56)
  spr(sprite, 88, 56)
  spr(sprite, 120, 56)
  spr(sprite, 24, 88)
  spr(sprite, 56, 88)
  spr(sprite, 88, 88)
  spr(sprite, 120, 88)
  spr(sprite, 24, 120)
  spr(sprite, 56, 120)
  spr(sprite, 88, 120)
  spr(sprite, 120, 120)
end
