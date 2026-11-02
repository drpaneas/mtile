#!/usr/bin/env python3
"""Audio-only mTile/Butano comparison; no product patches or game-specific tuning.

Requires the pinned Butano source, devkitARM, TinyGo, mmutil (MAS 0x18), and
the repository's pinned mGBA player. See the resulting report for measurement
boundaries. Builds and host wall time are never reported as GBA CPU time.
"""
import argparse, bisect, collections, csv, hashlib, json, math, os
import pathlib, re, shutil, statistics, struct, subprocess, sys, wave

HERE=pathlib.Path(__file__).resolve().parent
ROOT=HERE.parents[2]
REV="04374b034c76b82bfdc3aa6c38d652e5e7778a68"
PREFIX="/opt/devkitpro/devkitARM/bin/arm-none-eabi-"
BUDGET=561792

def write(p,s):
    p.parent.mkdir(parents=True,exist_ok=True)
    if not p.exists() or p.read_text()!=s:p.write_text(s)
def dump(p,o):write(p,json.dumps(o,indent=2)+"\n")
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def import_assets(a,driver,project,files,log):
    if a.prepare_quality=='raw':cmd=[driver,'import',project,a.mmutil,*files]
    else:cmd=[driver,'batch-import',project,a.mmutil,a.prepare_quality,str(a.trim).lower(),*files]
    run(cmd,ROOT,log)
def run(cmd,cwd=None,log=None):
    p=subprocess.run(list(map(str,cmd)),cwd=cwd,env=ENV,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=900)
    if log:write(log,p.stdout)
    if p.returncode:raise RuntimeError(f"{cmd[0]} failed:\n{p.stdout[-6000:]}")
    return p.stdout

def assets(work):
    out=work/'inputs';out.mkdir(exist_ok=True)
    # Original, looped sine instruments. One actual note per requested channel.
    # Retrigger every 16 rows; each channel is quiet enough to avoid clipping.
    for n in (1,4,16,32):
        b=bytearray(1084+n*64*4)
        b[:20]=b'Original PCM fixture'.ljust(20,b'\0')
        b[20:42]=b'looped sine'.ljust(22,b'\0')
        b[42:44]=struct.pack('>H',32);b[45]=8;b[48:50]=struct.pack('>H',32)
        b[950]=1;b[951]=0
        b[1080:1084]=(f'{n}CHN' if n<10 else f'{n}CH').encode()
        for row in (0,16,32,48):
            for ch in range(n):
                period=(428,381,339,320)[ch%4]
                i=1084+(row*n+ch)*4
                b[i:i+4]=bytes([period>>8,period&255,0x10,0])
        b.extend(bytes(round(70*math.sin(2*math.pi*i/64))&255 for i in range(64)))
        (out/f'voices{n:02d}.mod').write_bytes(b)
    with wave.open(str(out/'bell.wav'),'wb') as w:
        w.setnchannels(1);w.setsampwidth(1);w.setframerate(16000)
        samples=bytes(max(0,min(255,round(128+60*math.sin(2*math.pi*880*i/16000)*(1-i/8000)))) for i in range(8000))
        w.writeframes(samples)
    return out

