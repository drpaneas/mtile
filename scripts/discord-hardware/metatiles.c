#include <mgba/core/core.h>
#include <mgba/core/config.h>
#include <mgba/gba/core.h>
#include <inttypes.h>
#include <stdio.h>
int main(int argc,char **argv) {
 if(argc!=2)return 2;
 struct mCore *core=GBACoreCreate();
 if(!core)return 3;
 mCoreInitConfig(core,"metatile-trial");
 struct mCoreOptions options={.skipBios=true,.audioBuffers=1024,.sampleRate=32768,.fpsTarget=60,.volume=0x100};
 mCoreConfigLoadDefaults(&core->config,&options);
 if(!core->init(core)||!mCoreLoadFile(core,argv[1]))return 3;
 mCoreConfigMap(&core->config,&core->opts);
 core->setAudioBufferSize(core,core->opts.audioBuffers);
 core->loadConfig(core,&core->config);
 static color_t pixels[240*160];
 core->setVideoBuffer(core,pixels,240);
 core->reset(core);
 int frames=0;
 while(frames++<1000&&core->busRead32(core,0x0203f000)!=0x12340002)core->runFrame(core);
 uint32_t state=core->busRead32(core,0x0203f000);
 printf("{\"state\":%"PRIu32",\"frames\":%d,\"repeatedRawCycles\":%"PRIu32",\"repeatedMetatileCycles\":%"PRIu32",\"repeatedMismatches\":%"PRIu32",\"diverseRawCycles\":%"PRIu32",\"diverseMetatileCycles\":%"PRIu32",\"diverseMismatches\":%"PRIu32"}\n",state,frames,core->busRead32(core,0x0203f004),core->busRead32(core,0x0203f008),core->busRead32(core,0x0203f00c),core->busRead32(core,0x0203f010),core->busRead32(core,0x0203f014),core->busRead32(core,0x0203f018));
 int fail=state!=0x12340002||core->busRead32(core,0x0203f00c)||core->busRead32(core,0x0203f018);
 core->deinit(core);
 return fail;
}
