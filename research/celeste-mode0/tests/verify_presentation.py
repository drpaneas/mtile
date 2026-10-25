#!/usr/bin/env python3
"""Verify presentation against native PICO-8, bounding only accepted ambient drift."""
from pathlib import Path
import argparse
import json
import os
import re
import shutil
import subprocess
import tempfile


def lua(value):
    if isinstance(value, bool):
        return 'true' if value else 'false'
    if isinstance(value, str):
        return json.dumps(value)
    if isinstance(value, (int, float)):
        return str(value)
    if isinstance(value, list):
        return '{' + ','.join(lua(v) for v in value) + '}'
    return '{' + ','.join('[' + lua(k) + ']=' + lua(v) for k, v in value.items()) + '}'


def raw(value):
    number = int(value[2:].replace('.', ''), 16)
    return number - (1 << 32) if number & 0x80000000 else number


parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--build-dir', type=Path, help='Reuse a current mtile -keep-work build')
parser.add_argument('--reference-only', action='store_true', help='Generate the native PICO-8 oracle without building mtile')
args = parser.parse_args()
ROOT = Path(__file__).resolve().parents[3]
PROJECT = Path(__file__).resolve().parents[1]
OUT = Path(tempfile.mkdtemp(prefix='mtile-celeste-presentation-'))
env = dict(os.environ, CELESTE_PRESENTATION_DIR=str(OUT))
env.setdefault('GOCACHE', str(Path(tempfile.gettempdir()) / 'mtile-celeste-go-cache'))
env.setdefault('XDG_CACHE_HOME', str(Path(tempfile.gettempdir()) / 'mtile-celeste-cache'))
PICO8 = os.environ.get('CELESTE_PICO8_BIN') or shutil.which('pico8') or '/Applications/PICO-8.app/Contents/MacOS/pico8'
if not Path(PICO8).is_file():
    raise SystemExit('Set CELESTE_PICO8_BIN to the installed PICO-8 executable.')
src = (ROOT / 'celeste/smalleste.p8').read_text()
animation = src[src.index('  -- animation'):src.index('  -- exit level off the top')]
hair = src[src.index('function create_hair'):src.index('-- [other entities]')]
smoke = src[src.index('smoke={'):src.index('fruit={')]
death_init = src[src.index(' dead_particles={}\n', src.index('function kill_player')):src.index(' --restart_room()', src.index('function kill_player'))]
death_draw = src[src.index(' foreach(dead_particles', src.index('function _draw')):src.index(' -- credits', src.index('function _draw'))]
cloud_draw = src[src.index('  foreach(clouds', src.index('function _draw')):src.index('\n end', src.index('  foreach(clouds', src.index('function _draw'))) + len('\n end')]
# Strip the surrounding `if not is_title()` end; GBA room dimensions are intentional.
cloud_draw = cloud_draw.rsplit('\n end', 1)[0].replace('c.x>128', 'c.x>240').replace('rnd"120"', 'rnd"152"')
ambient_draw = src[src.index(' foreach(particles', src.index('function _draw')):src.index(' -- dead particles', src.index('function _draw'))]
ambient_draw = ambient_draw.replace('p.x>132', 'p.x>244').replace('rnd"128"', 'rnd"160"')

animation_cases = []
for ground in (False, True):
    for wall in (False, True):
        for direction in (-1, 0, 1):
            for velocity in (-1, 0, 1):
                for keys in (0, 4, 8, 12):
                    animation_cases.append(dict(ground=ground, wall=wall, direction=direction, vx=velocity, keys=keys))

hair_cases = []
for label in ('walk_turn', 'crouch_air', 'subpixel'):
    steps = []
    for tick in range(32):
        if label == 'walk_turn':
            x, y, flip, keys = 24 + min(tick, 31-tick), 32, tick >= 16, 0
        elif label == 'crouch_air':
            x, y, flip, keys = 48, 48 - min(tick, 31-tick), tick >= 24, 8 if tick < 12 else 0
        else:
            x, y, flip, keys = 32 + tick / 4, 32 - tick / 8, tick % 8 >= 4, 8 if tick % 5 == 0 else 0
        steps.append(dict(x=x, y=y, flip=flip, keys=keys))
    hair_cases.append(dict(name=label, x=steps[0]['x'], y=steps[0]['y'], steps=steps))

