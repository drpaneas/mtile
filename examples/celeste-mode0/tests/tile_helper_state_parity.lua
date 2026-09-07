-- Run through verify_tile_helpers.py, which supplies the repository root.
local repo = assert(arg[1], "repository root required") .. "/"
local function clone(x)
  if type(x) ~= "table" then return x end
  local r = {}; for k,v in pairs(x) do r[k]=clone(v) end; return r
end
local function equal(a,b,path)
  path = path or "value"
  assert(type(a)==type(b),path.." type mismatch")
  if type(a)=="table" then
    for k,v in pairs(a) do equal(v,b[k],path.."."..tostring(k)) end
    for k in pairs(b) do assert(a[k]~=nil,path.." extra "..tostring(k)) end
  else
    if type(a)=="number" and type(b)=="number" then
      assert(math.abs(a-b)<1e-9,path..": "..tostring(a).." != "..tostring(b))
    else assert(a==b,path..": "..tostring(a).." != "..tostring(b)) end
  end
end
local e = setmetatable({commands={},frames=0,input_mask=0,seed=12345,player={}}, {__index=_G})
e.flr=math.floor; e.min=math.min; e.max=math.max
e.mid=function(a,b,c) return math.max(math.min(a,b),math.min(math.max(a,b),c)) end
e.btn=function(n) return e.input_mask & (1 << n) ~= 0 end
e.add=function(t,v) table.insert(t,v); return v end
e.del=function(t,v) for i,x in ipairs(t) do if x==v then table.remove(t,i); return end end end
e.deli=function(t) return table.remove(t) end
e.all=function(t) local i=0; return function() i=i+1; return t[i] end end
e.rnd=function(n) e.seed=(e.seed*16807)%2147483647; return e.seed/2147483647*(n or 1) end
e.sin=function() return 0 end
e.cos=function() return 1 end
e.freeze=0
e.pal=function() end
e.spr=function() end
e.print=function() end
e.cls=function() end
e.camera=function() end
e.map=function() end
e.music=function() end
e.sfx=function() end
e.mget=function() return 0 end
e.mset=function() end
e.fget=function() return false end
e.loaded_modules={}
e.require=function(name)
  if e.loaded_modules[name] then return end
  e.loaded_modules[name]=true
  assert(loadfile(repo.."examples/celeste-mode0/src/"..name..".lua","t",e))()
end
assert(loadfile(repo.."examples/celeste-mode0/game.lua","t",e))()
local cases=0
e.player={x=24,y=32,vx=0,dead=false,flip_x=false,sprite=1,djump=1,hair={}}
e.create_hair()
assert(#e.player.hair==5,"create_hair must emit five nodes")
local origin=clone(e.player.hair)
e.draw_player()
equal(origin,e.player.hair,"draw_player must not move hair")
cases=cases+1
e.update_hair()
assert(not pcall(equal,origin,e.player.hair),"update_hair must move hair")
cases=cases+1
for djump=0,1 do
  e.player={x=24,y=32,vx=0,dead=false,flip_x=false,sprite=1,djump=djump,hair={}}
  e.create_hair()
  for tick=0,31 do
    e.player.x=24+math.min(tick,31-tick)/4
    e.player.y=32-tick/8
    e.player.flip_x=tick>=16
    e.input_mask=tick%5==0 and 8 or 0
    local before=clone(e.player.hair)
    e.update_hair()
    local after=clone(e.player.hair)
    e.draw_player()
    equal(after,e.player.hair,"draw after update "..djump.."/"..tick)
    if tick>0 then
      assert(not pcall(equal,before,after),"hair must follow the player "..djump.."/"..tick)
    end
    cases=cases+1
  end
end
io.write("PASS ",cases," hair snapshots: update moves nodes, draw does not.\n")
