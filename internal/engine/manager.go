package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type WorkflowConfigManager interface {
	Get(ctx context.Context, id string) (WorkflowConfig, error)
	GetWithName(ctx context.Context, name string) (WorkflowConfig, error)
	List(ctx context.Context) ([]WorkflowConfig, error)
	Save(ctx context.Context, cfg WorkflowConfig) error
	Delete(ctx context.Context, id string) error
}

var (
	ErrWorkflowNotFound   = errors.New("workflow not found")
	ErrWorkflowNameExists = errors.New("workflow name already exists")
)

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

func (m *fileWorkflowConfigManager) Get(ctx context.Context, id string) (WorkflowConfig, error) {
	var zero WorkflowConfig
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg, ok := m.configs[id]
	if !ok {
		return zero, ErrWorkflowNotFound
	}
	return cfg, nil
}

func (m *fileWorkflowConfigManager) GetWithName(ctx context.Context, name string) (WorkflowConfig, error) {
	var zero WorkflowConfig
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.nameMap[name]
	if !ok {
		return zero, ErrWorkflowNotFound
	}
	return m.configs[id], nil
}

func (m *fileWorkflowConfigManager) List(ctx context.Context) ([]WorkflowConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]WorkflowConfig, 0, len(m.configs))
	for _, cfg := range m.configs {
		list = append(list, cfg)
	}
	return list, nil
}

func (m *fileWorkflowConfigManager) Save(ctx context.Context, cfg WorkflowConfig) error {
	if _, err := BuildGraph(&cfg); err != nil {
		return fmt.Errorf("invalid workflow config: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existingID, ok := m.nameMap[cfg.Name]; ok && existingID != cfg.ID {
		return ErrWorkflowNameExists
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

func (m *fileWorkflowConfigManager) Delete(ctx context.Context, id string) error {
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

const (
	luaSaveWorkflow = `
local id = ARGV[1]
local name = ARGV[2]
local json = ARGV[3]
local workflows_key = KEYS[1]
local names_key = KEYS[2]

-- 1. check if name is taken by another id
local existing_id = redis.call('HGET', names_key, name)
if existing_id and existing_id ~= id then
    return redis.error_reply("workflow name already exists")
end

-- 2. cleanup old name index if name changed
local old_json = redis.call('HGET', workflows_key, id)
if old_json then
    local old_cfg = cjson.decode(old_json)
    if old_cfg.name ~= name then
        redis.call('HDEL', names_key, old_cfg.name)
    end
end

-- 3. update data and index
redis.call('HSET', workflows_key, id, json)
redis.call('HSET', names_key, name, id)
return "OK"
`
	luaDeleteWorkflow = `
local id = ARGV[1]
local workflows_key = KEYS[1]
local names_key = KEYS[2]

local old_json = redis.call('HGET', workflows_key, id)
if old_json then
    local old_cfg = cjson.decode(old_json)
    redis.call('HDEL', names_key, old_cfg.name)
end
redis.call('HDEL', workflows_key, id)
return "OK"
`
)

type redisWorkflowConfigManager struct {
	rdb          *redis.Client
	keyWorkflows string
	keyNames     string
}

// NewRedisWorkflowConfigManager creates a new Redis implementation of WorkflowConfigManager.
func NewRedisWorkflowConfigManager(rdb *redis.Client) WorkflowConfigManager {
	return &redisWorkflowConfigManager{
		rdb:          rdb,
		keyWorkflows: "voce:workflows",
		keyNames:     "voce:workflow_names",
	}
}

func (m *redisWorkflowConfigManager) Get(ctx context.Context, id string) (WorkflowConfig, error) {
	var zero WorkflowConfig
	data, err := m.rdb.HGet(ctx, m.keyWorkflows, id).Result()
	if errors.Is(err, redis.Nil) {
		return zero, ErrWorkflowNotFound
	}
	if err != nil {
		return zero, err
	}

	var cfg WorkflowConfig
	if err = sonic.UnmarshalString(data, &cfg); err != nil {
		return zero, err
	}
	return cfg, nil
}

func (m *redisWorkflowConfigManager) GetWithName(ctx context.Context, name string) (WorkflowConfig, error) {
	var zero WorkflowConfig
	id, err := m.rdb.HGet(ctx, m.keyNames, name).Result()
	if errors.Is(err, redis.Nil) {
		return zero, ErrWorkflowNotFound
	}
	if err != nil {
		return zero, err
	}
	return m.Get(ctx, id)
}

func (m *redisWorkflowConfigManager) List(ctx context.Context) ([]WorkflowConfig, error) {
	vals, err := m.rdb.HVals(ctx, m.keyWorkflows).Result()
	if err != nil {
		return nil, err
	}

	list := make([]WorkflowConfig, 0, len(vals))
	for _, data := range vals {
		var cfg WorkflowConfig
		if err = sonic.UnmarshalString(data, &cfg); err != nil {
			slog.Error("failed to unmarshal workflow config from redis", "error", err)
			continue
		}
		list = append(list, cfg)
	}
	return list, nil
}

func (m *redisWorkflowConfigManager) Save(ctx context.Context, cfg WorkflowConfig) error {
	if _, err := BuildGraph(&cfg); err != nil {
		return fmt.Errorf("invalid workflow config: %w", err)
	}

	if strings.TrimSpace(cfg.ID) == "" {
		cfg.ID = uuid.New().String()
	}

	data, err := sonic.MarshalString(cfg)
	if err != nil {
		return err
	}

	err = m.rdb.Eval(ctx, luaSaveWorkflow, []string{m.keyWorkflows, m.keyNames}, cfg.ID, cfg.Name, data).Err()
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return ErrWorkflowNameExists
		}
		return fmt.Errorf("redis save workflow: %w", err)
	}

	return nil
}

func (m *redisWorkflowConfigManager) Delete(ctx context.Context, id string) error {
	err := m.rdb.Eval(ctx, luaDeleteWorkflow, []string{m.keyWorkflows, m.keyNames}, id).Err()
	if err != nil {
		return fmt.Errorf("redis delete workflow: %w", err)
	}
	return nil
}
