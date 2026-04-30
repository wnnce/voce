package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/wnnce/voce/internal/protocol"
)

type Session struct {
	Key        protocol.SessionKey
	Workflow   *Workflow
	LastActive atomic.Int64 // Unix timestamp in milliseconds for idle timeout cleanup
	CreatedAt  time.Time
	busy       atomic.Bool // Internal flag to prevent multiple concurrent connections
}

type SessionObserver func(s *Session)

// SessionManager manages persistent sessions, allowing workflows to survive
// short-term client disconnections. It handles session creation, retrieval,
// cleanup of idle sessions, and dynamic configuration merging.
type SessionManager struct {
	sessions       map[protocol.SessionKey]*Session
	mu             sync.RWMutex
	wm             WorkflowConfigManager
	timeout        time.Duration
	ticker         *time.Ticker
	onCreated      []SessionObserver
	onCreatedCount int
	onDeleted      []SessionObserver
	onDeletedCount int
	ctx            context.Context
	cancel         context.CancelFunc
}

func NewSessionManager(wm WorkflowConfigManager, timeout time.Duration) *SessionManager {
	ctx, cancel := context.WithCancel(context.Background())
	sm := &SessionManager{
		sessions:  make(map[protocol.SessionKey]*Session),
		wm:        wm,
		timeout:   timeout,
		ctx:       ctx,
		cancel:    cancel,
		onCreated: make([]SessionObserver, 4),
		onDeleted: make([]SessionObserver, 4),
	}
	sm.startCleanupTicker()
	return sm
}

func (sm *SessionManager) AddCreatedObserver(fn SessionObserver) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	index, newObservers := addObserver(sm.onCreated, fn, &sm.onCreatedCount)
	sm.onCreated = newObservers
	return index
}

func (sm *SessionManager) RemoveCreatedObserver(index int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if index < 0 || index >= len(sm.onCreated) {
		return
	}
	if sm.onCreated[index] != nil {
		sm.onCreated[index] = nil
		sm.onCreatedCount--
	}
}

func (sm *SessionManager) AddDeletedObserver(fn SessionObserver) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	index, newObservers := addObserver(sm.onDeleted, fn, &sm.onDeletedCount)
	sm.onDeleted = newObservers
	return index
}

func addObserver(observers []SessionObserver, fn SessionObserver, count *int) (int, []SessionObserver) {
	if *count == len(observers) {
		index := len(observers)
		observers = append(observers, fn)
		*count++
		return index, observers
	}
	for i := 0; i < len(observers); i++ {
		if observers[i] == nil {
			observers[i] = fn
			*count++
			return i, observers
		}
	}
	return -1, observers
}

func (sm *SessionManager) RemoveDeletedObserver(index int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if index < 0 || index >= len(sm.onDeleted) {
		return
	}
	if sm.onDeleted[index] != nil {
		sm.onDeleted[index] = nil
		sm.onDeletedCount--
	}
}

func (sm *SessionManager) Context() context.Context {
	return sm.ctx
}

func (sm *SessionManager) startCleanupTicker() {
	sm.ticker = time.NewTicker(sm.timeout / 2)
	go func() {
		defer sm.ticker.Stop()
		for {
			if sm.ctx.Err() != nil {
				return
			}
			select {
			case <-sm.ticker.C:
				sm.Cleanup()
			case <-sm.ctx.Done():
				return
			}
		}
	}()
}

func (sm *SessionManager) Stop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, s := range sm.sessions {
		s.Workflow.Stop()
	}
	sm.sessions = make(map[protocol.SessionKey]*Session)

	if sm.cancel != nil {
		sm.cancel()
	}
}

// CreateSession instantiates a new workflow session based on a template and optional property overrides.
// It clones and merges configurations, builds the graph, and starts the workflow execution.
func (sm *SessionManager) CreateSession(
	ctx context.Context,
	key protocol.SessionKey,
	name string,
	properties map[string]json.RawMessage,
) (*Session, error) {
	cfg, err := sm.wm.GetWithName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("find workflow config by name %s: %w", name, err)
	}

	newCfg := sm.cloneAndMergeConfig(cfg, properties)

	graph, err := BuildGraph(&newCfg)
	if err != nil {
		return nil, fmt.Errorf("build graph: %w", err)
	}

	wf, err := NewWorkflow(ctx, graph)
	if err != nil {
		return nil, fmt.Errorf("new workflow: %w", err)
	}

	if err = wf.Start(); err != nil {
		return nil, fmt.Errorf("start workflow: %w", err)
	}
	session := &Session{
		Key:       key,
		Workflow:  wf,
		CreatedAt: time.Now(),
	}
	session.LastActive.Store(time.Now().Unix())

	sm.mu.Lock()
	sm.sessions[key] = session
	var observers []SessionObserver
	if sm.onCreatedCount > 0 {
		observers = make([]SessionObserver, 0, sm.onCreatedCount)
		for _, fn := range sm.onCreated {
			if fn != nil {
				observers = append(observers, fn)
			}
		}
	}
	sm.mu.Unlock()

	for _, fn := range observers {
		fn(session)
	}

	return session, nil
}