smoke_cases = [
    dict(name='fractional', x=32.25, y=40.75, vx=0.375, vy=-0.0999908447265625),
    dict(name='negative', x=-0.75, y=10.25, vx=0.499969482421875, vy=-0.0999908447265625),
    dict(name='minimum', x=64, y=80, vx=0.29998779296875, vy=-0.0999908447265625),
]
cloud_cases = [dict(x=10.25, y=20, spd=1.5, w=40), dict(x=238, y=50, spd=4.5, w=48)]
ambient_cases = [
    dict(x=40, y=50, s=1, spd=0.25, off=0, c=6),
    dict(x=243, y=70, s=0, spd=2.5, off=0.123443603515625, c=7),
    # Non-wrapping, off-grid phases expose stock-sine drift independently
    # of random wrapping positions and the decimal 0.05 phase cap.
    dict(x=80, y=100, s=1, spd=0.25, off=0.123443603515625, c=7),
    dict(x=10, y=100, s=1, spd=2.5, off=0.123443603515625, c=7),
]
ambient_pixels = [dict(x=x, y=y, s=s, spd=0, off=0, c=6) for x in (-0.25,0,0.25,95.75) for y in (-0.25,0,0.25,95.75) for s in (0,1)]
fixtures = dict(animation=animation_cases, hair=hair_cases, smoke=smoke_cases, clouds=cloud_cases, ambient=ambient_cases, ambient_pixels=ambient_pixels)
(OUT / 'presentation-inputs.json').write_text(json.dumps(fixtures, indent=2))
shutil.copyfile(Path(__file__).with_name('level_completion_route.json'), OUT / 'completion-route.json')

prelude = '''
function vector(x,y) return {x=x,y=y} end
function btn(n) return band(input_mask,shl(1,n))~=0 end
function crectfill(x1,y1,x2,y2,c)
 if x1<240 and x2>0 and y1<160 and y2>0 then
  rectfill(max(0,x1),max(0,y1),min(239,x2),min(159,y2),c)
 end
end
function number(v) return tostr(v or 0,true) end
function destroy_object(obj) obj.dead=true end
-- Random sequences intentionally differ between engines. Fixed wrap samples
-- isolate movement, wrapping, and dimensions from RNG implementation.
function rnd(n) return (tonum(n) or n or 1)*0.25 end
function pixels()
 local result=""
 for y=0,95 do for x=0,95 do
  if pget(x,y)~=0 then result..=(result=="" and "" or ":")..(y*96+x) end
 end end
 return result
end
function trace_table(kind,t,items,fields)
 local result=""
 for p in all(items) do for key in all(fields) do
  result..=","..number(p[key])
 end end
 printh(kind..","..t..","..#items..result)
end
'''
runner = '''
for djump=0,2 do for frame=0,5 do
 frames=frame
 set_hair_color(djump)
 printh("color,"..(djump*6+frame+1)..","..number(band(peek(0x5f08),15)))
end end
pal()
function reference_animation(this,h_input,on_ground)
''' + animation + '''
end
local actor={spr_off=0,spd={x=0}}
for i,c in ipairs(animation_cases) do
 actor.spd.x=c.vx
 actor.is_solid=function() return c.wall end
 input_mask=c.keys
 reference_animation(actor,c.direction,c.ground)
 printh("animation,"..i..","..number(actor.spr_off)..","..number(actor.spr))
end
for c in all(hair_cases) do
 local p={x=c.x,y=c.y,flip={x=false}}
 create_hair(p)
 for tick,step in ipairs(c.steps) do
  p.x=step.x p.y=step.y p.flip.x=step.flip input_mask=step.keys
  cls(0)
  draw_hair(p)
  local values=""
  for node in all(p.hair) do values..=","..number(node.x)..","..number(node.y) end
  printh("hair,"..c.name..","..tick..values..","..pixels())
 end
end
for c in all(smoke_cases) do
 local p={x=c.x,y=c.y,spd=vector(c.vx,c.vy),rem=vector(0,0),spr=29}
 for tick=1,17 do
  if not p.dead then
   for axis in all{"x","y"} do
    p.rem[axis]+=p.spd[axis]
    local amt=flr(p.rem[axis]+0.5)
    p.rem[axis]-=amt
    p[axis]+=amt
   end
   smoke.update(p)
  end
  printh("smoke,"..c.name..","..tick..","..number(p.x)..","..number(p.y)..","..number(p.rem.x)..","..number(p.rem.y)..","..number(p.spr)..","..(p.dead and 1 or 0))
 end
end
local obj={x=40,y=40}
''' + death_init + '''
trace_table("death",0,dead_particles,{"x","y","t","dx","dy"})
for tick=1,12 do
''' + death_draw + '''
 trace_table("death",tick,dead_particles,{"x","y","t","dx","dy"})
end
clouds=cloud_cases
for tick=1,20 do
''' + cloud_draw + '''
 trace_table("clouds",tick,clouds,{"x","y","spd","w"})
end
particles=ambient_cases
for tick=1,40 do
''' + ambient_draw + '''
 trace_table("ambient",tick,particles,{"x","y","s","spd","off","c"})
end
for i,c in ipairs(ambient_pixels) do
 particles={c}
 cls(0)
''' + ambient_draw + '''
 printh("ambient_pixels,"..i..","..pixels())
end
'''
cart = 'pico-8 cartridge // http://www.pico-8.com\nversion 42\n__lua__\n' + prelude + hair + smoke
for name, data in [('animation_cases', animation_cases), ('hair_cases', hair_cases), ('smoke_cases', smoke_cases), ('cloud_cases', cloud_cases), ('ambient_cases', ambient_cases), ('ambient_pixels', ambient_pixels)]:
    cart += '\n' + name + '=' + lua(data) + '\n'
