package engine

import (
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
)

// MockFlow implements the Flow interface with programmable hooks for testing purposes.
type MockFlow struct {
	onActivity func()

	OnSignalHook      func(port int, signal schema.Signal)
	OnPayloadHook     func(port int, payload schema.Payload)
	OnAudioHook       func(port int, audio schema.Audio)
	OnVideoHook       func(port int, video schema.Video)
	OnPublishHook     func(mt protocol.PacketType, payload []byte)
	OnPublishFullHook func(mt protocol.PacketType, encode protocol.PacketEncode, payload []byte)
}

func (m *MockFlow) ping() {
	if m.onActivity != nil {
		m.onActivity()
	}
}

func (m *MockFlow) Publish(mt protocol.PacketType, data []byte) {
	m.ping()
	if m.OnPublishHook != nil {
		m.OnPublishHook(mt, data)
	}
}

func (m *MockFlow) PublishFull(mt protocol.PacketType, encode protocol.PacketEncode, data []byte) {
	m.ping()
	if m.OnPublishFullHook != nil {
		m.OnPublishFullHook(mt, encode, data)
	}
}

func (m *MockFlow) SendSignal(v schema.Signal) { m.SendSignalToPort(0, v) }
func (m *MockFlow) SendSignalToPort(port int, v schema.Signal) {
	m.ping()
	if m.OnSignalHook != nil {
		m.OnSignalHook(port, v)
	}
}

func (m *MockFlow) SendPayload(v schema.Payload) { m.SendPayloadToPort(0, v) }
func (m *MockFlow) SendPayloadToPort(port int, v schema.Payload) {
	m.ping()
	if m.OnPayloadHook != nil {
		m.OnPayloadHook(port, v)
	}
}

func (m *MockFlow) SendAudio(v schema.Audio) { m.SendAudioToPort(0, v) }
func (m *MockFlow) SendAudioToPort(port int, v schema.Audio) {
	m.ping()
	v.Retain()
	defer v.Release()
	if m.OnAudioHook != nil {
		m.OnAudioHook(port, v)
	}
}

func (m *MockFlow) SendVideo(v schema.Video) { m.SendVideoToPort(0, v) }
func (m *MockFlow) SendVideoToPort(port int, v schema.Video) {
	m.ping()
	v.Retain()
	defer v.Release()
	if m.OnVideoHook != nil {
		m.OnVideoHook(port, v)
	}
}
