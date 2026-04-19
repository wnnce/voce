package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	pb "github.com/wnnce/voce/api/voce/v1"
	"github.com/wnnce/voce/biz/route"
	"github.com/wnnce/voce/config"
	"github.com/wnnce/voce/internal/machine"
	voceMiddleware "github.com/wnnce/voce/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

func initialize(server config.VoceConfig, container route.AppContainer) chi.Router {
	router := chi.NewRouter()
	router.Use(cors.AllowAll().Handler)
	router.Use(voceMiddleware.Logger)
	if server.Environment == "dev" {
		router.Mount("/debug", chiMiddleware.Profiler())
	}
	route.RegisterRouter(router, container)
	return router
}

func bootstrapHttp(cancel context.CancelFunc, srv *http.Server) {
	slog.Info("http server starting", slog.String("addr", srv.Addr))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("http server listen failed", slog.String("error", err.Error()))
		cancel()
		panic(err)
	}
}

func registerServices(services ...pb.VoceServiceServer) *grpc.Server {
	s := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,
			Time:              1 * time.Minute,
			Timeout:           20 * time.Second,
		}),
		grpc.MaxRecvMsgSize(1024*1024*10),
	)
	for _, service := range services {
		pb.RegisterVoceServiceServer(s, service)
	}
	return s
}

func bootstrapGRPC(cancel context.CancelFunc, cfg config.VoceConfig, server *grpc.Server) {
	addr := cfg.Host + ":" + strconv.Itoa(cfg.GrpcPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		cancel()
		panic(err)
	}
	slog.Info("gRPC server starting", "addr", addr)
	if err = server.Serve(lis); err != nil {
		cancel()
		panic(err)
	}
}

func runGateway(ctx context.Context, cancel context.CancelFunc, cfg config.VoceBootstrap, container route.AppContainer) {
	router := initialize(cfg.Server, container)
	srv := &http.Server{
		Addr:    cfg.Server.Address(),
		Handler: router,
	}

	go bootstrapHttp(cancel, srv)

	registrar := machine.NewRegistrar(ctx, uuid.NewString(), cfg.Server.GatewayAddr, cfg.Server.Port)
	registrar.Start()

	waitExit(ctx, srv, nil)
}

func runStandalone(ctx context.Context, cancel context.CancelFunc, cfg config.VoceBootstrap, container route.AppContainer) {
	router := initialize(cfg.Server, container)
	srv := &http.Server{
		Addr:    cfg.Server.Address(),
		Handler: router,
	}

	go bootstrapHttp(cancel, srv)

	var gSrv *grpc.Server
	if cfg.Server.GrpcPort > 0 {
		gSrv = registerServices(container.Grpc)
		go bootstrapGRPC(cancel, cfg.Server, gSrv)
	}

	waitExit(ctx, srv, gSrv)
}

func waitExit(ctx context.Context, srv *http.Server, gSrv *grpc.Server) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case s := <-sig:
		slog.Info("listen system exit signal, shutdown application!", "signal", s.String())
	case <-ctx.Done():
		slog.Info("context canceled, application exit")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if srv != nil {
		_ = srv.Shutdown(shutdownCtx)
	}
	if gSrv != nil {
		gSrv.GracefulStop()
	}
}
