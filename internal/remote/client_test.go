package remote

import (
	"context"
	"net"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pluginv1 "github.com/wnnce/voce/api/plugin/v1"
	"github.com/wnnce/voce/config"
	"github.com/wnnce/voce/internal/engine"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

// mockRemoteServer implements pluginv1.RemotePluginServiceServer for testing
type mockRemoteServer struct {
	pluginv1.UnimplementedRemotePluginServiceServer
	mu                     sync.Mutex
	pingHandler            func(context.Context, *pluginv1.PingRequest) (*pluginv1.PingResponse, error)
	destroyInstanceHandler func(context.Context, *pluginv1.DestroyInstanceRequest) (*pluginv1.DestroyInstanceResponse, error)
	listPluginsHandler     func(context.Context, *pluginv1.ListPluginsRequest) (*pluginv1.ListPluginsResponse, error)
	createInstanceHandler  func(context.Context, *pluginv1.CreateInstanceRequest) (*pluginv1.CreateInstanceResponse, error)
	destroyCount           int
}

func (m *mockRemoteServer) Ping(ctx context.Context, req *pluginv1.PingRequest) (*pluginv1.PingResponse, error) {
	m.mu.Lock()
	handler := m.pingHandler
	m.mu.Unlock()
	if handler != nil {
		return handler(ctx, req)
	}
	return &pluginv1.PingResponse{ServerId: "test-server-1"}, nil
}

func (m *mockRemoteServer) DestroyInstance(ctx context.Context, req *pluginv1.DestroyInstanceRequest) (*pluginv1.DestroyInstanceResponse, error) {
	m.mu.Lock()
	m.destroyCount++
	handler := m.destroyInstanceHandler
	m.mu.Unlock()
	if handler != nil {
		return handler(ctx, req)
	}
	return &pluginv1.DestroyInstanceResponse{}, nil
}

func (m *mockRemoteServer) ListPlugins(ctx context.Context, req *pluginv1.ListPluginsRequest) (*pluginv1.ListPluginsResponse, error) {
	m.mu.Lock()
	handler := m.listPluginsHandler
	m.mu.Unlock()
	if handler != nil {
		return handler(ctx, req)
	}
	return &pluginv1.ListPluginsResponse{Plugins: []*pluginv1.PluginMetadata{{Name: "test-plugin"}}}, nil
}

func (m *mockRemoteServer) CreateInstance(ctx context.Context, req *pluginv1.CreateInstanceRequest) (*pluginv1.CreateInstanceResponse, error) {
	m.mu.Lock()
	handler := m.createInstanceHandler
	m.mu.Unlock()
	if handler != nil {
		return handler(ctx, req)
	}
	return &pluginv1.CreateInstanceResponse{InstanceId: req.InstanceId}, nil
}

func (m *mockRemoteServer) GetDestroyCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.destroyCount
}

func (m *mockRemoteServer) SetPingHandler(h func(context.Context, *pluginv1.PingRequest) (*pluginv1.PingResponse, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pingHandler = h
}

func setupMockServer(t *testing.T) (*mockRemoteServer, *grpc.Server, *bufconn.Listener) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	mockServer := &mockRemoteServer{}
	pluginv1.RegisterRemotePluginServiceServer(s, mockServer)
	go func() {
		if err := s.Serve(lis); err != nil {
			// server closed
		}
	}()
	return mockServer, s, lis
}

func bufDialer(lis *bufconn.Listener) func(context.Context, string) (net.Conn, error) {
	return func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
}

func TestClient_DialAndPing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, srv, lis := setupMockServer(t)
		defer srv.Stop()

		cfg := config.PluginServerConfig{
			URL:       "passthrough:///bufnet",
			Namespace: "test-ns",
		}

		client, err := Dial(context.Background(), cfg, grpc.WithContextDialer(bufDialer(lis)))
		require.NoError(t, err)
		defer client.Close()

		assert.Equal(t, "test-ns", client.Namespace())

		// Test Ping
		event := client.DoPing(context.Background())
		assert.Equal(t, PingEventNone, event)
	})
}