def prepare(a):
    work=a.work;work.mkdir(parents=True,exist_ok=True);a.out.mkdir(parents=True,exist_ok=True)
    butano=work/'butano'
    if not (butano/'template/Makefile').exists():
        archive=subprocess.check_output(['git','-C',str(a.butano),'archive',REV])
        butano.mkdir(exist_ok=True);subprocess.run(['tar','-x','-C',butano],input=archive,check=True)
    # A source export cannot accidentally inherit earlier stopwatch hooks.
    clean=subprocess.check_output(['git','-C',str(a.butano),'show',f'{REV}:butano/src/bn_core.cpp'])
    assert (butano/'butano/src/bn_core.cpp').read_bytes()==clean
    driver=work/'driver';run(['go','build','-o',driver,HERE/'driver.go'],ROOT,work/'driver-build.log')
    profiler=work/'profile'
    cmd=['cc','-O2','-Ithird_party/mgba/include','-Ibuild/player/mgba/include',HERE/'native/profile.c','build/player/mgba/libmgba-core.a','-lm','-lz','-lpthread']
    if sys.platform=='darwin':cmd+=['-framework','CoreFoundation']
    run(cmd+['-o',profiler],ROOT,work/'profiler-build.log')
    inputs=assets(work);shared=work/'shared'
    if not shared.exists():shutil.copytree(ROOT/'examples/imported-audio',shared);shutil.rmtree(shared/'assets/audio/direct')
    files=sorted(inputs.glob('*.mod'))+[inputs/'bell.wav']
    import_assets(a,driver,shared,files,work/'import.log')
    manifest=json.loads((shared/'assets/audio/direct/manifest.json').read_text())
    assert manifest['format']==24,'Butano/devkitARM requires the MAS 0x18 converter for this comparison'
    return butano,driver,profiler,shared,manifest

def cases():
    return [dict(name='off',kind=0,voices=0,effects=0),dict(name='idle',kind=1,voices=0,effects=0),
            dict(name='wav',kind=2,voices=0,effects=1),
            *[dict(name=f'mod{n}',kind=3,voices=n,effects=0) for n in (1,4,16,32)],
            *[dict(name=f'mixed{n}',kind=4,voices=4,effects=n) for n in (4,8)],
            dict(name='longwav',kind=2,voices=0,effects=1,long=True)]

def long_assets(a,driver,shared):
    # A 10-second 16-bit recording tests whether a larger bank is copied into
    # RAM. Only this case uses the larger bank; ordinary cases stay identical.
    p=a.work/'shared-long';source=a.work/'long-input/bell.wav';source.parent.mkdir(exist_ok=True)
    if not p.exists():shutil.copytree(shared,p)
    with wave.open(str(source),'wb') as w:
        w.setnchannels(1);w.setsampwidth(2);w.setframerate(22050)
        w.writeframes(b''.join(struct.pack('<h',round(12000*math.sin(2*math.pi*880*i/22050))) for i in range(220500)))
    import_assets(a,driver,p,[source],a.work/'long-import.log')
    return p,json.loads((p/'assets/audio/direct/manifest.json').read_text())

def mtile_build(a,driver,shared,w,variant):
    p=a.work/'projects'/w['name']
    if not p.exists():shutil.copytree(shared,p)
    music={1:0,4:1,16:2,32:3}.get(w['voices'],0)
    init=f'pcm_music({music})' if w['kind']>=3 else 'pcm_music(-1)' if w['kind'] else ''
    update='ticks=ticks+1\n'
    if w['effects']:update+=f'if ticks%{300 if w.get("long") else 30}==1 then\n'+'sample(0,0.125)\n'*w['effects']+'end\n'
    write(p/'game.lua',f'ticks=0\nfunction _init() {init} end\nfunction _update()\n{update}end\nfunction _draw() cls(0) end\n')
    rom=a.work/'roms'/f'{variant}-{w["name"]}.gba';rom.parent.mkdir(exist_ok=True)
    if not rom.exists() or a.rebuild:
        ENV['PCM_RETAIN_HEAP']='1' if variant=='mtile-heap-control' else '0'
        ENV['PCM_RUNTIME']=''
        if variant in ('mtile-cap20-control','mtile-line16-control'):
            rt=a.work/('runtime-cap20' if variant=='mtile-cap20-control' else 'runtime-line16')
            if not rt.exists():shutil.copytree(ROOT/'runtime',rt)
            if variant=='mtile-cap20-control':
                bridge=(ROOT/'runtime/maxmod/bridge.c').read_text()
                assert bridge.count('[32]')==3 and bridge.count('[8]')==2
                bridge=bridge.replace('[32]','[20]').replace('count = 32','count = 20').replace('[8]','[4]').replace('i < 8','i < 4')
                write(rt/'maxmod/bridge.c',bridge)
            else:
                owner='hw/frame_tinygo.go' if (ROOT/'runtime/hw/frame_tinygo.go').exists() else 'audio/pcm_tinygo.go'
                source=(ROOT/'runtime'/owner).read_text()
                old='GetReg16(dispstat)&0x00ff|1<<3|1<<5';assert source.count(old)==1
                write(rt/owner,source.replace(old,'GetReg16(dispstat)&0x00ff|16<<8|1<<3|1<<5'))
            ENV['PCM_RUNTIME']=str(rt)
        run([driver,'build',p,'mode4' if variant=='mtile-mode4' else 'mode0',rom],ROOT,rom.with_suffix('.build.log'))
    return rom,pathlib.Path(str(rom)+'.game.elf')

