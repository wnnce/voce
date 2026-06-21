package remote

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pluginv1 "github.com/wnnce/voce/api/plugin/v1"
	"github.com/wnnce/voce/internal/engine"
	"google.golang.org/grpc"
)

type fakeRemoteClient struct {
	pluginv1.RemotePluginServiceClient
	createErr          error
	destroyErr         error
	createdInstances   []string
	destroyedInstances []string
}

func (f *fakeRemoteClient) CreateInstance(ctx context.Context, in *pluginv1.CreateInstanceRequest, opts ...grpc.CallOption) (*pluginv1.CreateInstanceResponse, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.createdInstances = append(f.createdInstances, in.InstanceId)
	return &pluginv1.CreateInstanceResponse{InstanceId: in.InstanceId}, nil
}

func (f *fakeRemoteClient) DestroyInstance(ctx context.Context, in *pluginv1.DestroyInstanceRequest, opts ...grpc.CallOption) (*pluginv1.DestroyInstanceResponse, error) {
	if f.destroyErr != nil {
		return nil, f.destroyErr
	}
	f.destroyedInstances = append(f.destroyedInstances, in.InstanceId)
	return &pluginv1.DestroyInstanceResponse{}, nil
}

func TestNewBuilder(t *testing.T) {
	t.Run("empty name should fail", func(t *testing.T) {
		meta := &pluginv1.PluginMetadata{}
		b, err := NewBuilder(nil, meta)
		require.Error(t, err)
		assert.Nil(t, b)
		assert.Contains(t, err.Error(), "remote plugin name is empty")
	})

	t.Run("invalid schema should fail", func(t *testing.T) {
		meta := &pluginv1.PluginMetadata{
			Name:   "test-plugin",
			Schema: "invalid-json",
		}
		b, err := NewBuilder(nil, meta)
		require.Error(t, err)
		assert.Nil(t, b)
		assert.Contains(t, err.Error(), "decode remote plugin schema")
	})

	t.Run("valid metadata should succeed", func(t *testing.T) {
		meta := &pluginv1.PluginMetadata{
			Name:        "test-plugin",
			Description: "a test plugin",
			Schema:      `{"type": "object"}`,
			Inputs: []*pluginv1.Property{
				{
					Type: pluginv1.EventType_EVENT_TYPE_SIGNAL,
					Name: "sig_in",
				},
			},
			Outputs: []*pluginv1.Property{
				{
					Type: pluginv1.EventType_EVENT_TYPE_PAYLOAD,
					Name: "pay_out",
				},
			},
			Ports: []*pluginv1.PortMetadata{
				{
					Type: pluginv1.EventType_EVENT_TYPE_SIGNAL,
					Port: 1,
					Name: "err_port",
				},
			},
		}

		b, err := NewBuilder(nil, meta)
		require.NoError(t, err)
		require.NotNil(t, b)

		assert.Equal(t, "test-plugin", b.Name())
		assert.Equal(t, "a test plugin", b.Description())
		assert.NotNil(t, b.Schema())

		inputs := b.Inputs()
		require.Len(t, inputs, 1)
		assert.Equal(t, engine.PrefixSignal, inputs[0].Prefix)
		assert.Equal(t, "sig_in", inputs[0].Name)

		outputs := b.Outputs()
		require.Len(t, outputs, 1)
		assert.Equal(t, engine.PrefixPayload, outputs[0].Prefix)
		assert.Equal(t, "pay_out", outputs[0].Name)

		ports := b.Ports()
		require.Len(t, ports, 1)
		assert.Equal(t, engine.EventSignal, ports[0].Type)
		assert.Equal(t, 1, ports[0].Port)
		assert.Equal(t, "err_port", ports[0].Name)
	})
}

