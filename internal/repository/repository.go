// Package repository fetches exchange-rate data from external sources.
package repository

import (
	"context"

	"github.com/2103Sanjay/currency-watcher-be/internal/model"
)

// RateRepository fetches live data from an external exchange-rate source.
type RateRepository interface {
	// Latest returns all current rates for base. It returns an error wrapping
	// model.ErrUnknownCurrency if base is not supported.
	Latest(ctx context.Context, base string) (model.RateTable, error)
	// Currencies returns the currencies currently supported by the source.
	Currencies(ctx context.Context) ([]model.Currency, error)
}