def butano_build(a,butano,shared,manifest,w,variant):
    config='off' if w['kind']==0 else variant+('-long' if w.get('long') else '')
    p=a.work/f'butano-{config}';(p/'src').mkdir(parents=True,exist_ok=True);(p/'include').mkdir(exist_ok=True)
    if w['kind']:
        src=shared/'assets/audio/direct/source';dst=p/'audio';dst.mkdir(exist_ok=True)
        for f in src.iterdir():shutil.copyfile(f,dst/f.name)
    mk=(butano/'template/Makefile').read_text().replace(':=  ../butano',f':=  {butano}/butano').replace(':=  python',':=  python3')
    mk=mk.replace(':=  default',':=  null').replace('TARGET      \t:=  $(notdir $(CURDIR))','TARGET      \t:=  benchmark')
    if not w['kind']:mk=mk.replace(':=  maxmod',':=  null')
    mk=re.sub(r'^AUDIOTOOL\s*:=.*$',f'AUDIOTOOL := {a.mmutil}',mk,flags=re.M)
    flags='-DBN_CFG_TIMER_FREQUENCY=1 -DBN_CFG_ASSERT_ENABLED=false -DBN_CFG_LOG_ENABLED=false'
    if variant=='butano-matched':flags+=' -DBN_CFG_AUDIO_MAX_MUSIC_CHANNELS=24 -DBN_CFG_AUDIO_MAX_SOUND_CHANNELS=8'
    mk=re.sub(r'^USERFLAGS\s*:=.*$',f'USERFLAGS := {flags}',mk,flags=re.M);write(p/'Makefile',mk)
    music={1:0,4:1,16:2,32:3}.get(w['voices'],0)
    sample=manifest['samples'][0]['bankId']
    definitions=f'#define WORKLOAD {w["kind"]}\n#define MUSIC_ID {music}\n#define SAMPLE_ID {sample}\n#define EFFECTS {w["effects"]}\n#define PERIOD {300 if w.get("long") else 30}\n'
    write(p/'src/main.cpp',definitions+(HERE/'native/main.cpp').read_text())
    rom=a.work/'roms'/f'{variant}-{w["name"]}.gba';rom.parent.mkdir(exist_ok=True)
    if not rom.exists() or a.rebuild:
        run(['make','-j4'],p,rom.with_suffix('.build.log'))
        shutil.copyfile(p/'benchmark.gba',rom);shutil.copyfile(p/'benchmark.elf',rom.with_suffix('.elf'))
    if w['kind']:
        assert sha(p/'build/bn_audio_soundbank.bin')==sha(shared/'assets/audio/direct'/manifest['bank']),'The soundbanks differ'
    return rom,rom.with_suffix('.elf')

