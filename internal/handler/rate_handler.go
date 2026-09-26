// Package handler translates HTTP requests into service calls and service
// results into JSON responses.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/2103Sanjay/currency-watcher-be/internal/model"
	"github.com/2103Sanjay/currency-watcher-be/internal/service"
)

// MaxTargets bounds how many target currencies one request may ask for.
const MaxTargets = 50

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

// RateHandler serves the rate endpoints.
type RateHandler struct {
	svc     service.RateService
	logger  *slog.Logger
	version string
}

// NewRateHandler returns a handler backed by svc. version is reported by the
// health endpoint.
func NewRateHandler(svc service.RateService, logger *slog.Logger, version string) *RateHandler {
	return &RateHandler{svc: svc, logger: logger, version: version}
}

// ratesResponse is the JSON body of GET /api/rates.
type ratesResponse struct {
	Base      string             `json:"base"`
	Date      string             `json:"date"`
	Rates     map[string]float64 `json:"rates"`
	FetchedAt time.Time          `json:"fetchedAt"`
	ExpiresAt time.Time          `json:"expiresAt"`
	Cached    bool               `json:"cached"`
	Stale     bool               `json:"stale"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Health handles GET /api/health.
func (h *RateHandler) Health(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": h.version})
}

// Rates handles GET /api/rates?base=USD&targets=EUR,SGD.
func (h *RateHandler) Rates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	base := normalise(q.Get("base"))
	if base == "" {
		WriteError(w, http.StatusBadRequest, "query parameter 'base' is required")
		return
	}
	if !currencyCode.MatchString(base) {
		WriteError(w, http.StatusBadRequest, "invalid base currency code: "+base)
		return
	}

	targets, err := parseTargets(q.Get("targets"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	res, err := h.svc.Rates(r.Context(), base, targets)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, ratesResponse{
		Base:      res.Base,
		Date:      res.Date,
		Rates:     res.Rates,
		FetchedAt: res.FetchedAt.UTC(),
		ExpiresAt: res.ExpiresAt.UTC(),
		Cached:    res.Cached,
		Stale:     res.Stale,
	})
}

// Currencies handles GET /api/currencies.
func (h *RateHandler) Currencies(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Currencies(r.Context())
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, list)
}

func (h *RateHandler) handleServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, model.ErrUnknownCurrency):
		WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(r.Context().Err(), context.Canceled):
		// Client went away; nobody is listening for a response.
	default:
		h.logger.Error("rates lookup failed", "path", r.URL.Path, "error", err)
		WriteError(w, http.StatusBadGateway, "exchange rate provider is unavailable, please retry later")
	}
}

// parseTargets splits a comma-separated list of currency codes, validating
// and de-duplicating them while preserving order.
func parseTargets(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, part := range strings.Split(raw, ",") {
		code := normalise(part)
		if code == "" {
			continue
		}
		if !currencyCode.MatchString(code) {
			return nil, errors.New("invalid target currency code: " + code)
		}
		if seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	if len(out) > MaxTargets {
		return nil, errors.New("too many target currencies")
	}
	return out, nil
}

func normalise(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// WriteJSON writes body as a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// WriteError writes a {"error": msg} JSON response with the given status.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, errorResponse{Error: msg})
}
