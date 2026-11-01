package agora

import (
	"encoding/base64"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/wnnce/voce/internal/protocol"
)

// rtmTopic is the single topic every session publishes non-audio packets to.
const rtmTopic = "events"

// messenger owns the process-wide RTM client shared by every binding. The
// client and its login are per process; each binding only adds a stream channel.
//
// mu guards sessions and is held across the SDK calls that use a channel, so a
// publish can never run against a released channel.
type messenger struct {
	client   *rtmClient
	token    string
	loggedIn atomic.Bool

	mu       sync.RWMutex
	sessions map[string]*rtmSession
}

type rtmSession struct {
	channel *rtmStreamChannel
	binding *Binding
	joined  atomic.Bool
}

func newMessenger(appID, userID, token string) *messenger {
	m := &messenger{
		token:    token,
		sessions: make(map[string]*rtmSession),
	}
	client, err := newRTMClient(appID, userID, rtmHandlers{
		OnLogin:           m.onLogin,
		OnJoin:            m.onJoin,
		OnJoinTopic:       m.onJoinTopic,
		OnPublish:         m.onPublish,
		OnLogout:          m.onLogout,
		OnTokenWillExpire: m.onTokenWillExpire,
	})
	if err != nil {
		slog.Error("rtm client create failed", "error", err)
		return nil
	}
	m.client = client
	if err := client.Login(token); err != nil {
		slog.Error("rtm login rejected", "error", err)
	}
	return m
}

// attach creates and joins the session's stream channel. The topic is joined
// from onJoin, because publishing before the channel is joined is rejected.
func (m *messenger) attach(binding *Binding) {
	channel, err := m.client.NewStreamChannel(binding.cred.Channel)
	if err != nil {
		slog.WarnContext(binding.ctx, "rtm stream channel unavailable", "error", err)
		return
	}
	session := &rtmSession{channel: channel, binding: binding}
	m.mu.Lock()
	m.sessions[binding.cred.Channel] = session
	m.mu.Unlock()

	m.join(session)
}

// join joins the stream channel, but only once the client is logged in: a join
// issued before login completes is rejected by the SDK.
func (m *messenger) join(session *rtmSession) {
	if !m.loggedIn.Load() || !session.joined.CompareAndSwap(false, true) {
		return
	}
	if err := session.channel.Join(m.token); err != nil {
		session.joined.Store(false)
		slog.WarnContext(session.binding.ctx, "rtm join rejected", "error", err)
	}
}

// detach leaves the topic and releases the stream channel. The release happens
// under the write lock so no publish can be in flight.
func (m *messenger) detach(binding *Binding) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.sessions[binding.cred.Channel]
	if session == nil {
		return
	}
	delete(m.sessions, binding.cred.Channel)
	session.channel.LeaveTopic()
	session.channel.Release()
}

// publish sends one packet to the session's topic. Packets are base64 encoded so
// the wire bytes never contain the NUL that Packet.Marshal always writes into
// its size field; the client decodes and reuses the normal packet parser.
func (m *messenger) publish(binding *Binding, packet *protocol.Packet) {
	if !m.loggedIn.Load() || !binding.rtmReady.Load() {
		return
	}

	encoded := base64.StdEncoding.EncodeToString(packet.Marshal())
	m.mu.RLock()
	defer m.mu.RUnlock()
	session := m.sessions[binding.cred.Channel]
	if session == nil {
		return
	}
	if err := session.channel.PublishTopicMessage(encoded); err != nil {
		slog.WarnContext(binding.ctx, "rtm publish rejected", "type", packet.Type, "error", err)
	}
}

func (m *messenger) close() {
	if m.client == nil {
		return
	}
	m.client.Logout()
	m.client.Release()
}

func (m *messenger) session(channel string) *rtmSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[channel]
}

func (m *messenger) onLogin(code int) {
	if code != 0 {
		slog.Error("rtm login failed", "code", code)
		return
	}
	m.loggedIn.Store(true)
	slog.Info("rtm logged in")

	// Sessions attached before the login finished joined nothing yet.
	m.mu.RLock()
	pending := make([]*rtmSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		pending = append(pending, session)
	}
	m.mu.RUnlock()
	for _, session := range pending {
		m.join(session)
	}
}

func (m *messenger) onJoin(channel string, code int) {
	session := m.session(channel)
	if session == nil {
		return
	}
	if code != 0 {
		slog.WarnContext(session.binding.ctx, "rtm channel join failed", "code", code)
		return
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := session.channel.JoinTopic(rtmTopic); err != nil {
		slog.WarnContext(session.binding.ctx, "rtm topic join rejected", "error", err)
	}
}

func (m *messenger) onJoinTopic(channel string, _ string, code int) {
	session := m.session(channel)
	if session == nil {
		return
	}
	if code != 0 {
		slog.WarnContext(session.binding.ctx, "rtm topic join failed", "code", code)
		return
	}
	session.binding.rtmReady.Store(true)
}

func (m *messenger) onPublish(channel string, _ string, code int) {
	if code != 0 {
		slog.Warn("rtm publish failed", "channel", channel, "code", code)
	}
}

func (m *messenger) onLogout(code int) {
	if code != 0 {
		slog.Warn("rtm logout failed", "code", code)
	}
}

func (m *messenger) onTokenWillExpire(channel string) {
	slog.Warn("rtm token about to expire", "channel", channel)
}
