package tts

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

// AudioStreamer manages audio chunking and signaling for TTS result output.
// It should be instantiated per plugin instance.
type AudioStreamer struct {
	flow       engine.Flow
	sampleRate int
	channels   int
	offset     int
	bufferSize int
	started    atomic.Bool
	buffer     []byte
}

// NewAudioStreamer creates a new TTS result handler.
func NewAudioStreamer(flow engine.Flow, sampleRate, channels int, duration time.Duration) *AudioStreamer {
	// sampleRate * channels * 2 * duration(Seconds)
	bufferSize := int(float64(sampleRate*channels*2) * duration.Seconds())
	return &AudioStreamer{
		sampleRate: sampleRate,
		channels:   channels,
		flow:       flow,
		bufferSize: bufferSize,
		buffer:     make([]byte, bufferSize),
	}
}

// Write transforms raw audio bytes into fixed-size Audio schemas and emits them.
// It also handles AgentSpeechStart and AgentSpeechEnd signals automatically.
func (s *AudioStreamer) Write(ctx context.Context, data []byte, isLast bool) {
	if !s.started.Load() && ctx.Err() == nil {
		s.flow.SendSignal(schema.NewSignal(schema.SignalAgentSpeechStart).ReadOnly())
		s.started.Store(true)
	}
	offset, size := 0, len(data)
	for ctx.Err() == nil && size > offset {
		if size-offset < s.bufferSize || s.offset != 0 {
			read := copy(s.buffer[s.offset:], data[offset:])
			s.offset += read
			offset += read
			if s.offset == s.bufferSize {
				s.sendAudioStream(s.buffer)
				s.offset = 0
			}
			continue
		}
		end := min(size, offset+s.bufferSize)
		s.sendAudioStream(data[offset:end])
		offset = end
	}

	if isLast && ctx.Err() == nil {
		if s.offset > 0 {
			s.sendAudioStream(s.buffer[:s.offset])
			s.offset = 0
		}
		s.flow.SendSignal(schema.NewSignal(schema.SignalAgentSpeechEnd).ReadOnly())
		s.started.Store(false)
	}
}

// Reset clears the internal state of the handler.
// Typically called when an Interrupter signal is received.
func (s *AudioStreamer) Reset() {
	s.started.Store(false)
	s.offset = 0
}

func (s *AudioStreamer) sendAudioStream(audio []byte) {
	au := schema.NewAudio(schema.AudioTTS, s.sampleRate, s.channels)
	defer au.Release()
	au.SetBytes(audio)
	s.flow.SendAudio(au.ReadOnly())
}

// ParsePayload is a helper to extract standard TTS fields from a generic payload.
func ParsePayload(data schema.Payload) (text, emotion string, final bool, ok bool) {
	final = schema.GetAs[bool](data, "is_final", false)
	text = schema.GetAs[string](data, "sentence", "")
	emotion = schema.GetAs[string](data, "emotion", "")

	if (text == "" || strings.TrimSpace(text) == "") && !final {
		return "", "", false, false
	}
	return text, emotion, final, true
}
