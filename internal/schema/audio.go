package schema

import (
	"sync/atomic"
	"time"

	"github.com/wnnce/voce/pkg/pool"
)

const (
	audioRecycleCap   = 64 * 1024
	entriesRecycleCap = 256
)

const (
	AudioInput = "input"
	AudioTTS   = "tts"
)

var (
	activeAudioCount atomic.Int64
	// audioPool reduces GC pressure by recycling large audio buffers.
	audioPool = pool.NewTypedPool[*builtinAudio](func() *builtinAudio {
		return &builtinAudio{
			builtinProperties: builtinProperties{
				entries: make([]entry, 0),
			},
		}
	})
)

func LoadActiveAudioCount() int64 {
	return activeAudioCount.Load()
}

// AudioView is the shared read-only base for audio objects.
// It combines View (property access + name) with audio-specific read methods and reference counting.
type AudioView interface {
	View
	RefCountable
	Bytes() []byte
	Duration() time.Duration
	SampleRate() int
	Channels() int
}

// Audio is the read-only interface for audio data.
// It leverages an internal pool (`audioPool`) to optimize memory allocation and reduce GC pressure.
type Audio interface {
	AudioView
	Mutable() MutableAudio // Upgrades to Writable, potentially cloning the data
}

// MutableAudio is the writable interface for audio data.
// It embeds AudioView directly (NOT Audio), so MutableAudio does NOT satisfy Audio.
// Callers must explicitly call ReadOnly() to get an Audio that can be passed to SendAudio.
type MutableAudio interface {
	AudioView
	Properties
	SetBytes(buf []byte)
	SetDuration(d time.Duration)
	SetSampleRate(sr int)
	SetChannels(c int)
	ReadOnly() Audio
}

type builtinAudio struct {
	builtinProperties
	name       string
	buffer     []byte
	sampleRate int
	channels   int
	duration   time.Duration
	count      atomic.Int32
}

func NewAudio(name string, sampleRate, channels int) MutableAudio {
	activeAudioCount.Add(1)
	val := audioPool.Acquire()
	val.name = name
	val.sampleRate = sampleRate
	val.channels = channels
	val.count.Store(1)
	return val
}

func (b *builtinAudio) Retain() {
	b.count.Add(1)
}

func (b *builtinAudio) Release() {
	if b.count.Add(-1) > 0 {
		return
	}
	audioPool.Release(b)
}

func (b *builtinAudio) Recycle() {
	activeAudioCount.Add(-1)
	b.resetReadOnly()
	if cap(b.builtinProperties.entries) >= entriesRecycleCap {
		b.builtinProperties.entries = make([]entry, 0)
	} else {
		b.builtinProperties.entries = b.builtinProperties.entries[:0]
	}

	b.name = ""
	if cap(b.buffer) > audioRecycleCap {
		b.buffer = nil
	} else {
		b.buffer = b.buffer[:0]
	}
	b.sampleRate = 0
	b.channels = 0
	b.duration = 0
	b.count.Store(0)
}

func (b *builtinAudio) Name() string {
	return b.name
}

func (b *builtinAudio) Bytes() []byte {
	return b.buffer
}

func (b *builtinAudio) Duration() time.Duration {
	return b.duration
}

func (b *builtinAudio) SampleRate() int {
	return b.sampleRate
}

func (b *builtinAudio) Channels() int {
	return b.channels
}

func (b *builtinAudio) SetBytes(buf []byte) {
	b.checkReadOnly("schema: call SetBytes on readonly audio")
	size := len(buf)
	if cap(b.buffer) < size {
		dst := make([]byte, len(buf))
		copy(dst, buf)
		b.buffer = dst
	} else {
		b.buffer = b.buffer[:size]
		copy(b.buffer, buf)
	}
}

func (b *builtinAudio) SetDuration(d time.Duration) {
	b.checkReadOnly("schema: call SetDuration on readonly audio")
	b.duration = d
}

func (b *builtinAudio) SetSampleRate(sr int) {
	b.checkReadOnly("schema: call SetSampleRate on readonly audio")
	b.sampleRate = sr
}

func (b *builtinAudio) SetChannels(c int) {
	b.checkReadOnly("schema: call SetChannels on readonly audio")
	b.channels = c
}

func (b *builtinAudio) ReadOnly() Audio {
	b.setReadOnly()
	return b
}

func (b *builtinAudio) Mutable() MutableAudio {
	if b.isReadOnly() {
		newAudio := NewAudio(b.Name(), b.sampleRate, b.channels).(*builtinAudio)
		b.builtinProperties.copyTo(&newAudio.builtinProperties)
		newAudio.SetBytes(b.buffer)
		newAudio.duration = b.duration
		return newAudio
	}
	return b
}
