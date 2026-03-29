package elevenlabs_tts

import (
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

// makePCM returns n bytes of valid 16-bit PCM samples (all zeros).
func makePCM(n int) []byte {
	// Round to even so every sample boundary is intact.
	if n%2 != 0 {
		n++
	}
	return make([]byte, n)
}

// makeConfig returns a minimal config pointing at the given test-server URL
// and injecting the supplied HTTP client.
func makeConfig(serverURL string, client *http.Client) *ElevenLabsConfig {
	return &ElevenLabsConfig{
		ApiKey:          "test-key",
		BaseURL:         serverURL,
		VoiceID:         "voice-id",
		ModelID:         "eleven_turbo_v2_5",
		Stability:       0.5,
		SimilarityBoost: 0.75,
		client:          client,
	}
}

// startPlugin creates and starts a plugin backed by a test HTTP server.
// Returns the PluginTester (already started) and a stop function.
func startPlugin(t *testing.T, handler http.HandlerFunc) (*engine.PluginTester, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	cfg := makeConfig(srv.URL, srv.Client())
	plg := NewPlugin(cfg)
	tester := engine.NewPluginTester(t, plg).Start()
	stop := func() {
		tester.Stop()
		srv.Close()
	}
	return tester, stop
}

// makePayload builds a PayloadLLMChunk with the given sentence and is_final flag.
func makePayload(sentence string, isFinal bool) schema.Payload {
	p := schema.NewPayload(schema.PayloadLLMChunk)
	_ = p.Set("sentence", sentence)
	_ = p.Set("is_final", isFinal)
	return p.ReadOnly()
}

// TestPlugin_HappyPath verifies that valid PCM bytes from the server are
// forwarded downstream as Audio frames and that the AudioStreamer emits the
// correct SpeechStart / SpeechEnd signals.
func TestPlugin_HappyPath(t *testing.T) {
	pcmData := makePCM(4096) // two full 16-bit PCM chunks

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pcmData)
	}

	var audioBytes []byte
	var signals []string

	tester, stop := startPlugin(t, handler)
	defer stop()

	tester.
		OnAudio(func(_ int, a schema.Audio) {
			audioBytes = append(audioBytes, a.Bytes()...)
		}).
		OnSignal(func(_ int, s schema.Signal) {
			signals = append(signals, s.Name())
		})

	tester.InjectPayload(makePayload("Hello world", true))

	// MultiTrackPlugin runs OnPayload in a goroutine; give it time to finish.
	time.Sleep(200 * time.Millisecond)

	assert.NotEmpty(t, audioBytes, "audio frames should have been sent downstream")
	assert.Contains(t, signals, schema.SignalAgentSpeechStart, "SpeechStart signal expected")
	assert.Contains(t, signals, schema.SignalAgentSpeechEnd, "SpeechEnd signal expected (from final=true)")
}

