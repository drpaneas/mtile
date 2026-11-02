#!/usr/bin/env python3
"""Summarize retained evidence and draw dependency-free, standalone SVG charts.

CPU instructions and emulator event advancement are deliberately separate:
HALT folds idle DMA scheduling into elapsed events whereas polling does not.
"""
import argparse, collections, csv, gzip, hashlib, html, io, json, math
import pathlib, statistics, subprocess

ROOT=pathlib.Path(__file__).resolve().parents[3]
BUDGET=561792
def dump(p,o):p.write_text(json.dumps(o,indent=2)+'\n')
def read(p):return p.read_text() if p.exists() else gzip.decompress(pathlib.Path(str(p)+'.gz').read_bytes()).decode()
def summary(cs):
    cs=sorted(cs)
    return dict(mean=statistics.mean(cs),median=statistics.median(cs),p95=cs[math.ceil(len(cs)*.95)-1],maximum=max(cs),minimum=min(cs),percent=statistics.mean(cs)/BUDGET*100)
def ram(d):
    secs=d['memory']['sections']
    iw=sum(s['bytes'] for s in secs if 0x03000000<=s['address']<0x03008000)
    stack=sum(s['bytes'] for s in secs if s['name']=='.stack')
    ew=d['profile']['metadata']['mailboxMemory'][0] if d['variant'].startswith('butano') else sum(s['bytes'] for s in secs if 0x02000000<=s['address']<0x0203fe00)
    return dict(ewramStatic=ew,iwramStatic=iw-stack,iwramStackReservation=stack,mailbox=512)

COLORS={'text':'#42384f','muted':'#6d6378','paper':'#fffdf6','grid':'#d9d3e0','mtile':'#6b5195','butano-matched':'#45674d','butano-default':'#92662f','stack':'#b9aecb'}
def svg_begin(w,h,title,desc):return [f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}" role="img" aria-labelledby="title desc"><title id="title">{html.escape(title)}</title><desc id="desc">{html.escape(desc)}</desc><rect width="100%" height="100%" fill="{COLORS["paper"]}" rx="16"/><g font-family="Trebuchet MS,Segoe UI,sans-serif" fill="{COLORS["text"]}">']
def text(a,x,y,s,size=15,color=None):a.append(f'<text x="{x}" y="{y}" font-size="{size}" fill="{color or COLORS["text"]}">{html.escape(str(s))}</text>')
def rect(a,x,y,w,h,color):a.append(f'<rect x="{x}" y="{y}" width="{w:.2f}" height="{h}" rx="3" fill="{color}"/>')
def save_svg(p,a):p.write_text(''.join(a)+ '</g></svg>\n')

def charts(out,lookup):
    rows=[('wav','One WAV (half duty)'),('longwav','10-second WAV'),('mod1','1 music voice'),('mod4','4 music voices'),('mod16','16 music voices'),('mod32','32 music voices'),('mixed8','4 music + 8 effects')]
    a=svg_begin(920,720,'Imported audio: CPU instruction work','Measured CPU instruction cycles excluding deliberate waits, as a percentage of the 561792-cycle 30 FPS budget. Hardware event advancement is separate.');text(a,28,38,'Same audio, measured on the emulated ARM7',24);text(a,28,65,'Instruction work / 30 FPS budget · lower is better · PCM output ≈15.77 kHz',15)
    for j,(v,label) in enumerate((('mtile','mTile'),('butano-matched','Butano 32 / 8'),('butano-default','Butano 20 / 4'))):
        rect(a,28+j*225,86,14,14,COLORS[v]);text(a,50+j*225,99,label)
    for i,(case,label) in enumerate(rows):
        y=145+i*77;text(a,28,y+13,label)
        for j,v in enumerate(('mtile','butano-matched','butano-default')):
            d=lookup.get((v,case))
            if not d:continue
            pct=d['profile']['cpu']['percent'];rect(a,250,y+j*20,8*pct,14,COLORS[v]);text(a,259+8*pct,y+j*20+12,f'{pct:.2f}%',13)
    text(a,28,704,'32-voice mTile: one 3-refresh gap in each 600-tick capture; all other rows kept 2-refresh cadence.',13)
    save_svg(out/'cpu.svg',a)
    a=svg_begin(920,385,'RAM is not the sound file','EWRAM and IWRAM are separate memory banks. The mTile stack reservation is shown separately. Debug mailbox is 512 bytes in both and is excluded.');text(a,28,36,'Where the RAM goes: four-voice music',24);text(a,28,63,'KiB · static data + code in RAM · 512-byte debug mailbox excluded',15)
    for i,(v,label) in enumerate((('mtile','mTile'),('butano-matched','Butano 32 / 8'),('butano-default','Butano 20 / 4'))):
        d=lookup[v,'mod4'];r=d['ram'];y=96+i*82;text(a,28,y+12,label)
        for j,(field,name) in enumerate((('ewramStatic','EWRAM'),('iwramStatic','IWRAM'))):
            val=r[field]/1024;rect(a,250,y+j*28,val*12,18,COLORS[v])
            if j and r['iwramStackReservation']:
                s=r['iwramStackReservation']/1024;rect(a,250+val*12,y+28,s*12,18,COLORS['stack']);text(a,250+(val+s)*12+8,y+43,f'{val:.2f} KiB IWRAM + 9 KiB reserved stacks',13)
            else:text(a,258+val*12,y+j*28+15,f'{val:.2f} KiB {name}',13)
    text(a,28,365,'Butano uses stacks above its static sections; it also needs stack RAM. Observed peaks are in the report.',13)
    save_svg(out/'memory.svg',a)
    a=svg_begin(920,330,'Useful work and waiting are different','With four music voices, mTile actively polls during intentional waits. Butano spends most remaining time HALTed. This comparison does not measure battery life.');text(a,28,38,'A small workload can still keep the CPU awake',24)
    for i,(v,label) in enumerate((('mtile','mTile'),('butano-matched','Butano'))):
        d=lookup[v,'mod4'];m=d['profile']['metadata'];elapsed=m['elapsedCycles'];work=(m['instructionCycles']-m['waitInstructionCycles'])/elapsed;poll=m['waitInstructionCycles']/elapsed;halt=m['haltEventCycles']/elapsed;events=m['activeEventCycles']/elapsed;y=95+i*85
        text(a,28,y+20,label);x=170
        for ratio,color in ((work,COLORS[v]),(poll,COLORS['stack']),(halt,COLORS['grid']),(events,COLORS['muted'])):
            rect(a,x,y,ratio*700,28,color);x+=ratio*700
        text(a,170,y+51,f'Work {work*100:.2f}% · polling {poll*100:.2f}% · HALT/events {halt*100:.2f}% · non-HALT events {events*100:.2f}%',13)
    text(a,28,294,'Primary CPU tables exclude deliberate waits. Polling still executes instructions; HALT pauses them.',15)
    save_svg(out/'waiting.svg',a)

