#!/usr/bin/env python3
"""Rebuild unchanged benchmark projects with current mTile; retain old results.

Does not rebuild or modify Butano. verify.py independently checks the captures.
"""
import argparse, json, os, pathlib, re, shutil
import run as suite

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument('--baseline',type=pathlib.Path,required=True)
    ap.add_argument('--projects',type=pathlib.Path,required=True)
    ap.add_argument('--out',type=pathlib.Path,required=True)
    ap.add_argument('--work',type=pathlib.Path,required=True)
    ap.add_argument('--only',default='')
    a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True);a.work.mkdir(parents=True,exist_ok=True)
    suite.ENV=os.environ.copy();suite.ENV['GOCACHE']=str(a.work/'go-cache')
    driver=a.work/'driver'
    suite.run(['go','build','-o',driver,suite.HERE/'driver.go'],cwd=suite.ROOT)
    results={p.parent.name:json.loads(p.read_text()) for p in a.out.glob('*/result.json')}
    # The original runner's aggregate can reflect only its last --only batch.
    # Per-workload records are authoritative, as in verify.py.
    for record in sorted(a.baseline.glob('*/result.json')):
        before=json.loads(record.read_text())
        w=before['workload'];name=w['name']
        if a.only and not re.search(a.only,name):continue
        project=a.projects/name;out=a.out/name;out.mkdir(exist_ok=True)
        expected=json.loads((a.baseline/name/'asset-manifest.json').read_text())
        actual={str(p.relative_to(project)):suite.sha(p) for p in sorted(project.rglob('*')) if p.is_file()}
        if expected!=actual:raise RuntimeError(f'fixture changed: {name}')
        suite.dump(out/'asset-manifest.json',actual);suite.dump(out/'workload.json',w)
        shutil.copyfile(project/'game.lua',out/'game.lua')
        row=dict(workload=w,baseline=before['engines'],engines={})
        for engine in ('mode0','mode4'):
            if engine not in before['engines'] or 'error' in before['engines'][engine]:continue
            # Older per-workload summaries predate cadence fields. Recover
            # those fields from the preserved, already verified raw capture.
            original=json.loads((a.baseline/name/f'{engine}-samples-0.json').read_text())
            before['engines'][engine].update(suite.summarize(original))
            print(name,engine,flush=True);rom=out/(engine+'.gba')
            suite.run([driver,'build',project,engine,rom],log=out/(engine+'-build.log'))
            summaries=[]
            for rep in range(2):
                capture=out/f'{engine}-samples-{rep}.json'
                suite.run([driver,'capture',suite.ROOT/'build/player/mtile-player',rom,capture,out/f'{engine}-final.png',60,600,0])
                summaries.append(suite.summarize(json.loads(capture.read_text())))
            summary=summaries[0];summary['repeatMedians']=[x['median'] for x in summaries]
            meta=json.loads(pathlib.Path(str(rom)+'.build.json').read_text())
            summary.update(romBytes=rom.stat().st_size,romSHA256=suite.sha(rom),ewramUsed=meta['EWRAMUsed'],iwramUsed=meta['IWRAMUsed'])
            row['engines'][engine]=summary
            old=before['engines'][engine]['median']
            print(json.dumps(dict(median=summary['median'],p95=summary['p95'],maximum=summary['maximum'],speedup=old/summary['median'],observedHz=summary['observedHz'])),flush=True)
        results[name]=row;suite.dump(out/'result.json',row)
        suite.dump(a.out/'results.json',[results[n] for n in sorted(results)])
    return 0

if __name__=='__main__':raise SystemExit(main())
