-- Stateful background tile using only the current map and sprite APIs.

block_x=14
block_y=14
block_gold=4
block_white=5
block_used_tile=6

frame=0
block_used=false
jump_tick=0
player_y=128
coin_x=112
coin_y=-16
coins=0

function reset_block()
  frame=0
  block_used=false
  jump_tick=0
  player_y=128
  coin_x=112
  coin_y=-16
  coins=0
end

function _init()
  reset_block()
end

function _update()
  frame=(frame+1)%180

  if btnp(4) then
    reset_block()
  end

  -- A starts the same short head-bump used by the automatic teaching demo.
  if not block_used and jump_tick==0 and (btnp(5) or frame==90) then
    jump_tick=1
  end

  if jump_tick>0 then
    if jump_tick<=4 then
      player_y=128-jump_tick*2
    else
      player_y=120+(jump_tick-4)*2
    end

    if jump_tick==4 and not block_used then
      block_used=true
      coins=1
      coin_x=112
      coin_y=104
    end

    jump_tick=jump_tick+1
    if jump_tick>8 then
      jump_tick=0
      player_y=128
    end
  end

  -- The one awarded coin rises from the block, then stays in the HUD.
  if coins==1 and coin_x<200 then
    coin_y=coin_y-2
    if coin_y<=80 then
      coin_x=216
      coin_y=8
    end
  end

  -- One direct mset() keeps this shape inside the measured AUTO profile.
  local tile=block_used_tile
  if not block_used then
    tile=block_gold
    if flr(frame/8)%2==1 then
      tile=block_white
    end
  end
  mset(block_x,block_y,tile)
end

function _draw()
  cls(1)
  map()
  spr(3,112,player_y)
  spr(7,coin_x,coin_y)
  print("a hit  b reset  auto",4,4,15)
end