// TestPlugin_APIError verifies that a non-200 response does not produce any
// audio output, so downstream nodes do not receive garbage data.
func TestPlugin_APIError(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"invalid_api_key"}`, http.StatusUnauthorized)
	}

	var audioCalled atomic.Bool

	tester, stop := startPlugin(t, handler)
	defer stop()

	tester.OnAudio(func(_ int, _ schema.Audio) {
		audioCalled.Store(true)
	})

	tester.InjectPayload(makePayload("Hello", true))
	time.Sleep(150 * time.Millisecond)

	assert.False(t, audioCalled.Load(), "no audio should be sent on API error")
}

// TestPlugin_EmptyTextSkipped verifies that empty sentence payloads (with
// is_final=false) do not trigger an HTTP call.
func TestPlugin_EmptyTextSkipped(t *testing.T) {
	var requestCount atomic.Int32
	handler := func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}

	tester, stop := startPlugin(t, handler)
	defer stop()

	// Empty sentence + not final → ParsePayload returns ok=false → no HTTP call.
	tester.InjectPayload(makePayload("", false))
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(0), requestCount.Load(), "no request should be made for empty non-final payload")
}

// TestPlugin_PreviousTextAccumulation verifies that each successive request
// carries the accumulated text from previous chunks via the previous_text field.
func TestPlugin_PreviousTextAccumulation(t *testing.T) {
	var bodies []string
	handler := func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makePCM(64))
	}

	tester, stop := startPlugin(t, handler)
	defer stop()

	// First chunk: no previous text.
	tester.InjectPayload(makePayload("Hello, ", false))
	time.Sleep(150 * time.Millisecond)

	// Second chunk: "Hello, " should appear in previous_text.
	tester.InjectPayload(makePayload("world!", true))
	time.Sleep(150 * time.Millisecond)

	require.Len(t, bodies, 2, "expected two HTTP requests")
	assert.NotContains(t, bodies[0], "previous_text", "first request must not have previous_text field")
	assert.Contains(t, bodies[1], "Hello, ", "second request should carry first chunk as previous_text")
}

// TestPlugin_PreviousTextResetOnFinal verifies that the previousText buffer is
// cleared after a payload with is_final=true, so the next utterance starts fresh.
func TestPlugin_PreviousTextResetOnFinal(t *testing.T) {
	var bodies []string
	handler := func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makePCM(64))
	}

	tester, stop := startPlugin(t, handler)
	defer stop()

	// Utterance 1.
	tester.InjectPayload(makePayload("First sentence.", true))
	time.Sleep(150 * time.Millisecond)

	// Utterance 2 — buffer must have been reset.
	tester.InjectPayload(makePayload("Second sentence.", false))
	time.Sleep(150 * time.Millisecond)

	require.Len(t, bodies, 2)
	assert.NotContains(t, bodies[1], "First sentence",
		"previous_text buffer should have been cleared after final=true")
}

// TestPlugin_InterruptResetsBuffer verifies that the SignalInterrupter clears
// the previousText buffer so the next request starts without stale context.
func TestPlugin_InterruptResetsBuffer(t *testing.T) {
	var bodies []string
	handler := func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makePCM(64))
	}

	tester, stop := startPlugin(t, handler)
	defer stop()

	// Send a chunk to populate previousText.
	tester.InjectPayload(makePayload("Before interrupt.", false))
	time.Sleep(150 * time.Millisecond)

	// Fire interrupt signal — OnSignal is synchronous, so this executes before
	// the next InjectPayload.
	tester.InjectSignal(schema.NewSignal(schema.SignalInterrupter).ReadOnly())

	// Next chunk should NOT carry the stale previous_text.
	tester.InjectPayload(makePayload("After interrupt.", false))
	time.Sleep(150 * time.Millisecond)

	require.GreaterOrEqual(t, len(bodies), 2)
	last := bodies[len(bodies)-1]
	assert.NotContains(t, last, "Before interrupt",
		"previous_text should be empty after interrupt")
}

// TestPlugin_RequestHeaders verifies that the xi-api-key header and
// Content-Type are set correctly on every request.
func TestPlugin_RequestHeaders(t *testing.T) {
	var capturedKey, capturedCT string

	handler := func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("xi-api-key")
		capturedCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makePCM(64))
	}

	tester, stop := startPlugin(t, handler)
	defer stop()

	tester.InjectPayload(makePayload("Test", true))
	time.Sleep(150 * time.Millisecond)

	assert.Equal(t, "test-key", capturedKey, "xi-api-key header must match config")
	assert.Equal(t, "application/json", capturedCT, "Content-Type must be application/json")
}

// TestPlugin_RequestPath verifies the URL path contains the voice_id.
func TestPlugin_RequestPath(t *testing.T) {
	var capturedPath string

	handler := func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makePCM(64))
	}

	tester, stop := startPlugin(t, handler)
	defer stop()

	tester.InjectPayload(makePayload("Test", true))
	time.Sleep(150 * time.Millisecond)

	assert.Equal(t, "/v1/text-to-speech/voice-id/stream", capturedPath)
}

// TestPlugin_AudioChunking verifies that the AudioStreamer correctly chunks
// large PCM responses into fixed-size Audio frames rather than one giant blob.
func TestPlugin_AudioChunking(t *testing.T) {
	// Send 8192 bytes — should produce multiple Audio frames.
	pcmData := makePCM(8192)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write in small pieces to exercise the streaming read loop.
		for i := 0; i < len(pcmData); i += 512 {
			end := i + 512
			if end > len(pcmData) {
				end = len(pcmData)
			}
			_, _ = w.Write(pcmData[i:end])
		}
	}

	var frameCount atomic.Int32

	tester, stop := startPlugin(t, handler)
	defer stop()

	tester.OnAudio(func(_ int, _ schema.Audio) {
		frameCount.Add(1)
	})

	tester.InjectPayload(makePayload("Long text chunk", true))
	time.Sleep(300 * time.Millisecond)

	assert.Greater(t, frameCount.Load(), int32(1), "large PCM should produce multiple Audio frames")
}

// TestPlugin_FinalWithNoText ensures that a payload with is_final=true but an
// empty sentence still flushes the streamer (sends SpeechEnd) without making
// an HTTP request.
func TestPlugin_FinalWithNoText(t *testing.T) {
	var requestCount atomic.Int32
	var signals []string

	handler := func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makePCM(64))
	}

	tester, stop := startPlugin(t, handler)
	defer stop()

	tester.OnSignal(func(_ int, s schema.Signal) {
		signals = append(signals, s.Name())
	})

	// First send a non-final chunk to start the stream.
	tester.InjectPayload(makePayload("Some text.", false))
	time.Sleep(150 * time.Millisecond)

	// Then send a final payload with no text — should flush without an extra HTTP call.
	tester.InjectPayload(makePayload("", true))
	time.Sleep(150 * time.Millisecond)

	assert.Equal(t, int32(1), requestCount.Load(), "empty+final should not trigger extra HTTP call")
	assert.Contains(t, signals, schema.SignalAgentSpeechEnd, "SpeechEnd should still be emitted on final flush")
}

// TestPlugin_PCMFrameFormat verifies that downstream Audio frames contain
// only valid 16-bit (2-byte-aligned) samples.
func TestPlugin_PCMFrameFormat(t *testing.T) {
	// Craft 4096 bytes of PCM where each int16 sample has a distinct known value.
	pcmData := make([]byte, 4096)
	for i := 0; i < len(pcmData)/2; i++ {
		binary.LittleEndian.PutUint16(pcmData[i*2:], uint16(i))
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pcmData)
	}

	var receivedBytes []byte

	tester, stop := startPlugin(t, handler)
	defer stop()

	tester.OnAudio(func(_ int, a schema.Audio) {
		receivedBytes = append(receivedBytes, a.Bytes()...)
	})

	tester.InjectPayload(makePayload("Test", true))
	time.Sleep(200 * time.Millisecond)

	// Frame size must always be even (2-byte aligned).
	assert.Equal(t, 0, len(receivedBytes)%2, "received PCM must be 2-byte aligned")
	// All sent bytes should be accounted for.
	assert.Len(t, receivedBytes, len(pcmData), "all PCM bytes should be forwarded")
}

// TestPlugin_SlowServer verifies that when the plugin is stopped while an
// HTTP request is in flight, no audio frames are emitted afterward (the
// in-flight request is canceled via the context and the plugin exits cleanly).
func TestPlugin_SlowServer(t *testing.T) {
	// The server blocks for a while before responding.
	unblock := make(chan struct{})
	handler := func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-unblock:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(makePCM(64))
		case <-time.After(3 * time.Second):
			http.Error(w, "timeout", http.StatusGatewayTimeout)
		}
	}

	var audioAfterCancel atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(handler))
	cfg := makeConfig(srv.URL, srv.Client())
	plg := NewPlugin(cfg)
	tester := engine.NewPluginTester(t, plg).Start()

	tester.OnAudio(func(_ int, _ schema.Audio) {
		audioAfterCancel.Store(true)
	})

	// Kick off the slow request.
	tester.InjectPayload(makePayload("Hello", false))
	time.Sleep(50 * time.Millisecond) // let the request reach the server

	// Cancel the plugin context — the in-flight HTTP request should fail with
	// context.Canceled and no audio should be emitted.
	tester.Done()
	time.Sleep(100 * time.Millisecond)

	// Unblock + close the server only after the plugin context is canceled.
	close(unblock)
	srv.Close()
	tester.Stop()

	assert.False(t, audioAfterCancel.Load(),
		"no audio should be emitted after the plugin context is canceled")
}

// TestPlugin_MultipleUtterances verifies correct sequential processing across
// several independent utterances (each starting fresh).
func TestPlugin_MultipleUtterances(t *testing.T) {
	var requestCount atomic.Int32

	handler := func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makePCM(64))
	}

	tester, stop := startPlugin(t, handler)
	defer stop()

	for i := 0; i < 3; i++ {
		tester.InjectPayload(makePayload("Sentence.", true))
		time.Sleep(150 * time.Millisecond)
	}

	assert.Equal(t, int32(3), requestCount.Load(), "each utterance should produce exactly one HTTP request")
}

// TestPlugin_DefaultBaseURL verifies that omitting BaseURL falls back to the
// ElevenLabs production endpoint.
func TestPlugin_DefaultBaseURL(t *testing.T) {
	ctx := context.Background()
	p := &Plugin{
		cfg:     &ElevenLabsConfig{VoiceID: "abc", ModelID: "eleven_turbo_v2_5"},
		readBuf: make([]byte, 4096),
	}
	req, err := p.buildRequest(ctx, "hello")
	require.NoError(t, err)
	assert.True(t,
		strings.HasPrefix(req.URL.String(), "https://api.elevenlabs.io"),
		"should fall back to production URL when BaseURL is empty",
	)
}
