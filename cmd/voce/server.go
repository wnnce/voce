package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	pb "github.com/wnnce/voce/api/voce/v1"
	"github.com/wnnce/voce/biz/route"
	"github.com/wnnce/voce/config"
	voceMiddleware "github.com/wnnce/voce/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

func initialize(server config.ServerConfig) chi.Router {
	router := chi.NewRouter()
	router.Use(cors.AllowAll().Handler)
	router.Use(voceMiddleware.Logger)
	if server.Environment == "dev" {
		router.Mount("/debug", chiMiddleware.Profiler())
	}
	route.RegisterRouter(router)
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

func bootstrapGRPC(cancel context.CancelFunc, cfg config.ServerConfig, server *grpc.Server) {
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
