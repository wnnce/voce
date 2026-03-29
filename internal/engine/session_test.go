package engine

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeScenarios(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		overlay  string
		expected string
	}{
		{
			name: "Simple Deep Merge",
			base: `{
				"params": {"temp": 0.7, "top_p": 1.0},
				"model": "base"
			}`,
			overlay: `{
				"params": {"temp": 0.1},
				"model": "overlay"
			}`,
			expected: `{"params":{"temp":0.1,"top_p":1},"model":"overlay"}`,
		},
		{
			name:     "Nested Object Creation",
			base:     `{"a": {"b": 1}}`,
			overlay:  `{"a": {"c": 2}, "d": 3}`,
			expected: `{"a":{"b":1,"c":2},"d":3}`,
		},
		{
			name:     "Array Overwrite (Atomic)",
			base:     `{"list": [1, 2], "stay": true}`,
			overlay:  `{"list": [3]}`,
			expected: `{"list":[3],"stay":true}`,
		},
		{
			name:     "Type Mismatch Overwrite",
			base:     `{"key": {"obj": true}}`,
			overlay:  `{"key": "string"}`,
			expected: `{"key":"string"}`,
		},
		{
			name: "Deep Recursive Merge",
			base: `{
				"x": {
					"y": {
						"z": 1,
						"w": 2
					}
				}
			}`,
			overlay: `{
				"x": {
					"y": {
						"z": 3
					}
				}
			}`,
			expected: `{"x":{"y":{"z":3,"w":2}}}`,
		},
		{
			name: "Enterprise LLM Config Merge (Multi-level & Mixed)",
			base: `{
				"provider": "openai",
				"settings": {
					"network": {
						"timeout": 30,
						"retry": { "count": 3, "backoff": 1.5 }
					},
					"auth": { "type": "api_key", "value": "secret" }
				},
				"runtime": { "debug": true }
			}`,
			overlay: `{
				"settings": {
					"network": {
						"retry": { "count": 10 }
					},
					"auth": { "type": "bearer" }
				},
				"runtime": "production_mode"
			}`,
			expected: `{
				"provider": "openai",
				"settings": {
					"network": {
						"timeout": 30,
						"retry": { "count": 10, "backoff": 1.5 }
					},
					"auth": { "type": "bearer", "value": "secret" }
				},
				"runtime": "production_mode"
			}`,
		},
		{
			name: "Array and Empty Object Edge Cases",
			base: `{
				"metadata": { "tags": ["v1"], "owner": "admin" },
				"plugins": { "asr": { "enabled": true } }
			}`,
			overlay: `{
				"metadata": { "tags": ["v2", "stable"] },
				"plugins": { "asr": {}, "tts": { "enabled": false } }
			}`,
			expected: `{
				"metadata": { "tags": ["v2", "stable"], "owner": "admin" },
				"plugins": { 
					"asr": { "enabled": true },
					"tts": { "enabled": false }
				}
			}`,
		},
		{
			name: "Deep Strategy & Prompt Engineering Inject",
			base: `{
				"prompt_template": {
					"system": "You are a bot.",
					"variables": { "user": "Guest", "context": "None" }
				}
			}`,
			overlay: `{
				"prompt_template": {
					"variables": { "user": "Alice", "role": "Manager" },
					"safety_filter": { "level": "high" }
				}
			}`,
			expected: `{
				"prompt_template": {
					"system": "You are a bot.",
					"variables": { "user": "Alice", "context": "None", "role": "Manager" },
					"safety_filter": { "level": "high" }
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseNode, err := sonic.Get([]byte(tt.base))
			if err != nil {
				t.Fatalf("failed to parse base: %v", err)
			}
			overlayNode, err := sonic.Get([]byte(tt.overlay))
			if err != nil {
				t.Fatalf("failed to parse overlay: %v", err)
			}

			if err = deepMergeAST(&baseNode, &overlayNode); err != nil {
				t.Fatalf("merge failed: %v", err)
			}

			gotRaw, _ := baseNode.Raw()

			var gotMap, expectedMap map[string]any
			if err = sonic.Unmarshal([]byte(gotRaw), &gotMap); err != nil {
				t.Fatalf("failed to unmarshal got: %v", err)
			}
			if err = sonic.Unmarshal([]byte(tt.expected), &expectedMap); err != nil {
				t.Fatalf("failed to unmarshal expected: %v", err)
			}

			if !reflect.DeepEqual(gotMap, expectedMap) {
				t.Errorf("expected %v, got %v", expectedMap, gotMap)
			}
		})
	}
}

func BenchmarkDeepMerge(b *testing.B) {
	baseJSON := []byte(`{
		"provider": "openai",
		"settings": {
			"network": {
				"timeout": 30,
				"retry": { "count": 3, "backoff": 1.5 }
			},
			"auth": { "type": "api_key", "value": "secret" }
		},
		"runtime": { "debug": true },
		"model": "gpt-4"
	}`)
	overlayJSON := []byte(`{
		"settings": {
			"network": {
				"retry": { "count": 10 }
			},
			"auth": { "type": "bearer" }
		},
		"runtime": "production_mode"
	}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		baseNode, _ := sonic.Get(baseJSON)
		overlayNode, _ := sonic.Get(overlayJSON)
		_ = deepMergeAST(&baseNode, &overlayNode)
		_, _ = baseNode.Raw()
	}
}

