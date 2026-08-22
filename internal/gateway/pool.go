package gateway

import (
	"container/heap"
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lesismal/nbio/nbhttp"
	"github.com/wnnce/voce/internal/protocol"
)

const (
	defaultMinConnections              = 1
	defaultTargetSessionsPerConnection = 16
	defaultMaxSessionsPerConnection    = 64
	defaultIdleTimeout                 = 30 * time.Second
	defaultCleanupInterval             = 5 * time.Second
)

// ConnectionPoolConfig controls one machine's data-plane connection pool.
type ConnectionPoolConfig struct {
	MinConnections              int
	TargetSessionsPerConnection int
	MaxSessionsPerConnection    int
	MaxConnections              int
	IdleTimeout                 time.Duration
	CleanupInterval             time.Duration
}

// SessionBinding is the session-local handle used on the data-plane hot path.
// Dynamic pools may keep the same Connection object through physical reconnects.
type SessionBinding struct {
	conn atomic.Pointer[Connection]
}

func newSessionBinding(conn *Connection) *SessionBinding {
	binding := &SessionBinding{}
	binding.conn.Store(conn)
	return binding
}

func (b *SessionBinding) Connection() *Connection {
	if b == nil {
		return nil
	}
	return b.conn.Load()
}

type ConnectionPoolSnapshot struct {
	ID        uint64                   `json:"id,omitempty"`
	State     protocol.ConnectionState `json:"state"`
	Sessions  int                      `json:"sessions"`
	IdleSince int64                    `json:"idle_since,omitempty"`
}

func NewConnectionPool(
	ctx context.Context,
	engine *nbhttp.Engine,
	machineID, address string,
	cfg ConnectionPoolConfig,
	dispatcher MessageDispatcher,
) (*ConnectionPool, error) {
	if engine == nil {
		return nil, ErrNilNBHTTPEngine
	}
	if cfg.MinConnections <= 0 {
		cfg.MinConnections = defaultMinConnections
	}
	p := newConnectionPool(ctx, engine, machineID, address, cfg, dispatcher)
	p.startMinConnections()
	return p, nil
}

// ConnectionPool owns one machine's dynamic data-plane connections and session bindings.
type ConnectionPool struct {
	ctx        context.Context
	cancel     context.CancelFunc
	engine     *nbhttp.Engine
	machineID  string
	address    string
	dispatcher MessageDispatcher

	minConns       int
	targetSessions int
	maxSessions    int
	maxConns       int
	idleTimeout    time.Duration
	cleanup        time.Duration

	mu            sync.Mutex
	nextID        uint64
	connections   map[*Connection]*pooledConnection
	routes        map[protocol.SessionKey]*pooledConnection
	bindings      map[protocol.SessionKey]*SessionBinding
	active        connectionMinHeap
	closed        atomic.Bool
	pendingDials  int
	newConnection func() (*Connection, error)
	connect       func(*Connection) error
}

type pooledConnection struct {
	id        uint64
	conn      *Connection
	load      int
	index     int
	idleSince time.Time
	sessions  map[protocol.SessionKey]struct{}
}

