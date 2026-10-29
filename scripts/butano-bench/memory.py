#!/usr/bin/env python3
"""Collect physical graphics-memory snapshots without ROM instrumentation."""
import concurrent.futures, json, os, pathlib, subprocess, sys
import run as suite

def main():
    root=pathlib.Path(sys.argv[1]);binary=pathlib.Path('/private/tmp/mtile-vram-inspect')
    cmd=['cc','-O2','-Ithird_party/mgba/include','-Ibuild/player/mgba/include',str(suite.HERE/'native/inspect.c'),'build/player/mgba/libmgba-core.a','-lm','-lz','-lpthread']
    if sys.platform=='darwin':cmd+=['-framework','CoreFoundation']
    subprocess.run(cmd+['-o',str(binary)],cwd=suite.ROOT,check=True)
    jobs=[]
    for result in sorted(root.glob('*/result.json')):
        row=json.loads(result.read_text())
        for engine,data in row['engines'].items():
            if 'error' not in data:jobs.append((result.parent,engine))
    def inspect(job):
        path,engine=job
        raw=subprocess.check_output([str(binary),str(path/(engine+'.gba'))],text=True,timeout=300)
        data=json.loads(raw);suite.dump(path/(engine+'-graphics-memory.json'),data)
        return path.name,engine,data
    results={}
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        for name,engine,data in pool.map(inspect,jobs):results.setdefault(name,{})[engine]=data;print(name,engine,flush=True)
    suite.dump(root/'graphics-memory.json',results)
if __name__=='__main__':main()
