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

func defaultConnectionPoolConfig(cfg ConnectionPoolConfig) ConnectionPoolConfig {
	if cfg.MinConnections <= 0 {
		cfg.MinConnections = defaultMinConnections
	}
	if cfg.MaxSessionsPerConnection <= 0 {
		cfg.MaxSessionsPerConnection = defaultMaxSessionsPerConnection
	}
	if cfg.TargetSessionsPerConnection <= 0 {
		cfg.TargetSessionsPerConnection = defaultTargetSessionsPerConnection
	}
	if cfg.TargetSessionsPerConnection > cfg.MaxSessionsPerConnection {
		cfg.TargetSessionsPerConnection = cfg.MaxSessionsPerConnection
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = defaultIdleTimeout
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = defaultCleanupInterval
	}
	return cfg
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
	queue         connectionMinHeap
	closed        atomic.Bool
	pendingDials  int
	newConnection func() (*Connection, error)
	connect       func(*Connection) error
}

var _ ConnectionObserver = (*ConnectionPool)(nil)

type pooledConnection struct {
	id        uint64
	conn      *Connection
	state     protocol.ConnectionState
	load      int
	priority  connectionPriority
	index     int
	idleSince time.Time
	sessions  map[protocol.SessionKey]struct{}
}

type connectionPriority uint8

const (
	connectionPriorityActive connectionPriority = iota
	connectionPriorityConnecting
	connectionPriorityUnavailable
)

