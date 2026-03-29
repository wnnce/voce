package schema

import (
	"sync/atomic"
	"time"

	"github.com/wnnce/voce/pkg/pool"
)

var (
	activeSDVideoCount  atomic.Int64
	activeHDVideoCount  atomic.Int64
	activeFHDVideoCount atomic.Int64
)

const (
	sdVideoPixels  = 640 * 480
	hdVideoPixels  = 1280 * 720
	fhdVideoPixels = 1920 * 1080
)

var (
	// Multilevel pools for video frames to match different resolution requirements
	// while minimizing runtime allocations.
	sdVideoPool = pool.NewTypedPool[*builtinVideo](func() *builtinVideo {
		return &builtinVideo{
			builtinProperties: builtinProperties{
				entries: make([]entry, 0),
			},
			data: make([]byte, sdVideoPixels*3/2),
		}
	})
	hdVideoPool = pool.NewTypedPool[*builtinVideo](func() *builtinVideo {
		return &builtinVideo{
			builtinProperties: builtinProperties{
				entries: make([]entry, 0),
			},
			data: make([]byte, hdVideoPixels*3/2),
		}
	})
	fhdVideoPool = pool.NewTypedPool[*builtinVideo](func() *builtinVideo {
		return &builtinVideo{
			builtinProperties: builtinProperties{
				entries: make([]entry, 0),
			},
			data: make([]byte, fhdVideoPixels*3/2),
		}
	})
)

func LoadActiveSDVideoCount() int64 {
	return activeSDVideoCount.Load()
}

func LoadActiveHDVideoCount() int64 {
	return activeHDVideoCount.Load()
}

func LoadActiveFHDVideoCount() int64 {
	return activeFHDVideoCount.Load()
}

// VideoView is the shared read-only base for video objects.
// It combines View (property access + name) with video-specific read methods and reference counting.
type VideoView interface {
	View
	RefCountable
	Height() int
	Width() int
	YBytes() []byte
	UBytes() []byte
	VBytes() []byte
	Duration() time.Duration
}

// Video is the read-only interface for a video frame, supporting reference counting.
type Video interface {
	VideoView
	Mutable() MutableVideo // Upgrades to Writable, potentially cloning the data
}

// MutableVideo is the writable interface for video data.
// It embeds VideoView directly (NOT Video), so MutableVideo does NOT satisfy Video.
// Callers must explicitly call ReadOnly() to get a Video that can be passed to SendVideo.
type MutableVideo interface {
	VideoView
	Properties
	SetYUV(y, u, v []byte)
	SetDuration(d time.Duration)
	ReadOnly() Video
}

type builtinVideo struct {
	builtinProperties
	name     string
	height   int
	width    int
	data     []byte
	duration time.Duration
	count    atomic.Int32
}

func NewVideo(name string, height, width int, duration time.Duration) MutableVideo {
	frame := acquireVideo(height, width)
	frame.name = name
	frame.height = height
	frame.width = width
	frame.duration = duration
	frame.count.Store(1)
	return frame
}

func (b *builtinVideo) Recycle() {
	if cap(b.builtinProperties.entries) >= entriesRecycleCap {
		b.builtinProperties.entries = make([]entry, 0)
	} else {
		b.builtinProperties.entries = b.builtinProperties.entries[:0]
	}
	b.resetReadOnly()
	b.name = ""
	b.height = 0
	b.width = 0
	b.count.Store(0)
	b.duration = 0
	b.data = b.data[:0]
}

func (b *builtinVideo) Retain() {
	b.count.Add(1)
}

func (b *builtinVideo) Release() {
	if b.count.Add(-1) > 0 {
		return
	}
	releaseVideo(b)
}

func (b *builtinVideo) Name() string {
	return b.name
}

func (b *builtinVideo) Height() int {
	return b.height
}

func (b *builtinVideo) Width() int {
	return b.width
}

func (b *builtinVideo) YBytes() []byte {
	size := b.width * b.height
	if len(b.data) < size {
		return nil
	}
	return b.data[:size]
}

func (b *builtinVideo) UBytes() []byte {
	pixels := b.width * b.height
	size := pixels / 4
	if len(b.data) < pixels+size {
		return nil
	}
	return b.data[pixels : pixels+size]
}

func (b *builtinVideo) VBytes() []byte {
	pixels := b.width * b.height
	size := pixels / 4
	if len(b.data) < pixels+size*2 {
		return nil
	}
	return b.data[pixels+size : pixels+size*2]
}

func (b *builtinVideo) Duration() time.Duration {
	return b.duration
}

func (b *builtinVideo) Mutable() MutableVideo {
	if b.isReadOnly() {
		frame := acquireVideo(b.height, b.width)
		frame.name = b.name
		frame.height = b.height
		frame.width = b.width
		frame.duration = b.duration
		b.builtinProperties.copyTo(&frame.builtinProperties)
		frame.setData(b.data)
		frame.count.Store(1)
		return frame
	}
	return b
}

func (b *builtinVideo) setData(src []byte) {
	size := len(src)
	if cap(b.data) < size {
		b.data = make([]byte, size)
	} else {
		b.data = b.data[:size]
	}
	copy(b.data, src)
}

func (b *builtinVideo) SetYUV(y, u, v []byte) {
	b.checkReadOnly("schema: call SetYUV on readonly video")
	ySize, uSize, vSize := len(y), len(u), len(v)
	total := ySize + uSize + vSize
	if cap(b.data) < total {
		b.data = make([]byte, total)
	} else {
		b.data = b.data[:total]
	}
	copy(b.data[0:ySize], y)
	copy(b.data[ySize:ySize+uSize], u)
	copy(b.data[ySize+uSize:total], v)
}

func (b *builtinVideo) SetDuration(d time.Duration) {
	b.checkReadOnly("schema: call SetDuration on readonly video")
	b.duration = d
}

func (b *builtinVideo) ReadOnly() Video {
	b.setReadOnly()
	return b
}

func acquireVideo(height, width int) *builtinVideo {
	pixels := height * width
	if pixels <= sdVideoPixels {
		activeSDVideoCount.Add(1)
		return sdVideoPool.Acquire()
	} else if pixels <= hdVideoPixels {
		activeHDVideoCount.Add(1)
		return hdVideoPool.Acquire()
	}
	activeFHDVideoCount.Add(1)
	return fhdVideoPool.Acquire()
}

func releaseVideo(frame *builtinVideo) {
	pixels := frame.width * frame.height
	if pixels <= sdVideoPixels {
		activeSDVideoCount.Add(-1)
		sdVideoPool.Release(frame)
	} else if pixels <= hdVideoPixels {
		activeHDVideoCount.Add(-1)
		hdVideoPool.Release(frame)
	} else {
		activeFHDVideoCount.Add(-1)
		fhdVideoPool.Release(frame)
	}
}
