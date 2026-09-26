package handler

import (
	"encoding/json"
	"net/http"
	"time"
)

// Values of Response.Status.
const (
	StatusSuccess = "Success"
	StatusFailed  = "Failed"
)

// Response is the envelope every API response is wrapped in, successful or
// not, so clients can handle all of them the same way.
type Response struct {
	Status     string    `json:"status"`
	Message    string    `json:"message"`
	StatusCode int       `json:"status_code"`
	Data       any       `json:"data,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// WriteSuccess writes data wrapped in a "Success" envelope.
func WriteSuccess(w http.ResponseWriter, status int, message string, data any) {
	writeResponse(w, Response{Status: StatusSuccess, Message: message, StatusCode: status, Data: data})
}

// WriteError writes a "Failed" envelope with msg and no data.
func WriteError(w http.ResponseWriter, status int, msg string) {
	writeResponse(w, Response{Status: StatusFailed, Message: msg, StatusCode: status})
}

// NotFound answers requests for paths the API doesn't serve.
func NotFound(w http.ResponseWriter, _ *http.Request) {
	WriteError(w, http.StatusNotFound, "resource not found")
}

// MethodNotAllowed answers requests using a method other than GET on an API path.
func MethodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD")
	WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeResponse(w http.ResponseWriter, res Response) {
	res.Timestamp = time.Now().UTC()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.StatusCode)
	_ = json.NewEncoder(w).Encode(res)
}
