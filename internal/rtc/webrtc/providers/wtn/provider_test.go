package wtn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/rtc/webrtc"
)

const (
	testAppID  = "test-wtn-appid"
	testAppKey = "test-wtn-secret-appkey"
)

func TestConfig_DefaultsAndValidate(t *testing.T) {
	cfg := Config{AppID: testAppID, AppKey: testAppKey}.withDefaults()
	assert.Equal(t, defaultBaseURL, cfg.BaseURL)
	assert.Equal(t, defaultTokenExpire, cfg.TokenExpire)
	assert.Equal(t, defaultStreamPrefix, cfg.StreamPrefix)
	require.NoError(t, cfg.validate())

	t.Run("missing app_id", func(t *testing.T) {
		bad := cfg
		bad.AppID = ""
		assert.ErrorIs(t, bad.validate(), ErrInvalidConfig)
	})

	t.Run("missing app_key", func(t *testing.T) {
		bad := cfg
		bad.AppKey = ""
		assert.ErrorIs(t, bad.validate(), ErrInvalidConfig)
	})
}

func TestProvider_BuildJWT(t *testing.T) {
	provider, err := New(Config{
		AppID:       testAppID,
		AppKey:      testAppKey,
		TokenExpire: time.Hour,
	}, nil)
	require.NoError(t, err)

	start := time.Now()
	token, err := provider.buildJWT("pub-sub", "streamA", "streamB")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	parsed, err := jwt.Parse(token, func(token *jwt.Token) (any, error) {
		assert.Equal(t, jwt.SigningMethodHS256, token.Method)
		return []byte(testAppKey), nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)

	claims, ok := parsed.Claims.(jwt.MapClaims)
	require.True(t, ok)
	assert.Equal(t, testAppID, claims["appID"])
	assert.Equal(t, "pub-sub", claims["action"])
	assert.Equal(t, "streamA/streamB", claims["streamID"])

	exp, ok := claims["exp"].(float64)
	require.True(t, ok)
	assert.WithinDuration(t, start.Add(time.Hour), time.Unix(int64(exp), 0), 5*time.Second)
}

func TestProvider_NegotiateAndTeardown(t *testing.T) {
	deleteCalled := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			assert.Contains(t, r.URL.Path, "/webrtc/pub-sub/"+testAppID)
			assert.Equal(t, "application/sdp", r.Header.Get("Content-Type"))
			assert.NotEmpty(t, r.Header.Get("Authorization"))

			w.Header().Set("Location", "http://example.com/resource/xyz")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("v=0\r\no=- 1 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n"))
		case http.MethodDelete:
			deleteCalled = true
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	provider, err := New(Config{
		AppID:   testAppID,
		AppKey:  testAppKey,
		BaseURL: ts.URL,
	}, ts.Client())
	require.NoError(t, err)

	sessionKey := protocol.SessionKey(uuid.New())
	res, err := provider.Negotiate(context.Background(), webrtc.NegotiateRequest{
		SessionKey: sessionKey,
		OfferSDP:   "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n",
	})
	require.NoError(t, err)
	assert.Equal(t, "http://example.com/resource/xyz", res.Handle)
	assert.Contains(t, res.AnswerSDP, "v=0")

	creds, ok := res.Credentials.(Credentials)
	require.True(t, ok)
	assert.Equal(t, testAppID, creds.AppID)
	assert.NotEmpty(t, creds.Token)

	// Teardown
	err = provider.Teardown(context.Background(), ts.URL+"/resource/xyz")
	require.NoError(t, err)
	assert.True(t, deleteCalled)
}

func TestProvider_Info(t *testing.T) {
	provider, err := New(Config{AppID: testAppID, AppKey: testAppKey}, nil)
	require.NoError(t, err)
	assert.Equal(t, "wtn", provider.Name())
	assert.Equal(t, "Volcengine WTN", provider.Metadata().Label)
	assert.NotEmpty(t, provider.Metadata().Description)
}
