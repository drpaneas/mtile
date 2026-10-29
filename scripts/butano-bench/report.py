#!/usr/bin/env python3
"""Publish verified measurements, plots, CSV, and a compact evidence archive."""
import csv, json, os, pathlib, shutil, sys, tarfile
os.environ.setdefault('MPLCONFIGDIR','/private/tmp/mtile-butano-mpl')
os.environ.setdefault('XDG_CACHE_HOME','/private/tmp/mtile-butano-plot-cache')
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
from matplotlib.ticker import FuncFormatter
import numpy as np
import run as suite

NAMES={'butano':'Butano','mode0':'mTile Mode 0','mode4':'mTile Mode 4'}
COLORS={'butano':'#d3912c','mode0':'#147d80','mode4':'#737e8a'}
def fmt(n):return f'{n:,.0f}'

def main():
    root=pathlib.Path(sys.argv[1]);out=pathlib.Path(sys.argv[2]);out.mkdir(parents=True,exist_ok=True)
    verification=json.loads((root/'verification.json').read_text());assert verification['passAll'],verification['failures']
    memory=json.loads((root/'graphics-memory.json').read_text())
    rows=[json.loads(p.read_text()) for p in sorted(root.glob('*/result.json'))]
    assert len(rows)==38,len(rows)
    for row in rows:
        name=row['workload']['name']
        for engine,s in row['engines'].items():
            assert 'error' not in s,(name,engine)
            captures=[json.loads((root/name/f'{engine}-samples-{i}.json').read_text()) for i in range(2)]
            s.update(suite.summarize(captures[0]));s['repeatMedians']=[suite.summarize(c)['median'] for c in captures]
            assert s['repeatMedians'][0]==s['repeatMedians'][1],(name,engine,s['repeatMedians'])
            s['graphicsMemorySnapshot']=memory[name][engine]
            if engine=='butano':
                sample=captures[0]['samples'][-1]
                s.update(ewramStatic=sample['heapCapacity'],iwramStatic=sample['heapAllocated'],observedStack=sample['heapLive'],heapAllocated=sample['heapPeak'])
        suite.dump(root/name/'result.json',row)
    by={r['workload']['name']:r for r in rows}
    env=json.loads((root/'environment.json').read_text())
    calibration=json.loads(pathlib.Path('/private/tmp/mtile-butano-calibration/result.json').read_text())
    audio=json.loads(pathlib.Path('/private/tmp/mtile-butano-audio/results.json').read_text())
    suite.dump(out/'results.json',dict(environment=env,verification=verification,calibration=calibration,workloads=rows,audioCharacterization=audio))
    for file in ('environment.json','verification.json','graphics-memory.json','butano-timing.patch'):shutil.copyfile(root/file,out/file)
    suite.dump(out/'calibration.json',calibration);suite.dump(out/'audio.json',audio)
    fields=['workload','engine','medianCycles','p95Cycles','maximumCycles','observedHz','overBudgetTicks','romBytes','ewramReservationBytes','iwramReportedBytes','iwramAccounting','firstPublicationVideoFrame','bgReferencedBytes','objReferencedBytes','bitmapReservedBytes','activeOAMEntries']
    with (out/'results.csv').open('w',newline='') as f:
        writer=csv.DictWriter(f,fieldnames=fields);writer.writeheader()
        for row in rows:
            for e,s in row['engines'].items():
                g=s['graphicsMemorySnapshot'];writer.writerow(dict(workload=row['workload']['name'],engine=e,medianCycles=s['median'],p95Cycles=s['p95'],maximumCycles=s['maximum'],observedHz=s['observedHz'],overBudgetTicks=s['overBudgetTicks'],romBytes=s['romBytes'],ewramReservationBytes=s.get('ewramUsed',s.get('ewramStatic')),iwramReportedBytes=s.get('iwramUsed',s.get('iwramStatic')),iwramAccounting='static only; observed stack separate' if e=='butano' else 'includes 9 KiB reserved stacks',firstPublicationVideoFrame=s['firstPublicationVideoFrame'],**{k:g[k] for k in fields[-4:]}))
    selected=['baseline','sprites8_64','sprites16_32','animation16','static_map','static_map_cached','parallax2','parallax4','scroll_chunk_cross','map_write_16','tile_edits_spread','text_static_32','text_numeric','physics_32','physics_64','physics_128','physics_drag_64','collision_1024','mixed_scene','bitmap_rects_16','bitmap_rects_64']
    plt.rcParams.update({'font.family':'DejaVu Sans','font.size':10,'axes.spines.top':False,'axes.spines.right':False})
    fig,ax=plt.subplots(figsize=(12,11));ys=np.arange(len(selected));height=.23
    for offset,e in enumerate(NAMES):
        vals=[by[n]['engines'].get(e,{}).get('median',np.nan) for n in selected];p95=[by[n]['engines'].get(e,{}).get('p95',np.nan) for n in selected]
        ax.barh(ys+(offset-1)*height,vals,height=height,color=COLORS[e],label=NAMES[e]);ax.scatter(p95,ys+(offset-1)*height,marker='|',color='#17232f',s=32,zorder=3)
    ax.axvline(suite.BUDGET,color='#ba3c35',ls='--',lw=1.3,label='30 Hz budget: 561,792 cycles')
    ax.set_xscale('log');ax.set_xlim(400,3_000_000);ax.xaxis.set_major_formatter(FuncFormatter(lambda v,p:f'{v/1_000_000:g}M' if v>=1_000_000 else f'{v/1000:g}k'))
    ax.set_yticks(ys,selected);ax.invert_yaxis();ax.grid(axis='x',alpha=.15);ax.set_axisbelow(True)
    ax.set_xlabel('Emulated ARM7 cycles per logical tick · logarithmic scale · bars median, marks p95')
    ax.set_title('Butano and mTile at 30 Hz',loc='left',fontsize=20,pad=76)
    ax.legend(loc='lower left',bbox_to_anchor=(0,1.015),ncols=2,frameon=False)
    fig.tight_layout();fig.savefig(out/'cycles.png',dpi=170);plt.close(fig)
    mnames=['baseline','sprites8_64','parallax4','scroll_chunk_cross','physics_64','mixed_scene']
    fig,ax=plt.subplots(figsize=(11,5));ys=np.arange(len(mnames))
    for offset,e in enumerate(NAMES):
        vals=[by[n]['engines'][e].get('ewramUsed',by[n]['engines'][e].get('ewramStatic'))/1024 for n in mnames]
        ax.barh(ys+(offset-1)*.23,vals,height=.23,label=NAMES[e],color=COLORS[e])
    ax.set_yticks(ys,mnames);ax.invert_yaxis();ax.set_xlabel('KiB of linked EWRAM reservation; includes the mTile heap arena where linked')
    ax.set_title('EWRAM capacity cost',loc='left',fontsize=18);ax.legend(frameon=False,ncols=3);ax.grid(axis='x',alpha=.15);ax.set_axisbelow(True);fig.tight_layout();fig.savefig(out/'memory.png',dpi=170);plt.close(fig)
    # The archive retains raw samples and source/layout evidence, without the
    # hundreds of MiB of intermediate ELF debug information.
    with tarfile.open(out/'evidence.tar.gz','w:gz',compresslevel=6) as archive:
        for path in sorted(root.rglob('*')):
            if not path.is_file():continue
            if path.suffix in ('.elf','.map','.gba') or path.name.endswith('.game.elf') or path.name.endswith('.game.map'):continue
            if path.name=='results.json' and path.parent==root:continue
            archive.add(path,arcname='main/'+str(path.relative_to(root)))
        for category,folder in [('audio',pathlib.Path('/private/tmp/mtile-butano-audio')),('calibration',pathlib.Path('/private/tmp/mtile-butano-calibration'))]:
            for path in sorted(folder.rglob('*')):
                if path.is_file() and path.suffix in ('.json','.png','.lua','.log'):archive.add(path,arcname=category+'/'+str(path.relative_to(folder)))
    counts={e:sum(e in r['engines'] and r['engines'][e]['overBudgetTicks']==0 and all(int(g)==2 for g in r['engines'][e]['refreshGapHistogram']) for r in rows) for e in NAMES}
    def cell(name,e):
        s=by[name]['engines'].get(e);return '—' if s is None else f"{fmt(s['median'])} / {fmt(s['p95'])} / {fmt(s['maximum'])}"
    lines=['# Butano and mTile performance at 30 Hz','',
      'Measured on 2026-10-29. This comparison covers 38 deterministic graphics and game-logic workloads on the same pinned mGBA core. Each ROM has 60 warm-up ticks followed by two independent runs of 600 measured ticks. All **134,400 measured tick images and state checksums** match an independent integer reference. Every pair of repeated medians is identical. These are emulator measurements; physical GBA confirmation remains outstanding.','',
      'mTile Mode 0 is faster in several small tile, sprite, collision-read and numeric-HUD scenes. Butano has much more CPU headroom for the tested entity simulation, four-background parallax, dirty-tile uploads and rapid map streaming. A renderer choice alone cannot bring the 64-entity mTile simulation inside its 30 Hz budget.','',
      '![Cycle comparison](cycles.png)','',
      '## Conditions and measurement boundaries','',
      f"- mTile revision: `{env['mtileRevision']}`. No engine source or public Lua API changes.",
      f"- Butano revision: `{env['butanoRevision']}`; devkitARM 65 / GCC 14.2.0; stock `-O2` Thumb build; assertions and logging disabled. Unused PCM and DMG backends are disabled for non-audio workloads.",
      '- mTile uses TinyGo 0.42.0 / LLVM 22.1.4, stock ARM `-opt=2 -gc=none`, all normal compiler optimizations enabled, plus the existing debug mailbox. Butano also writes its mailbox outside the measured interval.',
      '- Both engines target one logical update per two LCD refreshes. The GBA rate makes this approximately **29.864 Hz**, with a **561,792-cycle** budget. No 60 Hz game mode is compared.',
      '- Main-suite cycle totals include input/update/drawing, renderer preparation, deferred VRAM/OAM commits and useful VBlank interrupt work; intentional waiting is excluded. Butano receives timing hooks only around its VBlank waits and interrupt body. The exact patch is included. Audio is treated separately below.',
      f"- The stopwatch zero-work control, including two artificial wait/interrupt boundaries, measures **{calibration['stopwatchControlCycles']} cycles**. No overhead is subtracted from published results. Differences smaller than this control are treated as practical ties, including the 16-rectangle bitmap result.",
      '- The first complete scan for a new mailbox publication is captured for the same logical tick in both engines. The independent reference verifies every image and state in both repetitions. Capturing the last scan can include the next Butano image; the final dataset does not use that method.',
      '- Shared assets use the same 8bpp pixel indices and a 256-entry RGB15 palette repeating 16 colors. This also allows the 4bpp Butano text font to coexist with 8bpp sprite palettes. Font pixels, 6-pixel advance and 10-pixel cell height match exactly.',
      '- Physics uses the same signed Q16.16 state and rounding. C++ uses native typed arrays in its normal IWRAM placement. These Lua fixtures declare global arrays, so mTile uses generic tagged tables; its compact numeric-array optimization currently requires chunk-scope `local` bindings. This compares the shipped programming models, including their memory layout and access costs. See the [subsequent profiling and improvements](../../performance-improvements-2026-10-29.md).',
      '- No input is required: all games receive the same zero-key replay and deterministic tick sequence. Initial boot is excluded by warm-up; first-use work encountered later in the route remains part of measured tick cost.','',
      '## Main results','',
      'Each cell is **median / p95 / maximum cycles per tick**. Lower is better. Unsupported Mode 0 bitmap primitives have no Mode 0 result. The complete machine-readable table is in [results.csv](results.csv).','',
      '| Workload | Butano | mTile Mode 0 | mTile Mode 4 |', '| --- | ---: | ---: | ---: |']
    for r in rows:
        n=r['workload']['name'];lines.append(f'| `{n}` | {cell(n,"butano")} | {cell(n,"mode0")} | {cell(n,"mode4")} |')
    lines += ['', '## Sustained cadence and deadlines','',
      f"Butano maintains an exact two-refresh cadence in **{counts['butano']}/38** cases. mTile Mode 0 does so in **{counts['mode0']}/36** compatible cases. Mode 4 does so in **{counts['mode4']}/38** cases. The following rates come from observed refresh intervals, rather than a host FPS counter.",'',
      'The raw engine `MissedFrames` counters have different meanings. The common deadline columns below use measured cycle cost and refresh spacing instead. AUTO selects Mode 0 for the 36 compatible workloads and Mode 4 for the two bitmap workloads; AUTO does not guarantee that arbitrary update logic fits the budget.','',
      '| Workload | Butano Hz | Mode 0 Hz | Mode 4 Hz | Mode 0 over-budget ticks | Mode 4 over-budget ticks |', '| --- | ---: | ---: | ---: | ---: | ---: |']
    for r in rows:
        n=r['workload']['name'];e=r['engines']; hz=lambda k:f"{e[k]['observedHz']:.3f}" if k in e else '—'
        lines.append(f"| `{n}` | {hz('butano')} | {hz('mode0')} | {hz('mode4')} | {e.get('mode0',{}).get('overBudgetTicks','—')} | {e['mode4']['overBudgetTicks']} |")
    lines += ['', '## Interpretation of the workloads','',
      '- **Sprites:** 16, 32, 64 and 96 moving 8×8 objects, plus 16×16 and 32×32 shapes. Objects share resident artwork, remain within hardware scanline limits, and preserve overlap order. Butano uses one sort layer and `put_above`, rather than allocating one sort layer per object.',
      '- **Animation:** 16 objects select the same four resident frames. Both retain the frames in VRAM; Butano uses cached `sprite_tiles_ptr` handles. Lua uses explicit finite frame assignments accepted by the public Mode 0 proof. This does not characterize large pose-bank streaming.',
      '- **Maps:** `static_map` is a 32×32 map with many transparent holes, outside the small Mode 4 raster-cache shape. `static_map_cached` is an opaque 30×20 logical map, padded to the hardware-required 32×32 storage on Butano. Its Mode 4 cost is much lower; a dense uncached map must not stand in for every Mode 4 game.',
      '- **Scrolling:** the slow cases move one pixel per tick per layer. `scroll_chunk_cross` moves 9 pixels horizontally and 3 vertically through a 256×128-cell ROM-backed map. It crosses tile and 32×32-cell chunk boundaries, including wraps of the deterministic route.',
      '- **Mutations:** map writes change 1, 4 or 16 cells. Artwork edits change 16 pixels in either one tile or 16 separate tiles. Butano uses public `reload_cells_ref` and `overwrite_tile`; mTile uses `mset` and `sset`. The full visible mutation is included in commit time.',
      '- **Text:** static text is retained in Butano and redrawn through the mTile API. The numeric HUD changes every tick; Butano uses its native text generator and mTile its numeric-print path. Manual digit-sprite replacement could improve Butano further and is not measured here.',
      '- **Simulation:** 32, 64 and 128 entities update positions, velocity, gravity, boundary clamps and checksums. The drag variant additionally multiplies both velocity components by an exact Q16.16 factor. Rendering is a plain backdrop, isolating update cost.',
      '- **Collisions:** 64, 256 and 1,024 map reads feed an observable checksum. Butano reads its native imported map array; mTile uses public `mget`. These results include normal ROM/EWRAM placement costs.',
      '- **Mixed scene:** two independently scrolling layers, 32 sprites, 64 collision probes and a changing score. Mode 0 has a lower median than Butano here, but a substantially higher p95.',
      '- **Bitmap:** both use indexed Mode 4 and draw the same inclusive filled rectangles, clear and page flip. At 16 rectangles the result is a practical tie; 64 rectangles favor Butano.','',
      '## Memory and startup','',
      '![EWRAM reservation](memory.png)','',
      'EWRAM columns are linked capacity reservations, including the mTile heap arena where present and the Butano fixed resource pools. They are not peak live heap measurements. IWRAM figures require care: mTile includes its 8 KiB user plus 1 KiB IRQ stack reservation; Butano reports static IWRAM separately from a stack reading taken at publication. The latter is not a stack high-water mark. No GC collections occur during these measured runs.','',
      '| Workload | Butano EWRAM B | Mode 0 EWRAM B | Mode 4 EWRAM B | Butano ROM B | Mode 0 ROM B | Mode 4 ROM B |', '| --- | ---: | ---: | ---: | ---: | ---: | ---: |']
    for r in rows:
        e=r['engines'];n=r['workload']['name'];v=lambda k,key:fmt(e[k][key]) if k in e else '—'
        lines.append(f"| `{n}` | {v('butano','ewramStatic')} | {v('mode0','ewramUsed')} | {v('mode4','ewramUsed')} | {v('butano','romBytes')} | {v('mode0','romBytes')} | {v('mode4','romBytes')} |")
    lines += ['',
      'Physical VRAM/OAM snapshots at tick 660 are included in [graphics-memory.json](graphics-memory.json). Referenced BG/OBJ bytes are a **lower bound on current hardware references**, not an allocator high-water mark: unused resident animation frames, the font atlas, system-menu art and stale tile contents can occupy additional space. Nonzero VRAM bytes are also reported as a separate diagnostic, never as allocated bytes. Mode 4 reserves two 40 KiB pages in both engines.',
      'For example, four-background parallax references 10,240 BG bytes on Butano and 18,432 on mTile Mode 0. The mixed scene uses 34 active hardware OAM entries in both. Butano can use 2 KiB hardware maps, while mTile Mode 0 uses 4 KiB maps for its current scrolling contract.',
      'Startup is recorded separately as `firstPublicationVideoFrame` in the raw data and CSV. It is a refresh-quantized observation of the first completed debug publication, including normal engine boot waits. It is not a precise tile-upload stopwatch and is excluded from steady-state statistics.','',
      '## PSG characterization','',
      'Two additional four-channel PSG programs retrigger notes every eight logical ticks or every tick. Both maintain the two-refresh cadence. They are **excluded from cross-engine audio speedup rankings**: mTile commits at 256 Hz, advgm at VBlank, and the snapshots do not establish equal trigger/sample phase. Even the sampled register sequences differ at event boundaries. Graphics tick-cost counters also omit some mTile timer service work occurring during deliberate waits, so comparing those counters as complete audio CPU totals would be misleading.','',
      '| Program | Mode 0 register mismatches / 600 | Mode 4 register mismatches / 600 | mTile PSG service CPU estimate |', '| --- | ---: | ---: | ---: |']
    for a in audio:
        e=a['engines'];lines.append(f"| `{a['name']}` | {e['mode0']['registerMismatchTicks']} | {e['mode4']['registerMismatchTicks']} | {e['mode0']['serviceCpuPercentEstimate']:.3f}% |")
    lines += ['',
      'The mTile audio percentage uses differences in cumulative measured-service count × integer-rounded mean cycles, divided by observed emulated time. It excludes interrupt entry/exit overhead. The full register traces and service counters are retained. A timestamped register-write and PCM conformance capture is needed before publishing an equal-audio performance ratio.','',
      '## Reproduction and evidence','',
      'The [benchmark scripts](../../../scripts/butano-bench/README.md) generate assets and both programs, build all renderers, record the replay and run the independent oracle. [evidence.tar.gz](evidence.tar.gz) contains both raw repetitions, source/layout evidence, final images, memory snapshots and the separate audio/calibration captures. [results.json](results.json) contains the complete consolidated results. ROM SHA-256 values and the player SHA-256 identify the measured binaries.',
      'Validation: `go test ./internal/transpiler ./internal/build ./cmd/mtile`, `cd runtime && go test ./...`, benchmark driver compilation, and the independent 134,400-tick oracle all passed. The results establish emulator performance for these exact public-API workloads. They do not establish physical-hardware timing or the maximum performance obtainable from hand-tuned C++, assembly, custom renderers or alternative data layouts.','']
    suite.write(out/'README.md','\n'.join(lines))
    manifest={str(p.relative_to(out)):suite.sha(p) for p in sorted(out.iterdir()) if p.is_file() and p.name!='sha256.json'}
    suite.dump(out/'sha256.json',manifest)
    print(json.dumps(dict(workloads=len(rows),counts=counts,verifiedTicks=verification['totalTicks'],evidenceBytes=(out/'evidence.tar.gz').stat().st_size),indent=2))

if __name__=='__main__':main()
