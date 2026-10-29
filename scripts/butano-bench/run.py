#!/usr/bin/env python3
"""Reproducible 30 Hz Butano/mTile benchmark, using public rendering APIs.

Downloads/builds are separate from measured emulated ARM7 cycles. The Butano
checkout is benchmark-local: only stopwatch hooks are added, never optimizations.
"""
import argparse, collections, hashlib, json, math, os, pathlib, re, shutil, statistics, subprocess, sys

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[1]
BUTANO_REV = "04374b034c76b82bfdc3aa6c38d652e5e7778a68"
BUDGET = 561792

def write(path, text):
    path = pathlib.Path(path); path.parent.mkdir(parents=True, exist_ok=True)
    if not path.exists() or path.read_text() != text: path.write_text(text)

def dump(path, obj): write(path, json.dumps(obj, indent=2)+"\n")
def sha(path): return hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest()
def run(cmd, cwd=None, log=None):
    p=subprocess.run([str(x) for x in cmd],cwd=cwd,env=ENV,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=600)
    if log: write(log,p.stdout)
    if p.returncode: raise RuntimeError(f"{cmd[0]} failed ({p.returncode}):\n{p.stdout[-4500:]}")
    return p.stdout

def workloads():
    w=[dict(name="baseline",kind=0)]
    for n in (16,32,64,96):w.append(dict(name=f"sprites8_{n}",kind=1,objects=n))
    for size,n in ((16,16),(16,32),(32,8),(32,16)):
        w.append(dict(name=f"sprites{size}_{n}",kind=1,objects=n,size=size))
    w += [dict(name="animation16",kind=2,objects=16),dict(name="static_map",kind=3,layers=1),
          dict(name="scroll_small",kind=4,layers=1,width=128,height=32),
          dict(name="scroll_streamed",kind=4,layers=1,width=256,height=128),
          dict(name="parallax2",kind=5,layers=2,width=128,height=32),
          dict(name="parallax4",kind=5,layers=4,width=128,height=32)]
    for n in (1,4,16):w.append(dict(name=f"map_write_{n}",kind=6,layers=1,mutations=n))
    for spread in (0,1):w.append(dict(name=f"tile_edits_{'spread' if spread else 'clustered'}",kind=7,layers=1,mutations=16,spread=spread))
    for n in (8,16,32):w.append(dict(name=f"text_static_{n}",kind=8,hud=("ABCD0123"*4)[:n]))
    for n in (1,16):w.append(dict(name=f"palette_{n}",kind=9,layers=1,mutations=n))
    for n in (32,64,128):w.append(dict(name=f"physics_{n}",kind=10,entities=n))
    w.append(dict(name='scroll_chunk_cross',kind=4,layers=1,width=256,height=128,speed=9,verticalSpeed=3))
    w.append(dict(name='static_map_cached',kind=3,layers=1,logicalWidth=30,logicalHeight=20,opaque=True))
    w.append(dict(name='text_numeric',kind=15))
    w.append(dict(name='physics_drag_64',kind=10,entities=64,drag=True))
    for n in (64,256,1024):w.append(dict(name=f"collision_{n}",kind=11,entities=n))
    for n in (16,64):w.append(dict(name=f"bitmap_rects_{n}",kind=13,entities=n,mode0=False))
    w.append(dict(name="mixed_scene",kind=14,layers=2,width=128,height=32,objects=32,entities=64))
    return [dict(kind=0,objects=0,size=8,layers=0,entities=0,mutations=0,spread=0,width=32,height=32,hud="",mode0=True,drag=False,speed=1,verticalSpeed=0,**{}) | x for x in w]

