package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/wnnce/voce/biz/realtime"
	"github.com/wnnce/voce/config"
	"github.com/wnnce/voce/internal/engine"
	_ "github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/metadata"
	_ "github.com/wnnce/voce/internal/plugins"
	"github.com/wnnce/voce/pkg/logging"
	"google.golang.org/grpc"
)

func main() {
	var configFile string
	pflag.StringVarP(&configFile, "config", "c", "configs/config.yaml", "config file path")
	pflag.Parse()

	if err := config.LoadConfigWithFlag("config"); err != nil {
		panic(err)
	}
	var cfg config.Bootstrap
	if err := viper.Unmarshal(&cfg); err != nil {
		panic(err)
	}
	logger, err := logging.NewLoggerWithContext(cfg.Logging, metadata.ContextTraceKey)
	if err != nil {
		panic(err)
	}
	slog.SetDefault(logger)
	ctx, cancel := context.WithCancel(context.Background())
	cleanup, err := config.DoReaderConfiguration(ctx)
	if err != nil {
		panic(err)
	}
	defer func() {
		cleanup()
		cancel()
	}()
	exit := make(chan os.Signal, 1)
	signal.Notify(exit, syscall.SIGINT, syscall.SIGTERM)
	router := initialize(cfg.Server)
	srv := &http.Server{
		Addr:    cfg.Server.Host + ":" + strconv.Itoa(cfg.Server.Port),
		Handler: router,
	}
	go bootstrapHttp(cancel, srv)
	var gSrv *grpc.Server
	if cfg.Server.GrpcPort > 0 {
		gSrv = registerServices(realtime.NewStreamService(engine.DefaultSessionManager))
		go bootstrapGRPC(cancel, cfg.Server, gSrv)
	}
	select {
	case <-exit:
		slog.Info("listen system exit signal, shutdown application!")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		if err = srv.Shutdown(shutdownCtx); err != nil {
			slog.Info("shutdown app error", slog.String("error", err.Error()))
		}
		if gSrv != nil {
			gSrv.GracefulStop()
		}
	case <-ctx.Done():
		slog.Info("context canceled application exit")
	}
}