def profile(a,profiler,rom,elf,folder):
    folder.mkdir(parents=True,exist_ok=True);prefix=folder/'trace'
    addresses={p[2]:p[0] for line in run([PREFIX+'nm','-n',elf]).splitlines() if len(p:=line.split(maxsplit=2))==3}
    ENV['PCM_CHANNELS_POINTER']=addresses.get('mm_mix_channels','0')
    ENV['PCM_CHANNELS_END_POINTER']=addresses.get('mm_mixch_end','0')
    ENV['PCM_FRAME_ADDRESS']=addresses.get('mmFrame','0')
    run([profiler,rom,a.warmup,a.ticks,prefix],ROOT,folder/'trace.log')
    raw=list(csv.DictReader(pathlib.Path(str(prefix)+'.pc.csv').open()))
    dwarf=subprocess.run([PREFIX+'addr2line','-a','-f','-i','-C','-e',elf],input='\n'.join(r['pc'] for r in raw)+'\n',text=True,stdout=subprocess.PIPE,check=True).stdout
    write(folder/'inline-frames.txt',dwarf)
    stacks={};pc=None;items=[]
    for line in dwarf.splitlines():
        if line.startswith('0x'):
            if pc is not None:stacks[pc]=list(zip(items[::2],items[1::2]))
            pc=int(line,16);items=[]
        else:items.append(line)
    if pc is not None:stacks[pc]=list(zip(items[::2],items[1::2]))
    syms=[]
    # Include size-zero assembly labels. These labels are useful for identifying
    # mixer loops that the older profiler left unattributed.
    for line in run([PREFIX+'nm','-n',elf]).splitlines():
        parts=line.split(maxsplit=2)
        if len(parts)==3 and parts[1] in ('t','T') and not parts[2].startswith('$'):syms.append((int(parts[0],16)&~1,parts[2]))
    syms.sort();starts=[s[0] for s in syms];totals=collections.Counter();categories=collections.Counter();waitpcs=[]
    for r in raw:
        pc=int(r['pc'],16);cost=int(r['cycles']);frames=stacks.get(pc,[]);names=' '.join(f[0] for f in frames)
        i=bisect.bisect_right(starts,pc)-1;symbol=syms[i][1] if i>=0 else '<unmapped>'
        location=' '.join(f[1] for f in frames)
        if any(s in names for s in ('PaceWaitUntil','WaitFramePace','WaitFrameStart','waitDisplayPhase','WaitForPresentWindow','WaitVDraw','wait_for_vblank','VBlankIntrWait')) or symbol=='mtile_halt_display_wait':
            cat='wait';waitpcs.append(r['pc'])
        elif any(s in names for s in ('DebugMailbox','DebugSnapshot','DebugLog')) or any(f[0]=='publish' for f in frames) or symbol.startswith('_ZL7publish'):cat='publication'
        elif '/maxmod/' in location or symbol.startswith(('mm','mp','mpm')):cat='maxmod'
        elif 'handleInterrupt' in names or 'pcmMix' in names or 'pcmVBlank' in names or symbol.startswith(('IRQ_','interrupt_')):cat='irq'
        else:cat='other'
        categories[cat]+=cost
        if cat!='wait':totals[frames[0][0] if frames and frames[0][0]!='??' else symbol]+=cost
    write(folder/'wait-pcs.txt','\n'.join(waitpcs)+'\n')
    # Second pass adds per-tick min/p95/max without a huge instruction trace.
    run([profiler,rom,a.warmup,a.ticks,folder/'measured',folder/'wait-pcs.txt'],ROOT,folder/'measured.log')
    meta=json.loads((folder/'measured.meta.json').read_text())
    assert meta==json.loads((folder/'trace.meta.json').read_text())|{'waitInstructionCycles':meta['waitInstructionCycles']}
    assert sum(categories.values())+meta['otherAddressCycles']==meta['instructionCycles']
    assert meta['instructionCycles']+meta['activeEventCycles']+meta['haltEventCycles']==meta['elapsedCycles']
    ticks=[{k:int(v) for k,v in row.items()} for row in csv.DictReader((folder/'measured.ticks.csv').open())]
    cs=sorted(t['instructions']-t['wait'] for t in ticks)
    with_events=sorted(t['instructions']-t['wait']+t['activeEvents'] for t in ticks)
    assert len(cs)==a.ticks
    def summary(values):return dict(mean=statistics.mean(values),median=statistics.median(values),p95=values[math.ceil(len(values)*.95)-1],maximum=max(values),minimum=min(values),percent=statistics.mean(values)/BUDGET*100)
    result=dict(metadata=meta,categoriesPerTick={k:v/a.ticks for k,v in categories.items()},cpu=summary(cs),cpuWithNonHaltEvents=summary(with_events),top=[dict(name=k,cyclesPerTick=v/a.ticks) for k,v in totals.most_common(25)])
    dump(folder/'profile.json',result);return result

