package engine

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestWorkflow(t *testing.T) {
	store := NewPluginStore(LocalPluginResource())

	t.Run("TopologicalSortAndExecution", func(t *testing.T) {
		testWorkflowTopologicalSortAndExecution(t, store)
	})
	t.Run("Dispatch", func(t *testing.T) {
		testWorkflowDispatch(t, store)
	})
	t.Run("SendToNodeWithName", func(t *testing.T) {
		testWorkflowSendToNodeWithName(t, store)
	})
	t.Run("WorkerPoolExecution", func(t *testing.T) {
		testWorkflowWorkerPoolExecution(t, store)
	})
	t.Run("WorkerPoolDefaultWorkers", func(t *testing.T) {
		testWorkflowWorkerPoolDefaultWorkers(t, store)
	})
}

func testWorkflowTopologicalSortAndExecution(t *testing.T, store *PluginStore) {
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

	builders := localTestPluginBuilders(t, store, config.Nodes)
	graph, err := BuildGraph(config, builders)
	require.NoError(t, err)

	wf, err := NewWorkflow(context.Background(), graph, builders)
	require.NoError(t, err)
	require.Len(t, wf.nodes, 3)

	// Since n1 -> n2 -> n3, topological sort should guarantee this order for Start/Initialize
	assert.Equal(t, "Node 1", wf.nodes[0].Name())
	assert.Equal(t, "Node 2", wf.nodes[1].Name())
	assert.Equal(t, "Node 3", wf.nodes[2].Name())

	err = wf.Start()
	require.NoError(t, err)

	wf.Stop()
}

func testWorkflowDispatch(t *testing.T, store *PluginStore) {
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

	builders := localTestPluginBuilders(t, store, config.Nodes)
	graph, err := BuildGraph(config, builders)
	require.NoError(t, err)

	wf, err := NewWorkflow(context.Background(), graph, builders)
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

func testWorkflowSendToNodeWithName(t *testing.T, store *PluginStore) {
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

	builders := localTestPluginBuilders(t, store, config.Nodes)
	graph, err := BuildGraph(config, builders)
	require.NoError(t, err)

	wf, err := NewWorkflow(context.Background(), graph, builders)
	require.NoError(t, err)
	require.NoError(t, wf.Start())
	defer wf.Stop()

	require.Len(t, wf.nameMap, 2)
	idx1, ok1 := wf.nameMap["Node 1"]
	require.True(t, ok1)
	assert.Equal(t, "Node 1", wf.nodes[idx1].Name())

	idx2, ok2 := wf.nameMap["Node 2"]
	require.True(t, ok2)
	assert.Equal(t, "Node 2", wf.nodes[idx2].Name())

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

func testWorkflowWorkerPoolExecution(t *testing.T, store *PluginStore) {
	_ = RegisterPlugin[*MockPluginConfig](func(cfg *MockPluginConfig) Plugin {
		return &BuiltinPlugin{}
	}, PluginMetadata{
		Name: "test_node_wp",
	})

	config := &WorkflowConfig{
		ID:               "wf-workerpool",
		Head:             "n1",
		SchedulerMode:    "worker-pool",
		SchedulerWorkers: 2,
		Nodes: []NodeConfig{
			{ID: "n1", Name: "Node 1", Plugin: "test_node_wp"},
			{ID: "n2", Name: "Node 2", Plugin: "test_node_wp"},
		},
		Edges: []EdgeConfig{
			{Source: "n1", Target: "n2", Type: EventPayload, SourcePort: 0},
		},
	}

	builders := localTestPluginBuilders(t, store, config.Nodes)
	graph, err := BuildGraph(config, builders)
	require.NoError(t, err)

	wf, err := NewWorkflow(context.Background(), graph, builders)
	require.NoError(t, err)
	require.NotNil(t, wf.scheduler)

	err = wf.Start()
	require.NoError(t, err)

	payload := schema.NewPayload("")
	require.NoError(t, payload.Set("content", "hello"))

	err = wf.SendToHead(payload.ReadOnly())
	require.NoError(t, err)

	// Stop workflow
	wf.Stop()
}

func testWorkflowWorkerPoolDefaultWorkers(t *testing.T, store *PluginStore) {
	_ = RegisterPlugin[*MockPluginConfig](func(cfg *MockPluginConfig) Plugin {
		return &BuiltinPlugin{}
	}, PluginMetadata{
		Name: "test_node_wp_default",
	})

	tests := []struct {
		name            string
		nodeCount       int
		inputWorkers    int
		expectedWorkers int
	}{
		{"less than 4 nodes, invalid input", 3, 0, 1},
		{"less than 4 nodes, valid input", 3, 2, 2},
		{"5 nodes, invalid input", 5, 0, 2},
		{"8 nodes, invalid input", 8, 0, 2},
		{"8 nodes, too large input", 8, 9, 2},
		{"8 nodes, valid input", 8, 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := make([]NodeConfig, tt.nodeCount)
			for i := 0; i < tt.nodeCount; i++ {
				id := fmt.Sprintf("n%d", i+1)
				nodes[i] = NodeConfig{ID: id, Name: "Node " + id, Plugin: "test_node_wp_default"}
			}

			config := &WorkflowConfig{
				ID:               "wf-test",
				Head:             "n1",
				SchedulerMode:    "worker-pool",
				SchedulerWorkers: tt.inputWorkers,
				Nodes:            nodes,
			}

			builders := localTestPluginBuilders(t, store, config.Nodes)
			graph, err := BuildGraph(config, builders)
			require.NoError(t, err)

			wf, err := NewWorkflow(context.Background(), graph, builders)
			require.NoError(t, err)
			defer wf.Stop()

			require.NotNil(t, wf.scheduler)
			assert.Len(t, wf.scheduler.workers, tt.expectedWorkers)
		})
	}
}
