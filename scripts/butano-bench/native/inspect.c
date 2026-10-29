// Read-only physical VRAM/OAM snapshot using the same pinned mGBA core.
// Referenced bytes are a lower bound, NOT an engine allocator high-water mark.
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <mgba/core/core.h>
#include <mgba/core/config.h>
#include <mgba/core/log.h>

static color_t video[240*160];
static uint8_t used[96*1024];
static void quiet(struct mLogger* l,int c,enum mLogLevel v,const char* f,va_list a){(void)l;(void)c;(void)v;(void)f;(void)a;}
static void mark(unsigned address,unsigned count){for(unsigned i=address;i<address+count && i<sizeof(used);++i)used[i]=1;}
static unsigned count(unsigned a,unsigned b){unsigned n=0;for(unsigned i=a;i<b;++i)n+=used[i]!=0;return n;}
int main(int argc,char**argv){
 if(argc!=2)return 2;
 struct mLogger logger={.log=quiet,.filter=NULL};mLogSetDefaultLogger(&logger);
 struct mCore* c=mCoreFind(argv[1]);if(!c)return 3;
 mCoreInitConfig(c,"mtile-player");
 struct mCoreOptions options={.skipBios=true,.audioBuffers=1024,.sampleRate=48000,.fpsTarget=60,.volume=0x100};
 mCoreConfigLoadDefaults(&c->config,&options);
 if(!c->init(c)||!mCoreLoadFile(c,argv[1]))return 4;
 mCoreConfigMap(&c->config,&c->opts);c->setAudioBufferSize(c,c->opts.audioBuffers);c->loadConfig(c,&c->config);
 c->setVideoBuffer(c,video,240);c->reset(c);
 unsigned steps=0,tick=0;
 for(;steps<20000;++steps){c->runFrame(c);if(c->busRead32(c,0x0203fe00)==0x44554c47){tick=c->busRead32(c,0x0203fe0c);if(tick>=660)break;}}
 if(tick<660)return 5;
 unsigned display=c->busRead16(c,0x04000000),mode=display&7,map_bytes=0,nonzero=0;
 if(mode==0){
  for(unsigned layer=0;layer<4;++layer){
   if(!(display&(0x100<<layer)))continue;
   unsigned control=c->busRead16(c,0x04000008+layer*2),size=control>>14;
   unsigned cells=(size==3?4096:size==0?1024:2048),screen=((control>>8)&31)*2048;
   unsigned chars=((control>>2)&3)*16384,tile_bytes=(control&128)?64:32;
   map_bytes+=cells*2;mark(screen,cells*2);
   for(unsigned cell=0;cell<cells;++cell){unsigned tile=c->busRead16(c,0x06000000+screen+cell*2)&1023;mark(chars+tile*tile_bytes,tile_bytes);}
  }
 }
 unsigned bg_referenced=count(0,65536),objects=0;
 static const unsigned widths[3][4]={{8,16,32,64},{16,32,32,64},{8,8,16,32}};
 static const unsigned heights[3][4]={{8,16,32,64},{8,8,16,32},{16,32,32,64}};
 for(unsigned i=0;i<128 && (display&4096);++i){
  unsigned a=c->busRead16(c,0x07000000+i*8),b=c->busRead16(c,0x07000002+i*8),t=c->busRead16(c,0x07000004+i*8);
  if((a&0x300)==0x200 || (a>>14)==3)continue;
  ++objects;unsigned shape=a>>14,size=b>>14,w=widths[shape][size],h=heights[shape][size],units=(a&0x2000)?2:1;
  unsigned stride=(display&64)?(w/8)*units:32;
  for(unsigned y=0;y<h/8;++y)for(unsigned x=0;x<w/8;++x)mark(65536+((t&1023)+y*stride+x*units)*32,units*32);
 }
 for(unsigned i=0;i<96*1024;i+=4){uint32_t v=c->busRead32(c,0x06000000+i);for(unsigned k=0;k<4;++k)nonzero+=((v>>(k*8))&255)!=0;}
 printf("{\"tick\":%u,\"videoRefresh\":%u,\"displayMode\":%u,\"enabledBackgrounds\":%u,\"activeOAMEntries\":%u,\"bgMapBytes\":%u,\"bgReferencedBytes\":%u,\"objReferencedBytes\":%u,\"bitmapReservedBytes\":%u,\"nonzeroVRAMBytes\":%u}\n",tick,steps,mode,(display>>8)&15,objects,map_bytes,bg_referenced,count(65536,98304),mode==4?81920:0,nonzero);
 mCoreConfigDeinit(&c->config);c->deinit(c);mLogSetDefaultLogger(NULL);return 0;
}
