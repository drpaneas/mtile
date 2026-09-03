MAP_LAYER=1
TILE_SIZE=8
ROOM_TILES_W=30
ROOM_TILES_H=20
FLAG_SOLID=0
PLAYER_IDLE=1

player={x=16,y=120,width=8,height=8}
hits=0

function outside_room(tx,ty)
  return tx<0 or tx>=ROOM_TILES_W or ty<0 or ty>=ROOM_TILES_H
end

function solid_tile(tx,ty)
  if outside_room(tx,ty) then return true end
  local tile_id=mget(MAP_LAYER,tx,ty)
  return fget(tile_id,FLAG_SOLID)
end

function solid_rect(x,y,w,h)
  local left=flr(x/TILE_SIZE)
  local right=flr((x+w-1)/TILE_SIZE)
  local top=flr(y/TILE_SIZE)
  local bottom=flr((y+h-1)/TILE_SIZE)
  for ty=top,bottom do
    for tx=left,right do
      if solid_tile(tx,ty) then return true end
    end
  end
  return false
end

function _update()
  if solid_rect(player.x+1,player.y,player.width,player.height) then hits+=1 end
  if solid_rect(player.x-1,player.y,player.width,player.height) then hits+=1 end
  if solid_rect(player.x,player.y+1,player.width,player.height) then hits+=1 end
  if solid_rect(player.x,player.y-1,player.width,player.height) then hits+=1 end
end

function _draw()
  cls(0)
  map(1,0,0,0,0,30,20,0)
  spr(PLAYER_IDLE,player.x,player.y)
end

