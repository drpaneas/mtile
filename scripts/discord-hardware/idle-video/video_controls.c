#include <mgba/core/core.h>
#include <mgba/core/config.h>
#include <mgba/gba/core.h>
#include <mgba/internal/gba/gba.h>
#include <mgba/internal/arm/arm.h>
#include <stdio.h>
#include <stdlib.h>
#include <inttypes.h>

static struct mCore *core;
static struct GBA *gba;
static color_t video[240*160];
static struct mTimingEvent mutation;
static uint32_t mutateAddr;
static unsigned failed;
static uint64_t dmaCycles, dmaTransfers;
static void (*originalDMA)(struct mTiming*,void*,uint32_t);
static void dmaTiming(struct mTiming *t,void *ctx,uint32_t late) {
 int active=gba->memory.activeDMA;
 int transfer=active>=0 && gba->memory.dma[active].nextCount>0;
 uint32_t before=active>=0?gba->memory.dma[active].when:0;
 originalDMA(t,ctx,late);
 if(transfer){dmaCycles+=gba->memory.dma[active].when-before;dmaTransfers++;}
}
static void w16(uint32_t a,uint16_t v){core->busWrite16(core,a,v);}
static void w32(uint32_t a,uint32_t v){core->busWrite32(core,a,v);}
static void setup(const char *rom) {
 core=GBACoreCreate();core->init(core);
 mCoreConfigInit(&core->config,NULL);
 core->setVideoBuffer(core,video,240);
 if(!mCoreLoadFile(core,rom)){exit(2);}
 core->reset(core);gba=core->board;gba->idleOptimization=IDLE_LOOP_IGNORE;
 originalDMA=gba->memory.dmaEvent.callback;gba->memory.dmaEvent.callback=dmaTiming;dmaCycles=0;dmaTransfers=0;
 w16(0x04000208,0);w16(0x04000200,0);
 for(int i=0;i<128;i++)w16(0x07000000+i*8,0x0200);
}
static void finish(void){core->deinit(core);}
static void expect(const char *name,int x,int y,uint16_t expected) {
 color_t actual=video[y*240+x]&0xffffff,want=mColorFrom555(expected);
 if(expected==0x3c0f)want=0x007f007f;
 if(expected==0x01ef)want=0x00007f7f;
 printf("blend %s pixel(%d,%d)=%08x expected=%08x %s\n",name,x,y,(unsigned)actual,(unsigned)want,actual==want?"PASS":"FAIL");
 if(actual!=want)failed++;
}
static void blend(const char *rom,int kind) {
 setup(rom);
 w16(0x04000000,0x1140); // BG0, OBJ, 1D tiles, mode 0.
 w16(0x04000008,0x1f01); // BG screen base31, priority1.
 w16(0x05000002,kind==0?0x7c00:0x03e0);
 w16(0x05000202,0x001f);w16(0x05000204,0x7c00);
 for(int i=0;i<8;i++){w32(0x06000000+i*4,0x11111111);w32(0x06010000+i*4,0x11111111);w32(0x06010020+i*4,0x22222222);}
 for(int i=0;i<1024;i++)w16(0x0600f800+2*i,0);
 w16(0x07000000,0x0400);w16(0x07000002,0);w16(0x07000004,0);
 if(kind!=0){w16(0x07000008,0);w16(0x0700000a,0);w16(0x0700000c,1);}
 w16(0x04000050,kind==1?0x1040:0x1140);
 w16(0x04000052,0x0808);
 if(kind==3){
  w16(0x04000000,0x3140); // Enable WIN0 as well as BG0/OBJ.
  w16(0x04000040,0x0004); // WIN0 x=[0,4).
  w16(0x04000044,0x0008); // WIN0 y=[0,8).
  w16(0x04000048,0x0030); // Inside: OBJ and effects; mask BG0.
  w16(0x0400004a,0x0031); // Outside: BG0, OBJ and effects.
 }
 core->runFrame(core);core->runFrame(core);
 if(kind==3){
  expect("WIN0 masks BG but cannot make OBJ a blend target",3,3,0x001f);
  expect("outside WIN0 includes BG blend target",5,3,0x01ef);
 }else{
 expect(kind==0?"OBJ over BG positive control":kind==1?"OBJ over OBJ rejected by hardware":"OBJ over OBJ uses underlying BG",3,3,kind==0?0x3c0f:kind==1?0x001f:0x01ef);
 }
 finish();
}
static void fill(uint32_t addr,uint16_t value) {for(int i=0;i<400;i++)w16(addr+i*2,value);}
static void arm(uint32_t source) {
 w16(0x040000ba,0); // DMA0 disabled before source reset.
 w16(0x05000000,core->busRead16(core,source));
 w32(0x040000b0,source+2);w32(0x040000b4,0x05000000);
 w16(0x040000b8,1);w16(0x040000ba,0xa240);
}
static int rows(uint16_t expected,int lo,int hi) {
 int bad=0;for(int y=lo;y<hi;y++)for(int x=0;x<240;x++)if((video[y*240+x]&0xffffff)!=mColorFrom555(expected))bad++;
 return bad;
}
static void mutate(struct mTiming *t,void *ctx,uint32_t late){(void)t;(void)ctx;(void)late;fill(mutateAddr,0x7c00);}
static void scanline(const char *rom) {
 setup(rom);w16(0x04000000,0);fill(0x02001000,0x001f);fill(0x02002000,0x7c00);
 core->runFrame(core);
 arm(0x02001000);core->runFrame(core);
 int bad=rows(0x001f,0,160);
 uint32_t source1=gba->memory.dma[0].nextSource;
 printf("hblank transfer benchmark: transfers=%"PRIu64" scheduled_service_cycles=%"PRIu64" frame_fraction=%.6f\n",dmaTransfers,dmaCycles,(double)dmaCycles/280896);
 printf("hblank immutable table: bad_pixels=%d source=%08x PASS=%d\n",bad,source1,bad==0);if(bad)failed++;
 // No source rewind: overwrite memory after the 161-word table with poison.
 for(int i=161;i<400;i++)w16(0x02001000+i*2,0x7fff);
 core->runFrame(core);
 int unrewound=rows(0x001f,0,160);
 printf("hblank no source rewind: changed_pixels=%d source=%08x expected_failure=%d\n",unrewound,gba->memory.dma[0].nextSource,unrewound>30000);if(unrewound<=30000)failed++;
 fill(0x02001000,0x001f);arm(0x02001000);
 mutateAddr=0x02001000;
 mutation=(struct mTimingEvent){.callback=mutate,.name="active table overwrite",.priority=0x80};
 mTimingSchedule(&gba->timing,&mutation,(228-160+80)*1232);
 core->runFrame(core);
 int top=rows(0x001f,0,40),bottom=rows(0x7c00,120,160);
 printf("hblank active overwrite: top_red_bad=%d bottom_blue_bad=%d split_frame=%d\n",top,bottom,top==0&&bottom==0);if(top||bottom)failed++;
 fill(0x02001000,0x001f);fill(0x02002000,0x001f);arm(0x02001000);
 mutateAddr=0x02002000;
 mTimingSchedule(&gba->timing,&mutation,(228-160+80)*1232);
 core->runFrame(core);
 int inactive=rows(0x001f,0,160);
 arm(0x02002000);core->runFrame(core);
 int swapped=rows(0x7c00,0,160);
 printf("hblank double buffering: old_frame_bad=%d new_frame_bad=%d PASS=%d; source_bytes_per_frame=%u two_161_halfword_buffers=%u\n",inactive,swapped,inactive==0&&swapped==0,source1-0x02001002,2*161*2);
 if(inactive||swapped)failed++;
 finish();
}
int main(int argc,char **argv){if(argc!=2)return 2;blend(argv[1],0);blend(argv[1],1);blend(argv[1],2);blend(argv[1],3);scanline(argv[1]);return failed?1:0;}
