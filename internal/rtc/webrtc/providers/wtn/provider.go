package wtn

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/rtc/webrtc"
	"github.com/wnnce/voce/internal/rtc/webrtc/media"
)

// HTTPClient represents a standard HTTP client for REST calls.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Provider implements the webrtc.Provider interface for Volcengine WTN.
type Provider struct {
	cfg        Config
	client     HTTPClient
	iceServers []media.ICEConfig
}

// New creates a new signaling provider for Volcengine WTN.
func New(cfg Config, client HTTPClient) (*Provider, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	var iceServers []media.ICEConfig
	if len(cfg.STUNs) > 0 {
		iceServers = append(iceServers, media.ICEConfig{URLs: append([]string(nil), cfg.STUNs...)})
	}

	return &Provider{
		cfg:        cfg,
		client:     client,
		iceServers: iceServers,
	}, nil
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "wtn"
}

// Metadata returns descriptive information for WTN.
func (p *Provider) Metadata() webrtc.Metadata {
	return webrtc.Metadata{
		Label:       "Volcengine WTN",
		Description: "火山引擎 WebRTC 传输网络",
	}
}

// ICEServers returns the STUN servers configured for WTN.
func (p *Provider) ICEServers(_ context.Context, _ protocol.SessionKey) ([]media.ICEConfig, error) {
	servers := make([]media.ICEConfig, len(p.iceServers))
	for i, server := range p.iceServers {
		servers[i] = media.ICEConfig{
			URLs:       append([]string(nil), server.URLs...),
			Username:   server.Username,
			Credential: server.Credential,
		}
	}
	return servers, nil
}

// Negotiate exchanges SDP offer with WTN and returns SDP answer and client credentials.
func (p *Provider) Negotiate(ctx context.Context, request webrtc.NegotiateRequest) (*webrtc.NegotiateResult, error) {
	serverPubStreamID := fmt.Sprintf("%sserver-%s-%s", p.cfg.StreamPrefix, request.SessionKey.String()[:8], uuid.New().String()[:8])
	clientPubStreamID := fmt.Sprintf("%sclient-%s-%s", p.cfg.StreamPrefix, request.SessionKey.String()[:8], uuid.New().String()[:8])

	serverToken, err := p.buildJWT("pub-sub", serverPubStreamID, clientPubStreamID)
	if err != nil {
		return nil, fmt.Errorf("wtn: build server token: %w", err)
	}

	endpoint := fmt.Sprintf("%s/webrtc/pub-sub/%s/%s/%s", p.cfg.BaseURL, p.cfg.AppID, serverPubStreamID, clientPubStreamID)
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("wtn: parse endpoint url: %w", err)
	}

	query := u.Query()
	query.Add("SessionID", request.SessionKey.String())
	query.Add("PubMuteAudio", "false")
	query.Add("PubMuteVideo", "true")
	query.Add("SubMuteAudio", "false")
	query.Add("SubMuteVideo", "true")
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewBufferString(request.OfferSDP))
	if err != nil {
		return nil, fmt.Errorf("wtn: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/sdp")
	req.Header.Set("Authorization", "Bearer "+serverToken)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wtn: send pub-sub request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wtn: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("%w: status %d: %s", ErrNegotiationFailed, resp.StatusCode, string(body))
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return nil, fmt.Errorf("%w: response location is empty", ErrNegotiationFailed)
	}
	clientToken, err := p.buildJWT("pub-sub", clientPubStreamID, serverPubStreamID)
	if err != nil {
		_ = p.Teardown(context.Background(), location)
		return nil, fmt.Errorf("wtn: build client token: %w", err)
	}

	creds := Credentials{
		AppID:       p.cfg.AppID,
		PubStreamID: clientPubStreamID,
		SubStreamID: serverPubStreamID,
		Token:       clientToken,
		BaseURL:     p.cfg.BaseURL,
		STUNs:       p.cfg.STUNs,
		ExpireAt:    time.Now().Add(p.cfg.TokenExpire),
	}

	return &webrtc.NegotiateResult{
		AnswerSDP:   string(body),
		Handle:      location,
		Credentials: creds,
	}, nil
}

// Teardown releases the WTN PubSub session resource using its Location URL.
func (p *Provider) Teardown(ctx context.Context, handle string) error {
	if handle == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, handle, nil)
	if err != nil {
		return fmt.Errorf("wtn: create teardown request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("wtn: teardown request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		slog.WarnContext(ctx, "wtn teardown failed", "status", resp.StatusCode, "location", handle)
		return fmt.Errorf("%w: status %d", ErrNegotiationFailed, resp.StatusCode)
	}
	return nil
}

func (p *Provider) buildJWT(action string, streamIDs ...string) (string, error) {
	claims := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"version":  "1.0",
		"appID":    p.cfg.AppID,
		"exp":      time.Now().Add(p.cfg.TokenExpire).Unix(),
		"action":   action,
		"streamID": strings.Join(streamIDs, "/"),
	})
	token, err := claims.SignedString([]byte(p.cfg.AppKey))
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return token, nil
}