func newConnectionPool(
	parent context.Context,
	engine *nbhttp.Engine,
	machineID, address string,
	cfg ConnectionPoolConfig,
	dispatcher MessageDispatcher,
) *ConnectionPool {
	ctx, cancel := context.WithCancel(parent)
	p := &ConnectionPool{
		ctx:            ctx,
		cancel:         cancel,
		engine:         engine,
		machineID:      machineID,
		address:        address,
		dispatcher:     dispatcher,
		minConns:       cfg.MinConnections,
		targetSessions: cfg.TargetSessionsPerConnection,
		maxSessions:    cfg.MaxSessionsPerConnection,
		maxConns:       cfg.MaxConnections,
		idleTimeout:    cfg.IdleTimeout,
		cleanup:        cfg.CleanupInterval,
		connections:    make(map[*Connection]*pooledConnection),
		routes:         make(map[protocol.SessionKey]*pooledConnection),
		bindings:       make(map[protocol.SessionKey]*SessionBinding),
	}
	if p.maxSessions <= 0 {
		p.maxSessions = defaultMaxSessionsPerConnection
	}
	if p.targetSessions <= 0 {
		p.targetSessions = defaultTargetSessionsPerConnection
	}
	if p.targetSessions > p.maxSessions {
		p.targetSessions = p.maxSessions
	}
	if p.idleTimeout <= 0 {
		p.idleTimeout = defaultIdleTimeout
	}
	if p.cleanup <= 0 {
		p.cleanup = defaultCleanupInterval
	}
	p.newConnection = func() (*Connection, error) {
		return newPoolConnection(p.ctx, p.engine, p.machineID, p.address, p.dispatcher)
	}
	p.connect = func(conn *Connection) error {
		return conn.Connect()
	}
	go p.cleanupLoop()
	return p
}

func (p *ConnectionPool) Bind(key protocol.SessionKey) *SessionBinding {
	p.mu.Lock()
	if binding, ok := p.bindings[key]; ok {
		p.mu.Unlock()
		return binding
	}

	binding := newSessionBinding(nil)
	p.bindings[key] = binding
	if item := p.selectActiveLocked(); item != nil {
		p.attachLocked(key, binding, item)
		startDial := p.reserveScaleLocked()
		p.mu.Unlock()
		if startDial {
			go p.startDial()
		}
		return binding
	}
	if item := p.selectConnectingLocked(); item != nil {
		p.attachLocked(key, binding, item)
		p.mu.Unlock()
		return binding
	}
	if !p.reserveDialLocked() {
		p.mu.Unlock()
		return binding
	}
	p.mu.Unlock()
	go p.startDial()
	return binding
}

func (p *ConnectionPool) Unbind(key protocol.SessionKey) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.bindings, key)
	item, ok := p.routes[key]
	if !ok {
		return
	}
	delete(p.routes, key)
	delete(item.sessions, key)
	if item.load > 0 {
		item.load--
		heap.Fix(&p.active, item.index)
	}
	if item.load == 0 {
		item.idleSince = time.Now()
	}
	p.assignPendingLocked()
}

func (p *ConnectionPool) Shutdown() {
	if p.closed.Swap(true) {
		return
	}
	p.cancel()

	p.mu.Lock()
	connections := make([]*Connection, 0, len(p.connections))
	for conn := range p.connections {
		connections = append(connections, conn)
	}
	clear(p.connections)
	clear(p.routes)
	clear(p.bindings)
	p.active = nil
	p.mu.Unlock()

	for _, conn := range connections {
		conn.Close()
	}
}

func (p *ConnectionPool) Snapshots() []ConnectionPoolSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()

	snapshots := make([]ConnectionPoolSnapshot, 0, len(p.connections))
	for _, item := range p.connections {
		idleSince := int64(0)
		if !item.idleSince.IsZero() {
			idleSince = item.idleSince.UnixMilli()
		}
		snapshots = append(snapshots, ConnectionPoolSnapshot{
			ID:        item.id,
			State:     item.conn.State(),
			Sessions:  item.load,
			IdleSince: idleSince,
		})
	}
	return snapshots
}

func (p *ConnectionPool) startMinConnections() {
	p.mu.Lock()
	starts := 0
	for len(p.connections)+p.pendingDials < p.minConns {
		p.pendingDials++
		starts++
	}
	p.mu.Unlock()
	for range starts {
		go p.startDial()
	}
}

func (p *ConnectionPool) reserveScaleLocked() bool {
	if p.pendingDials > 0 {
		return false
	}
	item := p.selectActiveLocked()
	if item == nil || item.load < p.targetSessions {
		return false
	}
	return p.reserveDialLocked()
}

