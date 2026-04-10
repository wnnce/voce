package audioproc

/*
#cgo pkg-config: libswresample libavutil
#include <libswresample/swresample.h>
#include <libavutil/opt.h>
#include <libavutil/channel_layout.h>
#include <libavutil/samplefmt.h>
#include <stdint.h>

// Helper to get error string
static inline const char* av_err_str(int err) {
    static char buf[1024];
    av_strerror(err, buf, sizeof(buf));
    return buf;
}

static inline int swr_convert_wrapped(struct SwrContext *s, uint8_t *out, int out_count, const uint8_t *in, int in_count) {
    uint8_t *out_arr[1] = { out };
    const uint8_t *in_arr[1] = { in };
    return swr_convert(s, out_arr, out_count, (const uint8_t **)in_arr, in_count);
}
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
)

var (
	// ErrInitFailed is returned when FFmpeg swr context allocation or initialization fails.
	ErrInitFailed = errors.New("audioproc: failed to initialize resampler")

	// ErrResampleFailed is returned when the resampling operation fails.
	ErrResampleFailed = errors.New("audioproc: resample operation failed")
)

// Resampler is a high-performance PCM audio resampler using FFmpeg's libswresample.
// It maintains an internal sticky buffer to minimize allocations.
type Resampler struct {
	swrCtx *C.struct_SwrContext

	// Configuration
	srcRate     int
	srcChannels int
	dstRate     int
	dstChannels int

	// Internal sticky buffer for memory reuse.
	// The caller MUST copy the returned slice if they need to hold it beyond the next Resample call.
	outBuf []byte
}

// NewResampler creates a new Resampler for PCM S16 format.
func NewResampler(srcRate, srcCh, dstRate, dstCh int) (*Resampler, error) {
	var swr *C.struct_SwrContext

	// Define channel layouts (modern API)
	var srcLayout, dstLayout C.struct_AVChannelLayout
	C.av_channel_layout_default(&srcLayout, C.int(srcCh))
	C.av_channel_layout_default(&dstLayout, C.int(dstCh))

	ret := C.swr_alloc_set_opts2(
		&swr,
		&dstLayout,
		C.AV_SAMPLE_FMT_S16,
		C.int(dstRate),
		&srcLayout,
		C.AV_SAMPLE_FMT_S16,
		C.int(srcRate),
		0, nil,
	)

	if ret < 0 {
		return nil, fmt.Errorf("%w: failed to allocate swr context: %s", ErrInitFailed, C.GoString(C.av_err_str(ret)))
	}

	if ret = C.swr_init(swr); ret < 0 {
		C.swr_free(&swr)
		return nil, fmt.Errorf("%w: failed to initialize swr context: %s", ErrInitFailed, C.GoString(C.av_err_str(ret)))
	}

	return &Resampler{
		swrCtx:      swr,
		srcRate:     srcRate,
		srcChannels: srcCh,
		dstRate:     dstRate,
		dstChannels: dstCh,
	}, nil
}

// Resample converts the input PCM S16 data to the destination format.
// It returns a slice that points to the internal buffer, which is valid only until the next Resample call.
func (r *Resampler) Resample(in []byte) ([]byte, error) {
	// Bytes per sample for S16 is 2
	srcNbSamples := C.int(0)
	if len(in) > 0 {
		srcNbSamples = C.int(len(in) / (r.srcChannels * 2))
	}

	// Internal delay in source sample units
	delay := C.swr_get_delay(r.swrCtx, C.int64_t(r.srcRate))

	// If no input and no delay, avoid unnecessary work
	if srcNbSamples == 0 && delay == 0 {
		return nil, nil
	}

	// Estimate max output samples including internal delay
	maxDstNbSamples := C.av_rescale_rnd(
		delay+C.int64_t(srcNbSamples),
		C.int64_t(r.dstRate),
		C.int64_t(r.srcRate),
		C.AV_ROUND_UP,
	)

	// Calculate required output buffer size (bytes)
	requiredSize := int(maxDstNbSamples) * r.dstChannels * 2

	// Grow buffer if needed
	if cap(r.outBuf) < requiredSize {
		r.outBuf = make([]byte, requiredSize)
	}
	r.outBuf = r.outBuf[:requiredSize]

	var inPtr *C.uint8_t
	if len(in) > 0 {
		inPtr = (*C.uint8_t)(unsafe.Pointer(&in[0]))
	}

	var outPtr *C.uint8_t
	if len(r.outBuf) > 0 {
		outPtr = (*C.uint8_t)(unsafe.Pointer(&r.outBuf[0]))
	}

	ret := C.swr_convert_wrapped(
		r.swrCtx,
		outPtr,
		C.int(maxDstNbSamples),
		inPtr,
		srcNbSamples,
	)

	if ret < 0 {
		return nil, fmt.Errorf("%w: %s", ErrResampleFailed, C.GoString(C.av_err_str(ret)))
	}

	actualSize := int(ret) * r.dstChannels * 2
	if actualSize == 0 {
		return nil, nil
	}
	return r.outBuf[:actualSize], nil
}

// Flush retrieves any remaining samples buffered within the resampler.
func (r *Resampler) Flush() ([]byte, error) {
	return r.Resample(nil)
}

// Close releases the underlying FFmpeg resources.
func (r *Resampler) Close() {
	if r.swrCtx != nil {
		C.swr_free(&r.swrCtx)
	}
}
