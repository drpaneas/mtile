#!/usr/bin/env python3
"""Re-measure existing ROMs without rebuilding, then refresh their reports."""
import json, os, pathlib, sys
import run as suite

def main():
    root=pathlib.Path(sys.argv[1]);driver=pathlib.Path(sys.argv[2]);skip=set(sys.argv[3:])
    suite.ENV=os.environ.copy();helper=suite.ROOT/'build/player/mtile-player'
    for path in sorted(root.glob('*/result.json')):
        row=json.loads(path.read_text());name=row['workload']['name']
        if name in skip:continue
        print(name,flush=True)
        for engine,summary in row['engines'].items():
            if 'error' in summary:raise RuntimeError(summary['error'])
            data=[]
            for rep in range(2):
                output=path.parent/f'{engine}-samples-{rep}.json'
                suite.run([driver,'capture',helper,path.parent/(engine+'.gba'),output,path.parent/(engine+'-final.png'),60,600,0])
                data.append(json.loads(output.read_text()))
            summary.update(suite.summarize(data[0]));summary['repeatMedians']=[suite.summarize(x)['median'] for x in data]
        base=json.loads((path.parent/'butano-samples-0.json').read_text())['samples']
        for engine in ('mode0','mode4'):
            if engine not in row['engines']:continue
            other=json.loads((path.parent/f'{engine}-samples-0.json').read_text())['samples']
            diffs=[a['tick'] for a,b in zip(base,other) if a['sha256']!=b['sha256']]
            def state(s):return {v['Name']:v['Raw'] for v in s['watches'] if v['Name'] in ('tick','checksum')}
            states=[a['tick'] for a,b in zip(base,other) if state(a)!=state(b)]
            row['engines'][engine].update(pixelMismatchTicks=len(diffs),firstPixelMismatchTicks=diffs[:8],stateMismatchTicks=len(states),firstStateMismatchTicks=states[:8])
        suite.dump(path,row)
    print('recapture complete',flush=True)
if __name__=='__main__':main()
