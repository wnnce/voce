package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
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
		testWorkflowDispatch(t)
	})
	t.Run("StartFailureRollback", func(t *testing.T) {
		testWorkflowStartFailureRollback(t, SchedulerModeThreadPerNode)
		testWorkflowStartFailureRollback(t, SchedulerModeWorkerPool)
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

// payloadProbe records received payload content and forwards it unchanged, so a
// chain of probes observes both dispatch into the head and edge forwarding.
type payloadProbe struct {
	BuiltinPlugin
	mu       sync.Mutex
	payloads []string
}

func (p *payloadProbe) OnPayload(ctx context.Context, flow Flow, payload schema.Payload) {
	p.mu.Lock()
	p.payloads = append(p.payloads, schema.GetAs[string](payload, "content", ""))
	p.mu.Unlock()
	p.BuiltinPlugin.OnPayload(ctx, flow, payload)
}

func (p *payloadProbe) received() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.payloads...)
}

func testWorkflowDispatch(t *testing.T) {
	head := &payloadProbe{}
	tail := &payloadProbe{}

	// Builders are supplied directly so the test stays independent of the global registry.
	builders := map[string]PluginBuilder{
		"n1": testPluginBuilder{name: "payload_probe", factory: func() Plugin { return head }},
		"n2": testPluginBuilder{name: "payload_probe", factory: func() Plugin { return tail }},
	}

	config := &WorkflowConfig{
		ID:   "wf-dispatch",
		Head: "n1",
		Nodes: []NodeConfig{
			{ID: "n1", Name: "Node 1", Plugin: "payload_probe"},
			{ID: "n2", Name: "Node 2", Plugin: "payload_probe"},
		},
		Edges: []EdgeConfig{
			{Source: "n1", Target: "n2", Type: EventPayload},
		},
	}

	graph, err := BuildGraph(config, builders)
	require.NoError(t, err)

	wf, err := NewWorkflow(context.Background(), graph, builders)
	require.NoError(t, err)
	require.NoError(t, wf.Start())
	defer wf.Stop()

	payload := schema.NewPayload("")
	require.NoError(t, payload.Set("content", "hello"))
	require.NoError(t, wf.SendToHead(payload.ReadOnly()))

	require.Eventually(t, func() bool {
		return len(head.received()) == 1 && len(tail.received()) == 1
	}, time.Second, 10*time.Millisecond, "payload was not dispatched through the graph")

	assert.Equal(t, []string{"hello"}, head.received())
	assert.Equal(t, []string{"hello"}, tail.received())
}

// startProbe records lifecycle callbacks into a shared event log.
type startProbe struct {
	BuiltinPlugin
	name      string
	failStart bool
	mu        *sync.Mutex
	events    *[]string
}

func (p *startProbe) record(event string) {
	p.mu.Lock()
	*p.events = append(*p.events, event)
	p.mu.Unlock()
}

func (p *startProbe) OnStart(_ context.Context, _ Flow) error {
	p.record("start:" + p.name)
	if p.failStart {
		return fmt.Errorf("start failed: %s", p.name)
	}
	return nil
}

func (p *startProbe) OnReady(_ context.Context, _ Flow) { p.record("ready:" + p.name) }

func (p *startProbe) OnStop() { p.record("stop:" + p.name) }

func snapshotEvents(mu *sync.Mutex, events *[]string) []string {
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), *events...)
}

func countEventPrefix(events []string, prefix string) int {
	n := 0
	for _, event := range events {
		if strings.HasPrefix(event, prefix) {
			n++
		}
	}
	return n
}

// testWorkflowStartFailureRollback verifies that a node failing OnStart stops
// every node already started, in reverse start order, and skips OnReady.
func testWorkflowStartFailureRollback(t *testing.T, mode SchedulerMode) {
	t.Run(string(mode), func(t *testing.T) {
		var mu sync.Mutex
		var events []string
		newProbe := func(name string, failStart bool) Plugin {
			return &startProbe{name: name, failStart: failStart, mu: &mu, events: &events}
		}

		builders := map[string]PluginBuilder{
			"n1": testPluginBuilder{name: "start_probe", factory: func() Plugin { return newProbe("n1", false) }},
			"n2": testPluginBuilder{name: "start_probe", factory: func() Plugin { return newProbe("n2", false) }},
			"n3": testPluginBuilder{name: "start_probe", factory: func() Plugin { return newProbe("n3", true) }},
		}

		config := &WorkflowConfig{
			ID:            "wf-start-rollback",
			Head:          "n1",
			SchedulerMode: mode,
			Nodes: []NodeConfig{
				{ID: "n1", Name: "Node 1", Plugin: "start_probe"},
				{ID: "n2", Name: "Node 2", Plugin: "start_probe"},
				{ID: "n3", Name: "Node 3", Plugin: "start_probe"},
			},
			Edges: []EdgeConfig{
				{Source: "n1", Target: "n2", Type: EventPayload},
				{Source: "n2", Target: "n3", Type: EventPayload},
			},
		}

		graph, err := BuildGraph(config, builders)
		require.NoError(t, err)
		wf, err := NewWorkflow(context.Background(), graph, builders)
		require.NoError(t, err)
		// Start already moved the workflow to Stopped, so Workflow.Stop is a no-op
		// and never cancels ctx; cancel directly to release scheduler goroutines.
		t.Cleanup(wf.cancel)

		err = wf.Start()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Node 3")
		assert.Equal(t, WorkflowStateStopped, wf.State())

		// n3 failed before starting its loop, so only n1 and n2 must be stopped.
		require.Eventually(t, func() bool {
			return countEventPrefix(snapshotEvents(&mu, &events), "stop:") == 2
		}, time.Second, 10*time.Millisecond, "started nodes were not rolled back: %v", snapshotEvents(&mu, &events))

		got := snapshotEvents(&mu, &events)
		assert.NotContains(t, got, "stop:n3", "the failing node must not be stopped")
		assert.NotContains(t, got, "ready:n1", "OnReady must not run when startup fails")
		assert.Equal(t, 1, countEventPrefix(got, "start:n1"))
		assert.Equal(t, 1, countEventPrefix(got, "start:n2"))
		assert.Equal(t, 1, countEventPrefix(got, "start:n3"))

		// worker-pool stops nodes synchronously, so the reverse order is observable.
		if mode == SchedulerModeWorkerPool {
			assert.Equal(t, []string{
				"start:n1", "start:n2", "start:n3",
				"stop:n2", "stop:n1",
			}, got)
		}
	})
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
