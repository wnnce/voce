package machine

import (
	"log/slog"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
)

const (
	offset32 = 2166136261
	prime32  = 16777619
)

// ConnectionManager manages a pool of gateway connections on the machine side.
// It matches the pool structure on the gateway side to ensure session affinity.
type ConnectionManager struct {
	slots []*Connection
	sm    *engine.SessionManager
}

// NewConnectionManager initializes a new ConnectionManager and sets up session deletion monitoring.
func NewConnectionManager(sm *engine.SessionManager, size int) *ConnectionManager {
	cm := &ConnectionManager{
		sm: sm,
	}
	slots := make([]*Connection, size)
	for i := 0; i < size; i++ {
		slots[i] = NewConnection(cm.handleMessage)
	}
	cm.slots = slots
	sm.AddDeletedObserver(cm.onSessionDelete)
	return cm
}

func (m *ConnectionManager) Load(index int) *Connection {
	if index < 0 || index >= len(m.slots) {
		return nil
	}
	return m.slots[index]
}

func (m *ConnectionManager) Select(key protocol.SessionKey) *Connection {
	if len(m.slots) == 0 {
		return nil
	}

	hash := uint32(offset32)
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= prime32
	}

	idx := hash % uint32(len(m.slots))
	return m.slots[idx]
}

func (m *ConnectionManager) onSessionDelete(session *engine.Session) {
	packet := protocol.AcquirePacket()
	defer protocol.ReleasePacket(packet)
	packet.Type = protocol.TypeClose

	primary := m.Select(session.Key)
	if primary != nil && primary.State() == protocol.ConnectionActive {
		if err := primary.Write(session.Key, packet); err == nil {
			return
		}
	}

	for _, conn := range m.slots {
		if conn.State() == protocol.ConnectionActive {
			if err := conn.Write(session.Key, packet); err == nil {
				return
			}
		}
	}

	slog.Error("failed to notify gateway about session delete: all connections are down", "sessionID", session.Key.String())
}

func (m *ConnectionManager) handleMessage(key protocol.SessionKey, packet *protocol.Packet) {
	session, exist := m.sm.LoadSession(key)
	if !exist {
		slog.Warn("machine dropped pool packet for missing session",
			"session", key, "type", machinePacketTypeName(packet.Type), "payloadSize", len(packet.Payload),
		)
		return
	}
	session.UpdateActivity()
	switch packet.Type {
	case protocol.TypeAudio:
		audio := schema.NewAudio("audio", engine.AudioSampleRate, engine.AudioChannels)
		audio.SetBytes(packet.Payload)
		if err := session.Workflow.SendToHead(audio.ReadOnly()); err != nil {
			slog.Error("send audio to workflow failed", "error", err)
			audio.Release()
		}
	case protocol.TypePause:
		if err := session.Workflow.Pause(); err != nil {
			slog.Error("pause workflow failed", "error", err)
		}
	case protocol.TypeResume:
		if err := session.Workflow.Resume(); err != nil {
			slog.Error("resume workflow failed", "error", err)
		}
	case protocol.TypeClose:
		m.sm.RemoveSession(session.Key)
	}
}
