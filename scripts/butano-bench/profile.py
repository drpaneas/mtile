#!/usr/bin/env python3
"""Attribute unmodified-ROM instruction cycles using ELF/DWARF inline frames."""
import argparse, bisect, collections, csv, json, os, pathlib, subprocess, sys
import run as suite

def main():
    ap=argparse.ArgumentParser();ap.add_argument('rom',type=pathlib.Path);ap.add_argument('elf',type=pathlib.Path);ap.add_argument('out',type=pathlib.Path);ap.add_argument('--warmup',type=int,default=60);ap.add_argument('--ticks',type=int,default=120);a=ap.parse_args()
    a.out.mkdir(parents=True,exist_ok=True);binary=a.out/'profile'
    cmd=['cc','-O2','-Ithird_party/mgba/include','-Ibuild/player/mgba/include',str(suite.HERE/'native/profile.c'),'build/player/mgba/libmgba-core.a','-lm','-lz','-lpthread']
    if sys.platform=='darwin':cmd+=['-framework','CoreFoundation']
    subprocess.run(cmd+['-o',str(binary)],cwd=suite.ROOT,check=True)
    with (a.out/'pc.csv').open('w') as raw,(a.out/'metadata.json').open('w') as meta:subprocess.run([binary,a.rom,str(a.warmup),str(a.ticks)],stdout=raw,stderr=meta,check=True)
    raw=list(csv.DictReader((a.out/'pc.csv').open()));metadata=json.loads((a.out/'metadata.json').read_text())
    prefix='/opt/devkitpro/devkitARM/bin/arm-none-eabi-'
    nm=subprocess.check_output([prefix+'nm','-S','-n',a.elf],text=True)
    symbols=[]
    for line in nm.splitlines():
        parts=line.split(maxsplit=3)
        if len(parts)==4 and parts[2] in ('t','T') and int(parts[1],16):symbols.append((int(parts[0],16)&~1,int(parts[1],16),parts[3]))
    symbols.sort();starts=[s[0] for s in symbols]
    inp='\n'.join(r['pc'] for r in raw)+'\n'
    dwarf=subprocess.run([prefix+'addr2line','-a','-f','-i','-C','-e',a.elf],input=inp,text=True,stdout=subprocess.PIPE,check=True).stdout
    suite.write(a.out/'inline-frames.txt',dwarf)
    stacks={};pc=None;items=[]
    for line in dwarf.splitlines():
        if line.startswith('0x'):
            if pc is not None:stacks[pc]=list(zip(items[::2],items[1::2]))
            pc=int(line,16);items=[]
        else:items.append(line)
    if pc is not None:stacks[pc]=list(zip(items[::2],items[1::2]))
    totals=collections.Counter();leaves=collections.Counter();sources=collections.Counter();categories=collections.Counter();useful=0
    for r in raw:
        pc=int(r['pc'],16);cost=int(r['cycles']);i=bisect.bisect_right(starts,pc)-1
        symbol=symbols[i][2] if i>=0 and pc<symbols[i][0]+symbols[i][1] else '<unmapped>'
        totals[symbol]+=cost;frames=stacks.get(pc,[])
        labels=' '.join(name for name,_ in frames)
        if any(s in labels for s in ('PaceWaitUntil','WaitForPresentWindow','WaitVDraw')):category='intentional wait'
        elif any(s in labels for s in ('DebugMailbox','DebugSnapshot','DebugLog')):category='debug publication'
        else:category='useful/other instructions';useful+=cost
        categories[category]+=cost
        if category=='useful/other instructions':
            leaf=frames[0][0] if frames else symbol
            leaves[leaf]+=cost;sources[(leaf,frames[0][1] if frames else '?')]+=cost
    def records(counter,field='percentOfUseful',denominator=None):
        denominator=useful if denominator is None else denominator
        return [dict(name=k,cycles=v,cyclesPerTick=v/a.ticks,**{field:v/denominator*100})for k,v in counter.most_common()]
    report=dict(romSHA256=suite.sha(a.rom),elfSHA256=suite.sha(a.elf),warmup=a.warmup,ticks=a.ticks,metadata=metadata,categories=dict(categories),usefulInstructionCycles=useful,usefulCyclesPerTick=useful/a.ticks,linkedSymbols=records(totals,'percentOfAttributedInstructions',sum(totals.values())),inlineLeaves=records(leaves),sourceLines=[dict(function=k[0],location=k[1],cycles=v,cyclesPerTick=v/a.ticks)for k,v in sources.most_common()])
    assert sum(categories.values())+metadata['eventCycles']+metadata['otherAddressCycles']==metadata['elapsedCycles'],report
    suite.dump(a.out/'profile.json',report)
    print(json.dumps(dict(metadata=metadata,categories=dict(categories),top=report['inlineLeaves'][:15]),indent=2))
if __name__=='__main__':main()
