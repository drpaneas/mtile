-- Run through verify_tile_helpers.py: repository root, generated atlas fixture.
local repo=assert(arg[1], 'repository root required')
local atlas=dofile(assert(arg[2], 'atlas fixture required'))
local emitted={}
local emission_count=0
function flr(v) return math.floor(v) end
min, max = math.min, math.max
function mid(a,b,c) return math.max(math.min(a,b), math.min(math.max(a,b),c)) end
function add(t,v) table.insert(t,v); return v end
function del(t,v) for i,x in ipairs(t) do if x==v then table.remove(t,i); return end end end
function deli(t) return table.remove(t) end
function all(t) local i=0; return function() i=i+1; return t[i] end end
function rnd() return 0 end
function sin() return 0 end
function pal() end
function music() end
function sfx() end
function cls() end
function camera() end
function map() end
function print() end
function mget() return 0 end
function mset() end
function fget() return false end
function btn() return false end
function spr(id,x,y,w,h,fx,fy)
  emission_count=emission_count+1
  local tile=assert(atlas[id],'missing tile '..id)
  for ty=0,7 do for tx=0,7 do
    local color=tile[(fy and 7-ty or ty)+1][(fx and 7-tx or tx)+1]
    if color~=0 then emitted[(y+ty)..','..(x+tx)]=color end
  end end
end
package.path = repo..'/research/celeste-mode0/src/?.lua;'..package.path
dofile(repo..'/research/celeste-mode0/game.lua')
local cases=0
local function check(expected,label)
  for k,v in pairs(expected) do assert(emitted[k]==v,label..' missing/wrong pixel '..k) end
  for k in pairs(emitted) do assert(expected[k]~=nil,label..' extra pixel '..k) end
  emitted={}; emission_count=0; cases=cases+1
end
local function rectangle(x1,y1,x2,y2,color)
  local expected={}
  for y=math.min(y1,y2),math.max(y1,y2) do
    for x=math.min(x1,x2),math.max(x1,x2) do expected[y..','..x]=color end
  end
  return expected
end
for _,color in ipairs({11,14}) do
  for width=1,2 do for height=1,2 do
    for reversed=0,1 do
      local x1,y1,x2,y2=3,5,2+width,4+height
      if reversed==1 then x1,x2=x2,x1; y1,y2=y2,y1 end
      spr_ambient_rect(x1,y1,x2,y2,color)
      assert(emission_count==1,'every ambient particle must use exactly one tile')
      check(rectangle(x1,y1,x2,y2,color),'ambient')
    end
  end end
end
for _,color in ipairs({249,62}) do
  for width=1,5 do for height=1,5 do
    for reversed=0,1 do
      local x1,y1,x2,y2=-2,7,width-3,6+height
      if reversed==1 then x1,x2=x2,x1; y1,y2=y2,y1 end
      spr_death_rect(x1,y1,x2,y2,color)
      check(rectangle(x1,y1,x2,y2,color),'death')
    end
  end end
end
function draw_hair() end
for id=1,7 do
  for _,color in ipairs({158,214}) do
    for _,flip in ipairs({false,true}) do
      local expected={}
      local djump=1
      if color==214 then djump=0 end
      frames=0
      player={x=11,y=9,sprite=id,flip_x=flip,dead=false,djump=djump,hair={},vx=0}
      freeze=0
      local baked=id
      if color==214 then baked=7+id end
      expected={}
      for y=0,7 do for x=0,7 do
        local c=atlas[baked][y+1][(flip and 7-x or x)+1]
        if c~=0 then expected[(9+y)..','..(11+x)]=c end
      end end
      draw_player()
      check(expected,'player')
    end
  end
end
io.write('PASS ',cases,' exact atlas pixel comparisons: ambient clipping, death rectangles and baked player frames.\n')
