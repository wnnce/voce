package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/wnnce/voce/biz/dal"
	"github.com/wnnce/voce/biz/handler"
	"github.com/wnnce/voce/biz/realtime"
	"github.com/wnnce/voce/biz/route"
	"github.com/wnnce/voce/config"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/machine"
	"github.com/wnnce/voce/internal/metadata"
	"github.com/wnnce/voce/internal/remote"
	"github.com/wnnce/voce/internal/telemetry"
	"github.com/wnnce/voce/pkg/logging"
	"go.opentelemetry.io/otel"
)

type appBase struct {
	container route.AppContainer
	sm        *engine.SessionManager
	wm        engine.WorkflowConfigManager
	rm        *remote.Manager
	store     *engine.PluginStore
	telemetry *telemetry.Telemetry
}

func InitApplication(ctx context.Context, cfg config.VoceBootstrap) (route.AppContainer, func(), error) {
	base, err := initBaseApplication(ctx, cfg)
	if err != nil {
		return route.AppContainer{}, nil, err
	}

	if cfg.Server.Mode == "gateway" {
		initGatewayMode(base)
	} else {
		initStandaloneMode(base)
	}

	cleanup := func() {
		if base.rm != nil {
			base.rm.Shutdown()
		}
		base.sm.Stop()
		if err := base.telemetry.Shutdown(context.Background()); err != nil {
			slog.Error("telemetry shutdown failed", "error", err)
		}
	}
	return base.container, cleanup, nil
}

func initBaseApplication(ctx context.Context, cfg config.VoceBootstrap) (*appBase, error) {
	logger, err := logging.NewLoggerWithContext(
		cfg.Logging,
		metadata.ContextTraceKey,
		metadata.ContextNodeNameKey,
	)
	if err != nil {
		return nil, err
	}
	slog.SetDefault(logger)

	metrics, err := telemetry.New(ctx, telemetry.Config{
		ServiceName: cfg.Server.Name,
		Environment: cfg.Server.Environment,
	})
	if err != nil {
		return nil, err
	}
	otel.SetMeterProvider(metrics.MeterProvider())

	store := engine.NewPluginStore(engine.LocalPluginResource())

	var wm engine.WorkflowConfigManager
	if cfg.Server.WorkflowStore == "redis" {
		rCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		rdb, er := dal.NewRedisClient(rCtx, cfg.Redis)
		if er != nil {
			_ = metrics.Shutdown(ctx)
			return nil, er
		}
		wm = engine.NewRedisWorkflowConfigManager(rdb, store)
	} else {
		dir := cfg.Server.WorkflowDir
		if dir == "" {
			dir = "configs/workflows"
		}
		wm = engine.NewFileWorkflowConfigManager(dir, store)
	}

	sm := engine.NewSessionManager(wm, store, 1*time.Minute)

	base := &appBase{
		sm:        sm,
		wm:        wm,
		store:     store,
		telemetry: metrics,
	}
	base.container.Metrics = metrics.Handler()

	if hasEnabledPluginServers(cfg.Server.PluginServers) {
		base.rm = remote.NewManager(ctx, store)
		base.rm.AddRemotes(ctx, cfg.Server.PluginServers)
	}

	base.container.Workflow = handler.NewWorkflowHandler(wm)
	base.container.Plugin = handler.NewPluginHandler(store)
	base.container.Realtime = realtime.NewHandler(sm)
	base.container.Grpc = realtime.NewStreamService(sm)
	return base, nil
}

func initGatewayMode(base *appBase) {
	cm := machine.NewConnectionManager(base.sm)
	base.container.Machine = handler.NewMachineHandler(cm)
	base.container.Session = handler.NewGatewaySessionHandler(base.sm, cm)
}

func initStandaloneMode(base *appBase) {
	base.container.Session = handler.NewStandaloneSessionHandler(base.sm)
}

func hasEnabledPluginServers(configs []config.PluginServerConfig) bool {
	for _, cfg := range configs {
		if cfg.Enable && cfg.URL != "" {
			return true
		}
	}
	return false
}
