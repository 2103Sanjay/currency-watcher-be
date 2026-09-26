// Command server runs the Currency Watcher HTTP API.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/2103Sanjay/currency-watcher-be/internal/app"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := app.Run(ctx, logger, version)
	stop()
	if err != nil {
		logger.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}
