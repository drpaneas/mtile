#!/usr/bin/env python3
"""Compare the generated controller with the actual local cartridge in PICO-8."""
from pathlib import Path
import argparse, json, os, re, shutil, subprocess, sys, tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--build-dir', type=Path, help='Reuse a current mtile -keep-work build')
parser.add_argument('--gba', action='store_true', help='Also smoke-test the ROM in the native mGBA player')
args = parser.parse_args()
ROOT=Path(__file__).resolve().parents[3]
PROJECT=Path(__file__).resolve().parents[1]
OUT=Path(tempfile.mkdtemp(prefix=f'mtile-{PROJECT.name}-parity-'))
if args.gba:
 raise SystemExit('The copied verify_gba.py targets the original Celeste project. Use the native Mode 0 candidate diagnostic for this project instead.')
PICO8=os.environ.get('CELESTE_PICO8_BIN') or shutil.which('pico8') or '/Applications/PICO-8.app/Contents/MacOS/pico8'
if not Path(PICO8).is_file():
 raise SystemExit('Set CELESTE_PICO8_BIN to the installed PICO-8 executable.')
src=(ROOT/'celeste/smalleste.p8').read_text()
helpers=src[src.index('function splat'):src.index('-- [globals]')]
player=src[src.index('player={'):src.index('function create_hair')]
objects=src[src.index('function init_object'):src.index('function destroy_object')]
maths=src[src.index('function appr'):src.index('function tile_at')]
spikes=src[src.index('function spikes_at'):src.index('__gfx__')]
# Only adaptation: one GBA-sized room, same 8px tile scale.
player=player.replace('this.y>128','this.y>160').replace('mid(this.x,-1,121)','mid(this.x,-1,233)')
objects=objects.replace('min(15,(obj.right()+ox)/8)','min(29,(obj.right()+ox)/8)').replace('min(15,(obj.bottom()+oy)/8)','min(19,(obj.bottom()+oy)/8)')
spikes=spikes.replace('min(15,x2/8)','min(29,x2/8)').replace('min(15,y2/8)','min(19,y2/8)')
source_map=json.loads((PROJECT/'assets/map.json').read_text())
source_sprites=json.loads((PROJECT/'assets/spritesheet.json').read_text())
flags={int(s['id']):s.get('flags',0) for s in source_sprites['sprites']}
terrain=next(x['cells'] for x in source_map['layers'] if x['id']==3)
# The playable room remains 30x20; the map's other half repeats cloud motifs.
localcells=[terrain[y*source_map['width']+x] for y in range(20) for x in range(30)]
# Asset-local upward spike 22 is the reference's original 17.
localcells=[17 if t==22 else 201 if flags.get(t,0)&16 else 200 if flags.get(t,0)&1 else 0 for t in localcells]
blank=[0]*600
floor=blank.copy()
for x in range(30): floor[17*30+x]=200
ledge=blank.copy()
for x in range(4,10): ledge[17*30+x]=200
walls=floor.copy()
for y in range(3,17): walls[y*30+5]=walls[y*30+15]=200
ice=walls.copy()
for i,t in enumerate(ice):
 if t==200: ice[i]=201
ceiling=floor.copy()
for x in range(5,15): ceiling[11*30+x]=200
maps={'blank':blank,'floor':floor,'ledge':ledge,'walls':walls,'ice':ice,'ceiling':ceiling,'local':localcells}
A=32; B=16; R=2; L=1; U=4; D=8 # logical PICO button bitmask
cases=[]
def seq(*segments): return [key for n,key in segments for _ in range(n)]
def case(name,mapname='floor',x=64,y=128,initial=None,inputs=None):
 cases.append({'name':name,'map':mapname,'initial':dict(x=x,y=y,**(initial or {})),'inputs':inputs or [0]*30})