COLORS=[((i%16)*7%32,(i%16)*11%32,(i%16)*17%32) for i in range(256)]
def tile_pixels(n):
    if n==0:return [0]*64
    return [0 if (x+y+n)%7==0 else 2+(n*3+x//2+y//2)%30 for y in range(8) for x in range(8)]
TILES=[tile_pixels(n) for n in range(64)]

def map_plane(w,layer):
    return [0 if layer and (x+2*y+layer)%4 else 1+(x*3+y*5+layer*7)%31 for y in range(w['height']) for x in range(w['width'])]

def tile_words(pixels,bpp=8):
    if bpp==4:pixels=[pixels[i]|pixels[i+1]<<4 for i in range(0,len(pixels),2)]
    return [sum(pixels[i+j]<<(8*j) for j in range(4)) for i in range(0,len(pixels),4)]
def ctiles(name,pixels,bpp=8,mutable=False):
    words=tile_words(pixels,bpp)
    return f"alignas(4) {'const ' if not mutable else ''}bn::tile {name}[] = {{"+','.join("{{"+','.join(hex(x) for x in words[i:i+8])+"}}" for i in range(0,len(words),8))+"};\n"

def generate(w, work):
    project=work/'projects'/w['name']; assets=project/'assets'
    tiles=[[p if p or n==0 or not w.get('opaque') else 2 for p in tile] for n,tile in enumerate(TILES)]
    dump(project/'project.json',dict(activePalette="bench",renderer="auto"))
    dump(assets/'palettes/bench.json',dict(name="bench",colors=[dict(r=r,g=g,b=b) for r,g,b in COLORS]))
    records=[dict(id=n,x=(n%8)*8,y=(n//8)*8,width=8,height=8,pixels=[tiles[n][y*8:y*8+8] for y in range(8)],flags=0) for n in range(64)]
    dump(assets/'spritesheet.json',dict(width=64,height=64,sprites=records))
    planes=[map_plane(w,l) for l in range(max(1,w['layers']))]
    mw=w.get('logicalWidth',w['width']);mh=w.get('logicalHeight',w['height'])
    authored=[[plane[y*w['width']+x] for y in range(mh) for x in range(mw)] for plane in planes]
    dump(assets/'map.json',dict(version=2,width=mw,height=mh,layers=[dict(id=l,cells=authored[l] if l<len(authored) else [0]*(mw*mh)) for l in range(4)]))
    kind=w['kind']; upd=["tick=tick+1", "local frame=tick%120", "checksum=0"]; init=[]; draw=["cls(1)"]
    pre="tick=0\nchecksum=0\n"
    if kind==10:
        n=w['entities'];pre+='\n'.join(f"{a}={{"+','.join('0' for _ in range(n))+'}' for a in ('px','py','vx','vy'))+'\n'
        init=[f"for i=1,{n} do", "local j=i-1", "px[i]=8+(j*13)%220", "py[i]=8+(j*17)%136", "vx[i]=((j%7)-3)*0.125", "vy[i]=((j%5)-2)*0.125", "end"]
        upd += [f"for i=1,{n} do", "local x=px[i]", "local y=py[i]", "local dx=vx[i]", "local dy=vy[i]+0.0625", "x=x+dx", "y=y+dy", "if x<4 then x=4 dx=abs(dx) elseif x>235 then x=235 dx=-abs(dx) end", "if y<4 then y=4 dy=abs(dy) elseif y>155 then y=155 dy=-abs(dy) end", "px[i]=x py[i]=y vx[i]=dx vy[i]=dy", "checksum=(checksum+flr(x)+flr(y))%16001", "end"]
    if kind==11:
        upd += [f"for i=0,{w['entities']-1} do", "checksum=(checksum+mget((i*7+frame)%32,(i*11+frame)%32))%16001", "end"]
    if kind==6:upd += [f"mset({i},0,2+frame%2)" for i in range(w['mutations'])]
    if kind==7:
        for i in range(16):
            tile=i+1 if w['spread'] else 1;p=0 if w['spread'] else i
            upd.append(f"sset({(tile%8)*8+p%8},{(tile//8)*8+p//8},2+(frame%2)*3)")
    if kind==9:upd += [f"pal({i+2},2+({i}+frame)%30,1)" for i in range(w['mutations'])]
    for l in range(w['layers']):
        cam=f"frame*{l+1}*{w['speed']}" if kind in (4,5,14) else "0"
        camy=f"frame*{w['verticalSpeed']}" if kind in (4,5,14) else "0"
        draw += [f"camera({cam},{camy})", f"map({l},flr(({cam})/8),flr(({camy})/8),0,0,31,21)"]
    draw += ["camera(0,0)"]
    if kind==2:upd += ["local phase=flr(frame/4)%4", "if phase==0 then sprite_frame=1 elseif phase==1 then sprite_frame=2 elseif phase==2 then sprite_frame=3 else sprite_frame=4 end"] ;pre+='sprite_frame=1\n'
    for i in range(w['objects']):
        cols=12 if w["size"]==8 else 8;spacing=18 if w["size"]==8 else 28
        x=f"{4+(i%cols)*spacing}+(frame%8)";y=8+(i//cols)*18
        draw += [f"spr({'sprite_frame' if kind==2 else 1},{x},{y},{w['size']//8},{w['size']//8})"]
    if kind==8:draw += [f"print({json.dumps(w['hud'])},8,8,7)"]
    if kind==13:
        draw += [f"for i=0,{w['entities']-1} do", "local x=(i*13+frame)%220", "local y=(i*17)%140", "rectfill(x,y,x+9,y+9,2+i%30)", "end"]
    if kind==15:draw += ["print(frame,8,8,7)"]
    if kind==14:
        upd += [f"for i=0,{w['entities']-1} do", "checksum=(checksum+mget((i*7)%32,(i+frame)%32))%16001", "end"]
        draw += ["print(frame,8,8,7)"]
    # _draw has its own local, to preserve statically provable object frames.
    lua=pre+'function _init()\n'+'\n'.join(init)+'\nend\nfunction _update()\n'+'\n'.join(upd)+'\nend\nfunction _draw()\nlocal frame=tick%120\n'+'\n'.join(draw)+'\nend\n'
    if w['drag']:lua=lua.replace('local dx=vx[i]','local dx=vx[i]*0.99609375').replace('local dy=vy[i]+0.0625','local dy=(vy[i]+0.0625)*0.99609375')
    write(project/'game.lua',lua)
    # Shared raw asset data bypasses format-converter differences, not engine APIs.
    h='#pragma once\n#include "bn_tile.h"\n#include "bn_color.h"\n#include "bn_regular_bg_item.h"\n#include "bn_sprite_item.h"\n#include "bn_sprite_font.h"\n'
    for k,v in dict(KIND=kind,OBJECTS=w['objects'],SPRITE_SIZE=w['size'],LAYERS=w['layers'],ENTITIES=w['entities'],MUTATIONS=w['mutations'],SPREAD=w['spread'],MAP_WIDTH=w['width'],DRAG=int(w['drag']),SCROLL_SPEED=w['speed'],VERTICAL_SPEED=w['verticalSpeed']).items():h+=f'constexpr int {k}={v};\n'
    h+='const uint8_t music_data[]={1,0,0,0,0,0};\n'
    h+=f'constexpr char HUD[]={json.dumps(w["hud"])};\n'
    h+='const bn::color colors[]={'+','.join(f'bn::color({r},{g},{b})' for r,g,b in COLORS)+'};\n'
    flat=sum(tiles,[]);h+=ctiles('bg_tiles',flat)
    h+=ctiles('mutable_tiles',flat,mutable=True)
    for l,plane in enumerate(planes):
        h+=f'alignas(4) {"" if kind==6 else "const "}uint16_t map{l}[]={{'+','.join(map(str,plane))+'};\n'
    h+='const uint16_t* const map_cells[]={'+','.join(f'map{i}' for i in range(len(planes)))+'};\n'
    h+='const bn::regular_bg_item bg_items[]={'+','.join(f'bn::regular_bg_item(bn::span<const bn::tile>({"mutable_tiles" if kind==7 else "bg_tiles"},128),colors,bn::bpp_mode::BPP_8,map{i}[0],bn::size({w["width"]},{w["height"]}))' for i in range(len(planes)))+'};\n'
    sp=[]
    for f in range(4 if kind==2 else 1):
        source=1+f;size=w['size']
        for ty in range(size//8):
            for tx in range(size//8):sp+=tiles[source+ty*8+tx]
    h+=ctiles('sprite_tiles',sp)
    h+=f'const bn::sprite_item sprite_item(bn::sprite_shape_size({w["size"]},{w["size"]}),bn::span<const bn::tile>(sprite_tiles,{len(sp)//32}),colors,bn::bpp_mode::BPP_8,{4 if kind==2 else 1});\n'
    raw=(ROOT/'runtime/font_default.go').read_text().split('var builtinFont = Font{ASCII: [95][6]uint16{')[1].split('}, Extra:')[0]
    glyphs=[[int(x.strip()) for x in row.split(',') if x.strip()] for row in re.findall(r'\{([^{}]+)\}',raw)[:95]]
    assert len(glyphs)==95
    ft=[]
    for cols in glyphs[1:]:
        for ty in range(2):
            ft += [7 if x<6 and y+ty*8<10 and cols[x]>>(y+ty*8)&1 else 0 for y in range(8) for x in range(8)]
    h+=ctiles('font_tiles',ft,4)
    h+='const bn::color font_colors[]={'+','.join(f'bn::color({r},{g},{b})' for r,g,b in COLORS[:16])+'};\n'
    h+='const int8_t font_widths[]={'+','.join('6' for _ in range(95))+'};\n'
    h+='const bn::sprite_item font_item(bn::sprite_shape_size(8,16),font_tiles,font_colors,bn::bpp_mode::BPP_4,94);\nconst bn::sprite_font font(font_item,bn::utf8_characters_map_ref(),font_widths,0);\n'
    write(work/'butano-project/include/bench_assets.h',h)
    return project

def setup(work,butano):
    core=butano/'butano/src/bn_core.cpp'
    original=run(['git','show',f'{BUTANO_REV}:butano/src/bn_core.cpp'],cwd=butano)
    patched=original.replace('#include "bn_core.h"','#include "bn_core.h"\nextern "C" void bench_pause();\nextern "C" void bench_resume();\nextern "C" void bench_irq_start();\nextern "C" void bench_irq_end();')
    patched=patched.replace('        hw::core::wait_for_vblank(data.waiting_for_vblank);','        bench_pause();\n        hw::core::wait_for_vblank(data.waiting_for_vblank);\n        bench_resume();')
    patched=patched.replace('    void _vblank_intr()\n    {','    void _vblank_intr()\n    {\n        bench_irq_start();')
    patched=patched.replace('        data.vblank_intr_active = false;','        data.vblank_intr_active = false;\n        bench_irq_end();')
    write(core,patched);write(work/'butano-timing.patch',run(['git','diff','--','butano/src/bn_core.cpp'],cwd=butano))
    p=work/'butano-project';(p/'src').mkdir(parents=True,exist_ok=True);(p/'include').mkdir(exist_ok=True)
    shutil.copyfile(HERE/'native/butano.cpp',p/'src/main.cpp');write(p/'src/timing.cpp',(HERE/'native/timing.cpp').read_text().replace('bn_hw_timer.h',str(butano/'butano/hw/include/bn_hw_timer.h')))
    mk=(butano/'template/Makefile').read_text().replace(':=  ../butano',f':=  {butano}/butano').replace(':=  python',':=  python3').replace(':=  maxmod',':=  null').replace(':=  default',':=  null')
    mk=mk.replace('USERFLAGS   \t:=','USERFLAGS   \t:= -DBN_CFG_TIMER_FREQUENCY=1 -DBN_CFG_ASSERT_ENABLED=false -DBN_CFG_LOG_ENABLED=false')
    mk=mk.replace('TARGET      \t:=  $(notdir $(CURDIR))','TARGET      \t:=  benchmark')
    write(p/'Makefile',mk)

def summarize(data):
    ss=data['samples'];cs=sorted(s['cycles'] for s in ss)
    gaps=[b['videoFrame']-a['videoFrame'] for a,b in zip(ss,ss[1:])]
    return dict(observedHz=(len(ss)-1)*16777216/((ss[-1]['videoFrame']-ss[0]['videoFrame'])*280896),refreshGapHistogram=dict(collections.Counter(gaps)),overBudgetTicks=sum(c>BUDGET for c in cs),samples=len(ss),median=statistics.median(cs),p95=cs[math.ceil(len(cs)*.95)-1],maximum=max(cs),minimum=min(cs),budgetPercent=statistics.median(cs)/BUDGET*100,
                missed=ss[-1]['missed']-data['warmupMissed'],videoRefreshes=ss[-1]['videoFrame']-ss[0]['videoFrame'],
                firstPublicationVideoFrame=data['firstPublicationVideoFrame'],gcCollections=ss[-1]['gc']-ss[0]['gc'],heapPeak=max(s['heapPeak'] for s in ss))

def main():
    global ENV
    ap=argparse.ArgumentParser();ap.add_argument('--work',type=pathlib.Path,default=pathlib.Path('/private/tmp/mtile-butano-bench'));ap.add_argument('--butano',type=pathlib.Path,default=pathlib.Path('/private/tmp/mtile-butano-20261029'))
    ap.add_argument('--out',type=pathlib.Path,default=ROOT/'docs/benchmarks/butano-30hz');ap.add_argument('--samples',type=int,default=600);ap.add_argument('--warmup',type=int,default=60);ap.add_argument('--only',default='');ap.add_argument('--repeat',type=int,default=1)
    args=ap.parse_args();work=args.work;work.mkdir(parents=True,exist_ok=True);args.out.mkdir(parents=True,exist_ok=True)
    ENV=os.environ.copy();ENV.update(DEVKITPRO='/opt/devkitpro',DEVKITARM='/opt/devkitpro/devkitARM',GOCACHE=str(work/'go-cache'),TINYGOCACHE=str(work/'tinygo-cache'))
    ENV['PATH']='/opt/devkitpro/devkitARM/bin:/opt/devkitpro/tools/bin:'+ENV['PATH']
    rev=run(['git','rev-parse','HEAD'],cwd=args.butano).strip();assert rev==BUTANO_REV,(rev,BUTANO_REV)
    setup(work,args.butano)
    driver=work/'driver';run(['go','build','-o',driver,HERE/'driver.go'],cwd=ROOT)
    helper=ROOT/'build/player/mtile-player'
    dump(args.out/'environment.json',dict(mtileRevision=run(['git','rev-parse','HEAD'],cwd=ROOT).strip(),butanoRevision=rev,helperSHA256=sha(helper),tinygo=run(['tinygo','version']).strip(),gcc=run(['arm-none-eabi-g++','--version']).splitlines()[0],samples=args.samples,warmup=args.warmup,budget=BUDGET,butanoFlags='stock -O2 Thumb; asserts/log off; F/1 timer; null unused audio backends',mtileFlags='stock -opt=2 ARM; -gc=none; debug telemetry enabled'))
    shutil.copyfile(work/'butano-timing.patch',args.out/'butano-timing.patch')
    results=[]
    for w in workloads():
        if args.only and not re.search(args.only,w['name']):continue
        print('WORKLOAD',w['name'],flush=True)
        out=args.out/w['name'];out.mkdir(exist_ok=True);project=generate(w,work);dump(out/'workload.json',w);shutil.copyfile(project/'game.lua',out/'game.lua');dump(out/'asset-manifest.json',{str(p.relative_to(project)):sha(p) for p in sorted(project.rglob('*')) if p.is_file()})
        row=dict(workload=w,engines={});results.append(row)
        for engine in ('butano','mode0','mode4','auto'):
            if engine=='mode0' and not w['mode0']:continue
            print(' ',engine,flush=True);rom=out/(engine+'.gba')
            try:
                if engine=='butano':
                    run(['make','-j4'],cwd=work/'butano-project',log=out/'butano-build.log')
                    for ext in ('gba','elf'):shutil.copyfile(work/f'butano-project/benchmark.{ext}',out/f'butano.{ext}')
                    shutil.copyfile(work/'butano-project/build/benchmark.map',out/'butano.map')
                else:run([driver,'build',project,engine,rom],log=out/(engine+'-build.log'))
                if engine=='auto':
                    meta=json.loads(pathlib.Path(str(rom)+'.build.json').read_text());row['auto']={k:v for k,v in meta.items() if k.startswith('Renderer')};continue
                captures=[]
                for rep in range(args.repeat):
                    capture=out/f'{engine}-samples-{rep}.json'
                    run([driver,'capture',helper,rom,capture,out/f'{engine}-final.png',args.warmup,args.samples,0])
                    data=json.loads(capture.read_text());captures.append(data)
                summary=summarize(captures[0]);summary['romBytes']=rom.stat().st_size;summary['romSHA256']=sha(rom)
                summary['repeatMedians']=[summarize(x)['median'] for x in captures]
                if engine=='butano':
                    last=captures[0]['samples'][-1];summary.update(ewramStatic=last['heapCapacity'],iwramStatic=last['heapAllocated'],observedStack=last['heapLive'],heapAllocated=last['heapPeak'])
                if engine!='butano':
                    meta=json.loads(pathlib.Path(str(rom)+'.build.json').read_text());summary.update(ewramUsed=meta['EWRAMUsed'],iwramUsed=meta['IWRAMUsed'],oam=meta['RendererOAMEntries'],scanlineCycles=meta['RendererLineCycles'],selection=meta['RendererReason'])
                row['engines'][engine]=summary
            except Exception as e:
                row['engines'][engine]=dict(error=str(e));print(str(e)[-1200:],flush=True)
        # Compare the complete stable scan for the same published game tick.
        if 'butano' in row['engines'] and 'error' not in row['engines']['butano']:
            base=json.loads((out/'butano-samples-0.json').read_text())['samples']
            for engine in ('mode0','mode4'):
                if engine not in row['engines'] or 'error' in row['engines'][engine]:continue
                other=json.loads((out/f'{engine}-samples-0.json').read_text())['samples']
                assert [x['tick'] for x in base]==[x['tick'] for x in other]
                diff=[a['tick'] for a,b in zip(base,other) if a['sha256']!=b['sha256']]
                def check(s):return {w['Name']:w['Raw'] for w in s['watches'] if w['Name'] in ('tick','checksum')}
                states=[a['tick'] for a,b in zip(base,other) if check(a)!=check(b)]
                row['engines'][engine].update(pixelMismatchTicks=len(diff),firstPixelMismatchTicks=diff[:8],stateMismatchTicks=len(states),firstStateMismatchTicks=states[:8])
        dump(out/'result.json',row);dump(args.out/'results.json',results)
        print(json.dumps({e:{k:v for k,v in r.items() if k in ('median','p95','maximum','missed','pixelMismatchTicks','stateMismatchTicks','error')} for e,r in row['engines'].items()}),flush=True)
    return 0
if __name__=='__main__':sys.exit(main())
