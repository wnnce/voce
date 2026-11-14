package wtn

import (
	"fmt"
	"time"

	"github.com/wnnce/voce/internal/rtc/webrtc"
)

const (
	defaultBaseURL      = "https://wtn.volcvideo.com"
	defaultTokenExpire  = 24 * time.Hour
	defaultStreamPrefix = "voce-"
)

var (
	ErrInvalidConfig     = webrtc.ErrInvalidConfig
	ErrNegotiationFailed = webrtc.ErrNegotiationFailed
)

// Config contains connection parameters for Volcengine WTN.
type Config struct {
	AppID        string        `json:"app_id" yaml:"app_id"`
	AppKey       string        `json:"app_key" yaml:"app_key"`
	BaseURL      string        `json:"base_url" yaml:"base_url"`
	STUNs        []string      `json:"stuns" yaml:"stuns"`
	TokenExpire  time.Duration `json:"token_expire" yaml:"token_expire"`
	StreamPrefix string        `json:"stream_prefix" yaml:"stream_prefix"`
}

func (c Config) withDefaults() Config {
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
	if c.TokenExpire == 0 {
		c.TokenExpire = defaultTokenExpire
	}
	if c.StreamPrefix == "" {
		c.StreamPrefix = defaultStreamPrefix
	}
	return c
}

func (c Config) validate() error {
	if c.AppID == "" {
		return fmt.Errorf("%w: app_id is required", ErrInvalidConfig)
	}
	if c.AppKey == "" {
		return fmt.Errorf("%w: app_key is required", ErrInvalidConfig)
	}
	if c.BaseURL == "" {
		return fmt.Errorf("%w: base_url is required", ErrInvalidConfig)
	}
	if c.TokenExpire <= 0 {
		return fmt.Errorf("%w: token_expire must be positive", ErrInvalidConfig)
	}
	return nil
}

// Credentials represents the configuration returned to the client to connect to WTN.
type Credentials struct {
	AppID       string    `json:"app_id"`
	PubStreamID string    `json:"pub_stream_id"`
	SubStreamID string    `json:"sub_stream_id"`
	Token       string    `json:"token"`
	BaseURL     string    `json:"base_url"`
	STUNs       []string  `json:"stuns,omitempty"`
	ExpireAt    time.Time `json:"expire_at"`
}
