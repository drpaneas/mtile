#!/usr/bin/env python3
"""Compare dynamic objects with actual smalleste.p8 running in native PICO-8.

Uses a deterministic shared random input to isolate object rules from the
random-number generator. The production source_sin implementation is exercised.
"""
from pathlib import Path
import json, os, re, shutil, subprocess, tempfile
ROOT=Path(__file__).resolve().parents[3]
OUT=Path(tempfile.mkdtemp(prefix="mtile-celeste-entities-"))
PICO=os.environ.get("CELESTE_PICO8_BIN", "/Applications/PICO-8.app/Contents/MacOS/pico8")
src=(ROOT/"celeste/smalleste.p8").read_text()
helpers=src[src.index("function splat"):src.index("-- [globals]")]
objs=src[src.index("spring={"):src.index("message={")]
flag=src[src.index("flag={"):src.index("room_title={")]
engine=src[src.index("function init_object"):src.index("function kill_player")]
maths=src[src.index("function appr"):src.index("function tile_at")]
objs=objs.replace("this.x=128", "this.x=240").replace("this.x>128", "this.x>240")
CASES=[]
def case(name, objects, ticks=90, player=None, events=None):
 CASES.append(dict(name=name,objects=objects,ticks=ticks,player=player or {},events=events or {}))
def e(kind,x=80,y=80,**values): return dict(kind=kind,x=x,y=y,**values)
case("spring_falling",[e(18)],30,dict(x=80,y=76,vx=2,vy=2,djump=0))
case("spring_rising",[e(18)],15,dict(x=80,y=76,vy=-1))
case("spring_crumble_and_return",[e(18),e(23,y=88)],100,dict(x=80,y=76,vy=2))
case("floor_occupied_regrowth",[e(23)],100,dict(x=80,y=72),{70:dict(y=80),85:dict(x=150)})
case("floor_dead_player_probe",[e(23)],20,dict(x=80,y=72),{1:dict(dead=True)})
case("balloon_refill_reappear",[e(22,offset=0.25)],140,dict(x=80,y=76,djump=0),{65:dict(djump=0)})
case("balloon_full_dash",[e(22,offset=0.25)],120,dict(x=80,y=76,djump=1))
case("fruit_float",[e(26)],120)
case("fruit_collect_lifeup",[e(26)],40,dict(x=80,y=80,djump=0))
case("flying_fruit_flee",[e(28)],100,events={15:dict(has_dashed=True)})
case("flying_fruit_collect",[e(28)],40,dict(x=80,y=80,djump=0))
case("platform_carry_right",[e(12)],75,dict(x=80,y=72))
case("platform_carry_left",[e(11)],75,dict(x=80,y=72))
case("platform_wrap_right",[e(12,x=248)],10)
case("platform_wrap_left",[e(11,x=-14)],15)
case("platform_one_way_overlap",[e(12)],40,dict(x=80,y=80))
case("fake_wall_dash",[e(64)],40,dict(x=74,y=80,vx=5,dash_time=4,dash_effect_time=10))
case("key_opens_chest",[e(8),e(20,x=160)],60,dict(x=80,y=80))
case("fruit_mutes_spring",[e(26),e(18)],20,dict(x=80,y=76,vy=2,djump=0))
case("key_idle_animation",[e(8)],65)
case("flag_touch",[e(118)],20,dict(x=85,y=80))
shutil.copyfile(Path(__file__).with_name("numeric_tolerances.json"),OUT/"numeric_tolerances.json")
(OUT/"cases.json").write_text(json.dumps(CASES,indent=2))
def lua(v):
 if isinstance(v,bool): return "true" if v else "false"
 if isinstance(v,(int,float)): return str(v)
 if isinstance(v,str): return json.dumps(v)
 if isinstance(v,list): return "{"+",".join(map(lua,v))+"}"
 return "{"+",".join("["+lua(k)+"]="+lua(x) for k,x in v.items())+"}"