func TestBuilder_Build(t *testing.T) {
	t.Run("nil client should fail", func(t *testing.T) {
		b := &Builder{
			client: nil,
			meta:   engine.PluginMetadata{Name: "test-plugin"},
		}
		plugin, err := b.Build(nil)
		require.Error(t, err)
		assert.Nil(t, plugin)
		assert.Contains(t, err.Error(), "remote plugin client is nil")
	})

	t.Run("CreateInstance error should fail build", func(t *testing.T) {
		mockClient := &fakeRemoteClient{
			createErr: errors.New("network error"),
		}
		client := &Client{
			client: mockClient,
		}

		b := &Builder{
			client: client,
			meta:   engine.PluginMetadata{Name: "test-plugin"},
		}

		plugin, err := b.Build(nil)
		require.Error(t, err)
		assert.Nil(t, plugin)
		assert.Contains(t, err.Error(), "network error")
	})

	t.Run("successful build should register instance", func(t *testing.T) {
		mockClient := &fakeRemoteClient{}
		client := &Client{
			client:    mockClient,
			instances: make(map[string]*Plugin),
		}

		b := &Builder{
			client: client,
			meta:   engine.PluginMetadata{Name: "test-plugin"},
		}

		plugin, err := b.Build([]byte(`{"foo":"bar"}`))
		require.NoError(t, err)
		require.NotNil(t, plugin)

		// Assert CreateInstance was called with the new instance ID
		assert.Len(t, mockClient.createdInstances, 1)
		instanceID := mockClient.createdInstances[0]
		assert.NotEmpty(t, instanceID)

		// Assert plugin was registered in the client
		client.mu.RLock()
		defer client.mu.RUnlock()
		assert.Len(t, client.instances, 1)
		assert.Contains(t, client.instances, instanceID)
	})

	t.Run("multi track build should register raw instance and return wrapper", func(t *testing.T) {
		mockClient := &fakeRemoteClient{}
		client := &Client{
			client:    mockClient,
			instances: make(map[string]*Plugin),
		}

		b := &Builder{
			client: client,
			meta:   engine.PluginMetadata{Name: "test-plugin"},
			multiTrack: &pluginv1.MultiTrackConfig{
				Enabled: true,
				Payload: &pluginv1.TrackConfig{
					Enabled:          true,
					BufferSize:       64,
					DropStrategy:     pluginv1.DropStrategy_DROP_STRATEGY_DROP_OLDEST,
					InterruptSignals: []string{"interrupter"},
				},
			},
		}

		plugin, err := b.Build(nil)
		require.NoError(t, err)
		require.NotNil(t, plugin)
		wrapper, ok := plugin.(*engine.MultiTrackPlugin)
		require.True(t, ok)
		require.NotNil(t, wrapper)

		assert.Len(t, mockClient.createdInstances, 1)
		instanceID := mockClient.createdInstances[0]
		client.mu.RLock()
		defer client.mu.RUnlock()
		require.Len(t, client.instances, 1)
		assert.Contains(t, client.instances, instanceID)
	})
}

func TestConvertMultiTrackOptions(t *testing.T) {
	t.Run("disabled config returns no options", func(t *testing.T) {
		assert.Nil(t, convertMultiTrackOptions(nil))
		assert.Nil(t, convertMultiTrackOptions(&pluginv1.MultiTrackConfig{}))
		assert.Nil(t, convertMultiTrackOptions(&pluginv1.MultiTrackConfig{Enabled: true}))
		assert.Nil(t, convertMultiTrackOptions(&pluginv1.MultiTrackConfig{
			Enabled: true,
			Payload: &pluginv1.TrackConfig{},
		}))
	})

	t.Run("payload config returns track option", func(t *testing.T) {
		options := convertMultiTrackOptions(&pluginv1.MultiTrackConfig{
			Enabled: true,
			Payload: &pluginv1.TrackConfig{
				Enabled:          true,
				BufferSize:       0,
				DropStrategy:     pluginv1.DropStrategy_DROP_STRATEGY_DROP_NEWEST,
				InterruptSignals: []string{"interrupter"},
			},
		})

		require.Len(t, options, 1)
	})
}