case('walk_release_reverse',inputs=seq((12,R),(12,0),(20,L),(8,L|R),(10,0)))
case('ice_walk_release_reverse','ice',x=80,inputs=seq((20,R),(15,0),(35,L),(10,R)))
case('jump_hold_and_release',inputs=seq((1,B),(10,B),(20,0),(1,B),(2,0),(12,R)))
case('ceiling_jump','ceiling',inputs=seq((1,B),(30,0)))
case('coyote_jump','ledge',x=76,inputs=seq((2,R),(1,R|B),(25,R)))
case('coyote_expired','ledge',x=76,inputs=seq((10,R),(1,R|B),(20,R)))
case('jump_buffer','floor',x=64,y=117,initial={'vy':2},inputs=seq((2,0),(1,B),(25,0)))
case('wall_slide_left','walls',x=47,y=58,initial={'vy':2},inputs=seq((20,L),(15,0),(1,L|B),(25,L)))
case('wall_slide_right','walls',x=113,y=58,initial={'vy':2},inputs=seq((20,R),(1,R|B),(25,R)))
case('ice_wall_no_slide','ice',x=47,y=58,initial={'vy':2},inputs=seq((15,L),(1,L|B),(20,L)))
case('wall_jump_three_pixel_probe','walls',x=49,y=58,inputs=seq((1,B),(20,0)))
for label,direction in [('right',R),('left',L),('up',U),('down',D),('up_right',U|R),('up_left',U|L),('down_right',D|R),('down_left',D|L),('no_direction',0),('opposed',L|R|U|D)]:
 case('dash_'+label,'blank',x=100,y=70,inputs=seq((1,A|direction),(2,0),(4,L),(1,A|R),(12,0)))
case('dash_no_direction_left','blank',x=100,y=70,initial={'flip_x':True},inputs=seq((1,A),(20,0)))
case('dash_ground_recharge_held',inputs=seq((12,A|R),(1,0),(1,A|L),(15,0)))
case('dash_inputs_during_freeze','blank',x=100,y=70,inputs=seq((1,A|R),(1,B),(1,A|U),(1,0),(1,B),(20,0)))
case('dash_into_wall','walls',x=108,y=70,inputs=seq((1,A|R),(20,R)))
case('dash_into_ceiling','ceiling',x=64,y=105,inputs=seq((1,A|U),(20,0)))
case('land_during_dash','floor',x=64,y=121,inputs=seq((1,A|D),(15,0),(1,A|U),(10,0)))
case('left_screen_clamp','blank',x=0,y=70,inputs=seq((1,A|L),(20,L)))
case('right_screen_clamp','blank',x=233,y=70,inputs=seq((1,A|R),(20,R)))
for orient,tile,x,y,vx,vy in [('up',17,80,78,0,2),('down',27,80,86,0,-2),('left',43,87,80,-2,0),('right',59,73,80,2,0)]:
 arr=blank.copy();arr[10*30+10]=tile; maps['spike_'+orient]=arr
 case('spike_'+orient,'spike_'+orient,x=x,y=y,initial={'vx':vx,'vy':vy},inputs=[0]*10)
 # Set pre-move frame already intersecting strip but retreating.
 case('spike_'+orient+'_moving_away','spike_'+orient,x=80,y=80,initial={'vx':-vx,'vy':-vy},inputs=[0]*6)
case('top_exit','blank',x=100,y=-3,initial={'vy':-2},inputs=[0]*4)
case('bottom_death','blank',x=100,y=159,initial={'vy':2},inputs=[0]*4)
case('local_walk_to_spikes','local',x=16,y=128,inputs=seq((40,R)))
case('local_jump_dash','local',x=16,y=128,inputs=seq((1,B),(4,R),(1,A|R|U),(30,R),(1,B),(20,L)))
case('local_wall_jump','local',x=16,y=128,inputs=seq((5,L),(1,L|B),(10,L),(1,B),(20,R)))
completion_route=json.loads(Path(__file__).with_name('level_completion_route.json').read_text())
case('local_complete_level','local',x=16,y=128,inputs=completion_route['inputs'])
(OUT/'cases.json').write_text(json.dumps({'maps':maps,'cases':cases},indent=2))
def lua(v):
 if v is None:return 'nil'
 if v is True:return 'true'
 if v is False:return 'false'
 if isinstance(v,str):return json.dumps(v)
 if isinstance(v,(int,float)):return str(v)
 if isinstance(v,list) and all(isinstance(x,(int,float)) for x in v):return 'split('+json.dumps(','.join(str(x) for x in v))+')'
 if isinstance(v,list):return '{'+','.join(lua(x) for x in v)+'}'
 return '{'+','.join('['+lua(k)+']='+lua(x) for k,x in v.items())+'}'
