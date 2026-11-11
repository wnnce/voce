package cloudflare

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/rtc/webrtc"
)

type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

func TestConfig_DefaultsAndValidate(t *testing.T) {
	t.Run("missing app_id", func(t *testing.T) {
		cfg := Config{AppSecret: "secret"}.withDefaults()
		err := cfg.validate()
		assert.ErrorIs(t, err, ErrInvalidConfig)
	})

	t.Run("missing app_secret", func(t *testing.T) {
		cfg := Config{AppID: "app"}.withDefaults()
		err := cfg.validate()
		assert.ErrorIs(t, err, ErrInvalidConfig)
	})

	t.Run("valid config", func(t *testing.T) {
		cfg := Config{AppID: "app", AppSecret: "secret"}.withDefaults()
		err := cfg.validate()
		require.NoError(t, err)
		assert.Equal(t, defaultBaseURL, cfg.BaseURL)
		assert.Equal(t, []string{defaultSTUN}, cfg.STUNs)
	})
}

func TestProvider_NegotiateAndTeardown(t *testing.T) {
	callCount := 0
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			if req.Method == http.MethodPost && req.URL.Path == "/v1/apps/test-app/sessions/new" {
				assert.Equal(t, "Bearer test-secret", req.Header.Get("Authorization"))
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"sessionId":"sess-12345"}`))),
				}, nil
			}

			if req.Method == http.MethodPost && req.URL.Path == "/v1/apps/test-app/sessions/sess-12345/tracks/new" {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewReader([]byte(`{
						"sessionDescription": {"type":"answer","sdp":"v=0\r\no=cloudflare..."},
						"tracks": [{"trackName":"server-audio","status":"active"}]
					}`))),
				}, nil
			}

			if req.Method == http.MethodPut && req.URL.Path == "/v1/apps/test-app/sessions/sess-12345/tracks/close" {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(`{}`))),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}

	p, err := New(Config{
		AppID:     "test-app",
		AppSecret: "test-secret",
	}, mockClient)
	require.NoError(t, err)
	assert.Equal(t, "cloudflare", p.Name())
	assert.Equal(t, "Cloudflare Calls", p.Metadata().Label)
	assert.NotEmpty(t, p.Metadata().Description)

	ice, err := p.ICEServers(context.Background(), protocol.SessionKey{1, 2, 3, 4})
	require.NoError(t, err)
	require.Len(t, ice, 1)
	assert.Equal(t, defaultSTUN, ice[0].URLs[0])

	res, err := p.Negotiate(context.Background(), webrtc.NegotiateRequest{
		SessionKey: protocol.SessionKey{1, 2, 3, 4},
		OfferSDP:   "v=0\r\no=voce...",
	})
	require.NoError(t, err)
	assert.Equal(t, "v=0\r\no=cloudflare...", res.AnswerSDP)
	assert.Equal(t, "sess-12345", res.Handle)

	creds, ok := res.Credentials.(Credentials)
	require.True(t, ok)
	assert.Equal(t, "cloudflare", creds.Provider)
	assert.Equal(t, "sess-12345", creds.ServerSessionID)

	err = p.Teardown(context.Background(), res.Handle)
	require.NoError(t, err)
	assert.Equal(t, 3, callCount)
}
