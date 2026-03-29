package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/schema"
)

// Dummy configuration for test plugins
type MockPluginConfig struct {
	Value string `json:"value"`
}

func (m *MockPluginConfig) Schema() *jsonschema.Schema { return nil }
func (m *MockPluginConfig) Decode(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, m)
}

func TestWorkflow_TopologicalSort_And_Execution(t *testing.T) {
	// ignore duplicate registration error; plugin may already be registered from other tests
	_ = RegisterPlugin[*MockPluginConfig](func(cfg *MockPluginConfig) Plugin {
		return &BuiltinPlugin{}
	}, PluginMetadata{
		Name: "test_node",
	})

	config := &WorkflowConfig{
		ID:   "wf-2",
		Head: "n1",
		Nodes: []NodeConfig{
			{ID: "n3", Name: "Node 3", Plugin: "test_node"},
			{ID: "n1", Name: "Node 1", Plugin: "test_node"},
			{ID: "n2", Name: "Node 2", Plugin: "test_node"},
		},
		Edges: []EdgeConfig{
			{Source: "n1", Target: "n2", Type: EventPayload, SourcePort: 0},
			{Source: "n2", Target: "n3", Type: EventPayload, SourcePort: 0},
		},
	}

	graph, err := BuildGraph(config)
	require.NoError(t, err)

	wf, err := NewWorkflow(context.Background(), graph)
	require.NoError(t, err)
	require.Len(t, wf.nodes, 3)

	// Since n1 -> n2 -> n3, topological sort should guarantee this order for Start/Initialize
	assert.Equal(t, "Node 1", wf.nodes[0].name)
	assert.Equal(t, "Node 2", wf.nodes[1].name)
	assert.Equal(t, "Node 3", wf.nodes[2].name)

	err = wf.Start()
	require.NoError(t, err)

	wf.Stop()
}

func TestWorkflow_Dispatch(t *testing.T) {
	// ignore duplicate registration error; plugin may already be registered from other tests
	_ = RegisterPlugin[*MockPluginConfig](func(cfg *MockPluginConfig) Plugin {
		return &BuiltinPlugin{}
	}, PluginMetadata{
		Name: "test_node",
	})

	config := &WorkflowConfig{
		ID:   "wf-3",
		Head: "n1",
		Nodes: []NodeConfig{
			{ID: "n1", Name: "Node 1", Plugin: "test_node"},
		},
	}

	graph, err := BuildGraph(config)
	require.NoError(t, err)

	wf, err := NewWorkflow(context.Background(), graph)
	require.NoError(t, err)
	require.NoError(t, wf.Start())
	defer wf.Stop()

	// Intercept output
	wctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	payload := schema.NewPayload("")
	// Set data safely via Properties methods
	require.NoError(t, payload.Set("content", "hello"))

	// Test SendToHead wrapper function
	err = wf.SendToHead(payload.ReadOnly())
	require.NoError(t, err)

	// Since we are mocking, just wait a bit to ensure it doesn't crash
	<-wctx.Done()
}

func TestWorkflow_SendToNodeWithName(t *testing.T) {
	_ = RegisterPlugin[*MockPluginConfig](func(cfg *MockPluginConfig) Plugin {
		return &BuiltinPlugin{}
	}, PluginMetadata{
		Name: "test_node",
	})

	config := &WorkflowConfig{
		ID:   "wf-4",
		Head: "n1",
		Nodes: []NodeConfig{
			{ID: "n1", Name: "Node 1", Plugin: "test_node"},
			{ID: "n2", Name: "Node 2", Plugin: "test_node"},
		},
	}

	graph, err := BuildGraph(config)
	require.NoError(t, err)

	wf, err := NewWorkflow(context.Background(), graph)
	require.NoError(t, err)
	require.NoError(t, wf.Start())
	defer wf.Stop()

	require.Len(t, wf.nameMap, 2)
	idx1, ok1 := wf.nameMap["Node 1"]
	require.True(t, ok1)
	assert.Equal(t, "Node 1", wf.nodes[idx1].name)

	idx2, ok2 := wf.nameMap["Node 2"]
	require.True(t, ok2)
	assert.Equal(t, "Node 2", wf.nodes[idx2].name)

	payload := schema.NewPayload("")
	require.NoError(t, payload.Set("content", "hello"))

	// Test SendToNodeWithName
	err = wf.SendToNodeWithName("Node 2", payload.ReadOnly())
	require.NoError(t, err)

	// Test SendToNodeWithName with non-existent name
	err = wf.SendToNodeWithName("NonExistent", payload.ReadOnly())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "node Name NonExistent not found")
}
