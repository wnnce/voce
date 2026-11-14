package webrtc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/rtc/webrtc/media"
	"github.com/wnnce/voce/pkg/buf"
)

func TestDecodeLoop_ConsumesAndRecycles(t *testing.T) {
	encoder, err := media.NewAudioEncoder()
	require.NoError(t, err)

	pcmData := make([]byte, media.PCMFrameSize)
	for i := range pcmData {
		pcmData[i] = byte(i % 128)
	}
	opusPacket, err := encoder.Encode(pcmData)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	binding := &Binding{
		ctx:    ctx,
		cancel: cancel,
	}
	binding.connected.Store(true)

	samples := make(chan *buf.Buffer, 5)

	done := make(chan struct{})
	go func() {
		defer close(done)
		binding.decodeLoop(ctx, samples, nil)
	}()

	// Send 5 samples (640 bytes * 5 = 3200 bytes, triggers complete frame deliver)
	for i := 0; i < 5; i++ {
		sampleBuf := audioSamplePool.Acquire()
		sampleBuf.Set(opusPacket)
		samples <- sampleBuf
	}
	close(samples)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("decodeLoop timed out")
	}
}

func TestDecodeLoop_ContextCancelDrains(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	binding := &Binding{
		ctx:    ctx,
		cancel: cancel,
	}

	samples := make(chan *buf.Buffer, 5)
	for i := 0; i < 3; i++ {
		sampleBuf := audioSamplePool.Acquire()
		sampleBuf.Set([]byte{1, 2, 3, 4})
		samples <- sampleBuf
	}
	close(samples)

	cancel() // Cancel context

	done := make(chan struct{})
	go func() {
		defer close(done)
		binding.decodeLoop(ctx, samples, nil)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("decodeLoop timed out on canceled context")
	}
}
