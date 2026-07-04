package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildGraph(t *testing.T) {
	store := NewPluginStore(LocalPluginResource())

	t.Run("TopologicalSort", func(t *testing.T) {
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

		builders := localTestPluginBuilders(t, store, config.Nodes)
		g, err := BuildGraph(config, builders)
		require.NoError(t, err)
		require.Len(t, g.OrderedNodes, 3)

		assert.Equal(t, "n1", g.OrderedNodes[0].ID)
		assert.Equal(t, "n2", g.OrderedNodes[1].ID)
		assert.Equal(t, "n3", g.OrderedNodes[2].ID)
	})

	t.Run("CycleDetection", func(t *testing.T) {
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

		builders := localTestPluginBuilders(t, store, config.Nodes)
		_, err := BuildGraph(config, builders)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "self-loop")
	})
}

func localTestPluginBuilders(t *testing.T, store *PluginStore, nodes []NodeConfig) map[string]PluginBuilder {
	t.Helper()
	builders, err := ResolvePluginBuilders(store, nodes)
	require.NoError(t, err)
	return builders
}
