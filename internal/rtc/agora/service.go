package agora

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	agoraservice "github.com/AgoraIO-Extensions/Agora-Golang-Server-SDK/v2/go_sdk/rtc"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/pkg/buf"
)

// Service owns one Agora connection per session. Attach is idempotent per
// session and Close releases everything that is still attached.
type Service struct {
	cfg    Config
	tokens tokenBuilder
	sm     *engine.SessionManager

	// The Agora service is process-wide and the SDK only guards it with a plain
	// bool, so the first Attach has to win this race exactly once.
	initOnce sync.Once
	initErr  error

	pool *buf.ChunkPool
	rtm  *messenger

	mu         sync.Mutex
	closed     bool
	bindings   map[protocol.SessionKey]*Binding
	observerID int
}

// NewService prepares the RTC attach service. The Agora SDK is initialized on
// the first attach, so a broken AppID fails that request instead of the start.
func NewService(cfg Config, sm *engine.SessionManager) (*Service, error) {
	if sm == nil {
		return nil, fmt.Errorf("%w: session manager is nil", ErrInvalidConfig)
	}
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	service := &Service{
		cfg: cfg,
		sm:  sm,
		tokens: tokenBuilder{
			appID:           cfg.AppID,
			appCertificate:  cfg.AppCertificate,
			tokenExpire:     cfg.TokenExpire,
			privilegeExpire: cfg.PrivilegeExpire,
			rtmTokenExpire:  cfg.RTMTokenExpire,
		},
		pool:     buf.NewChunkPool(audioBlockSize),
		bindings: make(map[protocol.SessionKey]*Binding),
	}
	service.observerID = sm.AddDeletedObserver(service.onSessionDeleted)
	slog.Info("rtc service started", "server_uid", cfg.ServerUID)
	return service, nil
}

// Attach joins the session's channel and returns the credentials a client needs
// to join it. An already attached session returns its existing credentials.
func (s *Service) Attach(ctx context.Context, session *engine.Session) (Credentials, error) {
	if session == nil {
		return Credentials{}, fmt.Errorf("%w: session is nil", ErrInvalidSession)
	}

	s.mu.Lock()
	if existing, ok := s.bindings[session.Key]; ok {
		credentials := existing.cred
		s.mu.Unlock()
		return credentials, nil
	}
	s.mu.Unlock()

	// The session lease is what keeps two connections off one session.
	lease, err := session.Bind()
	if err != nil {
		return Credentials{}, fmt.Errorf("%w: %w", ErrBusy, err)
	}
	attached, err := s.connect(session, lease)
	if err != nil {
		session.Unbind(lease, false)
		return Credentials{}, err
	}
	// Installing the writer starts the session's single output loop, which
	// forwards every packet to Binding.Write.
	if !session.SetWriter(lease, attached.Write) {
		attached.close(false)
		return Credentials{}, ErrChannelOpen
	}

	// The service may have closed, or the session been deleted, while the
	// channel was joining. Such a binding would never be released, so it is
	// never stored; closing it is the whole point of this check.
	s.mu.Lock()
	_, alive := s.sm.LoadSession(session.Key)
	usable := !s.closed && alive
	if usable {
		s.bindings[session.Key] = attached
	}
	s.mu.Unlock()

	if !usable {
		attached.close(false)
		return Credentials{}, ErrServiceClosed
	}
	if err := session.Activate(lease); err != nil {
		attached.close(false)
		return Credentials{}, err
	}
	return attached.cred, nil
}

// Close releases every attached session. It is safe to call more than once.
func (s *Service) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	attached := make([]*Binding, 0, len(s.bindings))
	for _, b := range s.bindings {
		attached = append(attached, b)
	}
	clear(s.bindings)
	s.mu.Unlock()

	s.sm.RemoveDeletedObserver(s.observerID)
	for _, b := range attached {
		b.close(true)
	}
	if s.rtm != nil {
		s.rtm.close()
	}
	slog.Info("rtc service closed")
}

// connect signs both sides' tokens and joins the channel as the server account.
func (s *Service) connect(session *engine.Session, lease engine.Lease) (*Binding, error) {
	if err := s.initialize(); err != nil {
		return nil, err
	}

	channel := s.cfg.ChannelName(session.Key)
	server, err := s.tokens.build(channel, s.cfg.ServerUID)
	if err != nil {
		return nil, err
	}
	conn := agoraservice.NewRtcConnection(&agoraservice.RtcConnectionConfig{
		AutoSubscribeAudio: true,
		AutoSubscribeVideo: false,
		ClientRole:         agoraservice.ClientRoleBroadcaster,
		ChannelProfile:     agoraservice.ChannelProfileLiveBroadcasting,
	}, &agoraservice.RtcConnectionPublishConfig{})
	if conn == nil {
		return nil, fmt.Errorf("%w: create rtc connection returned nil", ErrChannelOpen)
	}
	if code := conn.Connect(server.RTCToken, channel, s.cfg.ServerUID, ""); code != 0 {
		conn.Release()
		return nil, fmt.Errorf("%w: connect returned code %d", ErrChannelOpen, code)
	}

	credentials, err := s.tokens.build(channel, s.cfg.ClientUID(session.Key))
	if err != nil {
		conn.Disconnect()
		conn.Release()
		return nil, err
	}
	return New(session, lease, credentials, conn, s.pool, s.rtm), nil
}

// initialize starts the process-level Agora service once. A failure is kept: a
// half-initialized SDK must not look like a per-request problem.
func (s *Service) initialize() error {
	s.initOnce.Do(func() {
		svcCfg := agoraservice.NewAgoraServiceConfig()
		svcCfg.AppId = s.cfg.AppID
		// Credentials carry the account name, so the SDK has to expect string
		// user ids instead of the default uint32 form.
		svcCfg.UseStringUid = true
		if code := agoraservice.Initialize(svcCfg); code != 0 {
			s.initErr = fmt.Errorf("%w: initialize returned code %d", ErrEngineInit, code)
			slog.Error("agora service initialize failed", "code", code)
			return
		}

		// RTM carries the non-audio packets. It is independent of the RTC engine,
		// so a failure here only costs control messages.
		token, err := s.tokens.rtmToken(s.cfg.ServerUID)
		if err != nil {
			slog.Error("rtm token build failed", "error", err)
			return
		}
		s.rtm = newMessenger(s.cfg.AppID, s.cfg.ServerUID, token)
		if s.rtm == nil {
			slog.Warn("rtm client unavailable, control packets will be dropped")
		}
	})
	return s.initErr
}

// onSessionDeleted runs before the workflow is stopped, so it only releases the
// connection and the lease and never waits on the workflow.
func (s *Service) onSessionDeleted(session *engine.Session) {
	s.mu.Lock()
	attached, ok := s.bindings[session.Key]
	if !ok || attached.session != session {
		s.mu.Unlock()
		return
	}
	delete(s.bindings, session.Key)
	s.mu.Unlock()

	attached.close(false)
}