func (sm *SessionManager) LoadSession(key protocol.SessionKey) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[key]
	if ok {
		s.UpdateActivity()
	}
	return s, ok
}

// RemoveSession stops the underlying workflow and deletes the session mapping immediately.
func (sm *SessionManager) RemoveSession(key protocol.SessionKey) {
	sm.mu.Lock()
	s, ok := sm.sessions[key]
	if !ok {
		sm.mu.Unlock()
		return
	}
	delete(sm.sessions, key)
	var observers []SessionObserver
	if sm.onDeletedCount > 0 {
		observers = make([]SessionObserver, 0, sm.onDeletedCount)
		for _, fn := range sm.onDeleted {
			if fn != nil {
				observers = append(observers, fn)
			}
		}
	}
	sm.mu.Unlock()

	for _, fn := range observers {
		fn(s)
	}

	if s.Workflow != nil && s.Workflow.State() != WorkflowStateStopped {
		slog.Info("removing session and stopping workflow", "sessionID", s.Key.String())
		s.Workflow.Stop()
	}
}

func (sm *SessionManager) Cleanup() {
	var toBeStopped []*Session

	sm.mu.Lock()
	now := time.Now().UnixMilli()
	duration := sm.timeout.Milliseconds()
	for id, s := range sm.sessions {
		if now-s.LastActive.Load() < duration {
			continue
		}
		toBeStopped = append(toBeStopped, s)
		delete(sm.sessions, id)
	}
	var observers []SessionObserver
	if sm.onDeletedCount > 0 {
		observers = make([]SessionObserver, 0, sm.onDeletedCount)
		for _, fn := range sm.onDeleted {
			if fn != nil {
				observers = append(observers, fn)
			}
		}
	}
	sm.mu.Unlock()

	for _, s := range toBeStopped {
		for _, fn := range observers {
			fn(s)
		}
		slog.Info("cleaning up idle session", "sessionID", s.Key.String())
		s.Workflow.Stop()
	}
}

func (s *Session) UpdateActivity() {
	s.LastActive.Store(time.Now().UnixMilli())
}

func (s *Session) Acquire() bool {
	return s.busy.CompareAndSwap(false, true)
}

func (s *Session) Release() {
	s.busy.Store(false)
}

// cloneAndMergeConfig creates a copy of the workflow configuration and applies
// property overrides to specific nodes based on the provided properties map.
func (sm *SessionManager) cloneAndMergeConfig(cfg WorkflowConfig, properties map[string]json.RawMessage) WorkflowConfig {
	newCfg := cfg
	newCfg.Nodes = make([]NodeConfig, len(cfg.Nodes))
	copy(newCfg.Nodes, cfg.Nodes)

	for i, nd := range newCfg.Nodes {
		if nodeProps, ok := properties[nd.Name]; ok {
			// Merge nodeProps into node.Config
			mergedConfig, err := sm.deepMergeJSON(nd.Config, nodeProps)
			if err != nil {
				slog.Error("failed to deep merge node config", "node", nd.Name, "error", err)
				continue
			}
			newCfg.Nodes[i].Config = mergedConfig
		}
	}
	return newCfg
}

func (sm *SessionManager) deepMergeJSON(base, overlay json.RawMessage) (json.RawMessage, error) {
	if len(base) == 0 {
		return overlay, nil
	}
	if len(overlay) == 0 {
		return base, nil
	}

	baseNode, err := sonic.Get(base)
	if err != nil {
		return nil, err
	}
	overlayNode, err := sonic.Get(overlay)
	if err != nil {
		return nil, err
	}

	if err = DeepMergeAST(&baseNode, &overlayNode); err != nil {
		return nil, err
	}

	raw, err := baseNode.Raw()
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func DeepMergeAST(base *ast.Node, overlay *ast.Node) error {
	if base.TypeSafe() != ast.V_OBJECT || overlay.TypeSafe() != ast.V_OBJECT {
		return nil
	}

	return overlay.ForEach(func(path ast.Sequence, node *ast.Node) bool {
		if path.Key == nil {
			return true
		}
		key := *path.Key

		baseMember := base.Get(key)
		if baseMember.Exists() && baseMember.TypeSafe() == ast.V_OBJECT && node.TypeSafe() == ast.V_OBJECT {
			if err := DeepMergeAST(baseMember, node); err != nil {
				return false
			}
			_, _ = base.Set(key, *baseMember)
		} else {
			_, _ = base.Set(key, *node)
		}
		return true
	})
}

func (sm *SessionManager) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}