func (p *ConnectionPool) reserveDialLocked() bool {
	if p.pendingDials > 0 || p.closed.Load() || (p.maxConns > 0 && len(p.connections)+p.pendingDials >= p.maxConns) {
		return false
	}
	p.pendingDials++
	return true
}

func (p *ConnectionPool) startDial() {
	conn, err := p.newConnection()
	if err != nil {
		p.mu.Lock()
		p.pendingDials--
		p.mu.Unlock()
		return
	}

	p.mu.Lock()
	p.pendingDials--
	if p.closed.Load() {
		p.mu.Unlock()
		conn.Close()
		return
	}
	p.nextID++
	item := &pooledConnection{
		id:       p.nextID,
		conn:     conn,
		index:    len(p.active),
		sessions: make(map[protocol.SessionKey]struct{}),
	}
	p.connections[conn] = item
	heap.Push(&p.active, item)
	p.assignPendingLocked()
	p.mu.Unlock()

	if err = p.connect(conn); err != nil {
		go conn.reconnectLoop()
	}
}

func (p *ConnectionPool) selectActiveLocked() *pooledConnection {
	p.removeClosedLocked()
	return p.selectByStateLocked(protocol.ConnectionActive)
}

func (p *ConnectionPool) selectConnectingLocked() *pooledConnection {
	p.removeClosedLocked()
	return p.selectByStateLocked(protocol.ConnectionConnecting)
}

func (p *ConnectionPool) selectByStateLocked(state protocol.ConnectionState) *pooledConnection {
	var selected *pooledConnection
	for _, item := range p.active {
		if item.conn.State() != state || item.load >= p.maxSessions {
			continue
		}
		if selected == nil || item.load < selected.load || (item.load == selected.load && item.id < selected.id) {
			selected = item
		}
	}
	return selected
}

func (p *ConnectionPool) removeClosedLocked() {
	for _, item := range p.connections {
		if item.conn.State() == protocol.ConnectionClosed {
			p.removeLocked(item)
		}
	}
}

func (p *ConnectionPool) attachLocked(key protocol.SessionKey, binding *SessionBinding, item *pooledConnection) {
	p.routes[key] = item
	item.sessions[key] = struct{}{}
	item.load++
	item.idleSince = time.Time{}
	binding.conn.Store(item.conn)
	heap.Fix(&p.active, item.index)
}

func (p *ConnectionPool) assignPendingLocked() {
	for key, binding := range p.bindings {
		if _, assigned := p.routes[key]; assigned {
			continue
		}
		item := p.selectActiveLocked()
		if item == nil {
			item = p.selectConnectingLocked()
		}
		if item == nil {
			return
		}
		p.attachLocked(key, binding, item)
	}
}

func (p *ConnectionPool) removeLocked(item *pooledConnection) {
	if p.connections[item.conn] != item {
		return
	}
	delete(p.connections, item.conn)
	if item.index >= 0 {
		heap.Remove(&p.active, item.index)
	}
	for key := range item.sessions {
		if p.routes[key] == item {
			delete(p.routes, key)
			if binding := p.bindings[key]; binding != nil {
				binding.conn.Store(nil)
			}
		}
	}
	clear(item.sessions)
}

func (p *ConnectionPool) cleanupLoop() {
	ticker := time.NewTicker(p.cleanup)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case now := <-ticker.C:
			p.cleanupIdle(now)
		}
	}
}

func (p *ConnectionPool) cleanupIdle(now time.Time) {
	p.mu.Lock()
	connections := make([]*Connection, 0)
	for _, item := range p.connections {
		if len(p.connections) > p.minConns && item.load == 0 && !item.idleSince.IsZero() && now.Sub(item.idleSince) >= p.idleTimeout {
			p.removeLocked(item)
			connections = append(connections, item.conn)
		}
	}
	p.mu.Unlock()

	for _, conn := range connections {
		conn.Close()
	}
}

type connectionMinHeap []*pooledConnection

func (h connectionMinHeap) Len() int { return len(h) }

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
	item := value.(*pooledConnection)
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
