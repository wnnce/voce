package remote

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	pluginv1 "github.com/wnnce/voce/api/plugin/v1"
	"github.com/wnnce/voce/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ServerState int

const (
	StateOnline ServerState = iota
	StateAbnormal
	StateOffline
)

type PingEvent int

const (
	PingEventNone PingEvent = iota
	PingEventOffline
	PingEventNeedReload
)

const maxFailCount = 3

type Client struct {
	cfg    config.PluginServerConfig
	conn   *grpc.ClientConn
	client pluginv1.RemotePluginServiceClient

	mu        sync.RWMutex
	state     ServerState
	failCount int
	serverId  string

	instances map[string]*Plugin
}

func Dial(ctx context.Context, cfg config.PluginServerConfig, opts ...grpc.DialOption) (*Client, error) {
	dialOpts := append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}, opts...)

	conn, err := grpc.NewClient(cfg.URL, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("create remote plugin client %s: %w", cfg.URL, err)
	}

	c := &Client{
		cfg:       cfg,
		conn:      conn,
		client:    pluginv1.NewRemotePluginServiceClient(conn),
		state:     StateOffline,
		instances: make(map[string]*Plugin),
	}

	pingCtx, cancel := context.WithTimeout(ctx, time.Duration(defaultRequestTimeout)*time.Second)
	defer cancel()

	resp, err := c.client.Ping(pingCtx, &pluginv1.PingRequest{ClientId: "voce"})
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping remote plugin server %s: %w", cfg.URL, err)
	}

	c.state = StateOnline
	c.serverId = resp.GetServerId()
	return c, nil
}

func (c *Client) URL() string {
	return c.cfg.URL
}

func (c *Client) Namespace() string {
	if c.cfg.Namespace != "" {
		return c.cfg.Namespace
	}
	return c.cfg.URL
}

func (c *Client) RPC() pluginv1.RemotePluginServiceClient {
	return c.client
}

func (c *Client) DoPing(ctx context.Context) PingEvent {
	pingCtx, cancel := context.WithTimeout(ctx, time.Duration(defaultRequestTimeout)*time.Second)
	defer cancel()

	resp, err := c.client.Ping(pingCtx, &pluginv1.PingRequest{ClientId: "voce"})

	c.mu.Lock()
	defer c.mu.Unlock()

	if err != nil {
		c.failCount++
		if c.failCount >= maxFailCount {
			if c.state != StateOffline {
				c.state = StateOffline
				return PingEventOffline
			}
		} else {
			c.state = StateAbnormal
		}
		return PingEventNone
	}

	// Ping success
	c.failCount = 0
	oldState := c.state
	c.state = StateOnline

	newServerId := resp.GetServerId()

	if oldState == StateOffline {
		c.serverId = newServerId
		return PingEventNeedReload
	}

	if c.serverId != newServerId {
		// Server restarted fast between pings
		c.serverId = newServerId
		return PingEventNeedReload
	}

	return PingEventNone
}

func (c *Client) ListPlugins(ctx context.Context) ([]*pluginv1.PluginMetadata, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(defaultRequestTimeout)*time.Second)
	defer cancel()

	resp, err := c.client.ListPlugins(ctx, &pluginv1.ListPluginsRequest{
		Namespace: c.cfg.Namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("list remote plugins %s: %w", c.cfg.URL, err)
	}
	return resp.GetPlugins(), nil
}

func (c *Client) CreateInstance(ctx context.Context, instanceID, pluginName string, config []byte) error {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(defaultRequestTimeout)*time.Second)
	defer cancel()

	resp, err := c.client.CreateInstance(ctx, &pluginv1.CreateInstanceRequest{
		InstanceId: instanceID,
		PluginName: pluginName,
		Config:     config,
	})
	if err != nil {
		return fmt.Errorf("create remote plugin instance %s/%s at %s: %w",
			pluginName, instanceID, c.cfg.URL, err)
	}
	if resp.GetInstanceId() != "" && resp.GetInstanceId() != instanceID {
		return fmt.Errorf("remote plugin instance id mismatch: expected %s, got %s",
			instanceID, resp.GetInstanceId())
	}
	return nil
}

func (c *Client) DestroyInstance(ctx context.Context, instanceID, reason string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(defaultRequestTimeout)*time.Second)
	defer cancel()

	if _, err := c.client.DestroyInstance(ctx, &pluginv1.DestroyInstanceRequest{
		InstanceId: instanceID,
		Reason:     reason,
	}); err != nil {
		return fmt.Errorf("destroy remote plugin instance %s at %s: %w", instanceID, c.cfg.URL, err)
	}
	return nil
}

func (c *Client) RegisterInstance(instance *Plugin) error {
	if instance == nil {
		return fmt.Errorf("remote instance is nil")
	}
	instanceID := instance.InstanceID()
	if instanceID == "" {
		return fmt.Errorf("remote instance id is empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.instances[instanceID]; ok {
		return fmt.Errorf("remote instance already registered: %s", instanceID)
	}
	c.instances[instanceID] = instance
	return nil
}

func (c *Client) RemoveInstance(instanceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.instances, instanceID)
}

func (c *Client) CleanupZombieInstances(ctx context.Context) {
	var zombies []string

	c.mu.RLock()
	for id, inst := range c.instances {
		state := inst.State()
		// Only Failed and Stopped need cleanup.
		// Destroyed is generally already removed, but we check to be safe.
		if state == PluginStateFailed || state == PluginStateStopped {
			zombies = append(zombies, id)
		}
	}
	c.mu.RUnlock()

	for _, id := range zombies {
		// Attempt to clean up resources on the remote server
		if err := c.DestroyInstance(ctx, id, "zombie instance cleanup"); err != nil {
			slog.WarnContext(ctx, "failed to destroy zombie instance on remote",
				"instance_id", id,
				"url", c.URL(),
				"error", err)
		}
		// Always remove from local map to prevent memory leak
		c.RemoveInstance(id)
	}
}

func (c *Client) Instance(instanceID string) (*Plugin, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	instance, ok := c.instances[instanceID]
	return instance, ok
}

func (c *Client) Instances() []*Plugin {
	c.mu.RLock()
	defer c.mu.RUnlock()

	instances := make([]*Plugin, 0, len(c.instances))
	for _, instance := range c.instances {
		instances = append(instances, instance)
	}
	return instances
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
