package openai_llm

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

func TestOpenai_StreamChat(t *testing.T) {
	// 1. Setup Mock Server for SSE
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock OpenAI SSE stream
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Send two chunks
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		time.Sleep(10 * time.Millisecond)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\" World!\"}}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	// 2. Initialize Plugin with Mock Config
	cfg := &OpenaiConfig{
		BaseUrl: server.URL,
		ApiKey:  "test-key",
		Model:   "gpt-4",
		client:  server.Client(),
	}
	cfg.BaseConfig.HistoryLimit = 5
	p := NewPlugin(cfg)

	// 3. Setup Result Capture
	var receivedChunks []string
	var isFinal bool

	// 4. Use PluginTester
	tester := engine.NewPluginTester(t, p)
	tester.OnPayload(func(port int, payload schema.Payload) {
		if payload.Name() != schema.PayloadLLMChunk {
			return
		}

		text := schema.GetAs(payload, "sentence", "")
		final := schema.GetAs(payload, "is_final", false)

		if text != "" {
			receivedChunks = append(receivedChunks, text)
		}
		if final {
			isFinal = true
			tester.Done()
		}
	})

	// 5. Start Lifecycle
	tester.Start()

	// 6. Inject Input MutablePayload (Simulating final user speech)
	input := schema.NewPayload(schema.PayloadASRResult)
	_ = input.Set("text", "Hi")
	_ = input.Set("is_final", true)
	tester.InjectPayload(input.ReadOnly())

	// 7. Wait for completion
	tester.Wait()

	// 8. Assertions
	combined := strings.Join(receivedChunks, "")
	assert.Equal(t, "Hello World!", combined)
	assert.True(t, isFinal)

	tester.Stop()
}

func TestOpenai_ErrorHandling(t *testing.T) {
	// 1. Setup Error Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("Invalid API Key"))
	}))
	defer server.Close()

	cfg := &OpenaiConfig{
		BaseUrl: server.URL,
		ApiKey:  "bad-key",
		client:  server.Client(),
	}
	cfg.FailedMessage = "Oops, error."

	p := NewPlugin(cfg)

	var lastMessage string
	tester := engine.NewPluginTester(t, p)
	tester.OnPayload(func(port int, payload schema.Payload) {
		if payload.Name() != schema.PayloadLLMChunk {
			return
		}
		if schema.GetAs(payload, "is_final", false) {
			lastMessage = schema.GetAs(payload, "sentence", "")
			tester.Done()
		}
	})

	tester.Start()

	input := schema.NewPayload(schema.PayloadASRResult)
	_ = input.Set("text", "Test error")
	_ = input.Set("is_final", true)
	tester.InjectPayload(input.ReadOnly())

	tester.Wait()

	assert.Equal(t, "Oops, error.", lastMessage)
	tester.Stop()
}
