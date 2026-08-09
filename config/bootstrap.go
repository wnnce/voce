package config

import (
	"strconv"
	"time"

	"github.com/wnnce/voce/pkg/logging"
)

// CommonConfig shared across all service types
type CommonConfig struct {
	Name        string `json:"name" yaml:"name"`
	Environment string `json:"environment" yaml:"environment"`
	Version     string `json:"version" yaml:"version"`
}

// NetworkConfig shared network settings
type NetworkConfig struct {
	Host string `json:"host" yaml:"host"`
	Port int    `json:"port" yaml:"port"`
}

// Address returns host:port string
func (c NetworkConfig) Address() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

// RedisConfig holds connection settings for Redis
type RedisConfig struct {
	Host               string `json:"host" yaml:"host"`
	Port               int    `json:"port" yaml:"port"`
	Username           string `json:"username" yaml:"username"`
	Password           string `json:"password" yaml:"password"`
	DB                 int    `json:"db" yaml:"db"`
	UseTLS             bool   `json:"use_tls" yaml:"use_tls"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" yaml:"insecure_skip_verify"`
}

// VoceConfig is the configuration for a business Pod
type VoceConfig struct {
	CommonConfig  `yaml:",inline"`
	NetworkConfig `yaml:",inline"`
	GrpcPort      int                  `json:"grpc_port" yaml:"grpc_port"`
	Mode          string               `json:"mode" yaml:"mode"`
	GatewayAddr   string               `json:"gateway_addr" yaml:"gateway_addr"`
	PoolSize      int                  `json:"pool_size" yaml:"pool_size"`
	WorkflowStore string               `json:"workflow_store" yaml:"workflow_store"` // "file" or "redis"
	WorkflowDir   string               `json:"workflow_dir" yaml:"workflow_dir"`     // directory for "file" store
	PluginServers []PluginServerConfig `json:"plugin_servers" yaml:"plugin_servers"`
}

type PluginServerConfig struct {
	URL       string `json:"url" yaml:"url"`
	Namespace string `json:"namespace" yaml:"namespace"`
	Enable    bool   `json:"enable" yaml:"enable"`
}

// GatewayServerConfig is the configuration for the Gateway service itself
type GatewayServerConfig struct {
	CommonConfig                    `yaml:",inline"`
	NetworkConfig                   `yaml:",inline"`
	PoolMode                        string        `json:"pool_mode" yaml:"pool_mode"`
	PoolSize                        int           `json:"pool_size" yaml:"pool_size"`
	PoolMinConnections              int           `json:"pool_min_connections" yaml:"pool_min_connections"`
	PoolTargetSessionsPerConnection int           `json:"pool_target_sessions_per_connection" yaml:"pool_target_sessions_per_connection"`
	PoolMaxSessionsPerConnection    int           `json:"pool_max_sessions_per_connection" yaml:"pool_max_sessions_per_connection"`
	PoolMaxConnections              int           `json:"pool_max_connections" yaml:"pool_max_connections"`
	PoolIdleTimeout                 time.Duration `json:"pool_idle_timeout" yaml:"pool_idle_timeout"`
	PoolCleanupInterval             time.Duration `json:"pool_cleanup_interval" yaml:"pool_cleanup_interval"`
	SuspendTimeout                  time.Duration `json:"suspend_timeout" yaml:"suspend_timeout"`
	CleanupInterval                 time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`
	HeartbeatInterval               time.Duration `json:"heartbeat_interval" yaml:"heartbeat_interval"`
}

// VoceBootstrap is the entry point for Voce application configuration
type VoceBootstrap struct {
	Logging logging.Config `json:"logging" yaml:"logging"`
	Redis   RedisConfig    `json:"redis" yaml:"redis"`
	Server  VoceConfig     `json:"server" yaml:"server"`
}

// GatewayBootstrap is the entry point for Gateway application configuration
type GatewayBootstrap struct {
	Logging logging.Config      `json:"logging" yaml:"logging"`
	Gateway GatewayServerConfig `json:"gateway" yaml:"gateway"`
}
