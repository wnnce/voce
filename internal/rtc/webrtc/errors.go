package webrtc

import "errors"

var (
	ErrInvalidConfig     = errors.New("webrtc: invalid config")
	ErrInvalidSession    = errors.New("webrtc: invalid session")
	ErrBusy              = errors.New("webrtc: session is busy")
	ErrServiceClosed     = errors.New("webrtc: service is closed")
	ErrChannelOpen       = errors.New("webrtc: channel cannot be opened")
	ErrStaleSession      = errors.New("webrtc: session was replaced")
	ErrNegotiationFailed = errors.New("webrtc: signaling negotiation failed")
)
