package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jarpsimoes/traffic-collector/pkg/collector"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Version = "dev"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Setup logging
	logger := setupLogger()
	defer logger.Sync()

	logger.Infow("Starting traffic collector", "version", Version)

	// Setup context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create and start collector
	cfg := collector.NewConfig()
	cfg.Logger = logger

	coll, err := collector.New(cfg)
	if err != nil {
		logger.Fatalw("Failed to create collector", "error", err)
	}

	// Start health check server
	healthSrv := startHealthServer(logger, coll)

	// Start collector
	if err := coll.Start(ctx); err != nil {
		logger.Fatalw("Failed to start collector", "error", err)
	}

	// Wait for signal
	<-sigChan
	logger.Infow("Received shutdown signal")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := coll.Stop(shutdownCtx); err != nil {
		logger.Errorw("Error stopping collector", "error", err)
	}

	if err := healthSrv.Shutdown(shutdownCtx); err != nil {
		logger.Errorw("Error stopping health server", "error", err)
	}

	logger.Infow("Traffic collector stopped")
	return nil
}

func setupLogger() *zap.Logger {
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	var level zapcore.Level
	if err := level.UnmarshalText([]byte(logLevel)); err != nil {
		level = zapcore.InfoLevel
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(level)
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, _ := cfg.Build()
	return logger
}

func startHealthServer(logger *zap.Logger, coll *collector.Collector) *http.Server {
	port := os.Getenv("HEALTH_CHECK_PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if coll.IsHealthy() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"status":"healthy","version":"%s"}\n`, Version)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"status":"unhealthy"}\n`)
		}
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if coll.IsReady() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"ready":true}\n`)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"ready":false}\n`)
		}
	})

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorw("Health server error", "error", err)
		}
	}()

	logger.Infow("Health check server started", "port", port)
	return srv
}
