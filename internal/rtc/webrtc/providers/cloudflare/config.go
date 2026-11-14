package cloudflare

import (
	"fmt"
	"time"

	"github.com/wnnce/voce/internal/rtc/webrtc"
)

const (
	defaultBaseURL = "https://rtc.live.cloudflare.com/v1/apps"
	defaultSTUN    = "stun:stun.cloudflare.com:3478"
)

var (
	ErrInvalidConfig     = webrtc.ErrInvalidConfig
	ErrNegotiationFailed = webrtc.ErrNegotiationFailed
)

// Config contains connection parameters for Cloudflare Calls.
type Config struct {
	AppID     string        `json:"app_id" yaml:"app_id"`
	AppSecret string        `json:"app_secret" yaml:"app_secret"`
	BaseURL   string        `json:"base_url" yaml:"base_url"`
	STUNs     []string      `json:"stuns" yaml:"stuns"`
	Timeout   time.Duration `json:"timeout" yaml:"timeout"`
}

func (c Config) withDefaults() Config {
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
	if len(c.STUNs) == 0 {
		c.STUNs = []string{defaultSTUN}
	}
	if c.Timeout == 0 {
		c.Timeout = 10 * time.Second
	}
	return c
}

func (c Config) validate() error {
	if c.AppID == "" {
		return fmt.Errorf("%w: app_id is required", ErrInvalidConfig)
	}
	if c.AppSecret == "" {
		return fmt.Errorf("%w: app_secret is required", ErrInvalidConfig)
	}
	if c.BaseURL == "" {
		return fmt.Errorf("%w: base_url is required", ErrInvalidConfig)
	}
	return nil
}

// Credentials contains connection details returned to the frontend client to pull audio.
type Credentials struct {
	Provider        string   `json:"provider"`
	AppID           string   `json:"app_id"`
	BaseURL         string   `json:"base_url"`
	ServerSessionID string   `json:"server_session_id"`
	ServerTrackName string   `json:"server_track_name"`
	ClientTrackName string   `json:"client_track_name"`
	STUNs           []string `json:"stuns,omitempty"`
}
