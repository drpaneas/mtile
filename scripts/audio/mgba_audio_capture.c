#include <errno.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <mgba/core/config.h>
#include <mgba/core/core.h>
#include <mgba/core/interface.h>

struct Capture {
	struct mAVStream stream;
	int16_t *samples;
	color_t video[240 * 160];
	size_t frames;
	size_t capacity;
	unsigned rate;
	int hasVideo;
	int failed;
};

static struct Capture *captureFromStream(struct mAVStream *stream) {
	return (struct Capture *)stream;
}

static void captureRate(struct mAVStream *stream, unsigned rate) {
	fprintf(stderr, "capture: audio rate changed to %u Hz\n", rate);
	captureFromStream(stream)->rate = rate;
}

static void captureAudio(struct mAVStream *stream, int16_t left, int16_t right) {
	struct Capture *capture = captureFromStream(stream);
	if (capture->failed) {
		return;
	}
	if (capture->frames == capture->capacity) {
		size_t capacity = capture->capacity ? capture->capacity * 2 : 131072;
		int16_t *samples = realloc(capture->samples, capacity * 2 * sizeof(*samples));
		if (!samples) {
			capture->failed = 1;
			return;
		}
		capture->samples = samples;
		capture->capacity = capacity;
	}
	capture->samples[capture->frames * 2] = left;
	capture->samples[capture->frames * 2 + 1] = right;
	++capture->frames;
}

static void captureVideo(struct mAVStream *stream, const color_t *buffer, size_t stride) {
	struct Capture *capture = captureFromStream(stream);
	if (!buffer || stride < 240 || stride > 4096) {
		return;
	}
	if (!capture->hasVideo) {
		fprintf(stderr, "capture: video stride is %zu pixels\n", stride);
	}
	for (size_t row = 0; row < 160; ++row) {
		memcpy(&capture->video[row * 240], &buffer[row * stride], 240 * sizeof(color_t));
	}
	capture->hasVideo = 1;
}

static int writeBytes(FILE *file, const void *bytes, size_t size) {
	return fwrite(bytes, 1, size, file) == size;
}

static int write16(FILE *file, uint16_t value) {
	uint8_t bytes[2] = {value & 0xff, value >> 8};
	return writeBytes(file, bytes, sizeof(bytes));
}

static int write32(FILE *file, uint32_t value) {
	uint8_t bytes[4] = {value & 0xff, value >> 8, value >> 16, value >> 24};
	return writeBytes(file, bytes, sizeof(bytes));
}

static int writeWAV(const char *path, const struct Capture *capture) {
	if (!capture->rate || capture->frames > (UINT32_MAX - 36) / 4) {
		return 0;
	}
	FILE *file = fopen(path, "wb");
	if (!file) {
		fprintf(stderr, "capture: open %s: %s\n", path, strerror(errno));
		return 0;
	}
	uint32_t dataSize = (uint32_t)capture->frames * 4;
	int ok = writeBytes(file, "RIFF", 4) && write32(file, 36 + dataSize) &&
	         writeBytes(file, "WAVEfmt ", 8) && write32(file, 16) &&
	         write16(file, 1) && write16(file, 2) && write32(file, capture->rate) &&
	         write32(file, capture->rate * 4) && write16(file, 4) && write16(file, 16) &&
	         writeBytes(file, "data", 4) && write32(file, dataSize) &&
	         fwrite(capture->samples, 4, capture->frames, file) == capture->frames;
	if (!ok) {
		fprintf(stderr, "capture: write %s: %s\n", path, strerror(errno));
	}
	if (fclose(file)) {
		fprintf(stderr, "capture: close %s: %s\n", path, strerror(errno));
		ok = 0;
	}
	return ok;
}

