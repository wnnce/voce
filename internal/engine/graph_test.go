package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildGraph_TopologicalSort(t *testing.T) {
	_ = RegisterPlugin[*MockPluginConfig](func(cfg *MockPluginConfig) Plugin {
		return &BuiltinPlugin{}
	}, PluginMetadata{
		Name: "graph_test_plg",
	})

	config := &WorkflowConfig{
		ID:   "wf-1",
		Head: "n1",
		Nodes: []NodeConfig{
			{ID: "n3", Name: "Node 3", Plugin: "graph_test_plg"},
			{ID: "n1", Name: "Node 1", Plugin: "graph_test_plg"},
			{ID: "n2", Name: "Node 2", Plugin: "graph_test_plg"},
		},
		Edges: []EdgeConfig{
			{Source: "n1", Target: "n2", Type: EventPayload, SourcePort: 0},
			{Source: "n2", Target: "n3", Type: EventPayload, SourcePort: 0},
		},
	}

	g, err := BuildGraph(config)
	require.NoError(t, err)
	require.Len(t, g.OrderedNodes, 3)

	assert.Equal(t, "n1", g.OrderedNodes[0].ID)
	assert.Equal(t, "n2", g.OrderedNodes[1].ID)
	assert.Equal(t, "n3", g.OrderedNodes[2].ID)
}

func TestBuildGraph_CycleDetection(t *testing.T) {
	_ = RegisterPlugin[*MockPluginConfig](func(cfg *MockPluginConfig) Plugin {
		return &BuiltinPlugin{}
	}, PluginMetadata{
		Name: "graph_test_plg_cycle",
	})

	config := &WorkflowConfig{
		ID:   "wf-2",
		Head: "n1",
		Nodes: []NodeConfig{
			{ID: "n1", Name: "Node 1", Plugin: "graph_test_plg_cycle"},
		},
		Edges: []EdgeConfig{
			{Source: "n1", Target: "n1", Type: EventSignal},
		},
	}

	_, err := BuildGraph(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "self-loop")
}
