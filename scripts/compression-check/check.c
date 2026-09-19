#include <mgba/core/core.h>
#include <mgba/gba/core.h>
#include <mgba/internal/arm/arm.h>
#include <stdint.h>
#include <stdio.h>
int main(int argc, char **argv) {
  struct mCore *c = GBACoreCreate();
  if (argc != 2 || !c)
    return 2;
  mCoreInitConfig(c, "trial");
  struct mCoreOptions options = {.skipBios = true,
                                 .audioBuffers = 1024,
                                 .sampleRate = 48000,
                                 .fpsTarget = 60,
                                 .volume = 0x100};
  mCoreConfigLoadDefaults(&c->config, &options);
  if (!c->init(c) || !mCoreLoadFile(c, argv[1]))
    return 2;
  mCoreConfigMap(&c->config, &c->opts);
  c->setAudioBufferSize(c, c->opts.audioBuffers);
  c->loadConfig(c, &c->config);
  static uint32_t video[240 * 160];
  c->setVideoBuffer(c, video, 240);
  c->reset(c);
  for (int f = 0; f < 3000; f++) {
    c->runFrame(c);
    if (c->busRead32(c, 0x02030000) == 0xD4A70001) {
      int n = c->busRead32(c, 0x02030800);
      if (n < 1 || n >= 512)
        return 1;
      for (int i = 0; i < n; i++)
        printf("%d %u\n", i, c->busRead32(c, 0x02030000 + i * 4));
      return 0;
    }
  }
  fprintf(stderr, "timeout pc=%08x\n", ((struct ARMCore *)c->cpu)->gprs[15]);
  return 1;
}