def memory(elf):
    sections=[]
    for line in run([PREFIX+'objdump','-h',elf]).splitlines():
        m=re.match(r'\s*\d+\s+(\S+)\s+([0-9a-f]+)\s+([0-9a-f]+)\s+([0-9a-f]+)',line)
        if m:
            name,size,vma,lma=m.groups();size=int(size,16);vma=int(vma,16)
            if 0x02000000<=vma<0x03008000:sections.append(dict(name=name,bytes=size,address=vma))
    symbols=[]
    for line in run([PREFIX+'nm','-S','--size-sort',elf]).splitlines():
        p=line.split(maxsplit=3)
        if len(p)==4:
            addr,size=int(p[0],16),int(p[1],16)
            if 0x02000000<=addr<0x03008000:symbols.append(dict(name=p[3],bytes=size,address=addr))
    return dict(sections=sections,largestSymbols=sorted(symbols,key=lambda x:x['bytes'],reverse=True)[:25])

def capture(a,driver,rom,out):
    run([driver,'capture',ROOT/'build/player/mtile-player',rom,out,a.warmup,a.samples],ROOT)
    data=json.loads(out.read_text());ss=data['observations'];gaps=collections.Counter(b['refresh']-a['refresh'] for a,b in zip(ss,ss[1:]))
    stats=ss[-1]['stats'];first=ss[0]['stats']
    result=dict(samples=len(ss),refreshGaps=dict(gaps),missedDelta=stats['MissedFrames']-first['MissedFrames'],pcm=data['pcm'],heapCapacity=stats['HeapCapacity'],heapAllocated=stats['HeapAllocated'],heapLive=stats['HeapLive'],heapPeak=stats['HeapPeakLive'],gcCollections=stats['GCCount']-first['GCCount'],heapError=stats['HeapError'])
    if rom.name.startswith('butano-'):
        # The existing GLUD benchmark convention repurposes these four slots.
        # They are NOT Lua heap statistics on a C++ cartridge.
        result['butanoMemory']=dict(staticEWRAM=stats['HeapCapacity'],staticIWRAM=stats['HeapAllocated'],currentStack=stats['HeapLive'],dynamicEWRAM=stats['HeapPeakLive'])
        for key in ('heapCapacity','heapAllocated','heapLive','heapPeak','gcCollections','heapError','missedDelta'):result[key]=None
    return result

