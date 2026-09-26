// Package model holds the domain types shared by the repository, service and
// handler layers.
package model

import (
	"errors"
	"time"
)

// ErrUnknownCurrency is returned when a currency code is not supported by
// the upstream provider.
var ErrUnknownCurrency = errors.New("unknown currency")

// RateTable is a snapshot of every rate the provider offers for one base
// currency.
type RateTable struct {
	Base string
	// Date is the most recent publication date among the rates (YYYY-MM-DD).
	Date  string
	Rates map[string]float64
}

// Currency describes a currency supported by the provider.
type Currency struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// RateResult is the answer to a rates query.
type RateResult struct {
	Base      string
	Date      string
	Rates     map[string]float64
	FetchedAt time.Time
	ExpiresAt time.Time
	// Cached is true when the result was served without calling upstream.
	Cached bool
	// Stale is true when upstream failed and an expired entry was served instead.
	Stale bool
}
