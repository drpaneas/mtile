#!/usr/bin/env python3
"""Reproducible research-only font substitution; never edits the working runtime."""
import argparse, hashlib, json, os, re, shutil, subprocess, sys
from pathlib import Path

SOURCES = {
    '5x7': ('6cdcaa87c2b22517a8265e3f5ea103a36d64642a9a727a7aeb6e1e9bc3961bdf', 5,7,6,8),
    '6x10': ('d61fc0a144077b59599d128f31de5c01a7a8a426cf2456d581ba4421af092e19',6,10,8,12),
}

def extract(baseline, output):
    reference=json.loads(Path(baseline).read_text())
    for name,(digest,w,h,cw,ch) in SOURCES.items():
        path=Path('/tmp/mtile-font-'+name+'.bdf')
        raw=path.read_bytes()
        assert hashlib.sha256(raw).hexdigest()==digest, 'source checksum changed'
        text=raw.decode(); glyphs={}
        for block in text.split('STARTCHAR ')[1:]:
            cp=int(re.search(r'^ENCODING (\d+)',block,re.M)[1])
            bw,bh,bx,by=map(int,re.search(r'^BBX ([-\d ]+)',block,re.M)[1].split())
            assert (bw,bh,bx,by)==(w,h,0,-1 if h==7 else -2)
            rows=re.search(r'\nBITMAP\n(.*?)\nENDCHAR',block,re.S)[1].splitlines()
            assert len(rows)==h
            columns=[0]*w
            for y,row in enumerate(rows):
                bits=int(row,16)
                for x in range(w):
                    if bits & (1<<(7-x)): columns[x]|=1<<y
            glyphs[cp]=columns
        # U+1E9E postdates these public-domain source fonts. An original capital
        # sharp-S glyph is supplied explicitly; no fallback or lowercase alias.
        patterns={'5x7':['.###.','#...#','#..#.','#.##.','#...#','#...#','#.##.'],
                  '6x10':['......','.###..','#...#.','#...#.','#..#..','#.##..','#...#.','#...#.','#.##..','......']}
        columns=[0]*w
        for y,row in enumerate(patterns[name]):
            for x,pixel in enumerate(row):
                if pixel=='#': columns[x]|=1<<y
        glyphs[0x1e9e]=columns
        missing=[g['codepoint'] for g in reference['glyphs'] if g['codepoint'] not in glyphs]
        assert not missing, missing
        result=dict(reference,name='misc-fixed-'+name,width=w,height=h,cellWidth=cw,cellHeight=ch)
        result['glyphs']=[dict(codepoint=g['codepoint'],columns=glyphs[g['codepoint']]) for g in reference['glyphs']]
        result['researchSource']={'url':'https://raw.githubusercontent.com/olikraus/u8g2/master/tools/font/bdf/'+name+'.bdf','sha256':digest,'license':'Public domain (BDF COPYRIGHT)','originalGlyphs':['U+1E9E']}
        Path(output,name+'.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
        print(name,len(result['glyphs']),'glyphs')

def prepare(root,out,fontpath):
    root=Path(root);out=Path(out);font=json.loads(Path(fontpath).read_text())
    w,h,cw,ch=(font[k] for k in ['width','height','cellWidth','cellHeight'])
    runtime=out/'runtime'
    def ignore(path,names):
        return [n for n in names if n.endswith('_test.go') or (not n.endswith('.go') and not n.endswith('.mod') and n not in ['LICENSE','crt0','hw','audio'] and not n.endswith('.s') and not n.endswith('.ld'))]
    shutil.copytree(root/'runtime',runtime,ignore=ignore,dirs_exist_ok=True)
    if (w,h,cw,ch)!=(4,5,6,8) or font.get('probePointer'):
        kind='byte' if h<=8 else 'uint16'
        f=runtime/'font.go';s=f.read_text().replace('[95][4]byte',f'[95][{w}]{kind}').replace('[4]byte',f'[{w}]{kind}').replace('col > 31',f'col > {(1<<h)-1}')
        if font.get('probePointer'):
            start=s.index('func (f *Font) Columns(')
            end=s.index('// SetFont',start)
            pointer_code="""//go:section .rodata
var emptyProbeGlyph = [W]KIND{}
func (f *Font) Columns(ch rune) *[W]KIND {
 if f==nil {f=DefaultFont();if f==nil{return &emptyProbeGlyph}}
 if ch>=32&&ch<=126{return &f.ASCII[ch-32]}
 lo,hi:=0,len(f.Extra)
 for lo<hi{mid:=lo+(hi-lo)/2;if f.Extra[mid].Codepoint<ch{lo=mid+1}else{hi=mid}}
 if lo<len(f.Extra)&&f.Extra[lo].Codepoint==ch{return &f.Extra[lo].Columns}
 return &f.ASCII['?'-32]
}
func glyphColumns(ch byte) *[W]KIND {return DefaultFont().Columns(rune(ch))}

""".replace('W',str(w)).replace('KIND',kind)
            s=s[:start]+pointer_code+s[end:]
        f.write_text(s)
        # Project build tag excludes the original table. Host probes likewise
        # supply the explicit same font; no unselected default bank is linked.
        (runtime/'font_default.go').write_text('''//go:build !mtile_project_font
package runtime
func DefaultFont() *Font {return nil}
''')
        f=runtime/'text.go';s=f.read_text().replace('glyphW = 6',f'glyphW = {cw}').replace('glyphH = 8',f'glyphH = {ch}').replace('glyphBitmapW = 4',f'glyphBitmapW = {w}').replace('glyphBitmapH = 5',f'glyphBitmapH = {h}').replace('columns[col] & 0x1F',f'columns[col] & {(1<<h)-1}')
        f.write_text(s)
        # 5x7 fits the existing pause-label layout and 8x8 OBJ glyph tiles.
        if h<=8:
            f=runtime/'mode0_glyph.go';f.write_text(f.read_text().replace('columns[col] & 0x1f',f'columns[col] & {(1<<h)-1}'))
            f=runtime/'pause_video.go';f.write_text(f.read_text().replace('row < 5',f'row < {h}'))
    (runtime/'typography_probe_test.go').write_text(host_tests(font))
    wrapper=out/'tinygo-probe'
    wrapper.write_text('#!/bin/sh\nexec python3 '+shquote(str(root/'research/font-typography/probe.py'))+' rewrite '+shquote(str(Path(fontpath).resolve()))+' -- "$@"\n')
    wrapper.chmod(0o755)
    return runtime,wrapper

def shquote(s): return "'"+s.replace("'","'\\''")+"'"

def declaration(f):
    w,h=f['width'],f['height'];kind='byte' if h<=8 else 'uint16';typ=f'[{w}]{kind}'
    glyphs={g['codepoint']:g['columns'] for g in f['glyphs']}
    lines=['// Typography probe font; research only.','//go:section .rodata','var projectFontExtraROM = [...]rt.FontGlyph{']
    for cp,cols in sorted(glyphs.items()):
        if cp>126:lines.append('{Codepoint:%d,Columns:%s{%s}},'%(cp,typ,','.join(map(str,cols))))
    lines+=['}','//go:section .rodata',f'var projectFontROM = rt.Font{{ASCII:[95]{typ}{{']
    for cp in range(32,127): lines.append('{'+','.join(map(str,glyphs[cp]))+'},')
    lines+=['},Extra:projectFontExtraROM[:]}']
    return '\n'.join(lines)+'\n'

def rewrite(fontpath,args):
    if args and args[0]=='build':
        path=Path('main.go');s=path.read_text()
        if '// Typography probe font;' not in s:
            f=json.loads(Path(fontpath).read_text())
            start=s.index('//go:section .rodata\nvar projectFontExtraROM')
            marker='},Extra:projectFontExtraROM[:]}'
            end=s.index(marker,start)+len(marker)
            s=s[:start]+declaration(f)+s[end:]
            if 'var mode0TextKeysROM' in s:
                assert f['width']<=8 and f['height']<=8, 'larger fonts require a new Mode0 proof'
                keys=re.search(r'var mode0TextKeysROM = \[\d+\]uint16\{([^}]+)\}',s)[1]
                keys=[int(v.strip(),0) for v in keys.split(',') if v.strip()]
                glyphs={g['codepoint']:g['columns'] for g in f['glyphs']}
                words=[]
                for key in keys:
                    tile=bytearray(64)
                    for x,col in enumerate(glyphs[key>>8]):
                        for y in range(f['height']):
                            if col&(1<<y): tile[y*8+x]=key&255
                    words.extend(int.from_bytes(tile[n:n+4],'little') for n in range(0,64,4))
                data='var mode0TextTilesROM = [%d]uint32{%s}'%(len(words),','.join(map(str,words)))
                s,n=re.subn(r'var mode0TextTilesROM = \[\d+\]uint32\{[^}]*\}',data,s);assert n==1
            path.write_text(s)
    os.execvp('tinygo',['tinygo']+args)

def host_tests(f):
    # An independent, row-major oracle checks actual bitmaps, clipping and camera.
    template=r'''package runtime
import ("testing";"image";"image/color";"image/png";"os";"path/filepath";"fmt")
FONT_DECL
const probeCellW=CELLW
const probeCellH=CELLH
const probeInkW=INKW
const probeInkH=INKH
func TestTypographyBoundsAndAPI(t *testing.T) {
 font:=&projectFontROM
 var c Context
 if !c.SetFont(font){t.Fatal("font rejected")}
 chars:=[]rune("AgjpqyΩάήΐΰäöüßẞ")
 for cp:=rune(32);cp<=126;cp++{chars=append(chars,cp)}
 for _,cp:=range chars {
  for _,origin:=range [][2]int{{0,0},{234,152},{239,159},{-3,-3},{238,150}} {
   for _,cam:=range [][2]int{{0,0},{3,4},{-2,-1}} {
    for _,clip:=range [][4]int{{0,0,240,160},{2,2,236,156},{230,150,4,5},{0,0,1,1}} {
     c.CameraReset();c.Cls(0);c.Camera(cam[0],cam[1]);c.Clip(clip[0],clip[1],clip[2],clip[3])
     c.Print(string(cp),origin[0],origin[1],7)
     columns:=font.Columns(cp);x0,y0,w,h:=c.ClipRect()
     for y:=0;y<160;y++{for x:=0;x<240;x++ {
      want:=byte(0);gx,gy:=x-(origin[0]-cam[0]),y-(origin[1]-cam[1])
      if x>=x0&&x<x0+w&&y>=y0&&y<y0+h&&gx>=0&&gx<probeInkW&&gy>=0&&gy<probeInkH&&columns[gx]&(1<<uint(gy))!=0{want=7}
      if got:=c.pixelRaw(x,y);got!=want{t.Fatalf("U+%04X origin%v cam%v clip%v pixel%d,%d=%d want%d",cp,origin,cam,clip,x,y,got,want)}
     }}
    }
   }
  }
 }
 c.CameraReset();c.Cls(0)
 if got:=c.Print("Ωä",10,10,7);got!=10+2*probeCellW{t.Fatal("explicit return",got)}
 c.Cursor(0,0,7);if got:=c.PrintCursor("Ωä");got!=2*probeCellW||c.cursor.x!=0||c.cursor.y!=probeCellH{t.Fatal("cursor metrics")}
 c.Cls(0);if got:=c.Print("A\tA",0,0,7);got!=5*probeCellW{t.Fatal("tab metrics",got)}
 c.Cls(0);if got:=c.Print("\x012Ω",0,0,7);got!=2*probeCellW{t.Fatal("repeat metrics",got)}
 c.Cls(0);c.Clip(0,0,2*probeCellW,3*probeCellH);if got:=c.Print("AAA",0,0,7);got!=2*probeCellW{t.Fatal("wrap return",got)}
 c.Cls(0);c.Print("\x022A",0,0,7);if c.pixelRaw(probeCellW-1,probeCellH-1)!=2{t.Fatal("background cell")}
 c.Cls(0);if got:=c.Print("🙂",0,0,7);got!=probeCellW{t.Fatal("fallback advance")}
 if got:=testing.AllocsPerRun(100,func(){c.Print("Άήΐΰ Grüße gjpqy",0,0,7)});got!=0{t.Fatal("render allocates",got)}
 bad:=*font;bad.ASCII[1][0]=1<<probeInkH;if c.SetFont(&bad){t.Fatal("out-of-height column accepted")}
}
func TestTypographyPause(t *testing.T) {
 if probeInkH>8 {t.Skip("6x10 needs a new pause layout; research does not claim compatibility")}
 // Compare every fixed pause label against the exact untruncated 1bpp source.
 labels:=[]struct{s string;y int}{{"PAUSED",14},{"Load",26},{"Save",36},{"Exit",46},{"SESSION",18},{"ENDED",26},{"PLEASE WAIT",44},{"Power off",42},{"or reset",50},{"Saved",49},{"Loaded",49},{"No save yet",49},{"Wrong save",49},{"Bad save",49},{"Save failed",49},{"Load failed",49},{"Unavailable",49},{"Please wait",57}}
 for _,label:=range labels {
  var v pauseVideo;v.font=&projectFontROM;x:=(64-(len(label.s)*5-1))/2
  v.text(label.s,x,label.y,4)
  for i,cp:=range label.s {cols:=projectFontROM.Columns(cp);for col,bits:=range cols{for row:=0;row<probeInkH;row++{if bits&(1<<uint(row))==0{continue};px,py:=x+i*5+col,label.y+row;if px<0||px>=64||py<0||py>=64{t.Fatalf("pause ink outside 64x64: %s",label.s)};word:=(py/8*8+px/8)*8+py%8;got:=(v.pixels[word]>>uint(px%8*4))&15;if got!=4{t.Fatalf("truncated pause %s at %d,%d",label.s,px,py)}}}}
 }
 out:=os.Getenv("TYPOGRAPHY_OUT");if out==""{return}
 for _,state:=range []struct{name string;result int8;wait uint8;ended bool}{{"paused",0,0,false},{"save-failed",-1,1,false},{"ended",0,0,true}} {
  var c Context;c.SetFont(&projectFontROM);c.pause.saving=true;c.pause.result=state.result;c.pause.wait=state.wait;c.pause.ended=state.ended;c.PreparePauseMenu()
  img:=image.NewRGBA(image.Rect(0,0,64,64));palette:=[]color.RGBA{{0,0,0,255},{12,18,35,255},{40,50,75,255},{100,160,210,255},{235,235,235,255},{130,150,175,255}}
  for y:=0;y<64;y++{for x:=0;x<64;x++{word:=(y/8*8+x/8)*8+y%8;value:=(c.pause.video.pixels[word]>>uint(x%8*4))&15;if int(value)<len(palette){img.SetRGBA(x,y,palette[value])}}}
  file,err:=os.Create(filepath.Join(out,fmt.Sprintf("pause-%s.png",state.name)));if err!=nil{t.Fatal(err)};err=png.Encode(file,img);file.Close();if err!=nil{t.Fatal(err)}
 }
}
'''
    result=template.replace('FONT_DECL',declaration(f).replace('rt.',''))
    for key,value in [('CELLW',f['cellWidth']),('CELLH',f['cellHeight']),('INKW',f['width']),('INKH',f['height'])]:result=result.replace(key,str(value))
    return result

if __name__=='__main__':
    cmd=sys.argv[1]
    if cmd=='extract':extract(sys.argv[2],sys.argv[3])
    elif cmd=='prepare':print(*prepare(*sys.argv[2:]),sep='\n')
    elif cmd=='rewrite':rewrite(sys.argv[2],sys.argv[4:])
    else:raise ValueError(cmd)
