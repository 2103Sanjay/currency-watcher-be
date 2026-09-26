// Package app wires the layers together (repository → service → handler →
// route) and runs the HTTP server.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/2103Sanjay/currency-watcher-be/internal/config"
	"github.com/2103Sanjay/currency-watcher-be/internal/handler"
	"github.com/2103Sanjay/currency-watcher-be/internal/repository"
	"github.com/2103Sanjay/currency-watcher-be/internal/route"
	"github.com/2103Sanjay/currency-watcher-be/internal/service"
)

// Run loads configuration, starts the server and blocks until ctx is
// cancelled, then shuts down gracefully. version is reported by the health
// endpoint.
func Run(ctx context.Context, logger *slog.Logger, version string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	repo := repository.NewFrankfurterRepository(cfg.UpstreamURL, &http.Client{Timeout: cfg.UpstreamTimeout})
	svc := service.NewCachedRateService(repo, cfg.CacheTTL, logger)
	h := handler.NewRateHandler(svc, logger, version)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           route.New(h, logger, cfg.AllowedOrigins),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting",
			"port", cfg.Port,
			"version", version,
			"cacheTTL", cfg.CacheTTL.String(),
			"allowedOrigins", cfg.AllowedOrigins,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