func TestSessionManager_Lifecycle(t *testing.T) {
	// Setup WorkflowManager with a temp directory
	tempDir := t.TempDir()
	DefaultWorkflowManager = newFileWorkflowConfigManager(tempDir)

	// Register a mock plugin
	_ = RegisterPlugin[*MockPluginConfig](func(cfg *MockPluginConfig) Plugin {
		return &BuiltinPlugin{}
	}, PluginMetadata{Name: "test_node"})

	// Save a test workflow config
	wfCfg := WorkflowConfig{
		ID:   "w1",
		Name: "TestWorkflow",
		Head: "n1",
		Nodes: []NodeConfig{
			{ID: "n1", Name: "Node1", Plugin: "test_node", Config: json.RawMessage(`{"value": "initial"}`)},
		},
	}
	require.NoError(t, DefaultWorkflowManager.Save(wfCfg))

	sm := NewSessionManager(500 * time.Millisecond)
	defer sm.Stop()

	// Test CreateSession
	ctx := context.Background()
	props := map[string]json.RawMessage{
		"Node1": json.RawMessage(`{"value": "overridden"}`),
	}
	session, err := sm.CreateSession(ctx, "s1", "TestWorkflow", props)
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "s1", session.ID)

	// Verify deep merge worked
	// Since NewWorkflow is called inside CreateSession, we can't easily check wf.graph but we can check if it started
	assert.Equal(t, int32(WorkflowStateRunning), session.Workflow.state.Load())

	// Test LoadSession
	loaded, ok := sm.LoadSession("s1")
	assert.True(t, ok)
	assert.Equal(t, session, loaded)

	// Test LoadSession with non-existent ID
	_, ok = sm.LoadSession("non-existent")
	assert.False(t, ok)

	// Test Session Activity Update
	oldLastActive := session.LastActive.Load()
	time.Sleep(10 * time.Millisecond)
	sm.LoadSession("s1")
	assert.Greater(t, session.LastActive.Load(), oldLastActive)

	// Test Stop
	sm.Stop()
	assert.Empty(t, sm.sessions)
	require.Error(t, sm.ctx.Err())
	assert.Equal(t, int32(WorkflowStateStopped), session.Workflow.state.Load())
}

