#!/usr/bin/env python3
"""Compare the frozen PCM baseline with LCD-event pacing and matched Butano."""
import argparse, gzip, hashlib, json, pathlib, subprocess
import report

ROOT=pathlib.Path(__file__).resolve().parents[3]
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def lookup(rows):return {(d['variant'],d['workload']['name']):d for d in rows}
def awake(d):
    m=d['profile']['metadata'];return 100*m['instructionCycles']/m['elapsedCycles']
def main():
    ap=argparse.ArgumentParser();ap.add_argument('out',type=pathlib.Path);a=ap.parse_args()
    old=lookup(json.loads((ROOT/'docs/benchmarks/imported-audio-performance-2026-11-02/results.json').read_text()))
    rows=[json.loads(p.read_text()) for p in sorted(a.out.glob('*/result.json'))];new=lookup(rows)
    for d in rows:
        d['ram']=report.ram(d);d['instructionExecutionPercent']=awake(d)
        d['profile']['cpuMetric']='non-wait instruction cycles; event advancement separate'
        p=a.out/(d['variant']+'-'+d['workload']['name'])
        report.dump(p/'result.json',d);report.dump(p/'profile.json',d['profile'])
    report.dump(a.out/'results.json',rows)
    env=json.loads((a.out/'environment.json').read_text())
    env['mgbaRevision']=subprocess.check_output(['git','-C',str(ROOT/'third_party/mgba'),'rev-parse','HEAD'],text=True).strip()
    env['mgbaCompatSHA256']=sha(ROOT/'cmd/mtile-player/mgba_halt_compat.cmake')
    env['mgbaPatchedSourceSHA256']=sha(ROOT/'build/player/mgba-compat/gba.c')
    env['coreArchiveSHA256']=sha(ROOT/'build/player/mgba/libmgba-core.a')
    report.dump(a.out/'environment.json',env)
    table='| Workload | Old mTile work cycles | New mTile work cycles | Butano work cycles | Old mTile instructions/time | New mTile instructions/time | Butano instructions/time |\n| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n'
    cases=[('off','No audio'),('idle','Silent PCM'),('wav','One WAV'),('longwav','10-second WAV'),('mod1','1 music voice'),('mod4','4 music voices'),('mod16','16 music voices'),('mod32','32 music voices'),('mixed4','4 music + 4 effects'),('mixed8','4 music + 8 effects')]
    for case,label in cases:
        before=old['mtile',case];after=new['mtile',case];b=new.get(('butano-matched',case))
        table+=f'| {label} | {before["profile"]["cpu"]["mean"]:,.0f} | {after["profile"]["cpu"]["mean"]:,.0f} | '+(f'{b["profile"]["cpu"]["mean"]:,.0f}' if b else '—')+f' | {awake(before):.2f}% | {awake(after):.2f}% | '+(f'{awake(b):.2f}%' if b else '—')+' |\n'
    (a.out/'matrix.md').write_text(table)
    c=report.COLORS
    svg=report.svg_begin(940,640,'CPU execution before and after event pacing','Percentage of emulated time spent executing instructions, including intentional polling. New mTile and Butano use the same corrected core. This is not a power measurement.')
    report.text(svg,28,37,'Let the CPU wait without running a loop',24)
    report.text(svg,28,65,'All executed instruction cycles / elapsed time · same audio · 30 FPS target',15)
    series=[('Before: mTile',c['butano-default']),('After: mTile',c['mtile']),('Butano 32 / 8',c['butano-matched'])]
    for j,(label,color) in enumerate(series):report.rect(svg,28+j*240,85,14,14,color);report.text(svg,50+j*240,98,label)
    for i,(case,label) in enumerate([cases[j] for j in (0,2,5,6,7,9)]):
        y=140+i*74;report.text(svg,28,y+14,label)
        values=[old['mtile',case],new['mtile',case],new.get(('butano-matched',case))]
        for j,(d,(_,color)) in enumerate(zip(values,series)):
            if d is None:continue
            val=awake(d);report.rect(svg,240,y+j*19,5.8*val,13,color);report.text(svg,248+5.8*val,y+j*19+11,f'{val:.2f}%',12)
    report.text(svg,28,615,'Polling counts as execution here. Main-loop work and emulator event time are separate in the numeric report.',13)
    report.save_svg(a.out/'execution.svg',svg)
    svg=report.svg_begin(940,390,'Frame events survive long mixer interrupts','The old main loop missed a visible interval while the mixer ran. A remembered LCD event lets the new main loop resume without adding a refresh.')
    report.text(svg,28,37,'Remember the frame event, even when mixing runs long',23)
    report.text(svg,28,66,'The mixer still starts at line 0. The renderer keeps its VBlank transfer guards.',15)
    for row,(label,seq,color) in enumerate([('Before',[1,1,2,2,2,3,3,4,4],c['butano-default']),('After',[1,1,2,2,3,3,4,4,5],c['mtile'])]):
        y=105+row*95;report.text(svg,28,y+26,label,18)
        for j,v in enumerate(seq):
            report.rect(svg,165+j*78,y,70,43,color if v%2 else c['stack']);report.text(svg,187+j*78,y+27,chr(64+v),18,'#fffdf6' if v%2 else c['text'])
        report.text(svg,875,y+27,'…',18,c['muted'])
        report.text(svg,165,y+64,'One box = one LCD refresh. One letter = one rendered game image.',13)
    report.text(svg,28,332,'Original 32-voice capture: 598 two-refresh intervals + 1 three-refresh interval.',14)
    report.text(svg,28,359,'New capture: all 599 intervals have two refreshes. Same result in both independent repeats.',14)
    report.save_svg(a.out/'cadence.svg',svg)
    files=subprocess.check_output(['git','ls-files','--cached','--others','--exclude-standard','runtime','internal/transpiler','internal/build','internal/emulator/frame_pacing_integration_test.go','cmd/mtile-player','scripts/butano-bench/pcm'],cwd=ROOT,text=True).splitlines()
    report.dump(a.out/'source-sha256.json',{f:sha(ROOT/f) for f in sorted(set(files)) if (ROOT/f).is_file() and '__pycache__' not in f and not f.endswith('.pyc')})
    for folder in a.out.iterdir():
        if folder.is_dir():
            for p in folder.iterdir():
                if p.name not in ('result.json','profile.json') and p.suffix in ('.csv','.json','.txt','.log'):
                    pathlib.Path(str(p)+'.gz').write_bytes(gzip.compress(p.read_bytes(),mtime=0));p.unlink()
    print(f'{len(rows)} current configurations summarized; historical baseline preserved')
if __name__=='__main__':main()
