package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lesismal/nbio/nbhttp"
	"github.com/spf13/pflag"
	"github.com/wnnce/voce/config"
	"github.com/wnnce/voce/internal/gateway"
)

func main() {
	var configFile string
	pflag.StringVarP(&configFile, "config", "c", "configs/gateway.yaml", "config file path")
	pflag.Parse()

	var cfg config.GatewayBootstrap
	if err := config.LoadYAML(configFile, &cfg); err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, cleanup, err := InitGateway(ctx, cfg)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	router := gateway.NewRouter(h)
	nbServer := nbhttp.NewServer(nbhttp.Config{
		Network: "tcp",
		Addrs:   []string{cfg.Gateway.Address()},
		Handler: router,
	})

	exit := make(chan os.Signal, 1)
	signal.Notify(exit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("gateway server starting", "addr", cfg.Gateway.Address())
		if err = nbServer.Start(); err != nil {
			slog.Error("gateway server start failed", "error", err)
			cancel()
		}
	}()
	select {
	case <-exit:
		slog.Info("received system exit signal, shutting down gateway")
	case <-ctx.Done():
		slog.Info("context canceled, shutting down gateway")
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err = nbServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("gateway shutdown error", "error", err)
	}
}
