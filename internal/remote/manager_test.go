package remote

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pluginv1 "github.com/wnnce/voce/api/plugin/v1"
	"github.com/wnnce/voce/config"
	"github.com/wnnce/voce/internal/engine"
	"google.golang.org/grpc"
)

func TestManager_AddRemote(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mockServer, srv, lis := setupMockServer(t)
		defer srv.Stop()

		store := engine.NewPluginStore(engine.LocalPluginResource())
		manager := NewManager(context.Background(), store)
		defer manager.Shutdown()

		cfg := config.PluginServerConfig{
			Enable:    true,
			URL:       "passthrough:///bufnet",
			Namespace: "test-ns",
		}

		err := manager.AddRemote(context.Background(), cfg, grpc.WithContextDialer(bufDialer(lis)))
		require.NoError(t, err)

		// Verification: The AddRemote calls handleReload which lists plugins and adds them to the store.
		assert.Len(t, manager.clients, 1)

		// ListPlugins on mockServer returned 1 plugin named "test-plugin"
		_, err = store.LoadBuilder("test-ns", "test-plugin")
		assert.NoError(t, err)

		_ = mockServer
	})
}

func TestManager_Heartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mockServer, srv, lis := setupMockServer(t)
		defer srv.Stop()

		store := engine.NewPluginStore(engine.LocalPluginResource())
		manager := NewManager(context.Background(), store)
		defer manager.Shutdown()

		cfg := config.PluginServerConfig{
			Enable:    true,
			URL:       "passthrough:///bufnet",
			Namespace: "test-ns",
		}

		err := manager.AddRemote(context.Background(), cfg, grpc.WithContextDialer(bufDialer(lis)))
		require.NoError(t, err)

		// Initially, plugin is in store
		_, err = store.LoadBuilder("test-ns", "test-plugin")
		require.NoError(t, err)

		// 1. Simulate server going offline (ping fails continuously)
		mockServer.SetPingHandler(func(ctx context.Context, pr *pluginv1.PingRequest) (*pluginv1.PingResponse, error) {
			return nil, assert.AnError
		})

		// Fast-forward time to trigger heartbeatLoop (defaultHeartbeatInterval is 10s)
		// 3 failures needed to reach Offline state (3 * 10s = 30s)
		time.Sleep(30 * time.Second)
		synctest.Wait()

		// Verify that handleOffline was called and resource was removed from store
		_, err = store.LoadBuilder("test-ns", "test-plugin")
		require.Error(t, err)

		// 2. Simulate server coming back online (ping succeeds)
		mockServer.SetPingHandler(func(ctx context.Context, pr *pluginv1.PingRequest) (*pluginv1.PingResponse, error) {
			return &pluginv1.PingResponse{ServerId: "test-server-new"}, nil
		})

		// Fast-forward 10s for the next heartbeat
		time.Sleep(10 * time.Second)
		synctest.Wait()

		// Verify that handleReload was called and resource was added back to store
		_, err = store.LoadBuilder("test-ns", "test-plugin")
		assert.NoError(t, err)
	})
}

func TestManager_CleanupLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mockServer, srv, lis := setupMockServer(t)
		defer srv.Stop()

		store := engine.NewPluginStore(engine.LocalPluginResource())
		manager := NewManager(context.Background(), store)
		defer manager.Shutdown()

		cfg := config.PluginServerConfig{
			Enable:    true,
			URL:       "passthrough:///bufnet",
			Namespace: "test-ns",
		}

		err := manager.AddRemote(context.Background(), cfg, grpc.WithContextDialer(bufDialer(lis)))
		require.NoError(t, err)

		// Inject a Zombie instance into the client
		manager.mu.RLock()
		client := manager.clients["test-ns"]
		manager.mu.RUnlock()

		stream := newFakeStream(context.Background())
		fakeClient := &fakePluginClient{stream: stream}
		p1 := NewPlugin(fakeClient, "inst-zombie", engine.PluginMetadata{Name: "test"})
		p1.setState(PluginStateStopped)
		require.NoError(t, client.RegisterInstance(p1))

		assert.Len(t, client.Instances(), 1)
		assert.Equal(t, 0, mockServer.GetDestroyCount())

		// Fast-forward time to trigger cleanupLoop (30 seconds)
		time.Sleep(30 * time.Second)
		synctest.Wait()

		// Verify zombie was cleaned up
		assert.Empty(t, client.Instances())
		assert.Equal(t, 1, mockServer.GetDestroyCount())
	})
}