prelude=r'''
player={}
objects={}
got_fruit={}
max_djump=1
spawning=false
frames=0
deaths=0
has_dashed=false
has_key=false
sfx_timer=0
sound_count=0
sound_sum=0
smoke_count=0
function rnd(n) return (tonum(n) or 1)*0.25 end
function sfx(n) sound_count+=1 sound_sum+=tonum(n) end
function psfx(n) if sfx_timer<=0 then sfx(n) end end
function level_index() return 1 end
function tile_at() return 0 end
function fget() return false end
function create_hair() end
function draw_obj_sprite() end
function draw_time() end
function print() end
function spr() end
function rectfill() end
local native_sin=sin
function sin(n)
 local value=native_sin(n)
 printh("sin,"..tostr(n%1,true)..","..tostr(value,true))
 return value
end
'''
runner=r'''
local raw_init=init_object
function init_object(type,x,y,tile)
 local p=raw_init(type,x,y,tile)
 if p then p.init_smoke=function() smoke_count+=1 end end
 return p
end
local kinds={[8]=key,[11]=platform,[12]=platform,[18]=spring,[20]=chest,[22]=balloon,[23]=fall_floor,[26]=fruit,[28]=fly_fruit,[64]=fake_wall,[118]=flag,[-1]=lifeup}
function kindof(p)
 if p.type==platform then return p.dir==-1 and 11 or 12 end
 for k,v in pairs(kinds) do if v==p.type then return k end end
 return 0
end
function num(v) return tostr(v or 0,true) end
function trace_entity(c,t,i,e)
 printh("entity,"..c..","..t..","..i..","..num(kindof(e))..","..
 num(e.spr)..","..num(e.x)..","..num(e.y)..","..num(e.spd.x)..","..num(e.spd.y)..","..
 num(e.rem.x)..","..num(e.rem.y)..","..num(e.hitbox.x)..","..num(e.hitbox.y)..","..num(e.hitbox.w)..","..num(e.hitbox.h)..","..
 num(e.state)..","..num(e.delay)..","..num(e.hide_in)..","..num(e.hide_for)..","..
 num(e.offset)..","..num(e.start)..","..num(e.timer)..","..num(e.off)..","..num(e.sfx_delay)..","..
 num(e.last)..","..num(e.dir)..","..num(e.duration)..","..num(e.flash)..","..
 num(e.flip.x and 1 or 0)..","..num(e.collideable and 1 or 0)..","..num(e.show and 1 or 0)..","..num(e.score))
end
for c in all(cases) do
 objects={} got_fruit={} has_dashed=false has_key=false frames=0
 sound_count=0 sound_sum=0 smoke_count=0 sfx_timer=0
 for ent in all(c.objects) do
  local obj=init_object(kinds[ent.kind],ent.x,ent.y,ent.kind)
  if ent.offset then obj.offset=ent.offset end
 end
 local p=init_object(player,200,120,1)
 p.hitbox=rectangle"1,3,6,5"
 p.solids=true
 p.djump=1 p.dash_time=0 p.dash_effect_time=0
 for k,v in pairs(c.player) do
  if k=="vx" then p.spd.x=v elseif k=="vy" then p.spd.y=v else p[k]=v end
 end
 for t=1,c.ticks do
  if sfx_timer>0 then sfx_timer-=1 end
  frames=(frames+1)%30
  if c.events[t] then
   for k,v in pairs(c.events[t]) do
    if k=="has_dashed" then has_dashed=v else p[k]=v end
    if k=="dead" and v then destroy_object(p) end
   end
  end
  foreach(objects,function(o)
   if o~=p then
    o.move(o.spd.x,o.spd.y,0)
    if o.type.update then o.type.update(o) end
   end
  end)
  foreach(objects,function(o)
   if o~=p and o.type.draw then o.type.draw(o) end
  end)
  local i=0
  for o in all(objects) do
   if o~=p then i+=1 trace_entity(c.name,t,i,o) end
  end
  printh("player,"..c.name..","..t..","..num(p.x)..","..num(p.y)..","..num(p.spd.x)..","..num(p.spd.y)..","..
  num(p.rem.x)..","..num(p.rem.y)..","..num(p.djump)..","..num(p.dash_time)..","..num(p.dash_effect_time)..","..
  num(got_fruit[1] and 1 or 0)..","..num(has_key and 1 or 0)..","..num(sound_count)..","..num(sound_sum)..","..num(smoke_count)..","..num(i)..","..
  num(p.is_solid(0,1) and 1 or 0)..","..num(p.is_solid(1,0) and 1 or 0)..","..num(p.is_solid(0,-1) and 1 or 0)..","..num(sfx_timer))
 end
end
'''
cart="pico-8 cartridge // http://www.pico-8.com\nversion 42\n__lua__\n"+helpers+prelude+objs+flag+engine+maths+"\ncases="+lua(CASES)+"\n"+runner
(OUT/"reference.p8").write_text(cart)
try:
 result=subprocess.run([PICO,"-home",str(OUT),"-x",str(OUT/"reference.p8")],env=dict(os.environ,SDL_VIDEODRIVER="dummy",SDL_AUDIODRIVER="dummy"),capture_output=True,text=True,timeout=20)
except subprocess.TimeoutExpired as e:
 print(e.stdout,e.stderr)
 raise
(OUT/"reference-output.txt").write_text(result.stdout+result.stderr)
def raw(value):
 n=int(value[2:].replace(".",""),16)
 return n-(1<<32) if n&0x80000000 else n
