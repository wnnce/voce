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
	"github.com/wnnce/voce/pkg/logging"
)

type appBase struct {
	container route.AppContainer
	sm        *engine.SessionManager
	wm        engine.WorkflowConfigManager
}

func InitApplication(_ context.Context, cfg config.VoceBootstrap) (route.AppContainer, func(), error) {
	base, err := initBaseApplication(cfg)
	if err != nil {
		return route.AppContainer{}, nil, err
	}

	if cfg.Server.Mode == "gateway" {
		initGatewayMode(base, cfg)
	} else {
		initStandaloneMode(base)
	}

	cleanup := func() {
		base.sm.Stop()
	}
	return base.container, cleanup, nil
}

func initBaseApplication(cfg config.VoceBootstrap) (*appBase, error) {
	logger, err := logging.NewLoggerWithContext(cfg.Logging, metadata.ContextTraceKey)
	if err != nil {
		return nil, err
	}
	slog.SetDefault(logger)

	var wm engine.WorkflowConfigManager
	if cfg.Server.WorkflowStore == "redis" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		rdb, err := dal.NewRedisClient(ctx, cfg.Redis)
		if err != nil {
			return nil, err
		}
		wm = engine.NewRedisWorkflowConfigManager(rdb)
	} else {
		dir := cfg.Server.WorkflowDir
		if dir == "" {
			dir = "configs/workflows"
		}
		wm = engine.NewFileWorkflowConfigManager(dir)
	}

	sm := engine.NewSessionManager(wm, 1*time.Minute)

	base := &appBase{
		sm: sm,
		wm: wm,
	}
	base.container.Workflow = handler.NewWorkflowHandler(wm)
	base.container.Plugin = handler.NewPluginHandler()
	base.container.Monitor = handler.NewMonitorHandler(sm)
	base.container.Realtime = realtime.NewHandler(sm)
	base.container.Grpc = realtime.NewStreamService(sm)
	return base, nil
}

func initGatewayMode(base *appBase, cfg config.VoceBootstrap) {
	cm := machine.NewConnectionManager(base.sm, cfg.Server.PoolSize)
	base.container.Machine = handler.NewMachineHandler(cm)
	base.container.Session = handler.NewGatewaySessionHandler(base.sm, cm)
}

func initStandaloneMode(base *appBase) {
	base.container.Session = handler.NewStandaloneSessionHandler(base.sm)
}
