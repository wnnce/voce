package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/rtc/webrtc"
	"github.com/wnnce/voce/internal/rtc/webrtc/media"
)

// HTTPClient represents a standard HTTP client for REST calls.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Provider implements webrtc.Provider for Cloudflare Calls.
type Provider struct {
	cfg        Config
	client     HTTPClient
	iceServers []media.ICEConfig
}

// New creates a new signaling provider for Cloudflare Calls.
func New(cfg Config, client HTTPClient) (*Provider, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
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
	return "cloudflare"
}

// Metadata returns descriptive information for Cloudflare Calls.
func (p *Provider) Metadata() webrtc.Metadata {
	return webrtc.Metadata{
		Label:       "Cloudflare Calls",
		Description: "Cloudflare 全球 Anycast 边缘 WebRTC",
	}
}

// ICEServers returns the STUN servers for Cloudflare Calls.
func (p *Provider) ICEServers(_ context.Context, _ protocol.SessionKey) ([]media.ICEConfig, error) {
	servers := make([]media.ICEConfig, len(p.iceServers))
	for i, server := range p.iceServers {
		servers[i] = media.ICEConfig{
			URLs: append([]string(nil), server.URLs...),
		}
	}
	return servers, nil
}

// Negotiate creates a Cloudflare Calls session, publishes the server audio track with the offer SDP,
// and returns the Answer SDP and client credentials.
func (p *Provider) Negotiate(ctx context.Context, request webrtc.NegotiateRequest) (*webrtc.NegotiateResult, error) {
	baseURL := strings.TrimRight(p.cfg.BaseURL, "/")
	appURL := fmt.Sprintf("%s/%s", baseURL, p.cfg.AppID)

	// 1. Create a new session on Cloudflare Calls
	newSessionURL := fmt.Sprintf("%s/sessions/new", appURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, newSessionURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: new session request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.AppSecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: cloudflare new session: %w", ErrNegotiationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("%w: cloudflare new session status %d: %s", ErrNegotiationFailed, resp.StatusCode, string(body))
	}

	var newSessionResp struct {
		SessionID string `json:"sessionId"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&newSessionResp); err != nil {
		return nil, fmt.Errorf("%w: decode new session response: %w", ErrNegotiationFailed, err)
	}
	if newSessionResp.SessionID == "" {
		return nil, fmt.Errorf("%w: empty sessionId from cloudflare", ErrNegotiationFailed)
	}
	serverSessionID := newSessionResp.SessionID

	// 2. Publish server audio track via /tracks/new
	tracksURL := fmt.Sprintf("%s/sessions/%s/tracks/new", appURL, serverSessionID)
	reqPayload := map[string]any{
		"sessionDescription": map[string]string{
			"type": "offer",
			"sdp":  request.OfferSDP,
		},
		"tracks": []map[string]any{
			{
				"location":  "local",
				"mid":       "0",
				"trackName": "server-audio",
			},
		},
	}
	reqBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: marshal tracks payload: %w", err)
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodPost, tracksURL, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("cloudflare: tracks request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.AppSecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err = p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: cloudflare publish track: %w", ErrNegotiationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("%w: cloudflare publish track status %d: %s", ErrNegotiationFailed, resp.StatusCode, string(body))
	}

	var tracksResp struct {
		SessionDescription struct {
			Type string `json:"type"`
			SDP  string `json:"sdp"`
		} `json:"sessionDescription"`
		Tracks []struct {
			TrackName string `json:"trackName"`
			Status    string `json:"status"`
		} `json:"tracks"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&tracksResp); err != nil {
		return nil, fmt.Errorf("%w: decode tracks response: %w", ErrNegotiationFailed, err)
	}
	if tracksResp.SessionDescription.SDP == "" {
		return nil, fmt.Errorf("%w: empty answer SDP from cloudflare", ErrNegotiationFailed)
	}

	return &webrtc.NegotiateResult{
		AnswerSDP: tracksResp.SessionDescription.SDP,
		Handle:    serverSessionID,
		Credentials: Credentials{
			Provider:        p.Name(),
			AppID:           p.cfg.AppID,
			BaseURL:         p.cfg.BaseURL,
			ServerSessionID: serverSessionID,
			ServerTrackName: "server-audio",
			ClientTrackName: "client-audio",
			STUNs:           append([]string(nil), p.cfg.STUNs...),
		},
	}, nil
}

// Teardown cleans up the remote session resources on Cloudflare Calls.
func (p *Provider) Teardown(ctx context.Context, handle string) error {
	if handle == "" {
		return nil
	}
	baseURL := strings.TrimRight(p.cfg.BaseURL, "/")
	closeURL := fmt.Sprintf("%s/%s/sessions/%s/tracks/close", baseURL, p.cfg.AppID, handle)

	reqPayload := map[string]any{
		"tracks": []map[string]string{
			{"trackName": "server-audio"},
		},
	}
	payload, _ := json.Marshal(reqPayload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, closeURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.AppSecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
