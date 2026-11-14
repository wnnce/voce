package agora

import "errors"

// Attach failures are part of the HTTP contract: callers map them to status
// codes, so each one has to name a distinct cause.
var (
	// ErrInvalidConfig means the service cannot start with the given settings.
	ErrInvalidConfig = errors.New("rtc: invalid config")
	// ErrInvalidSession means Attach was called without a session.
	ErrInvalidSession = errors.New("rtc: invalid session")
	// ErrServiceClosed means Attach was called after Close.
	ErrServiceClosed = errors.New("rtc: service closed")
	// ErrStaleSession means the caller holds a session object that is no longer
	// the one mapped under that key.
	ErrStaleSession = errors.New("rtc: session object is stale")
	// ErrBusy means the session lease is already owned by another connection.
	ErrBusy = errors.New("rtc: session is busy")
	// ErrEngineInit means the process-level SDK engine could not be started.
	ErrEngineInit = errors.New("rtc: engine init failed")
	// ErrChannelOpen means the channel could not be opened or joined.
	ErrChannelOpen = errors.New("rtc: open channel failed")
)
