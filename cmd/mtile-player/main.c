/*
 * Mtile's private headless mGBA frontend.
 *
 * The process communicates only through a small binary stdin/stdout protocol;
 * it never creates a native window. mGBA itself is MPL-2.0 and remains an
 * independently built component. See docs/third-party/mgba.md.
 */
#include <errno.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <mgba/core/config.h>
#include <mgba/core/blip_buf.h>
#include <mgba/core/core.h>
#include <mgba/core/interface.h>
#include <mgba/core/log.h>
#include <mgba/internal/gba/audio.h>

#define PLAYER_WIDTH 240
#define PLAYER_HEIGHT 160
#define PLAYER_RATE 48000
#define PLAYER_AUDIO_MAX 4096
#define PLAYER_PROTOCOL 2
#define PLAYER_TELEMETRY_SIZE 552
#define PLAYER_DEBUG_MAILBOX 0x0203FE00
#define PLAYER_DEBUG_MAILBOX_SIZE 512

enum PlayerCommand {
	PLAYER_STEP = 1,
	PLAYER_RESET = 2,
	PLAYER_QUIT = 3,
};

struct PlayerCapture {
	struct mAVStream stream;
	color_t video[PLAYER_WIDTH * PLAYER_HEIGHT];
	int16_t audio[PLAYER_AUDIO_MAX * 2];
	uint32_t audioFrames;
	uint32_t overflow;
};

/* stdout is the framed IPC transport. Route every mGBA diagnostic to stderr;
 * a single informational log line on stdout would corrupt the next frame. */
static void playerLog(struct mLogger* logger, int category, enum mLogLevel level, const char* format, va_list args) {
	(void) logger;
	(void) level;
	const char* name = mLogCategoryName(category);
	fprintf(stderr, "mGBA[%s]: ", name ? name : "core");
	vfprintf(stderr, format, args);
	fputc('\n', stderr);
}

static struct PlayerCapture* captureFromStream(struct mAVStream* stream) {
	return (struct PlayerCapture*) stream;
}

static void captureVideo(struct mAVStream* stream, const color_t* buffer, size_t stride) {
	struct PlayerCapture* capture = captureFromStream(stream);
	if (!buffer || stride < PLAYER_WIDTH || stride > 4096) {
		return;
	}
	for (size_t row = 0; row < PLAYER_HEIGHT; ++row) {
		memcpy(&capture->video[row * PLAYER_WIDTH], &buffer[row * stride], PLAYER_WIDTH * sizeof(color_t));
	}
}

/* mAVStream.postAudioFrame is the GBA SOUNDBIAS hardware stream (normally
 * 32768 Hz), not mGBA's configured frontend rate. Drain the same band-limited
 * blip buffers used by the Qt/SDL frontends so the 48 kHz protocol rate is
 * truthful and the Develop preview has the same pitch as standalone mGBA. */
static void configureAudioRate(struct mCore* core) {
	blip_t* left = core->getAudioChannel(core, 0);
	blip_t* right = core->getAudioChannel(core, 1);
	if (!left || !right) {
		return;
	}
	double rateRatio = 1.0;
	if (core->opts.fpsTarget > 0) {
		rateRatio = GBAAudioCalculateRatio(1, core->opts.fpsTarget, 1);
	}
	double outputRate = PLAYER_RATE * rateRatio;
	blip_set_rates(left, core->frequency(core), outputRate);
	blip_set_rates(right, core->frequency(core), outputRate);
}

static void captureResampledAudio(struct mCore* core, struct PlayerCapture* capture) {
	blip_t* left = core->getAudioChannel(core, 0);
	blip_t* right = core->getAudioChannel(core, 1);
	if (!left || !right) {
		capture->audioFrames = 0;
		return;
	}
	int available = blip_samples_avail(left);
	int rightAvailable = blip_samples_avail(right);
	if (rightAvailable < available) {
		available = rightAvailable;
	}
	if (available > PLAYER_AUDIO_MAX) {
		available = PLAYER_AUDIO_MAX;
		capture->overflow = 1;
	}
	if (available <= 0) {
		capture->audioFrames = 0;
		return;
	}
	blip_read_samples(left, capture->audio, available, 1);
	blip_read_samples(right, capture->audio + 1, available, 1);
	capture->audioFrames = (uint32_t) available;
}

