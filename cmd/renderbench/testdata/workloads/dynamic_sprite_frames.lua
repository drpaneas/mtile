FRAME_IDLE = 1
FRAME_RUN_A = 32
FRAME_RUN_B = 35

player_sprite = FRAME_IDLE
animation_tick = 0

function _update()
  animation_tick = (animation_tick + 1) % 12
  if animation_tick < 4 then
    player_sprite = FRAME_IDLE
  elseif animation_tick < 8 then
    player_sprite = FRAME_RUN_A
  else
    player_sprite = FRAME_RUN_B
  end
end

function _draw()
  cls(0)
  spr(player_sprite, 24, 24)
  spr(player_sprite, 56, 24)
  spr(player_sprite, 88, 24)
  spr(player_sprite, 120, 24)
  spr(player_sprite, 24, 56)
  spr(player_sprite, 56, 56)
  spr(player_sprite, 88, 56)
  spr(player_sprite, 120, 56)
  spr(player_sprite, 24, 88)
  spr(player_sprite, 56, 88)
  spr(player_sprite, 88, 88)
  spr(player_sprite, 120, 88)
  spr(player_sprite, 24, 120)
  spr(player_sprite, 56, 120)
  spr(player_sprite, 88, 120)
  spr(player_sprite, 120, 120)
end
