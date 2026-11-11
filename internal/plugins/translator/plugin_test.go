package translator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
)

func TestLanguage_NormalizationAndAdapters(t *testing.T) {
	// Normalization to BCP 47
	assert.Equal(t, LangZhCN, NormalizeLang("zh"))
	assert.Equal(t, LangZhCN, NormalizeLang("zh-CN"))
	assert.Equal(t, LangZhCN, NormalizeLang("zh_cn"))
	assert.Equal(t, LangEnUS, NormalizeLang("en"))
	assert.Equal(t, LangEnUS, NormalizeLang("en-US"))
	assert.Equal(t, LangJaJP, NormalizeLang("ja"))
	assert.Equal(t, "auto", NormalizeLang("auto"))

	// DeepL mappings
	assert.Equal(t, "ZH", ToDeepLTargetLang("zh-CN"))
	assert.Equal(t, "ZH", ToDeepLTargetLang("zh"))
	assert.Equal(t, "ZH-HANT", ToDeepLTargetLang("zh-TW"))
	assert.Equal(t, "EN-US", ToDeepLTargetLang("en"))
	assert.Equal(t, "EN-US", ToDeepLTargetLang("en-US"))
	assert.Equal(t, "EN-GB", ToDeepLTargetLang("en-GB"))
	assert.Equal(t, "PT-BR", ToDeepLTargetLang("pt-BR"))
	assert.Equal(t, "JA", ToDeepLTargetLang("ja-JP"))

	assert.Equal(t, "EN", ToDeepLSourceLang("en-US"))
	assert.Equal(t, "ZH", ToDeepLSourceLang("zh-CN"))
	assert.Empty(t, ToDeepLSourceLang("auto"))

	// OpenAI mappings
	assert.Contains(t, ToOpenAILangDesc("zh-CN"), "Simplified Chinese")
	assert.Contains(t, ToOpenAILangDesc("zh-TW"), "Traditional Chinese")
	assert.Contains(t, ToOpenAILangDesc("en-US"), "American English")
}

func TestTranslation_DraftAndRevise_AndCommit(t *testing.T) {
	cfg := &TranslationConfig{
		SourceLang: "en-US",
		TargetLang: "zh-CN",
		DebounceMs: 20, // fast debounce for test
		FastProvider: ProviderConfig{
			Type: ProviderMock,
		},
	}

	plugin := NewPlugin(cfg)
	tester := engine.NewPluginTester(t, plugin)

	var mu sync.Mutex
	var receivedCaptions []schema.Caption

	tester.OnPayload(func(port int, payload schema.Payload) {
		var cap schema.Caption
		if err := payload.Unpack(&cap); err == nil {
			mu.Lock()
			receivedCaptions = append(receivedCaptions, cap)
			mu.Unlock()
		}
	})

	tester.Start()

	// 1. Send interim draft 1
	p1 := schema.NewPayload(schema.PayloadASRResult)
	require.NoError(t, p1.Pack(&schema.Transcription{
		Text:    "Hello",
		Final:   false,
		Speaker: "speaker_0",
	}))
	tester.InjectPayload(p1.ReadOnly())

	// Wait for debounce to fire
	time.Sleep(60 * time.Millisecond)

	// 2. Send interim draft 2 (extended text)
	p2 := schema.NewPayload(schema.PayloadASRResult)
	require.NoError(t, p2.Pack(&schema.Transcription{
		Text:    "Hello world",
		Final:   false,
		Speaker: "speaker_0",
	}))
	tester.InjectPayload(p2.ReadOnly())

	time.Sleep(60 * time.Millisecond)

	// 3. Send final commit
	p3 := schema.NewPayload(schema.PayloadASRResult)
	require.NoError(t, p3.Pack(&schema.Transcription{
		Text:    "Hello world.",
		Final:   true,
		Speaker: "speaker_0",
	}))
	tester.InjectPayload(p3.ReadOnly())

	time.Sleep(60 * time.Millisecond)

	tester.Done()
	tester.Wait()
	tester.Stop()

	mu.Lock()
	defer mu.Unlock()

	require.GreaterOrEqual(t, len(receivedCaptions), 3)

	// Verify interim draft 1
	assert.Equal(t, "Hello", receivedCaptions[0].Text)
	assert.Equal(t, "Trans: Hello", receivedCaptions[0].Translation)
	assert.False(t, receivedCaptions[0].IsFinal)
	assert.Equal(t, "speaker_0", receivedCaptions[0].Speaker)

	// Verify interim draft 2 (draft revised)
	assert.Equal(t, "Hello world", receivedCaptions[1].Text)
	assert.Equal(t, "Trans: Hello world", receivedCaptions[1].Translation)
	assert.False(t, receivedCaptions[1].IsFinal)

	// Verify final commit
	last := receivedCaptions[len(receivedCaptions)-1]
	assert.Equal(t, "Hello world.", last.Text)
	assert.Equal(t, "Trans: Hello world.", last.Translation)
	assert.True(t, last.IsFinal)
}