func newConnectionPool(
	parent context.Context,
	engine *nbhttp.Engine,
	machineID, address string,
	cfg ConnectionPoolConfig,
	dispatcher MessageDispatcher,
) *ConnectionPool {
	cfg = defaultConnectionPoolConfig(cfg)
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
	p.newConnection = func() (*Connection, error) {
		return newPoolConnection(p.ctx, p.engine, p.machineID, p.address, p.dispatcher, p)
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
	if item := p.selectLocked(); item != nil {
		p.attachLocked(key, binding, item)
		startDial := item.priority == connectionPriorityActive && p.reserveScaleLocked()
		p.mu.Unlock()
		if startDial {
			go p.startDial()
		}
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
		gatewayPoolMetrics.sessionsRouted.Add(gatewayPoolMetricContext, -1)
		p.refreshLocked(item)
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
	for _, binding := range p.bindings {
		binding.conn.Store(nil)
	}
	for _, item := range p.connections {
		p.recordConnectionRemovalLocked(item)
	}
	for range p.routes {
		gatewayPoolMetrics.sessionsRouted.Add(gatewayPoolMetricContext, -1)
	}
	clear(p.connections)
	clear(p.routes)
	clear(p.bindings)
	p.queue = nil
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
		gatewayPoolMetrics.pendingDials.Add(gatewayPoolMetricContext, 1)
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
	item := p.selectLocked()
	if item == nil || item.priority != connectionPriorityActive || item.load < p.targetSessions {
		return false
	}
	return p.reserveDialLocked()
}

func (p *ConnectionPool) reserveDialLocked() bool {
	if p.pendingDials > 0 || p.closed.Load() || (p.maxConns > 0 && len(p.connections)+p.pendingDials >= p.maxConns) {
		return false
	}
	p.pendingDials++
	gatewayPoolMetrics.pendingDials.Add(gatewayPoolMetricContext, 1)
	return true
}

func (p *ConnectionPool) startDial() {
	conn, err := p.newConnection()
	if err != nil {
		p.mu.Lock()
		p.pendingDials--
		gatewayPoolMetrics.pendingDials.Add(gatewayPoolMetricContext, -1)
		p.mu.Unlock()
		return
	}

	p.mu.Lock()
	p.pendingDials--
	gatewayPoolMetrics.pendingDials.Add(gatewayPoolMetricContext, -1)
	if p.closed.Load() {
		p.mu.Unlock()
		conn.Close()
		return
	}
	p.nextID++
	item := &pooledConnection{
		id:       p.nextID,
		conn:     conn,
		state:    conn.State(),
		index:    len(p.queue),
		sessions: make(map[protocol.SessionKey]struct{}),
	}
	if item.state == protocol.ConnectionActive {
		gatewayPoolMetrics.connectionsActive.Add(gatewayPoolMetricContext, 1)
	} else if item.state == protocol.ConnectionConnecting {
		gatewayPoolMetrics.connectionsConnecting.Add(gatewayPoolMetricContext, 1)
	}
	item.priority = p.priorityLocked(item)
	p.connections[conn] = item
	heap.Push(&p.queue, item)
	p.assignPendingLocked()
	p.mu.Unlock()

	if err = p.connect(conn); err != nil {
		go conn.reconnectLoop()
	}
}

func (p *ConnectionPool) selectLocked() *pooledConnection {
	if len(p.queue) == 0 || p.queue[0].priority == connectionPriorityUnavailable {
		return nil
	}
	return p.queue[0]
}

func (p *ConnectionPool) priorityLocked(item *pooledConnection) connectionPriority {
	if item.load >= p.maxSessions {
		return connectionPriorityUnavailable
	}
	switch item.conn.State() {
	case protocol.ConnectionActive:
		return connectionPriorityActive
	case protocol.ConnectionConnecting:
		return connectionPriorityConnecting
	default:
		return connectionPriorityUnavailable
	}
}

func (p *ConnectionPool) refreshLocked(item *pooledConnection) {
	item.priority = p.priorityLocked(item)
	heap.Fix(&p.queue, item.index)
}

// syncConnectionState keeps the allocation heap aligned with a connection's
// current lifecycle state.
func (p *ConnectionPool) syncConnectionState(conn *Connection) {
	p.mu.Lock()
	defer p.mu.Unlock()

	item := p.connections[conn]
	if item == nil {
		return
	}
	state := conn.State()
	if item.state != state {
		switch item.state {
		case protocol.ConnectionActive:
			gatewayPoolMetrics.connectionsActive.Add(gatewayPoolMetricContext, -1)
		case protocol.ConnectionConnecting:
			gatewayPoolMetrics.connectionsConnecting.Add(gatewayPoolMetricContext, -1)
		}
		switch state {
		case protocol.ConnectionActive:
			gatewayPoolMetrics.connectionsActive.Add(gatewayPoolMetricContext, 1)
		case protocol.ConnectionConnecting:
			gatewayPoolMetrics.connectionsConnecting.Add(gatewayPoolMetricContext, 1)
		}
		if item.state == protocol.ConnectionActive && state == protocol.ConnectionConnecting {
			gatewayPoolMetrics.reconnects.Add(gatewayPoolMetricContext, 1)
		}
		item.state = state
	}
	if state == protocol.ConnectionClosed {
		p.removeLocked(item)
		p.assignPendingLocked()
		return
	}
	p.refreshLocked(item)
}

func (p *ConnectionPool) OnConnectionOpen(conn *Connection) {
	p.syncConnectionState(conn)
}

func (p *ConnectionPool) OnConnectionClose(conn *Connection) {
	p.syncConnectionState(conn)
}

func (p *ConnectionPool) attachLocked(key protocol.SessionKey, binding *SessionBinding, item *pooledConnection) {
	p.routes[key] = item
	item.sessions[key] = struct{}{}
	item.load++
	gatewayPoolMetrics.sessionsRouted.Add(gatewayPoolMetricContext, 1)
	item.idleSince = time.Time{}
	binding.conn.Store(item.conn)
	p.refreshLocked(item)
}

func (p *ConnectionPool) assignPendingLocked() {
	for key, binding := range p.bindings {
		if _, assigned := p.routes[key]; assigned {
			continue
		}
		item := p.selectLocked()
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
		heap.Remove(&p.queue, item.index)
	}
	p.recordConnectionRemovalLocked(item)
	for key := range item.sessions {
		if p.routes[key] == item {
			delete(p.routes, key)
			gatewayPoolMetrics.sessionsRouted.Add(gatewayPoolMetricContext, -1)
			if binding := p.bindings[key]; binding != nil {
				binding.conn.Store(nil)
			}
		}
	}
	clear(item.sessions)
}

func (p *ConnectionPool) recordConnectionRemovalLocked(item *pooledConnection) {
	switch item.state {
	case protocol.ConnectionActive:
		gatewayPoolMetrics.connectionsActive.Add(gatewayPoolMetricContext, -1)
	case protocol.ConnectionConnecting:
		gatewayPoolMetrics.connectionsConnecting.Add(gatewayPoolMetricContext, -1)
	}
	item.state = protocol.ConnectionClosed
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
	if h[i].priority != h[j].priority {
		return h[i].priority < h[j].priority
	}
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
