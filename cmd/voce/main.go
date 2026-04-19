package main

import (
	"context"

	"github.com/spf13/pflag"
	"github.com/wnnce/voce/config"
	_ "github.com/wnnce/voce/internal/engine"
	_ "github.com/wnnce/voce/internal/plugins"
)

func main() {
	var configFile string
	pflag.StringVarP(&configFile, "config", "c", "configs/config.yaml", "config file path")
	pflag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	var cfg config.VoceBootstrap
	if err := config.LoadYAML(configFile, &cfg); err != nil {
		panic(err)
	}
	container, cleanup, err := InitApplication(ctx, cfg)
	if err != nil {
		panic(err)
	}
	defer func() {
		cleanup()
		cancel()
	}()

	if cfg.Server.Mode == "gateway" {
		runGateway(ctx, cancel, cfg, container)
	} else {
		runStandalone(ctx, cancel, cfg, container)
	}
}