def main():
    ap=argparse.ArgumentParser();ap.add_argument('out',type=pathlib.Path);ap.add_argument('--compress',action='store_true');a=ap.parse_args();rows=[]
    env=json.loads((a.out/'environment.json').read_text())
    for p in sorted(a.out.glob('*/result.json')):
        d=json.loads(p.read_text());profile=d['profile'];ticks=list(csv.DictReader(io.StringIO(read(p.parent/'measured.ticks.csv'))))
        profile['cpu']=summary([int(t['instructions'])-int(t['wait']) for t in ticks]);profile['cpuWithNonHaltEvents']=summary([int(t['instructions'])-int(t['wait'])+int(t['activeEvents']) for t in ticks]);profile['cpuMetric']='non-wait instruction cycles; excludes event advancement'
        for c in d['captures']:
            if d['variant'].startswith('butano') and 'butanoMemory' not in c:
                c['butanoMemory']=dict(staticEWRAM=c['heapCapacity'],staticIWRAM=c['heapAllocated'],currentStack=c['heapLive'],dynamicEWRAM=c['heapPeak'])
                for key in ('heapCapacity','heapAllocated','heapLive','heapPeak','gcCollections','heapError','missedDelta'):c[key]=None
        d['ram']=ram(d);d.setdefault('bankBytes',0 if d['workload']['name']=='off' else env['soundbankBytes']);d.setdefault('bankSHA256',None if d['workload']['name']=='off' else env['soundbankSHA256'])
        dump(p,d);dump(p.parent/'profile.json',profile);rows.append(d)
    dump(a.out/'results.json',rows);lookup={(d['variant'],d['workload']['name']):d for d in rows};charts(a.out,lookup)
    header='| Build | Test | Mean work cycles | P95 | Max | Budget | EWRAM static | IWRAM static | Stack reserved | ROM |\n| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n'
    for d in rows:
        c=d['profile']['cpu'];r=d['ram'];header+=f'| {d["variant"]} | {d["workload"]["name"]} | {c["mean"]:,.0f} | {c["p95"]:,} | {c["maximum"]:,} | {c["percent"]:.2f}% | {r["ewramStatic"]:,} | {r["iwramStatic"]:,} | {r["iwramStackReservation"]:,} | {d["romBytes"]:,} |\n'
    (a.out/'matrix.md').write_text(header)
    # Source identifiers supplement a dirty Git base. ROM, ELF and input hashes
    # are retained per result; these hashes identify the source at report time.
    files=subprocess.check_output(['git','ls-files','--cached','--others','--exclude-standard','internal/build','internal/transpiler','runtime','scripts/butano-bench/pcm'],cwd=ROOT,text=True).splitlines()
    hashes={f:hashlib.sha256((ROOT/f).read_bytes()).hexdigest() for f in sorted(set(files)) if (ROOT/f).is_file() and not f.endswith(('_test.go','.pyc')) and '__pycache__' not in f}
    dump(a.out/'source-sha256.json',hashes)
    if a.compress:
        for folder in a.out.iterdir():
            if folder.is_dir():
                for p in folder.iterdir():
                    if p.name not in ('result.json','profile.json') and p.suffix in ('.txt','.csv','.json','.log'):
                        pathlib.Path(str(p)+'.gz').write_bytes(gzip.compress(p.read_bytes(),mtime=0));p.unlink()
    print(f'{len(rows)} ROM/configuration results; tables and charts written to {a.out}')
if __name__=='__main__':main()
