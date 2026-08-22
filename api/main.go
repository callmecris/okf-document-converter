package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"okf/api/internal/config"
	"okf/api/internal/handler"
	"okf/api/internal/middleware"
	"okf/api/internal/queue"
	"okf/api/internal/repository"
	"okf/api/internal/storage"
	"okf/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api stopped with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logger.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := repository.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		return err
	}

	store, err := storage.New(
		cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey,
		cfg.MinIOUseSSL, cfg.BucketOriginals, cfg.BucketBundles,
	)
	if err != nil {
		return err
	}
	if err := store.EnsureBuckets(ctx); err != nil {
		return err
	}

	pub, err := queue.NewPublisher(ctx, cfg.RabbitMQURL, cfg.RabbitMQQueue, log)
	if err != nil {
		return err
	}
	defer pub.Close()

	auth := handler.NewAuth(db, cfg.JWTSecret, log)
	metrics := handler.NewMetrics(db, log)
	jobs := handler.NewJobs(db, store, pub, log)
	bundles := handler.NewBundles(db, store, log)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /metrics", metrics.Prometheus)
	mux.Handle("GET /api/v1/metrics", middleware.JWTAuth(cfg.JWTSecret, http.HandlerFunc(metrics.JSON)))
	mux.HandleFunc("POST /api/v1/auth/register", auth.Register)
	mux.HandleFunc("POST /api/v1/auth/login", auth.Login)
	mux.Handle("POST /api/v1/jobs", middleware.JWTAuth(cfg.JWTSecret, http.HandlerFunc(jobs.Create)))
	mux.Handle("GET /api/v1/jobs", middleware.JWTAuth(cfg.JWTSecret, http.HandlerFunc(jobs.List)))
	mux.Handle("GET /api/v1/jobs/{id}", middleware.JWTAuth(cfg.JWTSecret, http.HandlerFunc(jobs.Get)))
	mux.Handle("POST /api/v1/jobs/{id}/cancel", middleware.JWTAuth(cfg.JWTSecret, http.HandlerFunc(jobs.Cancel)))
	mux.Handle("POST /api/v1/jobs/{id}/retry", middleware.JWTAuth(cfg.JWTSecret, http.HandlerFunc(jobs.Retry)))
	mux.Handle("GET /api/v1/jobs/{id}/download", middleware.JWTAuth(cfg.JWTSecret, http.HandlerFunc(bundles.DownloadZip)))
	mux.Handle("GET /api/v1/jobs/{id}/bundle/{path...}", middleware.JWTAuth(cfg.JWTSecret, http.HandlerFunc(bundles.DownloadFile)))

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      middleware.Logging(log, middleware.CORS(mux)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 2 * time.Minute,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", srv.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down api")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}