func TestSessionManager_Cleanup(t *testing.T) {
	// Setup WorkflowManager
	tempDir := t.TempDir()
	DefaultWorkflowManager = newFileWorkflowConfigManager(tempDir)

	_ = RegisterPlugin[*MockPluginConfig](func(cfg *MockPluginConfig) Plugin {
		return &BuiltinPlugin{}
	}, PluginMetadata{Name: "test_node"})

	wfCfg := WorkflowConfig{
		ID:   "w2",
		Name: "CleanupWorkflow",
		Head: "n1",
		Nodes: []NodeConfig{
			{ID: "n1", Name: "Node1", Plugin: "test_node"},
		},
	}
	require.NoError(t, DefaultWorkflowManager.Save(wfCfg))

	// Set a very short timeout for testing cleanup
	timeout := 100 * time.Millisecond
	sm := NewSessionManager(timeout)
	defer sm.Stop()

	session, err := sm.CreateSession(context.Background(), "s2", "CleanupWorkflow", nil)
	require.NoError(t, err)

	// Initially session should be there
	_, ok := sm.LoadSession("s2")
	assert.True(t, ok)

	// Wait for cleanup ticker to trigger (ticker runs at timeout / 2)
	// We wait slightly more than timeout to be sure
	time.Sleep(timeout * 2)

	// Now session should be gone
	_, ok = sm.LoadSession("s2")
	assert.False(t, ok)
	assert.Equal(t, int32(WorkflowStateStopped), session.Workflow.state.Load())
}

func TestDeepMergeJSON_NullInputs(t *testing.T) {
	sm := &SessionManager{}

	// Test both null
	merged, err := sm.deepMergeJSON(nil, nil)
	require.NoError(t, err)
	assert.Nil(t, merged)

	// Test base null
	overlay := json.RawMessage(`{"a":1}`)
	merged, err = sm.deepMergeJSON(nil, overlay)
	require.NoError(t, err)
	assert.Equal(t, overlay, merged)

	// Test overlay null
	base := json.RawMessage(`{"b":2}`)
	merged, err = sm.deepMergeJSON(base, nil)
	require.NoError(t, err)
	assert.Equal(t, base, merged)
}

func TestDeepMergeAST_NonObject(t *testing.T) {
	// If base or overlay are not objects, deepMergeAST should return nil and not modify base
	baseStr := `[1, 2]`
	overlayStr := `{"a": 1}`
	baseNode, _ := sonic.Get([]byte(baseStr))
	overlayNode, _ := sonic.Get([]byte(overlayStr))

	err := deepMergeAST(&baseNode, &overlayNode)
	require.NoError(t, err)

	raw, _ := baseNode.Raw()
	assert.Contains(t, raw, "[1, 2]")
}

func TestSessionManager_RemoveSession(t *testing.T) {
	// Setup environment
	tempDir := t.TempDir()
	DefaultWorkflowManager = newFileWorkflowConfigManager(tempDir)

	_ = RegisterPlugin[*MockPluginConfig](func(cfg *MockPluginConfig) Plugin {
		return &BuiltinPlugin{}
	}, PluginMetadata{Name: "test_node"})

	wfCfg := WorkflowConfig{
		ID: "wf_remove", Name: "RemoveTest", Head: "n1",
		Nodes: []NodeConfig{{ID: "n1", Name: "Node1", Plugin: "test_node"}},
	}
	require.NoError(t, DefaultWorkflowManager.Save(wfCfg))

	sm := NewSessionManager(5 * time.Second)
	defer sm.Stop()

	// 1. Create session
	session, err := sm.CreateSession(context.Background(), "s_remove", "RemoveTest", nil)
	require.NoError(t, err)
	assert.Equal(t, int32(WorkflowStateRunning), session.Workflow.state.Load())
	assert.Equal(t, int64(1), int64(sm.Count()))

	// 2. Remove session
	sm.RemoveSession("s_remove")

	// 3. Verify removal and stop
	_, ok := sm.LoadSession("s_remove")
	assert.False(t, ok)
	assert.Equal(t, int64(0), int64(sm.Count()))
	assert.Equal(t, int32(WorkflowStateStopped), session.Workflow.state.Load())

	// 4. Test idempotency (should not panic or error)
	assert.NotPanics(t, func() {
		sm.RemoveSession("s_remove")
		sm.RemoveSession("non_existent")
	})
}
