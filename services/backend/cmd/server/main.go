package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/redis/go-redis/v9"

	"eva/services/backend/internal/config"
	"eva/services/backend/internal/gen/openapi"
	"eva/services/backend/internal/migrate"
	"eva/services/backend/internal/observability"
	"eva/services/backend/internal/repository"
	httptransport "eva/services/backend/internal/transport/http"
	chatws "eva/services/backend/internal/transport/ws"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	logLevel := slog.LevelInfo
	if strings.EqualFold(cfg.LogLevel, "debug") {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))

	ctx := context.Background()
	if err := migrate.Up(ctx, cfg.PostgresDSN); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}

	store, err := repository.NewStore(ctx, cfg.PostgresDSN)
	if err != nil {
		slog.Error("postgres", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})

	apiSrv, err := httptransport.NewServer(cfg, store, rdb)
	if err != nil {
		slog.Error("http server init", "err", err)
		os.Exit(1)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(observability.MetricsMiddleware)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/metrics", observability.MetricsHandler().ServeHTTP)
	r.Handle("/ws/v1/realtime", chatws.NewHandler(cfg, apiSrv.Runner()))
	_ = openapi.HandlerFromMux(apiSrv, r)

	addr := cfg.Host + ":" + cfg.Port
	httpSrv := &http.Server{Addr: addr, Handler: r, ReadHeaderTimeout: 10 * time.Second}

	go func() {
		slog.Info("listening", "addr", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)
}