static int writePPM(const char *path, const struct Capture *capture) {
	if (!path || !*path || !capture->hasVideo) {
		return 0;
	}
	FILE *file = fopen(path, "wb");
	if (!file) {
		fprintf(stderr, "capture: open screenshot %s: %s\n", path, strerror(errno));
		return 0;
	}
	int ok = fprintf(file, "P6\n240 160\n255\n") > 0;
	for (size_t index = 0; ok && index < 240 * 160; ++index) {
		color_t color = capture->video[index];
#ifdef COLOR_16_BIT
		uint8_t pixel[3] = {M_R8(color), M_G8(color), M_B8(color)};
#else
		uint8_t pixel[3] = {color & 0xff, (color >> 8) & 0xff, (color >> 16) & 0xff};
#endif
		ok = fwrite(pixel, 1, sizeof(pixel), file) == sizeof(pixel);
	}
	if (fclose(file)) {
		ok = 0;
	}
	return ok;
}

static unsigned captureFrameCount(void) {
	const char *text = getenv("MGBA_CAPTURE_FRAMES");
	if (!text || !*text) {
		return 120;
	}
	char *end = NULL;
	unsigned long value = strtoul(text, &end, 10);
	if (!value || *end || value > 36000) {
		fprintf(stderr, "capture: invalid MGBA_CAPTURE_FRAMES=%s\n", text);
		return 0;
	}
	return (unsigned)value;
}

__attribute__((constructor)) static void captureROM(void) {
	const char *rom = getenv("MGBA_CAPTURE_ROM");
	const char *output = getenv("MGBA_CAPTURE_OUTPUT");
	if (!rom || !output) {
		return;
	}
	unsigned frameCount = captureFrameCount();
	if (!frameCount) {
		_exit(1);
	}
	fprintf(stderr, "capture: starting mGBA core for %s\n", rom);

	int status = 1;
	struct mCore *core = mCoreFind(rom);
	if (!core) {
		fprintf(stderr, "capture: mGBA could not identify %s\n", rom);
		_exit(status);
	}
	mCoreInitConfig(core, "mtile-capture");
	struct mCoreOptions options = {
	    .skipBios = true,
	    .audioBuffers = 1024,
	    .sampleRate = 44100,
	    .fpsTarget = 60,
	    .volume = 0x100,
	};
	mCoreConfigLoadDefaults(&core->config, &options);
	if (!core->init(core)) {
		fprintf(stderr, "capture: mGBA core initialization failed\n");
		goto config;
	}
	if (!mCoreLoadFile(core, rom)) {
		fprintf(stderr, "capture: mGBA could not load %s\n", rom);
		goto initialized;
	}
	mCoreConfigMap(&core->config, &core->opts);
	core->setAudioBufferSize(core, core->opts.audioBuffers);
	core->loadConfig(core, &core->config);

	struct Capture capture = {
	    .stream = {
	        .audioRateChanged = captureRate,
	        .postVideoFrame = captureVideo,
	        .postAudioFrame = captureAudio,
	    },
	    .rate = 32768,
	};
	core->setVideoBuffer(core, capture.video, 240);
	capture.hasVideo = 1;
	core->reset(core);
	core->setAVStream(core, &capture.stream);
	for (unsigned frame = 0; frame < frameCount && !capture.failed; ++frame) {
		core->runFrame(core);
	}
	core->setAVStream(core, NULL);
	fprintf(stderr, "capture: collected %zu frames at %u Hz (failed=%d)\n",
	        capture.frames, capture.rate, capture.failed);

	if (capture.failed) {
		fprintf(stderr, "capture: sample allocation failed\n");
	} else if (writeWAV(output, &capture)) {
		fprintf(stderr, "capture: wrote %zu stereo frames at %u Hz to %s\n",
		        capture.frames, capture.rate, output);
		status = 0;
	}
	const char *screenshot = getenv("MGBA_CAPTURE_SCREENSHOT");
	if (screenshot && *screenshot) {
		if (writePPM(screenshot, &capture)) {
			fprintf(stderr, "capture: wrote final video frame to %s\n", screenshot);
		} else {
			fprintf(stderr, "capture: failed to write final video frame to %s\n", screenshot);
			status = 1;
		}
	}
	free(capture.samples);

initialized:
	mCoreConfigDeinit(&core->config);
	core->deinit(core);
	_exit(status);
config:
	mCoreConfigDeinit(&core->config);
	_exit(status);
}
