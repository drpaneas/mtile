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
#include <mgba/core/serialize.h>
#include <mgba/internal/gba/audio.h>
#include <mgba/internal/gba/gba.h>
#include <mgba/internal/gba/serialize.h>
#include <mgba-util/vfs.h>

#define PLAYER_WIDTH 240
#define PLAYER_HEIGHT 160
#define PLAYER_RATE 48000
#define PLAYER_AUDIO_MAX 4096
#define PLAYER_PROTOCOL 3
#define PLAYER_STATE_MAX (4U * 1024U * 1024U)
#define PLAYER_FRAMEBUFFER_MAX (PLAYER_WIDTH * PLAYER_HEIGHT * 4U)
#define PLAYER_STATE_HEADER (1088U + PLAYER_FRAMEBUFFER_MAX)
#define PLAYER_TELEMETRY_SIZE 552
#define PLAYER_DEBUG_MAILBOX 0x0203FE00
#define PLAYER_DEBUG_MAILBOX_SIZE 512

enum PlayerCommand {
	PLAYER_STEP = 1,
	PLAYER_RESET = 2,
	PLAYER_QUIT = 3,
	PLAYER_SAVE_STATE = 4,
	PLAYER_LOAD_STATE = 5,
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

static uint32_t read32(const uint8_t* bytes) {
	return (uint32_t) bytes[0] | (uint32_t) bytes[1] << 8 |
	       (uint32_t) bytes[2] << 16 | (uint32_t) bytes[3] << 24;
}

static void store32(uint8_t* bytes, uint32_t value) {
	bytes[0] = value;
	bytes[1] = value >> 8;
	bytes[2] = value >> 16;
	bytes[3] = value >> 24;
}

static uint32_t updateCRC(uint32_t crc, const uint8_t* bytes, size_t size) {
	for (size_t i = 0; i < size; ++i) {
		crc ^= bytes[i];
		for (int bit = 0; bit < 8; ++bit) {
			crc = (crc >> 1) ^ (0xEDB88320U & (0U - (crc & 1U)));
		}
	}
	return crc;
}

static uint32_t payloadCRC(const uint8_t* bytes, size_t size) {
	return ~updateCRC(UINT32_MAX, bytes, size);
}

static int sourceROMCRC(const char* path, uint32_t* output) {
	FILE* input = fopen(path, "rb");
	if (!input) {
		return 0;
	}
	uint8_t buffer[65536];
	size_t size, total = 0;
	uint32_t crc = UINT32_MAX;
	while ((size = fread(buffer, 1, sizeof(buffer), input)) != 0) {
		total += size;
		if (total > 32U * 1024U * 1024U) {
			fclose(input);
			return 0;
		}
		crc = updateCRC(crc, buffer, size);
	}
	int ok = !ferror(input) && total > 0;
	fclose(input);
	*output = ~crc;
	return ok;
}

/* Control replies are independently framed. A rejected, bounded command leaves
 * the next STEP readable; an oversized incoming payload retires the process. */
static int writeReply(uint32_t status, const void* bytes, uint32_t size) {
	return writeExact("DATA", 4) && write32(status) && write32(size) &&
	       writeExact(bytes, size) && fflush(stdout) == 0;
}

static int writeFailure(const char* message) {
	return writeReply(1, message, (uint32_t) strlen(message));
}

static int saveCheckpoint(struct mCore* core, const struct PlayerCapture* capture, uint32_t frameNumber, uint32_t romCRC) {
	struct VFile* vf = VFileMemChunk(NULL, 0);
	if (!vf) {
		return writeFailure("cannot allocate checkpoint");
	}
	/* A wall-clock RTC becomes an emulation-clock RTC in the saved copy. Every
 * replay then sees the same time, even if the experiment is run tomorrow. */
	enum mRTCGenericType rtcOverride = core->rtc.override;
	int64_t rtcValue = core->rtc.value;
	if (rtcOverride == RTC_NO_OVERRIDE || rtcOverride == RTC_WALLCLOCK_OFFSET) {
		int64_t now = (int64_t) core->rtc.d.unixTime(&core->rtc.d) * 1000LL;
		core->rtc.override = RTC_FAKE_EPOCH;
		core->rtc.value = now - core->frameCounter(core) * (core->frameCycles(core) * 1000LL) / core->frequency(core);
	}
	int saved = mCoreSaveStateNamed(core, vf, SAVESTATE_SAVEDATA | SAVESTATE_RTC);
	core->rtc.override = rtcOverride;
	core->rtc.value = rtcValue;
	ssize_t size = vf->size(vf);
	if (!saved || size <= 0 || size > PLAYER_STATE_MAX - PLAYER_STATE_HEADER) {
		vf->close(vf);
		return writeFailure("cannot serialize checkpoint");
	}
	uint32_t total = (uint32_t) size + PLAYER_STATE_HEADER;
	uint8_t* state = calloc(1, total);
	if (!state) {
		vf->close(vf);
		return writeFailure("cannot allocate checkpoint");
	}
	vf->seek(vf, 0, SEEK_SET);
	if (vf->read(vf, state + PLAYER_STATE_HEADER, size) != size) {
		free(state);
		vf->close(vf);
		return writeFailure("cannot read checkpoint");
	}
	vf->close(vf);
	memcpy(state, "MTSS", 4);
	store32(state + 4, 4);
	store32(state + 8, romCRC);
	store32(state + 12, frameNumber);
	store32(state + 16, core->getKeys(core));
	store32(state + 20, (uint32_t) size);
	/* The pinned core format omits PSG sample caches and the sweep divider.
 * Preserve those scalar fields in our own versioned envelope; leaving them
 * from the previous run makes the first restored audio samples differ. */
	struct GBA* gba = core->board;
	state[28] = (uint8_t) gba->audio.psg.ch1.sample;
	state[29] = (uint8_t) gba->audio.psg.ch2.sample;
	state[30] = (uint8_t) gba->audio.psg.ch3.sample;
	state[31] = (uint8_t) gba->audio.psg.ch4.sample;
	store32(state + 32, (uint32_t) gba->audio.psg.ch1.sweep.step);
	store32(state + 36, (uint32_t) gba->audio.psg.ch1.control.frequency);
	store32(state + 40, (uint32_t) gba->audio.psg.ch3.window);
	store32(state + 44, (uint32_t) gba->audio.psg.ch4.nSamples);
	store32(state + 48, (uint32_t) gba->audio.psg.ch4.samples);
	/* Bytes 52..1079 are reserved for compatible presentation metadata. */
	/* The core state omits its output framebuffer. In particular, BIOS-skip
 * starts at scanline126; replaying that early checkpoint must not retain
 * rows from whatever the previous run happened to render. */
	store32(state + 1080, sizeof(color_t));
	store32(state + 1084, sizeof(capture->video));
	memcpy(state + 1088, capture->video, sizeof(capture->video));
	store32(state + 24, payloadCRC(state, total));
	int result = writeReply(0, state, total);
	free(state);
	return result;
}

struct PlayerStateExtras {
	struct mStateExtdataItem savedata;
	struct mStateExtdataItem rtc;
};

/* Our checkpoint contains the pinned, raw mGBA state plus only savedata and
 * generic RTC entries. Validate their complete ranges before calling the
 * core: the upstream extdata parser trusts allocation sizes, and its named
 * loader applies extdata even when the machine state itself is rejected. */
static int checkpointExtras(struct mCore* core, uint8_t* bytes, uint32_t size, struct PlayerStateExtras* extras) {
	size_t nativeSize = core->stateSize(core);
	if (nativeSize != sizeof(struct GBASerializedState) || size < nativeSize ||
	    read32(bytes) != GBASavestateMagic + GBASavestateVersion) {
		return 0;
	}
	uint32_t position = (uint32_t) nativeSize;
	uint32_t dataStart = position;
	uint32_t dataEnd = position;
	for (int entry = 0; entry < 3; ++entry) {
		if (position > size || size - position < 16) {
			return 0;
		}
		const uint8_t* header = bytes + position;
		uint32_t tag = read32(header);
		uint32_t length = read32(header + 4);
		uint32_t offset = read32(header + 8);
		position += 16;
		if (!tag) {
			if (length || offset || read32(header + 12) || !extras->rtc.data ||
			    dataStart < position) {
				return 0;
			}
			return 1;
		}
		if (read32(header + 12) || !length || offset > size || length > size - offset) {
			return 0;
		}
		if (offset < nativeSize || offset < dataEnd) {
			return 0;
		}
		if (entry == 0) {
			dataStart = offset;
		}
		struct mStateExtdataItem* item;
		if (tag == EXTDATA_RTC && length == sizeof(struct mRTCGenericState) && !extras->rtc.data) {
			uint32_t kind = read32(bytes + offset);
			if (kind != RTC_FIXED && kind != RTC_FAKE_EPOCH) {
				return 0;
			}
			item = &extras->rtc;
		} else if (tag == EXTDATA_SAVEDATA && length <= 128U * 1024U && !extras->savedata.data) {
			item = &extras->savedata;
		} else {
			return 0;
		}
		item->data = bytes + offset;
		item->size = (int32_t) length;
		dataEnd = offset + length;
	}
	return 0;
}

static int loadCheckpoint(struct mCore* core, struct PlayerCapture* capture, uint32_t* frameNumber, uint32_t romCRC) {
	uint8_t lengthBytes[4];
	if (!readExact(lengthBytes, sizeof(lengthBytes))) {
		return 0;
	}
	uint32_t size = read32(lengthBytes);
	if (size > PLAYER_STATE_MAX) {
		writeFailure("checkpoint exceeds limit");
		return 0;
	}
	uint8_t* state = malloc(size ? size : 1);
	if (!state) {
		return 0;
	}
	if (!readExact(state, size)) {
		free(state);
		return 0;
	}
	uint32_t expectedCRC = size >= PLAYER_STATE_HEADER ? read32(state + 24) : 0;
	if (size >= PLAYER_STATE_HEADER) {
		store32(state + 24, 0);
	}
	if (size < PLAYER_STATE_HEADER || memcmp(state, "MTSS", 4) ||
	    read32(state + 4) != 4 || read32(state + 8) != romCRC ||
	    read32(state + 20) != size - PLAYER_STATE_HEADER ||
	    read32(state + 16) > 0x3FF ||
	    read32(state + 32) > 8 || read32(state + 36) > 2047 ||
	    read32(state + 40) > 63 || read32(state + 52) > 1 ||
	    read32(state + 1080) != sizeof(color_t) || read32(state + 1084) != sizeof(capture->video) ||
	    expectedCRC != payloadCRC(state, size)) {
		free(state);
		return writeFailure("invalid checkpoint or ROM mismatch");
	}
	struct PlayerStateExtras extras = {0};
	if (!checkpointExtras(core, state + PLAYER_STATE_HEADER, size - PLAYER_STATE_HEADER, &extras)) {
		free(state);
		return writeFailure("invalid native checkpoint data");
	}
	struct mRTCGenericState rtc;
	memcpy(&rtc, extras.rtc.data, sizeof(rtc));
	extras.rtc.data = &rtc;
	int loaded = core->loadState(core, state + PLAYER_STATE_HEADER);
	if (loaded) {
		/* Savedata is masked into private memory, never written to a user's
 * cartridge save. Apply it and RTC only after a successful machine load. */
		if ((extras.savedata.data && !core->savedataRestore(core, extras.savedata.data, (size_t) extras.savedata.size, false)) ||
		    !core->rtc.d.deserialize || !core->rtc.d.deserialize(&core->rtc.d, &extras.rtc)) {
			free(state);
			writeFailure("cannot restore checkpoint peripherals; restart emulator");
			return 0;
		}
		*frameNumber = read32(state + 12);
		core->setKeys(core, read32(state + 16));
		/* mGBA's state does not contain the frontend resampler history. Start
 * every restored run with an identical empty filter, never stale audio. */
		core->setAudioBufferSize(core, core->opts.audioBuffers);
		struct GBA* gba = core->board;
		/* The core restores PSG state before replaying four frequency-register
 * writes. Those writes sample audio and can advance loaded oscillators using
 * the previous run's mixer timestamp. Reapply only the serialized PSG state
 * after register replay, without executing any guest instruction. */
		const struct GBASerializedState* serialized = (const struct GBASerializedState*) (state + PLAYER_STATE_HEADER);
		mTimingDeschedule(gba->audio.psg.timing, &gba->audio.psg.frameEvent);
		GBAudioPSGDeserialize(&gba->audio.psg, &serialized->audio.psg, &serialized->audio.flags);
		gba->audio.psg.ch1.sample = (int8_t) state[28];
		gba->audio.psg.ch2.sample = (int8_t) state[29];
		gba->audio.psg.ch3.sample = (int8_t) state[30];
		gba->audio.psg.ch4.sample = (int8_t) state[31];
		gba->audio.psg.ch1.sweep.step = (int32_t) read32(state + 32);
		gba->audio.psg.ch1.control.frequency = (int32_t) read32(state + 36);
		gba->audio.psg.ch3.window = (int32_t) read32(state + 40);
		gba->audio.psg.ch4.nSamples = (int32_t) read32(state + 44);
		gba->audio.psg.ch4.samples = (int32_t) read32(state + 48);
		memcpy(capture->video, state + 1088, sizeof(capture->video));
		core->putPixels(core, capture->video, PLAYER_WIDTH);
		gba->audio.lastLeft = 0;
		gba->audio.lastRight = 0;
		configureAudioRate(core);
		capture->audioFrames = 0;
		capture->overflow = 0;
	}
	free(state);
	return loaded ? writeReply(0, NULL, 0) : writeFailure("mGBA rejected checkpoint");
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
	/* Multiboot images have no mapped ROM checksum. Bind checkpoints to the
	 * supplied file bytes, independently of the core's cartridge interpretation. */
	uint32_t romCRC;
	if (!sourceROMCRC(romPath, &romCRC)) {
		fprintf(stderr, "mtile-player: cannot checksum source ROM %s\n", romPath);
		goto loaded;
	}
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
		if (operation == PLAYER_SAVE_STATE || operation == PLAYER_LOAD_STATE) {
			int ok = operation == PLAYER_SAVE_STATE ? saveCheckpoint(core, &capture, frameNumber, romCRC) :
			         loadCheckpoint(core, &capture, &frameNumber, romCRC);
			if (!ok) {
				status = 1;
				break;
			}
			continue;
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
