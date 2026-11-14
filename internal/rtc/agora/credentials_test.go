package agora

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
)

// Agora only accepts 32 hex character app ids and certificates.
const (
	testAppID          = "0123456789abcdef0123456789abcdef"
	testAppCertificate = "fedcba9876543210fedcba9876543210"
)

func TestTokenBuilder_Build(t *testing.T) {
	builder := tokenBuilder{
		appID:           testAppID,
		appCertificate:  testAppCertificate,
		tokenExpire:     time.Hour,
		privilegeExpire: 30 * time.Minute,
		rtmTokenExpire:  2 * time.Hour,
	}

	start := time.Now()
	credentials, err := builder.build("voce-test-channel", "client-account")
	require.NoError(t, err)

	assert.Equal(t, testAppID, credentials.AppID)
	assert.Equal(t, "voce-test-channel", credentials.Channel)
	assert.Equal(t, "client-account", credentials.UID)
	assert.NotEmpty(t, credentials.RTCToken)
	assert.NotEmpty(t, credentials.RTMToken)
	assert.NotEqual(t, credentials.RTCToken, credentials.RTMToken)

	// The shortest token wins: credentials must not outlive any of the three.
	assert.WithinDuration(t, start.Add(30*time.Minute), credentials.ExpireAt, 5*time.Second)
}

func TestTokenBuilder_BuildWithoutCertificate(t *testing.T) {
	builder := tokenBuilder{
		appID:           testAppID,
		tokenExpire:     time.Hour,
		privilegeExpire: time.Hour,
		rtmTokenExpire:  time.Hour,
	}

	start := time.Now()
	credentials, err := builder.build("voce-test-channel", "client-account")
	require.NoError(t, err)

	// An app without a certificate accepts an empty token; nothing to sign.
	assert.Empty(t, credentials.RTCToken)
	assert.Empty(t, credentials.RTMToken)
	assert.WithinDuration(t, start.Add(time.Hour), credentials.ExpireAt, 5*time.Second)
}

func TestTokenBuilder_ExpireWindow(t *testing.T) {
	tests := []struct {
		name      string
		builder   tokenBuilder
		expectMin time.Duration
	}{
		{
			name:      "shortest is privilege",
			builder:   tokenBuilder{tokenExpire: time.Hour, privilegeExpire: time.Minute, rtmTokenExpire: 2 * time.Hour},
			expectMin: time.Minute,
		},
		{
			name:      "shortest is rtm",
			builder:   tokenBuilder{tokenExpire: time.Hour, privilegeExpire: time.Hour, rtmTokenExpire: time.Second},
			expectMin: time.Second,
		},
		{
			name:      "shortest is token",
			builder:   tokenBuilder{tokenExpire: 2 * time.Minute, privilegeExpire: time.Hour, rtmTokenExpire: time.Hour},
			expectMin: 2 * time.Minute,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectMin, test.builder.expireWindow())
		})
	}
}

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{AppID: testAppID}.withDefaults()
	assert.Equal(t, defaultChannelPrefix, cfg.ChannelPrefix)
	assert.Equal(t, defaultClientUIDPrefix, cfg.ClientUIDPrefix)
	assert.Equal(t, defaultServerUID, cfg.ServerUID)
	assert.Equal(t, defaultTokenExpire, cfg.TokenExpire)
	assert.Equal(t, defaultPrivilegeExpire, cfg.PrivilegeExpire)
	assert.Equal(t, defaultRTMTokenExpire, cfg.RTMTokenExpire)
	require.NoError(t, cfg.validate())
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "missing app id", cfg: Config{}},
		{name: "sub second expiry", cfg: Config{AppID: testAppID, TokenExpire: 500 * time.Millisecond}},
		{name: "fractional seconds", cfg: Config{AppID: testAppID, PrivilegeExpire: 1500 * time.Millisecond}},
		{name: "negative expiry", cfg: Config{AppID: testAppID, RTMTokenExpire: -time.Hour}},
		{name: "server uid too long", cfg: Config{AppID: testAppID, ServerUID: string(make([]byte, maxAgoraNameLen+1))}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, test.cfg.withDefaults().validate(), ErrInvalidConfig)
		})
	}
}

func TestConfig_NamesDerivedFromSessionKey(t *testing.T) {
	cfg := Config{AppID: testAppID}.withDefaults()
	key := protocol.NewSessionKey()

	assert.Equal(t, defaultChannelPrefix+key.String(), cfg.ChannelName(key))
	assert.Equal(t, defaultClientUIDPrefix+key.String(), cfg.ClientUID(key))
	assert.LessOrEqual(t, len(cfg.ChannelName(key)), maxAgoraNameLen)
	assert.LessOrEqual(t, len(cfg.ClientUID(key)), maxAgoraNameLen)
}
