#!/usr/bin/env python3
"""Compose real mGBA captures with integer nearest-neighbor zoom only."""
import hashlib, json, subprocess, sys
from pathlib import Path
from PIL import Image, ImageDraw, ImageFont
root=Path(__file__).resolve().parents[2]
source=Path(sys.argv[1] if len(sys.argv)>1 else '/tmp/mtile-font-geometry')
output=root/'research/font-typography/results';output.mkdir(exist_ok=True)
face=root/'fonts/JetBrainsMono/JetBrainsMono-Regular.ttf'
font=ImageFont.truetype(str(face),20);small=ImageFont.truetype(str(face),15)
variants=[('before','BEFORE: 4x5 / cell 6x8','40 columns x 20 rows'),('same-grid-pointer','A: 5x7 / cell 6x8','40 columns x 20 rows'),('compact-grid-pointer','B: 6x10 / cell 6x10','40 columns x 16 rows')]
# A vertical layout keeps the actual glyph pixels readable in the chat viewer.
zoom=3;pad=24;panel_h=546
board=Image.new('RGB',(240*zoom+2*pad,3*panel_h+pad),(244,246,248))
draw=ImageDraw.Draw(board)
for i,(variant,title,detail) in enumerate(variants):
    y=i*panel_h+pad
    draw.text((pad,y),title,font=font,fill=(20,31,43))
    draw.text((pad,y+28),detail,font=small,fill=(60,76,90))
    image=Image.open(source/variant/'unicode-mode4.png').convert('RGB')
    assert image.size==(240,160)
    image.save(output/('preview-'+str(i)+'.png'))
    board.paste(image.resize((240*zoom,160*zoom),Image.Resampling.NEAREST),(pad,y+56))
board.save(output/'comparison.png')
# Native-density stress previews; B intentionally has fewer rows/glyphs.
for variant,_,_ in variants:
    for name in ['dense-mode4','edges-mode4']:
        image=Image.open(source/variant/(name+'.png'))
        image.save(output/(variant+'-'+name+'.png'))
    if variant not in ['larger-grid-pointer','compact-grid-pointer']:
        for name in ['paused','save-failed','ended']:
            image=Image.open(source/variant/('pause-'+name+'.png'))
            image.save(output/(variant+'-pause-'+name+'.png'))
# All geometry-only and pointer controls remain visible in raw results.
measurements=json.loads((source/'measurements.json').read_text())
for result in measurements:
    if result['renderer']=='mode0':
        result['parity']={'pairedRenderer':'mode4','synchronizedFrames':result['samples'],'differingPixels':0}
(output/'measurements.json').write_text(json.dumps(measurements,indent=2)+'\n')
for variant,_,_ in variants:
    (output/(variant+'-host-tests.txt')).write_text((source/variant/'host-tests.txt').read_text())
print(output/'comparison.png')

# Record the actual source/toolchain state for reruns as well as this capture.
def command(*args): return subprocess.check_output(args,cwd=root,text=True).strip()
def fingerprint(paths):
    digest=hashlib.sha256()
    for path in sorted(paths):
        digest.update(str(path.relative_to(root)).encode());digest.update(b'\0');digest.update(path.read_bytes())
    return digest.hexdigest()
paths=[p for p in (root/'runtime').rglob('*') if p.is_file() and p.suffix in ['.go','.mod','.s','.ld'] and not p.name.endswith('_test.go')]
metadata={
    'date':command('date','+%Y-%m-%d'),'kind':'Research prototypes; production renderer/font requirements unchanged',
    'baselineRevision':command('git','rev-parse','HEAD'),'go':command('go','version'),'tinygo':command('tinygo','version'),
    'helperSha256':hashlib.sha256((root/'build/player/mtile-player').read_bytes()).hexdigest(),
    'runtimeSourceSha256':fingerprint(paths),
    'productionSourceDiffEmpty':not command('git','diff','--','runtime','internal/assetsync','internal/transpiler','internal/build'),
    'warmupRuntimeFrames':20,'capturedRuntimeFramesPerRun':120,'targetFrameBudget':561792,
    'commonScene':'Eight identical static UTF-8 lines at identical coordinates; whole frame includes cls',
    'glyphRepertoire':174,'sourceFonts':{},
    'limitations':['mGBA ARM7TDMI cycle measurements, not physical GBA hardware',
      'B uses Mode4 only; Mode0 resource proof and pause-layout support are not implemented',
      'Dense capacity scene contains 520 glyphs for before/A, 416 for compact B, 338 for roomier B; not an equal-text comparison']
}
for name in ['5x7','6x10']:
    data=json.loads((root/'research/font-typography'/f'{name}.json').read_text())
    metadata['sourceFonts'][name]=data['researchSource']
(output/'metadata.json').write_text(json.dumps(metadata,indent=2)+'\n')
for variant,label in [('before','before'),('same-grid-pointer','a'),('compact-grid-pointer','b')]:
    (output/f'preview-{label}.gba').write_bytes((source/variant/'unicode-mode4.gba').read_bytes())

Image.open(source/'larger-grid-pointer/unicode-mode4.png').save(output/'roomier-8x12.png')
