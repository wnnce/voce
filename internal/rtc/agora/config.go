package agora

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/wnnce/voce/internal/protocol"
)

const (
	defaultChannelPrefix   = "voce-"
	defaultClientUIDPrefix = "client-"
	defaultServerUID       = "voce-agent"
	defaultTokenExpire     = 24 * time.Hour
	defaultPrivilegeExpire = 24 * time.Hour
	defaultRTMTokenExpire  = 24 * time.Hour

	// Agora limits both the channel name and the user account to 64 bytes.
	maxAgoraNameLen = 64
	// AppID and AppCertificate are UUIDs without dashes; the token builder
	// rejects anything that is not 32 hex characters.
	agoraAppKeyLen = 32
)

// Config describes how one Service signs credentials and names channels. The
// AppID and AppCertificate are also what the SDK engine is initialized with.
type Config struct {
	AppID           string
	AppCertificate  string
	ChannelPrefix   string
	ClientUIDPrefix string
	ServerUID       string
	TokenExpire     time.Duration
	PrivilegeExpire time.Duration
	RTMTokenExpire  time.Duration
}

func (c Config) withDefaults() Config {
	if c.ChannelPrefix == "" {
		c.ChannelPrefix = defaultChannelPrefix
	}
	if c.ClientUIDPrefix == "" {
		c.ClientUIDPrefix = defaultClientUIDPrefix
	}
	if c.ServerUID == "" {
		c.ServerUID = defaultServerUID
	}
	if c.TokenExpire == 0 {
		c.TokenExpire = defaultTokenExpire
	}
	if c.PrivilegeExpire == 0 {
		c.PrivilegeExpire = defaultPrivilegeExpire
	}
	if c.RTMTokenExpire == 0 {
		c.RTMTokenExpire = defaultRTMTokenExpire
	}
	return c
}

func (c Config) validate() error {
	if !isAppKey(c.AppID) {
		return fmt.Errorf("%w: app id must be %d hex characters", ErrInvalidConfig, agoraAppKeyLen)
	}
	if c.AppCertificate != "" && !isAppKey(c.AppCertificate) {
		return fmt.Errorf("%w: app certificate must be %d hex characters", ErrInvalidConfig, agoraAppKeyLen)
	}
	for name, value := range map[string]string{
		"channel prefix":    c.ChannelPrefix,
		"client uid prefix": c.ClientUIDPrefix,
		"server uid":        c.ServerUID,
	} {
		if value == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidConfig, name)
		}
	}
	for name, value := range map[string]time.Duration{
		"token expire":     c.TokenExpire,
		"privilege expire": c.PrivilegeExpire,
		"rtm token expire": c.RTMTokenExpire,
	} {
		if value <= 0 {
			return fmt.Errorf("%w: %s must be positive", ErrInvalidConfig, name)
		}
		if value < time.Second || value%time.Second != 0 {
			return fmt.Errorf("%w: %s must be a whole number of seconds", ErrInvalidConfig, name)
		}
	}
	if len(c.ServerUID) > maxAgoraNameLen {
		return fmt.Errorf("%w: server uid exceeds %d bytes", ErrInvalidConfig, maxAgoraNameLen)
	}
	if len(c.ChannelPrefix) > maxAgoraNameLen {
		return fmt.Errorf("%w: channel prefix exceeds %d bytes", ErrInvalidConfig, maxAgoraNameLen)
	}
	if len(c.ClientUIDPrefix) > maxAgoraNameLen {
		return fmt.Errorf("%w: client uid prefix exceeds %d bytes", ErrInvalidConfig, maxAgoraNameLen)
	}
	return nil
}

// isAppKey mirrors the token builder's own validation so a bad deployment fails
// at startup instead of on the first attach.
func isAppKey(value string) bool {
	if len(value) != agoraAppKeyLen {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// ChannelName derives the Agora channel for one session. The session key is the
// only input, so a recreated session never collides with a stale channel.
func (c Config) ChannelName(key protocol.SessionKey) string {
	return c.ChannelPrefix + key.String()
}

// ClientUID derives the account signed into the client's tokens. The client
// must join with exactly this account; it cannot choose its own.
func (c Config) ClientUID(key protocol.SessionKey) string {
	return c.ClientUIDPrefix + key.String()
}
