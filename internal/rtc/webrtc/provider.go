package webrtc

import (
	"context"

	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/rtc/webrtc/media"
)

// ICEConfig is re-exported from media for backward compatibility and provider convenience.
type ICEConfig = media.ICEConfig

// NegotiateRequest contains the information needed by a provider to exchange SDP with an SFU network.
type NegotiateRequest struct {
	SessionKey protocol.SessionKey
	OfferSDP   string
}

// NegotiateResult represents the output of exchanging SDP with the signaling provider.
type NegotiateResult struct {
	// AnswerSDP is the remote SDP answer from the SFU/network.
	AnswerSDP string
	// Handle is an opaque identifier (e.g. resource URL or session ID) used for later cleanup.
	Handle string
	// Credentials contains client connection info returned via HTTP to the user/client.
	Credentials any
}

// Metadata provides descriptive information for a WebRTC provider.
type Metadata struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Provider abstracts the WebRTC signaling and cloud network provider (e.g. WTN, Cloudflare Calls, WHIP/WHEP).
// It decouples the Pion media/data transport from any vendor-specific REST/SDP exchange protocols.
type Provider interface {
	// Name returns the provider identifier (e.g. "wtn", "cloudflare") used as the routing and configuration key.
	Name() string

	// Metadata returns descriptive information for discovery and UI presentation.
	Metadata() Metadata

	// Negotiate performs the signaling exchange: sends the server's local SDP offer to the SFU
	// and returns the remote SDP answer, session handle, and client credentials.
	Negotiate(ctx context.Context, request NegotiateRequest) (*NegotiateResult, error)

	// ICEServers returns the provider-neutral STUN/TURN configuration used while
	// creating a media session. It may fetch short-lived credentials for a
	// particular session; the provider does not need to know about Pion.
	ICEServers(ctx context.Context, sessionKey protocol.SessionKey) ([]media.ICEConfig, error)

	// Teardown cleans up any remote session or channel resources created for this handle.
	Teardown(ctx context.Context, handle string) error
}
