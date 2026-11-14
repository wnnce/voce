package translator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVolcengine_LanguageMapping(t *testing.T) {
	assert.Equal(t, "zh", ToVolcengineLang("zh-CN"))
	assert.Equal(t, "zh", ToVolcengineLang("zh"))
	assert.Equal(t, "zh-Hant", ToVolcengineLang("zh-TW"))
	assert.Equal(t, "zh-Hant", ToVolcengineLang("zh-HK"))
	assert.Equal(t, "en", ToVolcengineLang("en-US"))
	assert.Equal(t, "en", ToVolcengineLang("en-GB"))
	assert.Equal(t, "ja", ToVolcengineLang("ja-JP"))
	assert.Equal(t, "ko", ToVolcengineLang("ko-KR"))
	assert.Empty(t, ToVolcengineLang("auto"))
	assert.Empty(t, ToVolcengineLang(""))
}

func TestVolcengineProvider_New_Validation(t *testing.T) {
	ctx := context.Background()

	// Missing access_key
	_, err := NewVolcengineProvider(ctx, json.RawMessage(`{"secret_key":"sk"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access_key is required")

	// Missing secret_key
	_, err = NewVolcengineProvider(ctx, json.RawMessage(`{"access_key":"ak"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secret_key is required")

	// Valid with defaults
	p, err := NewVolcengineProvider(ctx, json.RawMessage(`{"access_key":"ak","secret_key":"sk"}`))
	require.NoError(t, err)
	vp, ok := p.(*VolcengineProvider)
	require.True(t, ok)
	assert.Equal(t, "ak", vp.accessKey)
	assert.Equal(t, "sk", vp.secretKey)
	assert.Equal(t, defaultVolcRegion, vp.region)
	assert.Equal(t, defaultVolcEndpoint, vp.endpoint)
	assert.Equal(t, ProviderVolcengine, p.Name())
	assert.NoError(t, p.Close())
}

func TestVolcengineProvider_SignatureAndTranslation(t *testing.T) {
	var capturedAuth string
	var capturedDate string
	var capturedSha string
	var capturedAction string
	var capturedVersion string
	var capturedReqBody volcTranslateRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedDate = r.Header.Get("X-Date")
		capturedSha = r.Header.Get("X-Content-Sha256")
		capturedAction = r.URL.Query().Get("Action")
		capturedVersion = r.URL.Query().Get("Version")

		_ = sonic.ConfigDefault.NewDecoder(r.Body).Decode(&capturedReqBody)

		w.Header().Set("Content-Type", "application/json")
		resp := volcTranslateResponse{
			TranslationList: []struct {
				Translation            string `json:"Translation"`
				DetectedSourceLanguage string `json:"DetectedSourceLanguage"`
			}{
				{
					Translation:            "Hello world",
					DetectedSourceLanguage: "zh",
				},
			},
		}
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	props, _ := sonic.Marshal(map[string]any{
		"access_key": "test_ak",
		"secret_key": "test_sk",
		"endpoint":   server.URL,
	})

	provider, err := NewVolcengineProvider(context.Background(), props)
	require.NoError(t, err)

	var result string
	err = provider.TranslateStream(context.Background(), "你好世界", TranslateOptions{
		SourceLang: "zh-CN",
		TargetLang: "en-US",
		IsFinal:    true,
	}, func(delta, accumulated string, isDone bool) error {
		result = accumulated
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, "Hello world", result)

	// Validate query parameters
	assert.Equal(t, "TranslateText", capturedAction)
	assert.Equal(t, "2020-06-01", capturedVersion)

	// Validate SigV4 headers
	assert.NotEmpty(t, capturedDate)
	assert.NotEmpty(t, capturedSha)
	assert.Contains(t, capturedAuth, "HMAC-SHA256 Credential=test_ak/")
	assert.Contains(t, capturedAuth, "/cn-north-1/translate/request")
	assert.Contains(t, capturedAuth, "SignedHeaders=")
	assert.Contains(t, capturedAuth, "Signature=")

	// Validate request body
	assert.Equal(t, "en", capturedReqBody.TargetLanguage)
	assert.Equal(t, "zh", capturedReqBody.SourceLanguage)
	require.Len(t, capturedReqBody.TextList, 1)
	assert.Equal(t, "你好世界", capturedReqBody.TextList[0])
}

func TestVolcengineProvider_ApiError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]any{
			"ResponseMetadata": map[string]any{
				"Error": map[string]any{
					"Code":    "InvalidParameter",
					"Message": "Language not supported",
				},
			},
		}
		_ = sonic.ConfigDefault.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	props, _ := sonic.Marshal(map[string]any{
		"access_key": "test_ak",
		"secret_key": "test_sk",
		"endpoint":   server.URL,
	})

	provider, err := NewVolcengineProvider(context.Background(), props)
	require.NoError(t, err)

	err = provider.TranslateStream(context.Background(), "test", TranslateOptions{
		SourceLang: "auto",
		TargetLang: "en-US",
	}, func(_, _ string, _ bool) error {
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 400")
}

func TestVolcengineProvider_EmptyText(t *testing.T) {
	props, _ := sonic.Marshal(map[string]any{
		"access_key": "test_ak",
		"secret_key": "test_sk",
	})
	provider, err := NewVolcengineProvider(context.Background(), props)
	require.NoError(t, err)

	var called bool
	err = provider.TranslateStream(context.Background(), "   ", TranslateOptions{}, func(d, a string, done bool) error {
		called = true
		assert.True(t, done)
		assert.Empty(t, a)
		return nil
	})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestVolcengine_SignVerification(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://translate.volcengineapi.com/?Action=TranslateText&Version=2020-06-01", nil)
	fixedTime := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	body := []byte(`{"TargetLanguage":"en","TextList":["test"]}`)

	signVolcengineRequest(req, body, "my_ak", "my_sk", "cn-north-1", "translate", fixedTime)

	auth := req.Header.Get("Authorization")
	assert.Contains(t, auth, "HMAC-SHA256 Credential=my_ak/20261114/cn-north-1/translate/request")
	assert.Contains(t, auth, "SignedHeaders=content-type;host;x-content-sha256;x-date")
	assert.Contains(t, auth, "Signature=")
	assert.Equal(t, "20261114T120000Z", req.Header.Get("X-Date"))
}
