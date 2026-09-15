package machine

import (
	"container/heap"
	"log/slog"
	"sync"

	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/schema"
)

// ConnectionManager assigns each session to the least-loaded active
// connection, then keeps that assignment until the session or connection is
// removed. Connections are added as dynamic pool WebSockets are accepted.
//
// Store must be called after a connection becomes active, and Remove must be
// called from its close path. Connection writes are deliberately performed
// outside this manager's lock.
type ConnectionManager struct {
	mu sync.Mutex

	nextID      uint64
	sm          *engine.SessionManager
	connections map[*Connection]*heapConnection
	routes      map[protocol.SessionKey]*heapConnection
	active      connectionMinHeap
}

type heapConnection struct {
	id       uint64
	conn     *Connection
	load     int
	index    int
	sessions map[protocol.SessionKey]struct{}
}

// NewConnectionManager creates an empty dynamic connection pool and registers
// its session lifecycle observer.
func NewConnectionManager(sm *engine.SessionManager) *ConnectionManager {
	m := &ConnectionManager{
		sm:          sm,
		connections: make(map[*Connection]*heapConnection),
		routes:      make(map[protocol.SessionKey]*heapConnection),
	}
	sm.AddDeletedObserver(m.onSessionDelete)
	return m
}

// NewConnection creates a connection whose lifecycle is managed by this pool.
func (m *ConnectionManager) NewConnection() *Connection {
	return NewConnection(m, m.handleMessage)
}

// Store adds an active connection to the allocation heap. Storing the same
// connection more than once is a no-op.
func (m *ConnectionManager) Store(conn *Connection) {
	if conn == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.connections[conn]; exists {
		return
	}

	m.nextID++
	item := &heapConnection{
		id:       m.nextID,
		conn:     conn,
		index:    len(m.active),
		sessions: make(map[protocol.SessionKey]struct{}),
	}
	m.connections[conn] = item
	heap.Push(&m.active, item)
	machinePoolMetrics.connectionsActive.Add(machinePoolMetricContext, 1)
}

// Select returns the existing route for key. A previously unseen key is
// assigned to the least-loaded active connection.
func (m *ConnectionManager) Select(key protocol.SessionKey) *Connection {
	m.mu.Lock()
	defer m.mu.Unlock()

	if item, ok := m.routes[key]; ok {
		if item.conn.State() == protocol.ConnectionActive {
			return item.conn
		}
		m.removeConnectionLocked(item)
	}

	for len(m.active) > 0 {
		item := m.active[0]
		if item.conn.State() != protocol.ConnectionActive {
			m.removeConnectionLocked(item)
			continue
		}

		m.routes[key] = item
		item.sessions[key] = struct{}{}
		item.load++
		machinePoolMetrics.sessionsRouted.Add(machinePoolMetricContext, 1)
		heap.Fix(&m.active, item.index)
		return item.conn
	}
	return nil
}

// Lookup returns an existing active route without allocating a new one.
func (m *ConnectionManager) Lookup(key protocol.SessionKey) *Connection {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.routes[key]
	if !ok {
		return nil
	}
	if item.conn.State() == protocol.ConnectionActive {
		return item.conn
	}
	m.removeConnectionLocked(item)
	return nil
}

// Release removes key's route and makes its connection available for another
// assignment. It is safe to call more than once.
func (m *ConnectionManager) Release(key protocol.SessionKey) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.routes[key]
	if !ok {
		return
	}
	m.releaseLocked(key, item)
}

// Remove evicts conn and all session routes assigned to it. Existing callers
// holding conn must observe its closed state before attempting another write.
func (m *ConnectionManager) Remove(conn *Connection) {
	if conn == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.connections[conn]
	if !ok {
		return
	}
	m.removeConnectionLocked(item)
}

func (m *ConnectionManager) releaseLocked(key protocol.SessionKey, item *heapConnection) {
	if m.routes[key] != item {
		return
	}
	delete(m.routes, key)
	delete(item.sessions, key)
	if item.load > 0 {
		item.load--
		machinePoolMetrics.sessionsRouted.Add(machinePoolMetricContext, -1)
		heap.Fix(&m.active, item.index)
	}
}

func (m *ConnectionManager) removeConnectionLocked(item *heapConnection) {
	if m.connections[item.conn] != item {
		return
	}

	delete(m.connections, item.conn)
	machinePoolMetrics.connectionsActive.Add(machinePoolMetricContext, -1)
	if item.index >= 0 {
		heap.Remove(&m.active, item.index)
	}
	for key := range item.sessions {
		if m.routes[key] == item {
			delete(m.routes, key)
			machinePoolMetrics.sessionsRouted.Add(machinePoolMetricContext, -1)
		}
	}
	clear(item.sessions)
}

func (m *ConnectionManager) onSessionDelete(session *engine.Session) {
	packet := protocol.AcquirePacket()
	defer protocol.ReleasePacket(packet)
	packet.Type = protocol.TypeClose

	conn := m.Lookup(session.Key)
	if conn == nil {
		conn = m.Select(session.Key)
	}
	if conn != nil {
		if err := conn.Write(session.Key, packet); err == nil {
			m.Release(session.Key)
			return
		}
		m.Remove(conn)

		if conn = m.Select(session.Key); conn != nil {
			if err := conn.Write(session.Key, packet); err == nil {
				m.Release(session.Key)
				return
			}
			m.Remove(conn)
		}
	}

	m.Release(session.Key)
	slog.Error("failed to notify gateway about session delete: all connections are down", "sessionID", session.Key.String())
}

func (m *ConnectionManager) handleMessage(key protocol.SessionKey, packet *protocol.Packet) {
	session, exists := m.sm.LoadSession(key)
	if !exists {
		slog.Warn("machine dropped pool packet for missing session",
			"session", key, "type", packet.Type, "payloadSize", len(packet.Payload),
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

type connectionMinHeap []*heapConnection

func (h connectionMinHeap) Len() int {
	return len(h)
}

func (h connectionMinHeap) Less(i, j int) bool {
	if h[i].load != h[j].load {
		return h[i].load < h[j].load
	}
	return h[i].id < h[j].id
}

func (h connectionMinHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *connectionMinHeap) Push(value any) {
	item := value.(*heapConnection)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *connectionMinHeap) Pop() any {
	old := *h
	last := len(old) - 1
	item := old[last]
	old[last] = nil
	item.index = -1
	*h = old[:last]
	return item
}