def main():
    global ENV
    ap=argparse.ArgumentParser();ap.add_argument('--work',type=pathlib.Path,default=pathlib.Path('/private/tmp/mtile-pcm-compare-20261102'));ap.add_argument('--out',type=pathlib.Path,default=ROOT/'docs/benchmarks/imported-audio-performance-2026-11-02');ap.add_argument('--butano',type=pathlib.Path,default=pathlib.Path('/private/tmp/mtile-butano-20261029'));ap.add_argument('--mmutil',type=pathlib.Path,required=True);ap.add_argument('--ticks',type=int,default=300);ap.add_argument('--samples',type=int,default=600);ap.add_argument('--warmup',type=int,default=60);ap.add_argument('--only',default='');ap.add_argument('--variants',default='',help='Exact comma-separated build variants');ap.add_argument('--rebuild',action='store_true');ap.add_argument('--prepare-quality',choices=['raw','balanced','small','source'],default='raw',help='Use the real PCM batch preparation path before comparing');ap.add_argument('--trim',action='store_true');a=ap.parse_args()
    ENV=os.environ.copy();ENV.update(DEVKITPRO='/opt/devkitpro',DEVKITARM='/opt/devkitpro/devkitARM',GOCACHE='/private/tmp/mtile-effective-guide-20261031/go-cache');ENV['PATH']='/opt/devkitpro/devkitARM/bin:/opt/devkitpro/tools/bin:'+ENV['PATH']
    butano,driver,profiler,shared,manifest=prepare(a)
    dump(a.out/'environment.json',dict(preparationQuality=a.prepare_quality,trimEligible=a.trim,butanoRevision=REV,mtileRevision=run(['git','rev-parse','HEAD'],ROOT).strip(),mtileDirty=True,tinygo=run(['tinygo','version']).strip(),gcc=run([PREFIX+'g++','--version']).splitlines()[0],playerSHA256=sha(ROOT/'build/player/mtile-player'),mgbaCompatSHA256=sha(ROOT/'cmd/mtile-player/mgba_halt_compat.cmake') if (ROOT/'cmd/mtile-player/mgba_halt_compat.cmake').exists() else None,profilerSHA256=sha(HERE/'native/profile.c'),mmutilSHA256=sha(a.mmutil),soundbankSHA256=sha(shared/'assets/audio/direct'/manifest['bank']),soundbankBytes=(shared/'assets/audio/direct'/manifest['bank']).stat().st_size,inputs={f.name:sha(f) for f in (a.work/'inputs').iterdir()},warmup=a.warmup,profileTicks=a.ticks,captureTicks=a.samples,cyclesPer30HzTick=BUDGET))
    results=[]
    for w in cases():
        case_shared,case_manifest=long_assets(a,driver,shared) if w.get('long') else (shared,manifest)
        variants=['mtile','butano-default']
        if w['kind']:variants+=['butano-matched']
        if w['name'] in ('mod32','mixed8'):variants.remove('butano-default')
        if w['name']=='mod4':variants+=['mtile-heap-control','mtile-mode4','mtile-cap20-control']
        if w['name']=='mod32':variants+=['mtile-line16-control']
        if a.variants:variants=[v for v in variants if v in a.variants.split(',')]
        for variant in variants:
            label=f'{variant}-{w["name"]}'
            if a.only and not any(x in label for x in a.only.split(',')):continue
            print(f'Building/measuring {label}',flush=True)
            rom,elf=mtile_build(a,driver,case_shared,w,variant) if variant.startswith('mtile') else butano_build(a,butano,case_shared,case_manifest,w,variant)
            folder=a.out/label
            if (folder/'result.json').exists() and not a.rebuild and (cached:=json.loads((folder/'result.json').read_text()))['profile']['metadata']['lastTick']-cached['profile']['metadata']['firstTick']==a.ticks:
                result=json.loads((folder/'result.json').read_text())
            else:
                p=profile(a,profiler,rom,elf,folder)
                assert p['metadata']['maxObservedVoices']==w['voices']+w['effects'],p['metadata']
                captures=[capture(a,driver,rom,folder/f'capture-{i}.json') for i in (1,2)]
                for c in captures:
                    assert c['pcm']['overflows']==0
                    if w['kind']>=2:assert c['pcm']['rmsMono']>20
                    else:assert c['pcm']['rmsMono']<5
                result=dict(variant=variant,workload=w,romBytes=rom.stat().st_size,romSHA256=sha(rom),elfSHA256=sha(elf),profile=p,captures=captures,memory=memory(elf),bankSHA256=sha(case_shared/'assets/audio/direct'/case_manifest['bank']),bankBytes=(case_shared/'assets/audio/direct'/case_manifest['bank']).stat().st_size)
                if variant.startswith('mtile'):result['build']=json.loads(pathlib.Path(str(rom)+'.build.json').read_text())
                dump(folder/'result.json',result)
            results.append(result);print(json.dumps(dict(label=label,cpu=result['profile']['cpu'],rom=result['romBytes'],memory=result['memory']['sections'])),flush=True)
    dump(a.out/('results-'+a.only.replace(',','_')+'.json' if a.only else 'results.json'),results)

if __name__=='__main__':main()