fields="kind spr x y vx vy rx ry hx hy w h state delay hide_in hide_for offset start timer off sfx_delay last dir duration flash flip collideable show score".split()
playerfields="x y vx vy x_remainder y_remainder djump dash_time dash_effect_time berry_collected has_key sound_count sound_sum smoke_count count solid_down solid_right solid_up sfx_timer".split()
rows={}
sine={}
for line in result.stdout.splitlines():
 v=line.split(",")
 if v[0]=="sin": sine[raw(v[1])]=raw(v[2])
 if v[0] not in ("entity","player"): continue
 row=rows.setdefault((v[1],int(v[2])),dict(case=v[1],tick=int(v[2]),entities=[]))
 if v[0]=="entity": row["entities"].append(dict(zip(fields,map(raw,v[4:]))))
 else: row["player"]=dict(zip(playerfields,map(raw,v[3:])))
if result.returncode or len(rows)!=sum(c["ticks"] for c in CASES):
 raise SystemExit("Incomplete native reference: "+str(OUT/"reference-output.txt"))
(OUT/"traces.json").write_text(json.dumps(list(rows.values()),indent=2))
# Build the real entity module in a small controller-independent harness.
project=OUT/"game"
(project/"src").mkdir(parents=True)
shutil.copytree(ROOT/"examples/celeste/assets",project/"assets")
(project/"src/entities.lua").write_text((ROOT/"examples/celeste/src/entities.lua").read_text().replace("rnd(","entity_test_random("))
(project/"project.json").write_text('{"activePalette":"aurora","renderer":"mode4"}')
harness=r'''
require("entities")
require("source_math")
player={}
spawning=false
frames=0
sound_count=0
sound_sum=0
smoke_count=0
sfx_timer=0
level_complete=false
function entity_test_random(n) return n*0.25 end
function effect_smoke() smoke_count=smoke_count+1 end
function record_sound(n,gated) if gated and sfx_timer>0 then return end sound_count=sound_count+1 sound_sum=sound_sum+n end
function play_break_floor() record_sound(15,true) end
function play_berry() record_sound(13,false) end
function play_fruit_release() record_sound(16,false) end
function play_spring() record_sound(8,true) end
function play_balloon() record_sound(6,true) end
function play_reappear() record_sound(7,true) end
function play_winged_fruit() record_sound(14,false) end
function play_key() record_sound(23,false) end
function play_complete() record_sound(55,false) end
function finish_level() level_complete=true play_complete() sfx_timer=30 end
function sign(n) if n<0 then return -1 elseif n>0 then return 1 end return 0 end
function approach(v,target,amount) if v>target then return max(v-amount,target) end return min(v+amount,target) end
function move_x(n,start)
 player.x_remainder=player.x_remainder+n
 local amt=flr(player.x_remainder+0.5)
 player.x_remainder=player.x_remainder-amt
 local step=sign(amt)
 for i=start,abs(amt) do
  if entities_solid(step,0) then player.vx=0 player.x_remainder=0 break end
  player.x=player.x+step
 end
end
function move_y(n,start)
 player.y_remainder=player.y_remainder+n
 local amt=flr(player.y_remainder+0.5)
 player.y_remainder=player.y_remainder-amt
 local step=sign(amt)
 for i=start,abs(amt) do
  if entities_solid(0,step) then player.vy=0 player.y_remainder=0 break end
  player.y=player.y+step
 end
end
function _init()
 player={x=200,y=120,vx=0,vy=0,x_remainder=0,y_remainder=0,
 hitbox_x=1,hitbox_y=3,w=6,h=5,djump=1,dash_time=0,dash_effect_time=0,dead=false}
 entities={}
end
function _update() if sfx_timer>0 then sfx_timer=sfx_timer-1 end frames=(frames+1)%30 entities_update() end
function _draw() entities_draw_platforms() entities_draw() end
'''
shutil.copyfile(ROOT/"examples/celeste/src/source_math.lua",project/"src/source_math.lua")
(project/"game.lua").write_text(harness)
build_env=dict(os.environ, GOCACHE=str(Path(tempfile.gettempdir())/"mtile-celeste-go-cache"), XDG_CACHE_HOME=str(Path(tempfile.gettempdir())/"mtile-celeste-build-cache"))
build=subprocess.run(["go","run","./cmd/mtile","build","-keep-work",str(project)],cwd=ROOT,capture_output=True,text=True,env=build_env)
print(build.stdout,build.stderr)
if build.returncode: raise SystemExit(build.returncode)
work=Path(re.search(r"^workdir (.+)$",build.stdout,re.M).group(1))
shutil.copyfile(Path(__file__).with_name("entities_parity_test.go.tmpl"),work/"entities_parity_test.go")
env=dict(os.environ,CELESTE_ENTITIES_DIR=str(OUT))
env.setdefault("GOCACHE",str(Path(tempfile.gettempdir())/"mtile-celeste-go-cache"))
subprocess.run(["go","test","-run","TestNativeEntities","-v","."],cwd=work,env=env,check=True)
print("Entity native reference, traces, isolated generated-Go harness:",OUT)
