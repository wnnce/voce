package gateway

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lesismal/nbio/nbhttp/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/pkg/syncx"
)

func TestSessionAcquireAndRelease(t *testing.T) {
	session := NewSession(testSessionKey(1), nil, nil)
	assert.Equal(t, SessionIdle, session.State())
	assert.True(t, session.Acquire())
	assert.Equal(t, SessionPending, session.State())
	assert.False(t, session.Acquire())
	assert.True(t, session.Release())
	assert.Equal(t, SessionIdle, session.State())
	assert.False(t, session.Release())

	session.state.Store(int32(SessionReady))
	assert.True(t, session.Release())
	session.Close()
	assert.False(t, session.Release())
}

func TestSessionOnClientMessageDropsInvalidOrUnavailablePackets(t *testing.T) {
	machine := &Machine{}
	machine.state.Store(int32(MachineStateSuspended))
	session := NewSession(testSessionKey(1), newSessionBinding(nil), machine)
	last := session.lastActiveAt.Load()

	session.OnClientMessage(nil, websocket.TextMessage, []byte("ignored"))
	session.OnClientMessage(nil, websocket.BinaryMessage, []byte{protocol.MagicNumber1})
	session.OnClientMessage(nil, websocket.BinaryMessage, []byte{protocol.MagicNumber1, protocol.MagicNumber2, 0, 0, 0, 0, 0, 1})
	assert.Equal(t, last, session.lastActiveAt.Load())

	machine.state.Store(int32(MachineStateActive))
	packet := &protocol.Packet{Type: protocol.TypeText}
	data := packet.Header()
	session.OnClientMessage(nil, websocket.BinaryMessage, data)
	assert.Equal(t, last, session.lastActiveAt.Load())
}

func TestSessionManagerCleanupRemovesExpiredSession(t *testing.T) {
	p := newTestConnectionPool(4, 0)
	defer p.Shutdown()
	key := testSessionKey(1)
	machine := &Machine{Pool: p, sessions: map[protocol.SessionKey]struct{}{key: {}}}
	sm := newTestSessionManager()
	session := NewSession(key, p.Bind(key), machine)
	session.lastActiveAt.Store(time.Now().Add(-time.Second).UnixMilli())
	sm.Store(session)

	sm.cleanup(time.Millisecond)
	_, exists := sm.Load(key)
	assert.False(t, exists)
	assert.Empty(t, machine.sessions)
	p.mu.Lock()
	_, bound := p.bindings[key]
	p.mu.Unlock()
	assert.False(t, bound)
}

func TestSessionManagerDispatchMessageDropsInvalidOrUnreadyPackets(t *testing.T) {
	sm := newTestSessionManager()
	key := testSessionKey(1)
	session := NewSession(key, nil, nil)
	sm.Store(session)
	defer sm.Delete(key)

	sm.DispatchMessage(testSessionKey(2), []byte("missing"))
	sm.DispatchMessage(key, []byte("invalid"))
	require.Equal(t, SessionIdle, session.State())

	packet := &protocol.Packet{Type: protocol.TypeText}
	sm.DispatchMessage(key, packet.Header())
	_, exists := sm.Load(key)
	assert.True(t, exists)
}

func TestSessionManagerDeleteUnbindsPool(t *testing.T) {
	pool := newTestConnectionPool(1, 0)
	defer pool.Shutdown()
	key := testSessionKey(1)
	machine := &Machine{
		Pool:     pool,
		sessions: map[protocol.SessionKey]struct{}{key: {}},
	}
	sm := newTestSessionManager()
	sm.Store(NewSession(key, pool.Bind(key), machine))
	sm.Delete(key)

	pool.mu.Lock()
	_, bound := pool.bindings[key]
	pool.mu.Unlock()
	assert.False(t, bound)
	assert.Empty(t, machine.sessions)
}

func TestSessionManagerDispatchCloseCleansGatewayResources(t *testing.T) {
	pool := newTestConnectionPool(1, 0)
	defer pool.Shutdown()
	key := testSessionKey(2)
	machine := &Machine{
		Pool:     pool,
		sessions: map[protocol.SessionKey]struct{}{key: {}},
	}
	sm := newTestSessionManager()
	session := NewSession(key, pool.Bind(key), machine)
	session.state.Store(int32(SessionReady))
	sm.Store(session)

	packet := protocol.AcquirePacket()
	defer protocol.ReleasePacket(packet)
	packet.Type = protocol.TypeClose
	data := append(append([]byte(nil), packet.Header()...), packet.Payload...)
	NewHandler(nil, sm).sm.DispatchMessage(key, data)

	assert.Equal(t, SessionClosed, session.State())
	_, exists := sm.Load(key)
	require.False(t, exists)
	pool.mu.Lock()
	_, bound := pool.bindings[key]
	pool.mu.Unlock()
	assert.False(t, bound)
	assert.Empty(t, machine.sessions)
}

func sessionHash(key protocol.SessionKey) uint64 {
	return binary.BigEndian.Uint64(key[:8]) ^ binary.BigEndian.Uint64(key[8:])
}

func newTestSessionManager() *SessionManager {
	return &SessionManager{
		shards: syncx.NewShardedMap[protocol.SessionKey, *Session](64, sessionHash),
	}
}

func BenchmarkSessionManagerRouteAndHeader(b *testing.B) {
	for _, sessionCount := range []int{1, 64, 1024, 65536} {
		b.Run(fmt.Sprintf("sessions=%d", sessionCount), func(b *testing.B) {
			sm, keys := newBenchmarkSessionManager(sessionCount)
			data := benchmarkPacketData(320)

			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				session, ok := sm.Load(keys[i%len(keys)])
				if !ok || session == nil {
					b.Fatal("expected session")
				}
				packet := protocol.AcquirePacket()
				if err := packet.UnmarshalHeader(data); err != nil {
					b.Fatal(err)
				}
				protocol.ReleasePacket(packet)
			}
		})
	}
}

func BenchmarkSessionManagerRouteAndHeaderParallel(b *testing.B) {
	for _, sessionCount := range []int{64, 1024, 65536} {
		b.Run(fmt.Sprintf("sessions=%d", sessionCount), func(b *testing.B) {
			sm, keys := newBenchmarkSessionManager(sessionCount)
			data := benchmarkPacketData(320)
			var next atomic.Uint64

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				index := next.Add(1)
				for pb.Next() {
					session, ok := sm.Load(keys[index%uint64(len(keys))])
					if !ok || session == nil {
						b.Fatal("expected session")
					}
					packet := protocol.AcquirePacket()
					if err := packet.UnmarshalHeader(data); err != nil {
						b.Fatal(err)
					}
					protocol.ReleasePacket(packet)
					index++
				}
			})
		})
	}
}

func newBenchmarkSessionManager(sessionCount int) (*SessionManager, []protocol.SessionKey) {
	sm := newTestSessionManager()
	keys := make([]protocol.SessionKey, sessionCount)
	for i := range sessionCount {
		key := benchmarkSessionKey(uint64(i + 1))
		keys[i] = key
		sm.shards.Store(key, &Session{})
	}
	return sm, keys
}

func benchmarkPacketData(payloadSize int) []byte {
	packet := &protocol.Packet{Type: protocol.TypeAudio}
	packet.SetPayload(make([]byte, payloadSize))
	return packet.Marshal()
}