prelude='''
objects={}
got_fruit={}
max_djump=1
freeze=0
pause_player=false
function create_hair() end
function set_hair_color() end
function draw_hair() end
function draw_obj_sprite() end
function unset_hair_color() end
function psfx() end
function level_index() return 1 end
function next_room() exited=true end
function btn(n) return band(input_mask,shl(1,n))~=0 end
function fget(t,f) return (t==200 and f==0) or (t==201 and (f==0 or f==4)) end
function tile_at(x,y) return cells[1+x+y*30] or 0 end
function kill_player(obj) obj.dead=true del(objects,obj) end
'''
runner='''
local function number(v) return tostr(v or 0,true) end
local function boolean(v) return v and "1" or "0" end
function trace(name,t,p)
 printh(name..","..t..","..
 number(p.x)..","..number(p.y)..","..number(p.spd.x)..","..number(p.spd.y)..","..
 number(p.rem.x)..","..number(p.rem.y)..","..
 number(p.djump)..","..number(p.grace)..","..number(p.jbuffer)..","..
 number(p.dash_time)..","..number(p.dash_effect_time)..","..
 number(p.dash_target_x)..","..number(p.dash_target_y)..","..
 number(p.dash_accel_x)..","..number(p.dash_accel_y)..","..
 boolean(p.flip.x)..","..boolean(p.p_jump)..","..boolean(p.p_dash)..","..
 boolean(p.was_on_ground)..","..number(freeze)..","..boolean(p.dead)..","..boolean(exited))
end
for c in all(cases) do
 cells=maps[c.map]
 objects={}
 freeze=0
 exited=false
 input_mask=0
 local p=init_object(player,c.initial.x,c.initial.y,1)
 p.init_smoke=function() end
 p.spd.x=c.initial.vx or 0
 p.spd.y=c.initial.vy or 0
 p.flip.x=c.initial.flip_x or false
 for t=1,#c.inputs do\n  local key=c.inputs[t]
  input_mask=key
  if freeze>0 then
   freeze-=1
  else
   p.move(p.spd.x,p.spd.y,0)
   player.update(p)
  end
  if freeze<=0 and not p.dead then player.draw(p) end
  trace(c.name,t,p)
  if p.dead or exited then break end
 end
end
'''
# Exercise the source spawn object and native foreach mutation semantics too.
spawn=src[src.index('player_spawn={'):src.index('spring={')].replace('this.y=128','this.y=160')
lifecycle_cases=[
 {'name':'spawn_neutral','inputs':[0]*65},
 {'name':'spawn_held_jump','inputs':[B]*65},
 {'name':'spawn_held_dash','inputs':[A|R]*65},
 {'name':'death_and_respawn','inputs':seq((35,0),(20,R),(80,0))},
 {'name':'respawn_held_jump','inputs':seq((35,0),(20,R),(80,B))},
 {'name':'respawn_held_dash','inputs':seq((35,0),(20,R),(80,A))},
]
(OUT/'lifecycle-cases.json').write_text(json.dumps(lifecycle_cases,indent=2))
lifecycle_runner=r'''
function sfx() end
function destroy_object(obj) del(objects,obj) end
local raw_init_object=init_object
function init_object(type,x,y,tile)
 local p=raw_init_object(type,x,y,tile)
 p.init_smoke=function() end
 last_entity=p
 return p
end
function load_room()
 objects={}
 init_object(player_spawn,16,128,1)
end
function kill_player(p)
 p.dead=true
 del(objects,p)
 delay_restart=15
end
for c in all(lifecycle_cases) do
 freeze=0
 delay_restart=0
 input_mask=0
 exited=false
 cells=maps["local"]
 load_room()
 for t=1,#c.inputs do
  input_mask=c.inputs[t]
  if freeze>0 then
   freeze-=1
  else
   if delay_restart>0 then
    delay_restart-=1
    if delay_restart==0 then load_room() end
   end
   foreach(objects,function(p)
    p.move(p.spd.x,p.spd.y,0)
    p.type.update(p)
   end)
  end
  if freeze<=0 then
   foreach(objects,function(p) p.type.draw(p) end)
  end
  local p=last_entity
  local phase=#objects==0 and 0 or p.type==player_spawn and 1 or 2
  printh("life,"..c.name..","..t..","..phase..","..
   number(p.x)..","..number(p.y)..","..number(p.spd.x)..","..number(p.spd.y)..","..
   number(p.rem.x)..","..number(p.rem.y)..","..number(delay_restart)..","..number(freeze)..","..
   number(phase==1 and p.state or 0)..","..number(phase==1 and p.delay or 0))
 end
end
'''
# init_object calls create_hair only; later smoke disabled immediately.
cart='pico-8 cartridge // http://www.pico-8.com\nversion 42\n__lua__\n'+helpers+prelude+player+objects+maths+spikes+'\nmaps={}\n'+''.join('maps['+lua(k)+']=split('+lua(','.join(str(t) for t in v))+')\n' for k,v in maps.items())+'\ncases='+lua(cases)+'\n'+runner+spawn+'\nlifecycle_cases='+lua(lifecycle_cases)+'\n'+lifecycle_runner
(OUT/'reference.p8').write_text(cart)
try:
 proc=subprocess.run([PICO8,'-home',str(OUT),'-x',str(OUT/'reference.p8')],env=dict(os.environ,SDL_VIDEODRIVER='dummy',SDL_AUDIODRIVER='dummy'),capture_output=True,text=True,timeout=10)
