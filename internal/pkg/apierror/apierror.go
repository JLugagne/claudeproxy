// Package apierror writes errors in the Anthropic Messages API envelope so
// that Claude Code surfaces proxy-side failures like any upstream error.
// It is a leaf utility and must not import a bounded context.
package apierror

import (
	"encoding/json"
	"net/http"
)

// Error types understood by Anthropic clients.
const (
	TypeInvalidRequest  = "invalid_request_error"
	TypeNotFound        = "not_found_error"
	TypeRequestTooLarge = "request_too_large"
	TypeAPI             = "api_error"
)

type envelope struct {
	Type  string `json:"type"`
	Error body   `json:"error"`
}

type body struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Write sends status with an Anthropic error body. It must be called before
// anything else has been written to w.
func Write(w http.ResponseWriter, status int, errType, message string) {
	payload, _ := json.Marshal(envelope{Type: "error", Error: body{Type: errType, Message: message}})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}
