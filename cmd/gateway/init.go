package main

import (
	"context"
	"log/slog"

	nlog "github.com/lesismal/nbio/logging"
	"github.com/lesismal/nbio/nbhttp"
	"github.com/wnnce/voce/config"
	"github.com/wnnce/voce/internal/gateway"
	"github.com/wnnce/voce/internal/metadata"
	"github.com/wnnce/voce/pkg/logging"
)

func InitGateway(ctx context.Context, cfg config.GatewayBootstrap) (*gateway.GatewayHandler, func(), error) {
	logger, err := logging.NewLoggerWithContext(cfg.Logging, metadata.ContextTraceKey)
	if err != nil {
		return nil, nil, err
	}
	slog.SetDefault(logger)
	nlog.SetLevel(nlog.LevelNone)

	nbEngine := nbhttp.NewEngine(nbhttp.Config{
		Name: "gateway-client",
	})
	if err = nbEngine.Start(); err != nil {
		return nil, nil, err
	}

	sm := gateway.NewSessionManager(ctx, cfg.Gateway.SuspendTimeout, cfg.Gateway.CleanupInterval)
	mm := gateway.NewMachineManager(ctx, cfg.Gateway, sm, nbEngine)

	h := gateway.NewGatewayHandler(mm, sm)

	cleanup := func() {
		nbEngine.Stop()
	}
	return h, cleanup, nil
}