func TestTranslation_DraftReference_PassedToFinalLLM(t *testing.T) {
	var capturedOpts []TranslateOptions
	var mu sync.Mutex

	mock := &MockProvider{
		TranslateHook: func(ctx context.Context, text string, opts TranslateOptions, handler StreamHandler) error {
			mu.Lock()
			capturedOpts = append(capturedOpts, opts)
			mu.Unlock()
			return handler("Trans: "+text, "Trans: "+text, true)
		},
	}

	RegisterProvider("test_mock_ref", func(_ context.Context, _ json.RawMessage) (Provider, error) {
		return mock, nil
	})

	cfg := &TranslationConfig{
		SourceLang: "auto",
		TargetLang: "zh-CN",
		DebounceMs: 15,
		FastProvider: ProviderConfig{
			Type: "test_mock_ref",
		},
	}

	plugin := NewPlugin(cfg)
	tester := engine.NewPluginTester(t, plugin)
	tester.Start()

	// 1. Send interim draft with detected LanguageCode "en-US"
	p1 := schema.NewPayload(schema.PayloadASRResult)
	_ = p1.Pack(&schema.Transcription{
		Text:         "The revenue",
		Final:        false,
		LanguageCode: "en-US",
	})
	tester.InjectPayload(p1.ReadOnly())

	// Wait for draft to finish
	time.Sleep(50 * time.Millisecond)

	// 2. Send final commit
	p2 := schema.NewPayload(schema.PayloadASRResult)
	_ = p2.Pack(&schema.Transcription{
		Text:         "The revenue has increased.",
		Final:        true,
		LanguageCode: "en-US",
	})
	tester.InjectPayload(p2.ReadOnly())

	time.Sleep(50 * time.Millisecond)

	tester.Done()
	tester.Wait()
	tester.Stop()

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, capturedOpts, 2)
	// First was draft: no draft ref, IsFinal = false, SourceLang resolved to "en-US"
	assert.False(t, capturedOpts[0].IsFinal)
	assert.Equal(t, "en-US", capturedOpts[0].SourceLang)
	assert.Empty(t, capturedOpts[0].DraftRef)

	// Second was final: has DraftRef from previous draft!
	assert.True(t, capturedOpts[1].IsFinal)
	assert.Equal(t, "en-US", capturedOpts[1].SourceLang)
	assert.Equal(t, "Trans: The revenue", capturedOpts[1].DraftRef)
}