static int readExact(void* output, size_t size) {
	uint8_t* bytes = output;
	while (size) {
		size_t count = fread(bytes, 1, size, stdin);
		if (!count) {
			return 0;
		}
		bytes += count;
		size -= count;
	}
	return 1;
}

static int writeExact(const void* input, size_t size) {
	return fwrite(input, 1, size, stdout) == size;
}

static int write16(uint16_t value) {
	uint8_t bytes[2] = {(uint8_t) value, (uint8_t) (value >> 8)};
	return writeExact(bytes, sizeof(bytes));
}

static int write32(uint32_t value) {
	uint8_t bytes[4] = {
		(uint8_t) value, (uint8_t) (value >> 8),
		(uint8_t) (value >> 16), (uint8_t) (value >> 24),
	};
	return writeExact(bytes, sizeof(bytes));
}

static void store16(uint8_t* output, uint16_t value) {
	output[0] = (uint8_t) value;
	output[1] = (uint8_t) (value >> 8);
}

static void captureTelemetry(struct mCore* core, uint8_t* output) {
	static const uint32_t registers[] = {
		0x04000060, 0x04000062, 0x04000064,
		0x04000068, 0x0400006C,
		0x04000070, 0x04000072, 0x04000074,
		0x04000078, 0x0400007C,
		0x04000080, 0x04000082, 0x04000084, 0x04000088,
	};
	memset(output, 0, PLAYER_TELEMETRY_SIZE);
	memcpy(output, "TELM", 4);
	store16(&output[4], 1);
	store16(&output[6], PLAYER_TELEMETRY_SIZE);
	store16(&output[8], (uint16_t) core->busRead16(core, 0x04000006));
	store16(&output[10], (uint16_t) core->busRead16(core, 0x04000130));
	for (size_t index = 0; index < sizeof(registers) / sizeof(registers[0]); ++index) {
		store16(&output[12 + index * 2], (uint16_t) core->busRead16(core, registers[index]));
	}
	for (size_t index = 0; index < PLAYER_DEBUG_MAILBOX_SIZE; ++index) {
		output[40 + index] = (uint8_t) core->busRead8(core, PLAYER_DEBUG_MAILBOX + (uint32_t) index);
	}
}

static uint16_t read16(const uint8_t* bytes) {
	return (uint16_t) bytes[0] | (uint16_t) bytes[1] << 8;
}

static int writeHandshake(void) {
	return writeExact("GLUP", 4) && write16(PLAYER_PROTOCOL) &&
	       write16(PLAYER_WIDTH) && write16(PLAYER_HEIGHT) &&
	       write16(0) && write32(PLAYER_RATE);
}

static int writeFrame(struct mCore* core, const struct PlayerCapture* capture, uint32_t frameNumber) {
	uint8_t telemetry[PLAYER_TELEMETRY_SIZE];
	captureTelemetry(core, telemetry);
	if (!writeExact("FRAM", 4) || !write32(frameNumber) ||
	    !write32(capture->audioFrames) || !write32(capture->overflow) ||
	    !write32(PLAYER_TELEMETRY_SIZE)) {
		return 0;
	}
	uint8_t rgba[PLAYER_WIDTH * PLAYER_HEIGHT * 4];
	for (size_t index = 0; index < PLAYER_WIDTH * PLAYER_HEIGHT; ++index) {
		color_t pixel = capture->video[index];
#ifdef COLOR_16_BIT
		rgba[index * 4] = M_R8(pixel);
		rgba[index * 4 + 1] = M_G8(pixel);
		rgba[index * 4 + 2] = M_B8(pixel);
#else
		rgba[index * 4] = pixel & 0xff;
		rgba[index * 4 + 1] = (pixel >> 8) & 0xff;
		rgba[index * 4 + 2] = (pixel >> 16) & 0xff;
#endif
		rgba[index * 4 + 3] = 0xff;
	}
	if (!writeExact(rgba, sizeof(rgba))) {
		return 0;
	}
	/* All currently supported desktop release targets are little endian, but
	 * write explicitly so the protocol stays stable and inspectable. */
	for (uint32_t index = 0; index < capture->audioFrames * 2; ++index) {
		if (!write16((uint16_t) capture->audio[index])) {
			return 0;
		}
	}
	if (!writeExact(telemetry, sizeof(telemetry))) {
		return 0;
	}
	return fflush(stdout) == 0;
}

