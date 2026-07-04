package remote

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/wnnce/voce/config"
	"github.com/wnnce/voce/internal/engine"
	"google.golang.org/grpc"
)

const maxConcurrentPings = 16

type Manager struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	store  *engine.PluginStore

	mu      sync.RWMutex
	clients map[string]*Client
}

func NewManager(ctx context.Context, store *engine.PluginStore) *Manager {
	ctx, cancel := context.WithCancel(ctx)
	m := &Manager{
		ctx:     ctx,
		cancel:  cancel,
		store:   store,
		clients: make(map[string]*Client),
	}
	m.wg.Add(2)
	go m.heartbeatLoop()
	go m.cleanupLoop()
	return m
}

func (m *Manager) AddRemote(ctx context.Context, cfg config.PluginServerConfig, opts ...grpc.DialOption) error {
	if !cfg.Enable || cfg.URL == "" {
		return fmt.Errorf("invalid or disabled config")
	}

	client, err := Dial(ctx, cfg, opts...)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.clients[client.Namespace()] = client
	m.mu.Unlock()

	// 初始加载拉取插件列表
	m.handleReload(client)

	slog.InfoContext(ctx, "remote plugin server added", "url", client.URL(), "namespace", client.Namespace())
	return nil
}

func (m *Manager) AddRemotes(ctx context.Context, configs []config.PluginServerConfig) {
	for _, cfg := range configs {
		if !cfg.Enable || cfg.URL == "" {
			continue
		}
		if err := m.AddRemote(ctx, cfg); err != nil {
			slog.ErrorContext(ctx, "failed to add remote plugin server", "url", cfg.URL, "error", err)
		}
	}
}

func (m *Manager) Shutdown() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, client := range m.clients {
		m.store.RemoveResource(client.Namespace())
		if err := client.Close(); err != nil {
			slog.Warn("remote plugin client close failed",
				"url", client.URL(),
				"error", err)
		}
	}
	m.clients = make(map[string]*Client)
}

func (m *Manager) heartbeatLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(time.Duration(defaultHeartbeatInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.pingAll()
		}
	}
}

func (m *Manager) cleanupLoop() {
	defer m.wg.Done()

	// Run cleanup every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanupAll()
		}
	}
}

func (m *Manager) cleanupAll() {
	m.mu.RLock()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	sem := make(chan struct{}, maxConcurrentPings)
	var wg sync.WaitGroup
	for _, client := range clients {
		wg.Add(1)
		sem <- struct{}{}
		go func(c *Client) {
			defer wg.Done()
			defer func() { <-sem }()
			c.CleanupZombieInstances(m.ctx)
		}(client)
	}
	wg.Wait()
}

func (m *Manager) pingAll() {
	m.mu.RLock()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	// 信号量控制最大并发
	sem := make(chan struct{}, maxConcurrentPings)
	var wg sync.WaitGroup

	for _, client := range clients {
		wg.Add(1)
		sem <- struct{}{}
		go func(c *Client) {
			defer wg.Done()
			defer func() { <-sem }()

			event := c.DoPing(m.ctx)
			switch event {
			case PingEventOffline:
				m.handleOffline(c)
			case PingEventNeedReload:
				m.handleReload(c)
			default:
			}
		}(client)
	}
	wg.Wait()
}

func (m *Manager) handleOffline(client *Client) {
	slog.WarnContext(m.ctx, "remote plugin server offline", "url", client.URL(), "namespace", client.Namespace())
	m.store.RemoveResource(client.Namespace())
}

func (m *Manager) handleReload(client *Client) {
	slog.InfoContext(m.ctx, "remote plugin server online/reloaded", "url", client.URL(), "namespace", client.Namespace())

	plugins, err := client.ListPlugins(m.ctx)
	if err != nil {
		slog.ErrorContext(m.ctx, "failed to list remote plugins", "url", client.URL(), "error", err)
		return
	}

	resource := engine.NewPluginResource(client.Namespace())
	for _, metadata := range plugins {
		builder, err := NewBuilder(client, metadata)
		if err != nil {
			slog.ErrorContext(m.ctx, "failed to create builder for remote plugin", "plugin", metadata.GetName(), "error", err)
			continue
		}
		if err := resource.RegisterBuilder(builder); err != nil {
			slog.ErrorContext(m.ctx, "failed to register remote plugin", "plugin", builder.Name(), "error", err)
		}
	}

	if err := m.store.AddResource(resource); err != nil {
		slog.ErrorContext(m.ctx, "failed to add remote resource to store", "namespace", resource.Namespace(), "error", err)
	}
}
