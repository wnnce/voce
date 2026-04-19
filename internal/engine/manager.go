package engine

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
)

type WorkflowConfigManager interface {
	Get(id string) (WorkflowConfig, error)
	GetWithName(name string) (WorkflowConfig, error)
	List() ([]WorkflowConfig, error)
	Save(cfg WorkflowConfig) error
	Delete(id string) error
}

type fileWorkflowConfigManager struct {
	dirPath string
	mu      sync.RWMutex
	configs map[string]WorkflowConfig
	nameMap map[string]string // Name -> ID
}

// NewFileWorkflowConfigManager creates a new instance.
func NewFileWorkflowConfigManager(dirPath string) WorkflowConfigManager {
	m := &fileWorkflowConfigManager{
		dirPath: dirPath,
		configs: make(map[string]WorkflowConfig),
		nameMap: make(map[string]string),
	}
	m.load()
	return m
}

func (m *fileWorkflowConfigManager) load() {
	if err := os.MkdirAll(m.dirPath, 0755); err != nil {
		slog.Error("failed to create workflow config directory", "path", m.dirPath, "error", err)
		return
	}

	files, err := os.ReadDir(m.dirPath)
	if err != nil {
		slog.Error("failed to read config directory", "path", m.dirPath, "error", err)
		return
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(m.dirPath, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			slog.Error("failed to read workflow config file", "path", filePath, "error", err)
			continue
		}

		var cfg WorkflowConfig
		if err := sonic.Unmarshal(data, &cfg); err != nil {
			slog.Error("failed to unmarshal workflow config", "path", filePath, "error", err)
			continue
		}
		m.configs[cfg.ID] = cfg
		if cfg.Name != "" {
			m.nameMap[cfg.Name] = cfg.ID
		}
	}
}

func (m *fileWorkflowConfigManager) Get(id string) (WorkflowConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg, ok := m.configs[id]
	if !ok {
		return WorkflowConfig{}, fmt.Errorf("workflow config %s not found", id)
	}
	return cfg, nil
}

func (m *fileWorkflowConfigManager) GetWithName(name string) (WorkflowConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.nameMap[name]
	if !ok {
		return WorkflowConfig{}, fmt.Errorf("workflow config with name %s not found", name)
	}
	return m.configs[id], nil
}

func (m *fileWorkflowConfigManager) List() ([]WorkflowConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]WorkflowConfig, 0, len(m.configs))
	for _, cfg := range m.configs {
		list = append(list, cfg)
	}
	return list, nil
}

func (m *fileWorkflowConfigManager) Save(cfg WorkflowConfig) error {
	if _, err := BuildGraph(&cfg); err != nil {
		return fmt.Errorf("invalid workflow config: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existingID, ok := m.nameMap[cfg.Name]; ok && existingID != cfg.ID {
		return fmt.Errorf("workflow name %s is already used by another workflow", cfg.Name)
	}

	if strings.TrimSpace(cfg.ID) == "" {
		cfg.ID = uuid.New().String()
	}

	data, err := sonic.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal workflow config: %w", err)
	}

	filePath := filepath.Join(m.dirPath, cfg.ID+".json")
	tmpPath := filePath + ".tmp"
	if err = os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write workflow config tmp: %w", err)
	}

	if err = os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("rename workflow config: %w", err)
	}

	if oldCfg, ok := m.configs[cfg.ID]; ok && oldCfg.Name != cfg.Name {
		delete(m.nameMap, oldCfg.Name)
	}

	m.configs[cfg.ID] = cfg
	m.nameMap[cfg.Name] = cfg.ID

	return nil
}

func (m *fileWorkflowConfigManager) Delete(id string) error {
	filePath := filepath.Join(m.dirPath, id+".json")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove workflow config file: %w", err)
	}

	m.mu.Lock()
	if cfg, ok := m.configs[id]; ok && cfg.Name != "" {
		delete(m.nameMap, cfg.Name)
	}
	delete(m.configs, id)
	m.mu.Unlock()

	return nil
}
