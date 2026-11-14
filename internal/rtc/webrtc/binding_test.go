package webrtc

import (
	"context"
	"testing"

	pionwebrtc "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/rtc/webrtc/media"
	"github.com/wnnce/voce/pkg/buf"
)

func TestBinding_WriteQueueAndReset(t *testing.T) {
	pool := buf.NewChunkPool(audioBlockSize)
	encoder, err := media.NewAudioEncoder()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	binding := &Binding{
		ctx:     ctx,
		cancel:  cancel,
		queue:   buf.NewChunkQueue(pool, 0),
		encoder: encoder,
		frame:   make([]byte, media.PCMFrameSize),
	}

	pcmData := make([]byte, 640)
	for i := range pcmData {
		pcmData[i] = byte(i % 256)
	}

	// 1. Write audio packet
	audioPacket := protocol.AcquirePacket()
	audioPacket.Type = protocol.TypeAudio
	audioPacket.SetPayload(pcmData)
	err = binding.Write(audioPacket)
	require.NoError(t, err)
	protocol.ReleasePacket(audioPacket)

	assert.Equal(t, 640, binding.queue.Len())

	// 2. Write interrupter packet -> should reset queue
	interruptPacket := protocol.AcquirePacket()
	interruptPacket.Type = protocol.TypeInterrupter
	err = binding.Write(interruptPacket)
	require.NoError(t, err)
	protocol.ReleasePacket(interruptPacket)

	assert.Equal(t, 0, binding.queue.Len())
}

func TestBinding_DisconnectGraceLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	binding := &Binding{
		ctx:    ctx,
		cancel: cancel,
		queue:  buf.NewChunkQueue(buf.NewChunkPool(audioBlockSize), 0),
	}

	binding.onConnectionStateChange(pionwebrtc.PeerConnectionStateDisconnected)
	assert.False(t, binding.connected.Load())
	assert.False(t, binding.closed.Load())
	assert.NotNil(t, binding.disconnectTimer.Load())

	binding.onConnectionStateChange(pionwebrtc.PeerConnectionStateConnected)
	assert.True(t, binding.connected.Load())
	assert.False(t, binding.closed.Load())
	assert.Nil(t, binding.disconnectTimer.Load())

	binding.onConnectionStateChange(pionwebrtc.PeerConnectionStateFailed)
	assert.False(t, binding.connected.Load())
	assert.True(t, binding.closed.Load())
}

func TestBinding_DataChannelOversizedPacketDropped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	delivered := false
	binding := &Binding{
		ctx:    ctx,
		cancel: cancel,
	}

	oversized := make([]byte, maxDataChannelPacketSize+1)
	binding.onDataChannelMessage(pionwebrtc.DataChannelMessage{Data: oversized})
	assert.False(t, delivered)
}
