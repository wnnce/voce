package engine

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPluginName = "cfg_mgr_test_plg"

func init() {
	_ = RegisterPlugin[*MockPluginConfig](func(cfg *MockPluginConfig) Plugin {
		return &BuiltinPlugin{}
	}, PluginMetadata{Name: testPluginName})
}

func testWorkflowConfig(id string, name string) WorkflowConfig {
	return WorkflowConfig{
		ID:   id,
		Name: name,
		Head: "n1",
		Nodes: []NodeConfig{
			{ID: "n1", Plugin: testPluginName},
		},
	}
}

func TestFileWorkflowConfigManager_LazyLoad(t *testing.T) {
	dirPath := t.TempDir()

	mgr := newFileWorkflowConfigManager(dirPath)

	list, err := mgr.List()
	require.NoError(t, err)
	assert.Empty(t, list)

	err = mgr.Save(testWorkflowConfig("w1", "Workflow 1"))
	require.NoError(t, err)

	// Verify file exists immediately
	_, err = os.Stat(filepath.Join(dirPath, "w1.json"))
	require.NoError(t, err)

	// New manager should load from dir
	mgr2 := newFileWorkflowConfigManager(dirPath)
	list2, err := mgr2.List()
	require.NoError(t, err)
	assert.Len(t, list2, 1)
	assert.Equal(t, "w1", list2[0].ID)
	assert.Equal(t, "Workflow 1", list2[0].Name)

	// Test GetWithName
	cfg, err := mgr2.GetWithName("Workflow 1")
	require.NoError(t, err)
	assert.Equal(t, "w1", cfg.ID)
}

func TestFileWorkflowConfigManager_NameUniqueness(t *testing.T) {
	dirPath := t.TempDir()
	mgr := newFileWorkflowConfigManager(dirPath)

	err := mgr.Save(testWorkflowConfig("w1", "DuplicateName"))
	require.NoError(t, err)

	// Try to save another workflow with the same name
	err = mgr.Save(testWorkflowConfig("w2", "DuplicateName"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already used")

	// Verify we can update the same workflow with the same name
	err = mgr.Save(testWorkflowConfig("w1", "DuplicateName"))
	require.NoError(t, err)

	// Verify we can change the name and the old name is released
	err = mgr.Save(testWorkflowConfig("w1", "NewName"))
	require.NoError(t, err)

	err = mgr.Save(testWorkflowConfig("w2", "DuplicateName"))
	require.NoError(t, err)
}

func TestFileWorkflowConfigManager_Concurrency(t *testing.T) {
	dirPath := t.TempDir()

	mgr := newFileWorkflowConfigManager(dirPath)

	const count = 50
	var wg sync.WaitGroup
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()
			id := string(rune('a'+idx%26)) + string(rune('0'+idx/26))
			name := "Workflow_" + id
			_ = mgr.Save(testWorkflowConfig(id, name))
		}(i)
	}
	wg.Wait()

	list, err := mgr.List()
	require.NoError(t, err)
	assert.Len(t, list, count)
}

func TestFileWorkflowConfigManager_Delete(t *testing.T) {
	dirPath := t.TempDir()
	mgr := newFileWorkflowConfigManager(dirPath)

	err := mgr.Save(testWorkflowConfig("d1", "DeleteMe"))
	require.NoError(t, err)

	// Verify file exists
	filePath := filepath.Join(dirPath, "d1.json")
	_, err = os.Stat(filePath)
	require.NoError(t, err)

	// Check name map
	_, err = mgr.GetWithName("DeleteMe")
	require.NoError(t, err)

	// Delete
	err = mgr.Delete("d1")
	require.NoError(t, err)

	// Verify file is gone
	_, err = os.Stat(filePath)
	assert.True(t, os.IsNotExist(err))

	// Verify name map is updated
	_, err = mgr.GetWithName("DeleteMe")
	require.Error(t, err)

	// Verify map is updated
	list, _ := mgr.List()
	assert.Empty(t, list)
}