func TestTranslation_OutOfOrder_Discard(t *testing.T) {
	// Custom mock provider with conditional delays
	mock := &MockProvider{
		TranslateHook: func(ctx context.Context, text string, opts TranslateOptions, handler StreamHandler) error {
			if text == "slow draft" {
				// Delay slow draft so fast draft finishes first
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(80 * time.Millisecond):
				}
			} else {
				// Fast draft
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(10 * time.Millisecond):
				}
			}
			return handler("Trans: "+text, "Trans: "+text, true)
		},
	}

	RegisterProvider("test_mock_delay", func(_ context.Context, _ json.RawMessage) (Provider, error) {
		return mock, nil
	})

	cfg := &TranslationConfig{
		DebounceMs: 10,
		FastProvider: ProviderConfig{
			Type: "test_mock_delay",
		},
	}

	plugin := NewPlugin(cfg)
	tester := engine.NewPluginTester(t, plugin)

	var mu sync.Mutex
	var received []schema.Caption

	tester.OnPayload(func(port int, payload schema.Payload) {
		var cap schema.Caption
		if err := payload.Unpack(&cap); err == nil {
			mu.Lock()
			received = append(received, cap)
			mu.Unlock()
		}
	})

	tester.Start()

	// Send slow draft first
	p1 := schema.NewPayload(schema.PayloadASRResult)
	_ = p1.Pack(&schema.Transcription{Text: "slow draft", Final: false})
	tester.InjectPayload(p1.ReadOnly())

	// Wait 15ms so debounce fires for p1 and launches slow request
	time.Sleep(20 * time.Millisecond)

	// Send fast draft second
	p2 := schema.NewPayload(schema.PayloadASRResult)
	_ = p2.Pack(&schema.Transcription{Text: "fast draft", Final: false})
	tester.InjectPayload(p2.ReadOnly())

	// Wait for both to settle
	time.Sleep(150 * time.Millisecond)

	tester.Done()
	tester.Wait()
	tester.Stop()

	mu.Lock()
	defer mu.Unlock()

	// The fast draft must be received; the slow draft must NOT overwrite it
	require.NotEmpty(t, received)
	last := received[len(received)-1]
	assert.Equal(t, "fast draft", last.Text)
}

func TestTranslation_MultiSpeaker_Isolation(t *testing.T) {
	cfg := &TranslationConfig{
		DebounceMs: 10,
		FastProvider: ProviderConfig{
			Type: ProviderMock,
		},
	}

	plugin := NewPlugin(cfg)
	tester := engine.NewPluginTester(t, plugin)

	var mu sync.Mutex
	var received []schema.Caption

	tester.OnPayload(func(port int, payload schema.Payload) {
		var cap schema.Caption
		if err := payload.Unpack(&cap); err == nil {
			mu.Lock()
			received = append(received, cap)
			mu.Unlock()
		}
	})

	tester.Start()

	// Speaker A speaks
	pA := schema.NewPayload(schema.PayloadASRResult)
	_ = pA.Pack(&schema.Transcription{Speaker: "speaker_A", Text: "Speaker A talking", Final: true})
	tester.InjectPayload(pA.ReadOnly())

	// Speaker B speaks
	pB := schema.NewPayload(schema.PayloadASRResult)
	_ = pB.Pack(&schema.Transcription{Speaker: "speaker_B", Text: "Speaker B reply", Final: true})
	tester.InjectPayload(pB.ReadOnly())

	time.Sleep(50 * time.Millisecond)

	tester.Done()
	tester.Wait()
	tester.Stop()

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, received, 2)
	bySpeaker := make(map[string]schema.Caption)
	for _, c := range received {
		bySpeaker[c.Speaker] = c
	}
	assert.Equal(t, "Speaker A talking", bySpeaker["speaker_A"].Text)
	assert.Equal(t, "Trans: Speaker A talking", bySpeaker["speaker_A"].Translation)
	assert.Equal(t, "Speaker B reply", bySpeaker["speaker_B"].Text)
	assert.Equal(t, "Trans: Speaker B reply", bySpeaker["speaker_B"].Translation)
}

func TestTranslation_Interruption_BargeIn(t *testing.T) {
	cfg := &TranslationConfig{
		DebounceMs: 100, // long debounce
		FastProvider: ProviderConfig{
			Type: ProviderMock,
		},
	}

	plugin := NewPlugin(cfg)
	tester := engine.NewPluginTester(t, plugin)

	var mu sync.Mutex
	var received []schema.Caption

	tester.OnPayload(func(port int, payload schema.Payload) {
		var cap schema.Caption
		if err := payload.Unpack(&cap); err == nil {
			mu.Lock()
			received = append(received, cap)
			mu.Unlock()
		}
	})

	tester.Start()

	// Inject speech start
	p := schema.NewPayload(schema.PayloadASRResult)
	_ = p.Pack(&schema.Transcription{Text: "Wait I am speaking", Final: false})
	tester.InjectPayload(p.ReadOnly())

	// Immediately inject interruption before debounce timer fires
	sig := schema.NewSignal(schema.SignalInterrupter)
	tester.InjectSignal(sig.ReadOnly())

	// Wait past the debounce time
	time.Sleep(150 * time.Millisecond)

	tester.Done()
	tester.Wait()
	tester.Stop()

	mu.Lock()
	defer mu.Unlock()

	// Draft should have been canceled by interruption
	assert.Empty(t, received)
}