except subprocess.TimeoutExpired as e:
 print((e.stdout or b'').decode()[-4000:])
 raise
(OUT/'reference-output.txt').write_text(proc.stdout+proc.stderr)
rows=[]
fields='x y vx vy x_remainder y_remainder djump grace jbuffer dash_time dash_effect_time dash_target_x dash_target_y dash_accel_x dash_accel_y flip_x jump_was_down dash_was_down was_on_ground freeze dead exited'.split()
for line in proc.stdout.splitlines():
 parts=line.split(',')
 if len(parts)!=len(fields)+2:continue
 values=[]
 for p in parts[2:]:
  if p.startswith('0x'):
   raw=int(p[2:].replace('.',''),16)
   if raw&0x80000000:raw-=1<<32
   values.append(raw)
  else:values.append(p=='1')
 rows.append(dict(case=parts[0],tick=int(parts[1]),state=dict(zip(fields,values))))
(OUT/'reference-traces.json').write_text(json.dumps(rows,indent=2))
completion_rows=[r for r in rows if r['case']=='local_complete_level']
if not completion_rows or not completion_rows[-1]['state']['exited'] or any(r['state']['dead'] for r in completion_rows):
 raise SystemExit('The native PICO-8 reference did not complete the stored route without dying.')
print('returncode',proc.returncode,'cases',len(cases),'trace rows',len(rows))
if not rows: print(proc.stdout,proc.stderr)

life=[]
for line in proc.stdout.splitlines():
 parts=line.split(',')
 if len(parts)!=14 or parts[0]!='life':continue
 raw=[]
 for v in parts[4:]:
  n=int(v[2:].replace('.',''),16)
  raw.append(n-(1<<32) if n&0x80000000 else n)
 life.append(dict(case=parts[1],tick=int(parts[2]),phase=int(parts[3]),values=raw))
(OUT/'lifecycle-traces.json').write_text(json.dumps(life,indent=2))
print('lifecycle rows',len(life))

if proc.returncode or not rows or not life:
 raise SystemExit('PICO-8 reference failed; see '+str(OUT/'reference-output.txt'))

shutil.copyfile(Path(__file__).with_name('numeric_tolerances.json'),OUT/'numeric_tolerances.json')
build_dir=args.build_dir
env=dict(os.environ,CELESTE_PARITY_DIR=str(OUT))
env.setdefault('GOCACHE',str(Path(tempfile.gettempdir())/'mtile-celeste-go-cache'))
if build_dir is None:
 build=subprocess.run(['go','run','./cmd/mtile','build','-keep-work',str(PROJECT)],cwd=ROOT,env=env,check=True,text=True,capture_output=True)
 print(build.stdout,end='')
 match=re.search(r'^workdir (.+)$',build.stdout,re.M)
 if not match:raise SystemExit('Build did not report a kept workspace')
 build_dir=Path(match.group(1))
shutil.copyfile(Path(__file__).with_name('physics_parity_test.go.tmpl'),build_dir/'physics_parity_test.go')
# The generated candidate main installs a Mode 0 text atlas and calls its
# initialization lifecycle. Compile those same renderer features for the host
# controller harness; the default Mode 4 runtime cannot provide these methods.
host_tags='mtile_mode0,mtile_mode0_layers4,mtile_mode0_obj,mtile_mode0_text'
subprocess.run(['go','test','-tags='+host_tags,'-run','TestNativePico8(Physics|Lifecycle)','-v','.'],cwd=build_dir,env=env,check=True)
print('Verified candidate project:',PROJECT)
print('Generated candidate build:',build_dir)
print('Reference inputs, raw16.16 traces and PICO-8 cartridge:',OUT)