func TestClient_DoPing_Failure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mockServer, srv, lis := setupMockServer(t)
		defer srv.Stop()

		cfg := config.PluginServerConfig{
			URL:       "passthrough:///bufnet",
			Namespace: "test-ns",
		}

		client, err := Dial(context.Background(), cfg, grpc.WithContextDialer(bufDialer(lis)))
		require.NoError(t, err)
		defer client.Close()

		// Mock ping to return error
		mockServer.SetPingHandler(func(ctx context.Context, pr *pluginv1.PingRequest) (*pluginv1.PingResponse, error) {
			return nil, assert.AnError
		})

		// First failure -> Abnormal (failCount=1)
		event := client.DoPing(context.Background())
		assert.Equal(t, PingEventNone, event)

		// Second failure -> Abnormal (failCount=2)
		event = client.DoPing(context.Background())
		assert.Equal(t, PingEventNone, event)

		// Third failure -> Offline (failCount=3)
		event = client.DoPing(context.Background())
		assert.Equal(t, PingEventOffline, event)

		// Fourth failure -> Already offline, returns None
		event = client.DoPing(context.Background())
		assert.Equal(t, PingEventNone, event)
	})
}

func TestClient_DoPing_Restart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mockServer, srv, lis := setupMockServer(t)
		defer srv.Stop()

		cfg := config.PluginServerConfig{
			URL:       "passthrough:///bufnet",
			Namespace: "test-ns",
		}

		client, err := Dial(context.Background(), cfg, grpc.WithContextDialer(bufDialer(lis)))
		require.NoError(t, err)
		defer client.Close()

		// Server id changes -> NeedReload
		mockServer.SetPingHandler(func(ctx context.Context, pr *pluginv1.PingRequest) (*pluginv1.PingResponse, error) {
			return &pluginv1.PingResponse{ServerId: "test-server-2"}, nil
		})

		event := client.DoPing(context.Background())
		assert.Equal(t, PingEventNeedReload, event)
	})
}

func TestClient_ListPluginsAndCreateInstance(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, srv, lis := setupMockServer(t)
		defer srv.Stop()

		cfg := config.PluginServerConfig{
			URL:       "passthrough:///bufnet",
			Namespace: "test-ns",
		}

		client, err := Dial(context.Background(), cfg, grpc.WithContextDialer(bufDialer(lis)))
		require.NoError(t, err)
		defer client.Close()

		plugins, err := client.ListPlugins(context.Background())
		require.NoError(t, err)
		require.Len(t, plugins, 1)
		assert.Equal(t, "test-plugin", plugins[0].Name)

		err = client.CreateInstance(context.Background(), "inst-test", "test-plugin", nil)
		require.NoError(t, err)
	})
}

func TestClient_CleanupZombieInstances(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mockServer, srv, lis := setupMockServer(t)
		defer srv.Stop()

		cfg := config.PluginServerConfig{
			URL:       "passthrough:///bufnet",
			Namespace: "test-ns",
		}

		client, err := Dial(context.Background(), cfg, grpc.WithContextDialer(bufDialer(lis)))
		require.NoError(t, err)
		defer client.Close()

		// Manually create some plugin instances in different states using the fake client from plugin_test.go
		stream := newFakeStream(context.Background())
		fakeClient := &fakePluginClient{stream: stream}

		// 1. Streaming (Healthy)
		p1 := NewPlugin(fakeClient, "inst-1", engine.PluginMetadata{Name: "test"})
		p1.setState(PluginStateStreaming)

		// 2. Failed (Zombie)
		p2 := NewPlugin(fakeClient, "inst-2", engine.PluginMetadata{Name: "test"})
		p2.setState(PluginStateFailed)

		// 3. Stopped (Zombie)
		p3 := NewPlugin(fakeClient, "inst-3", engine.PluginMetadata{Name: "test"})
		p3.setState(PluginStateStopped)

		// 4. Destroyed (Should be ignored by cleanup, though unlikely to be in map)
		p4 := NewPlugin(fakeClient, "inst-4", engine.PluginMetadata{Name: "test"})
		p4.setState(PluginStateDestroyed)

		require.NoError(t, client.RegisterInstance(p1))
		require.NoError(t, client.RegisterInstance(p2))
		require.NoError(t, client.RegisterInstance(p3))
		require.NoError(t, client.RegisterInstance(p4))

		require.Len(t, client.Instances(), 4)

		// Trigger cleanup
		client.CleanupZombieInstances(context.Background())

		// Verify instances were removed locally
		instances := client.Instances()
		assert.Len(t, instances, 2) // inst-1 and inst-4 remain

		_, ok1 := client.Instance("inst-1")
		assert.True(t, ok1)
		_, ok4 := client.Instance("inst-4")
		assert.True(t, ok4)

		// Verify remote DestroyInstance was called exactly twice (for Failed and Stopped)
		assert.Equal(t, 2, mockServer.GetDestroyCount())
	})
}