static int runPlayer(const char* romPath) {
	int status = 1;
	struct mLogger logger = {
		.log = playerLog,
		.filter = NULL,
	};
	mLogSetDefaultLogger(&logger);
	struct mCore* core = mCoreFind(romPath);
	if (!core) {
		fprintf(stderr, "mtile-player: mGBA could not identify %s\n", romPath);
		mLogSetDefaultLogger(NULL);
		return status;
	}
	mCoreInitConfig(core, "mtile-player");
	struct mCoreOptions options = {
		.skipBios = true,
		.audioBuffers = 1024,
		.sampleRate = PLAYER_RATE,
		.fpsTarget = 60,
		.volume = 0x100,
	};
	mCoreConfigLoadDefaults(&core->config, &options);
	if (!core->init(core)) {
		fprintf(stderr, "mtile-player: mGBA core initialization failed\n");
		goto config;
	}
	if (!mCoreLoadFile(core, romPath)) {
		fprintf(stderr, "mtile-player: could not load %s\n", romPath);
		goto initialized;
	}
	mCoreConfigMap(&core->config, &core->opts);
	core->setAudioBufferSize(core, core->opts.audioBuffers);
	core->loadConfig(core, &core->config);

	struct PlayerCapture capture = {
		.stream = {
			.postVideoFrame = captureVideo,
		},
	};
	core->setVideoBuffer(core, capture.video, PLAYER_WIDTH);
	core->setAVStream(core, &capture.stream);
	core->reset(core);
	configureAudioRate(core);
	if (!writeHandshake() || fflush(stdout)) {
		goto loaded;
	}

	uint32_t frameNumber = 0;
	uint8_t command[4];
	uint16_t tracedKeys = UINT16_MAX;
	int traceKeys = getenv("MTILE_PLAYER_TRACE_KEYS") != NULL;
	status = 0;
	while (readExact(command, sizeof(command))) {
		uint8_t operation = command[0];
		if (operation == PLAYER_QUIT) {
			break;
		}
		if (operation != PLAYER_STEP && operation != PLAYER_RESET) {
			fprintf(stderr, "mtile-player: unsupported operation %u\n", operation);
			status = 1;
			break;
		}
		if (operation == PLAYER_RESET) {
			core->reset(core);
			configureAudioRate(core);
			frameNumber = 0;
		}
		capture.audioFrames = 0;
		capture.overflow = 0;
		uint16_t keys = read16(&command[2]);
		core->setKeys(core, keys);
		if (traceKeys && keys != tracedKeys) {
			fprintf(stderr, "input set=%03x core=%03x keyinput=%03x\n",
			        keys, (unsigned) core->getKeys(core),
			        (unsigned) core->busRead16(core, 0x04000130));
			tracedKeys = keys;
		}
		core->runFrame(core);
		captureResampledAudio(core, &capture);
		if (!writeFrame(core, &capture, frameNumber++)) {
			status = ferror(stdout) ? 1 : 0;
			break;
		}
	}

loaded:
	core->setAVStream(core, NULL);
initialized:
	mCoreConfigDeinit(&core->config);
	core->deinit(core);
	mLogSetDefaultLogger(NULL);
	return status;
config:
	mCoreConfigDeinit(&core->config);
	mLogSetDefaultLogger(NULL);
	return status;
}

int main(int argc, char** argv) {
#ifdef _WIN32
	_setmode(_fileno(stdin), _O_BINARY);
	_setmode(_fileno(stdout), _O_BINARY);
#endif
	if (argc != 2) {
		fprintf(stderr, "usage: mtile-player game.gba\n");
		return 2;
	}
	return runPlayer(argv[1]);
}
