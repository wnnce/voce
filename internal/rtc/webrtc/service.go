package webrtc

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/rtc/webrtc/media"
	"github.com/wnnce/voce/pkg/buf"
)

// Service coordinates WebRTC sessions using Pion and a pluggable signaling Provider.
// Attach is idempotent per session and Close releases all active bindings.
type Service struct {
	registry *Registry
	sm       *engine.SessionManager
	pool     *buf.ChunkPool
	media    *media.Factory

	mu         sync.Mutex
	closed     atomic.Bool
	bindings   map[protocol.SessionKey]*Binding
	observerID int
}

// Registry returns the underlying WebRTC provider registry.
func (s *Service) Registry() *Registry {
	return s.registry
}

// NewService creates a new WebRTC transport service with the given signaling provider registry.
func NewService(registry *Registry, sm *engine.SessionManager) (*Service, error) {
	if registry == nil || registry.Len() == 0 {
		return nil, fmt.Errorf("%w: provider registry is empty", ErrInvalidConfig)
	}
	if sm == nil {
		return nil, fmt.Errorf("%w: session manager is nil", ErrInvalidSession)
	}

	mediaFactory, err := media.NewFactory()
	if err != nil {
		return nil, err
	}

	service := &Service{
		registry: registry,
		sm:       sm,
		pool:     buf.NewChunkPool(audioBlockSize),
		media:    mediaFactory,
		bindings: make(map[protocol.SessionKey]*Binding),
	}

	service.observerID = sm.AddDeletedObserver(service.onSessionDeleted)
	slog.Info("webrtc rtc service started", "providers_count", registry.Len(), "default_provider", registry.Default())
	return service, nil
}

// NewServiceWithProvider wraps a single provider in a Registry and initializes Service.
func NewServiceWithProvider(provider Provider, sm *engine.SessionManager) (*Service, error) {
	reg := NewRegistry()
	if provider != nil {
		_ = reg.Register(provider)
	}
	return NewService(reg, sm)
}

// Attach negotiates a WebRTC connection via the signaling Provider and returns client credentials.
// An already attached session returns its existing credentials.
func (s *Service) Attach(ctx context.Context, session *engine.Session, providerName ...string) (any, error) {
	if session == nil {
		return nil, fmt.Errorf("%w: session is nil", ErrInvalidSession)
	}

	if s.closed.Load() {
		return nil, ErrServiceClosed
	}

	var targetProvider string
	if len(providerName) > 0 {
		targetProvider = providerName[0]
	}
	provider, ok := s.registry.Get(targetProvider)
	if !ok {
		return nil, fmt.Errorf("%w: unknown or unavailable webrtc provider %q", ErrInvalidConfig, targetProvider)
	}

	s.mu.Lock()
	if existing, ok := s.bindings[session.Key]; ok {
		if !existing.closed.Load() {
			cred := existing.cred
			s.mu.Unlock()
			return cred, nil
		}
		delete(s.bindings, session.Key)
	}
	s.mu.Unlock()

	lease, err := session.Bind()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBusy, err)
	}

	attached, cred, err := s.connect(ctx, session, lease, provider)
	if err != nil {
		session.Unbind(lease, false)
		return nil, err
	}

	if !session.SetWriter(lease, attached.Write) {
		attached.close(false)
		return nil, ErrChannelOpen
	}

	s.mu.Lock()
	if s.closed.Load() {
		s.mu.Unlock()
		attached.close(false)
		return nil, ErrServiceClosed
	}
	if attached.closed.Load() {
		s.mu.Unlock()
		attached.close(false)
		return nil, ErrChannelOpen
	}
	if _, ok := s.sm.LoadSession(session.Key); !ok {
		s.mu.Unlock()
		attached.close(false)
		return nil, ErrStaleSession
	}
	s.bindings[session.Key] = attached
	s.mu.Unlock()

	return cred, nil
}

func (s *Service) connect(ctx context.Context, session *engine.Session, lease engine.Lease, provider Provider) (*Binding, any, error) {
	iceServers, err := provider.ICEServers(ctx, session.Key)
	if err != nil {
		return nil, nil, fmt.Errorf("webrtc: provider ICE configuration: %w", err)
	}
	mediaSession, err := s.media.NewSession(iceServers)
	if err != nil {
		return nil, nil, err
	}
	offer, err := mediaSession.CreateOffer(ctx)
	if err != nil {
		mediaSession.Close()
		return nil, nil, fmt.Errorf("webrtc: create offer: %w", err)
	}

	result, err := provider.Negotiate(ctx, NegotiateRequest{
		SessionKey: session.Key,
		OfferSDP:   offer,
	})
	if err != nil {
		mediaSession.Close()
		return nil, nil, fmt.Errorf("%w: provider negotiate: %w", ErrNegotiationFailed, err)
	}
	if result == nil {
		mediaSession.Close()
		return nil, nil, fmt.Errorf("%w: provider returned empty negotiation result", ErrNegotiationFailed)
	}

	encoder, err := media.NewAudioEncoder()
	if err != nil {
		mediaSession.Close()
		_ = provider.Teardown(context.Background(), result.Handle)
		return nil, nil, fmt.Errorf("webrtc: init audio encoder: %w", err)
	}

	var binding *Binding
	binding = newBinding(session, lease, result.Credentials, mediaSession, provider, result.Handle, s.pool, encoder,
		func() { s.removeBinding(session.Key, binding) },
		func() { s.sm.RemoveSession(session.Key) },
	)
	if err = mediaSession.SetAnswer(result.AnswerSDP); err != nil {
		binding.close(false)
		return nil, nil, err
	}

	return binding, result.Credentials, nil
}

func (s *Service) removeBinding(key protocol.SessionKey, binding *Binding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.bindings[key]; ok && current == binding {
		delete(s.bindings, key)
	}
}

func (s *Service) onSessionDeleted(session *engine.Session) {
	if session == nil {
		return
	}
	s.mu.Lock()
	binding, ok := s.bindings[session.Key]
	if ok {
		delete(s.bindings, session.Key)
	}
	s.mu.Unlock()

	if ok && binding != nil {
		binding.close(false)
	}
}

// Close terminates all active WebRTC session bindings and releases observers.
// Active workflows are paused so session state is retained.
func (s *Service) Close() {
	if !s.closed.CompareAndSwap(false, true) {
		return
	}

	s.mu.Lock()
	bindings := s.bindings
	s.bindings = make(map[protocol.SessionKey]*Binding)
	s.mu.Unlock()

	s.sm.RemoveDeletedObserver(s.observerID)
	for _, b := range bindings {
		b.close(true)
	}
}
