package main

import (
	"context"
	"log/slog"
	"net/http"

	nlog "github.com/lesismal/nbio/logging"
	"github.com/lesismal/nbio/nbhttp"
	"github.com/wnnce/voce/config"
	"github.com/wnnce/voce/internal/gateway"
	"github.com/wnnce/voce/internal/metadata"
	"github.com/wnnce/voce/internal/telemetry"
	"github.com/wnnce/voce/pkg/logging"
	"go.opentelemetry.io/otel"
)

func InitGateway(ctx context.Context, cfg config.GatewayBootstrap) (*gateway.Handler, http.Handler, func(), error) {
	metrics, err := telemetry.New(ctx, telemetry.Config{
		ServiceName: cfg.Gateway.Name,
		Environment: cfg.Gateway.Environment,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	otel.SetMeterProvider(metrics.MeterProvider())

	logger, err := logging.NewLoggerWithContext(cfg.Logging, metadata.ContextTraceKey)
	if err != nil {
		_ = metrics.Shutdown(ctx)
		return nil, nil, nil, err
	}
	slog.SetDefault(logger)
	nlog.SetLevel(nlog.LevelNone)

	nbEngine := nbhttp.NewEngine(nbhttp.Config{
		Name: "gateway-client",
	})
	if err = nbEngine.Start(); err != nil {
		_ = metrics.Shutdown(ctx)
		return nil, nil, nil, err
	}

	sm := gateway.NewSessionManager(ctx, cfg.Gateway.SuspendTimeout, cfg.Gateway.CleanupInterval)
	mm := gateway.NewMachineManager(ctx, cfg.Gateway, sm, nbEngine)

	h := gateway.NewHandler(mm, sm)

	cleanup := func() {
		nbEngine.Stop()
		if err := metrics.Shutdown(context.Background()); err != nil {
			slog.Error("telemetry shutdown failed", "error", err)
		}
	}
	return h, metrics.Handler(), cleanup, nil
}