cart += runner
(OUT / 'presentation-reference.p8').write_text(cart)
try:
    result = subprocess.run([PICO8, '-home', str(OUT), '-x', str(OUT / 'presentation-reference.p8')], env=dict(os.environ, SDL_VIDEODRIVER='dummy', SDL_AUDIODRIVER='dummy'), capture_output=True, text=True, timeout=20)
except subprocess.TimeoutExpired as error:
    print((error.stdout or b'').decode()[-4000:])
    raise
(OUT / 'presentation-output.txt').write_text(result.stdout + result.stderr)
rows = []
for line in result.stdout.splitlines():
    fields = line.split(',')
    kind = fields[0]
    if kind in ('animation', 'color'):
        rows.append(dict(kind=kind, tick=int(fields[1]), values=[raw(v) for v in fields[2:]]))
    elif kind == 'hair':
        rows.append(dict(kind=kind, case=fields[1], tick=int(fields[2]), values=[raw(v) for v in fields[3:13]], pixels=[int(v) for v in fields[13].split(':') if v]))
    elif kind == 'smoke':
        rows.append(dict(kind=kind, case=fields[1], tick=int(fields[2]), values=[raw(v) for v in fields[3:8]], count=1-int(fields[8])))
    elif kind in ('death', 'clouds', 'ambient'):
        rows.append(dict(kind=kind, tick=int(fields[1]), count=int(fields[2]), values=[raw(v) for v in fields[3:]]))
    elif kind == 'ambient_pixels':
        rows.append(dict(kind=kind, tick=int(fields[1]), pixels=[int(v) for v in fields[2].split(':') if v]))
(OUT / 'presentation-traces.json').write_text(json.dumps(rows, indent=2))
expected = len(animation_cases) + sum(len(c['steps']) for c in hair_cases) + len(smoke_cases)*17 + 13 + 20 + 40 + 18 + len(ambient_pixels)
if result.returncode or len(rows) != expected:
    print(result.stdout[-4000:], result.stderr)
    raise SystemExit(f'Native PICO-8 oracle returned {len(rows)}/{expected} rows; inspect {OUT}')
if args.reference_only:
    print(f'Native PICO-8 produced {len(rows)} presentation trace rows: {OUT}')
    raise SystemExit(0)

build_dir = args.build_dir
if build_dir is None:
    build = subprocess.run(['go', 'run', './cmd/mtile', 'build', '-keep-work', str(PROJECT)], cwd=ROOT, env=env, text=True, capture_output=True)
    print(build.stdout, end='')
    if build.returncode:
        print(build.stderr)
        raise SystemExit(build.returncode)
    match = re.search(r'^workdir (.+)$', build.stdout, re.M)
    if not match:
        raise SystemExit('Build did not report a kept workspace')
    build_dir = Path(match.group(1))
shutil.copyfile(Path(__file__).with_name('presentation_parity_test.go.tmpl'), build_dir / 'presentation_parity_test.go')
# The generated candidate main installs a Mode 0 text atlas and calls its
# initialization lifecycle. Compile those same renderer features for the host
# controller harness; the default Mode 4 runtime cannot provide these methods.
host_tags = 'mtile_mode0,mtile_mode0_layers4,mtile_mode0_obj,mtile_mode0_text'
subprocess.run(['go', 'test', '-tags=' + host_tags, '-run', 'TestNativePico8Presentation', '-v', '.'], cwd=build_dir, env=env, check=True)
print(f'Verified {len(rows)} presentation rows: exact animation, hair state, lifetimes and clipping; only ambient sine/phase drift has explicit bounds. Native PICO-8 inputs/traces: {OUT}')
