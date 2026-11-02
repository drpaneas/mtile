#include <maxmod.h>
#include <mm_mas.h>
#include "src/core/channel_types.h"
#include "src/core/player_types.h"
#include "src/gba/mixer.h"

// Only the short-lived accumulator needs fast RAM. All persistent channel
// state and DMA buffers have fixed storage in EWRAM, outside the Lua heap.
static unsigned int mix[MM_MIXLEN_16KHZ / 4];
static unsigned int wave[MM_MIXLEN_16KHZ / 4] __attribute__((section(".ewram")));
static mm_module_channel modules[32] __attribute__((section(".ewram")));
static mm_active_channel active[32] __attribute__((section(".ewram")));
static mm_mixer_channel channels[32] __attribute__((section(".ewram")));
static mm_sfxhand effects[8];
static unsigned char priorities[8];

int mtile_mm_init(void *bank)
{
    mm_gba_system setup = {
        .mixing_mode = MM_MIX_16KHZ,
        .mod_channel_count = 32, .mix_channel_count = 32,
        .module_channels = modules, .active_channels = active,
        .mixing_channels = channels, .mixing_memory = mix,
        .wave_memory = wave, .soundbank = bank
    };
    return mmInit(&setup);
}

void mtile_mm_music(int id, int loop)
{
    if (id < 0) mmStop();
    else mmStart(id, loop ? MM_PLAY_LOOP : MM_PLAY_ONCE);
}

void mtile_mm_sample(int id, int volume, int priority)
{
    if (id < 0) {
        mmEffectCancelAll();
        return;
    }
    int slot = -1;
    for (int i = 0; i < 8; i++) {
        if (!mmEffectActive(effects[i])) { slot = i; break; }
        if (slot < 0 || priorities[i] < priorities[slot]) slot = i;
    }
    if (mmEffectActive(effects[slot])) {
        if (priority < priorities[slot]) return;
        mmEffectCancel(effects[slot]);
    }
    mm_sound_effect sound = {
        .id = id, .rate = 1024, .handle = 0,
        .volume = volume, .panning = 128
    };
    effects[slot] = mmEffectEx(&sound);
    priorities[slot] = priority;
}
