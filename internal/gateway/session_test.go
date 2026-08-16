package gateway

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/pkg/syncx"
)

func TestSessionManagerDeleteUnbindsPool(t *testing.T) {
	pool := &testDataConnectionPool{}
	key := testSessionKey(1)
	machine := &Machine{
		Pool:     pool,
		sessions: map[protocol.SessionKey]struct{}{key: {}},
	}
	sm := &SessionManager{
		shards: syncx.NewShardedMap[protocol.SessionKey, *Session](64, sessionHash),
	}
	sm.Store(NewSession(key, newSessionBinding(nil), machine))
	sm.Delete(key)

	assert.Equal(t, []protocol.SessionKey{key}, pool.unbound)
	assert.Empty(t, machine.sessions)
}

func TestSessionManagerDispatchCloseCleansGatewayResources(t *testing.T) {
	pool := &testDataConnectionPool{}
	key := testSessionKey(2)
	machine := &Machine{
		Pool:     pool,
		sessions: map[protocol.SessionKey]struct{}{key: {}},
	}
	sm := &SessionManager{
		shards: syncx.NewShardedMap[protocol.SessionKey, *Session](64, sessionHash),
	}
	session := NewSession(key, newSessionBinding(nil), machine)
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
	assert.Equal(t, []protocol.SessionKey{key}, pool.unbound)
	assert.Empty(t, machine.sessions)
}

func sessionHash(key protocol.SessionKey) uint64 {
	return binary.BigEndian.Uint64(key[:8]) ^ binary.BigEndian.Uint64(key[8:])
}

type testDataConnectionPool struct {
	unbound []protocol.SessionKey
}

func (p *testDataConnectionPool) Bind(protocol.SessionKey) *SessionBinding {
	return newSessionBinding(nil)
}

func (p *testDataConnectionPool) Unbind(key protocol.SessionKey) {
	p.unbound = append(p.unbound, key)
}

func (p *testDataConnectionPool) Shutdown() {}

func (p *testDataConnectionPool) Snapshots() []ConnectionPoolSnapshot { return nil }