func TestTranslation_DeepLProvider_Passthrough(t *testing.T) {
	var receivedBody map[string]any
	var authHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		_ = sonic.ConfigDefault.NewDecoder(r.Body).Decode(&receivedBody)

		w.Header().Set("Content-Type", "application/json")
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(map[string]any{
			"translations": []map[string]any{
				{
					"detected_source_language": "EN",
					"text":                     "你好世界",
				},
			},
		})
	}))
	defer server.Close()

	propsJSON, err := sonic.Marshal(map[string]any{
		"api_key":     "dummy-key",
		"endpoint":    server.URL,
		"formality":   "prefer_more",
		"extra_param": 123,
	})
	require.NoError(t, err)

	provider, err := NewDeepLProvider(context.Background(), propsJSON)
	require.NoError(t, err)

	opts := TranslateOptions{
		SourceLang: "en-US",
		TargetLang: "zh-CN",
		IsFinal:    false,
	}

	var streamedText string
	err = provider.TranslateStream(context.Background(), "Hello world", opts,
		func(delta, accumulated string, isDone bool) error {
			streamedText = accumulated
			return nil
		})
	require.NoError(t, err)
	assert.Equal(t, "你好世界", streamedText)

	// Verify Auth Header
	assert.Equal(t, "DeepL-Auth-Key dummy-key", authHeader)

	// Verify BCP 47 mapped to DeepL requirements
	assert.Equal(t, "ZH", receivedBody["target_lang"])
	assert.Equal(t, "EN", receivedBody["source_lang"])

	// Verify Passthrough Parameters
	assert.Equal(t, "prefer_more", receivedBody["formality"])
	assert.InDelta(t, 123.0, receivedBody["extra_param"], 0.0001)
}

func TestTranslation_OpenAIProvider_StreamingAndDraftRef(t *testing.T) {
	var receivedBody map[string]any
	var authHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		_ = sonic.ConfigDefault.NewDecoder(r.Body).Decode(&receivedBody)

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.ResponseWriter to be a Flusher")
		}

		// Stream SSE chunks
		fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"content":"营收"}}]}`)
		fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"content":"增长了20%"}}]}`)
		flusher.Flush()
		time.Sleep(10 * time.Millisecond)

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	propsJSON, err := sonic.Marshal(map[string]any{
		"base_url":    server.URL,
		"api_key":     "sk-test123",
		"model":       "deepseek-v3",
		"temperature": 0.2,
		"extra_body": map[string]any{
			"top_p": 0.95,
		},
	})
	require.NoError(t, err)

	provider, err := NewOpenAIProvider(context.Background(), propsJSON)
	require.NoError(t, err)

	opts := TranslateOptions{
		SourceLang: "en-US",
		TargetLang: "zh-CN",
		IsFinal:    true,
		DraftRef:   "营收已增长", // Reference draft from interim MT!
	}

	var finalAccumulated string
	err = provider.TranslateStream(context.Background(), "Revenue has increased by 20%", opts,
		func(delta, accumulated string, isDone bool) error {
			finalAccumulated = accumulated
			return nil
		})
	require.NoError(t, err)
	assert.Equal(t, "营收增长了20%", finalAccumulated)

	// Verify Authorization
	assert.Equal(t, "Bearer sk-test123", authHeader)

	// Verify Stream option and Passthrough Parameters
	assert.Equal(t, true, receivedBody["stream"])
	assert.Equal(t, "deepseek-v3", receivedBody["model"])
	assert.InDelta(t, 0.2, receivedBody["temperature"], 0.0001)
	assert.InDelta(t, 0.95, receivedBody["top_p"], 0.0001)

	// Verify Prompt contains Draft Reference and BCP-47 Language descriptions!
	messages := receivedBody["messages"].([]any)
	userMsg := messages[1].(map[string]any)["content"].(string)
	assert.Contains(t, userMsg, "Simplified Chinese")
	assert.Contains(t, userMsg, "Reference draft translation previously displayed to the audience:")
	assert.Contains(t, userMsg, "营收已增长")
}
