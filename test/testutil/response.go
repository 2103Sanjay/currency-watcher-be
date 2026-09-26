package testutil

import (
	"encoding/json"
	"io"
	"testing"
	"time"
)

// Envelope mirrors the JSON envelope every API response is wrapped in, with
// Data left raw so each test can decode it into the type it expects.
type Envelope struct {
	Status     string          `json:"status"`
	Message    string          `json:"message"`
	StatusCode int             `json:"status_code"`
	Data       json.RawMessage `json:"data"`
	Timestamp  time.Time       `json:"timestamp"`
}

// DecodeResponse decodes an API response body, failing the test if it isn't
// a well-formed envelope whose status matches wantCode. If data is non-nil,
// the envelope's data is decoded into it.
func DecodeResponse(t *testing.T, body io.Reader, wantCode int, data any) Envelope {
	t.Helper()

	var env Envelope
	if err := json.NewDecoder(body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}

	wantStatus := "Success"
	if wantCode >= 400 {
		wantStatus = "Failed"
	}
	if env.Status != wantStatus || env.StatusCode != wantCode || env.Message == "" || env.Timestamp.IsZero() {
		t.Fatalf("unexpected envelope: %+v (want status %q, status_code %d)", env, wantStatus, wantCode)
	}
	if wantCode >= 400 && env.Data != nil {
		t.Errorf("error response should have no data, got %s", env.Data)
	}

	if data != nil {
		if err := json.Unmarshal(env.Data, data); err != nil {
			t.Fatalf("decode data: %v (data: %s)", err, env.Data)
		}
	}
	return env
}
