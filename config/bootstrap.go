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
	GrpcPort      int    `json:"grpc_port" yaml:"grpc_port"`
	Mode          string `json:"mode" yaml:"mode"`
	GatewayAddr   string `json:"gateway_addr" yaml:"gateway_addr"`
	PoolSize      int    `json:"pool_size" yaml:"pool_size"`
	WorkflowStore string `json:"workflow_store" yaml:"workflow_store"` // "file" or "redis"
	WorkflowDir   string `json:"workflow_dir" yaml:"workflow_dir"`     // directory for "file" store
}

// GatewayServerConfig is the configuration for the Gateway service itself
type GatewayServerConfig struct {
	CommonConfig      `yaml:",inline"`
	NetworkConfig     `yaml:",inline"`
	PoolSize          int           `json:"pool_size" yaml:"pool_size"`
	SuspendTimeout    time.Duration `json:"suspend_timeout" yaml:"suspend_timeout"`
	CleanupInterval   time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval" yaml:"heartbeat_interval"`
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
