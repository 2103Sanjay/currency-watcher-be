// Package route maps URLs to handlers and applies cross-cutting middleware.
package route

import (
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/2103Sanjay/currency-watcher-be/internal/handler"
)

// New wires the API routes and middleware.
func New(h *handler.RateHandler, logger *slog.Logger, allowedOrigins []string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", h.Health)
	mux.HandleFunc("GET /api/rates", h.Rates)
	mux.HandleFunc("GET /api/currencies", h.Currencies)

	return recoverPanics(logger, logRequests(logger, cors(allowedOrigins, mux)))
}

// cors allows browsers on the configured origins to call the API.
func cors(allowedOrigins []string, next http.Handler) http.Handler {
	allowAll := slices.Contains(allowedOrigins, "*")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowAll || slices.Contains(allowedOrigins, origin)) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		// Health checks run constantly; keep them out of the logs.
		if r.URL.Path == "/api/health" {
			return
		}
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", rec.status,
			"duration", time.Since(start).String(),
		)
	})
}

func recoverPanics(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				logger.Error("panic serving request", "path", r.URL.Path, "panic", v)
				handler.WriteError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

