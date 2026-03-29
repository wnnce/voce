package config

import "github.com/wnnce/voce/pkg/logging"

type ServerConfig struct {
	Name        string `json:"name" yaml:"name" mapstructure:"name"`
	Environment string `json:"environment" yaml:"environment" mapstructure:"environment"`
	Version     string `json:"version" yaml:"version" mapstructure:"version"`
	Host        string `json:"host" yaml:"host" mapstructure:"host"`
	Port        int    `json:"port" yaml:"port" mapstructure:"port"`
	GrpcPort    int    `json:"grpc_port" yaml:"grpc_port" mapstructure:"grpc_port"`
}

type Bootstrap struct {
	Logging logging.Config `json:"logging" yaml:"logging" mapstructure:"logging"`
	Server  ServerConfig   `json:"server" yaml:"server" mapstructure:"server"`
}
