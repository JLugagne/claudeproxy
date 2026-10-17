// Package http is the inbound transport: it reads the model of Messages API
// requests, asks the routing use case where to send them, rewrites the model
// when the route requires it, and hands the request to the provider's handler.
package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/JLugagne/claudeproxy/internal/pkg/apierror"
	"github.com/JLugagne/claudeproxy/internal/pkg/jsonscan"
	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
	"github.com/JLugagne/claudeproxy/internal/proxy/domain/usecases/routeuc"
)

// HealthPath is answered by the proxy itself and never forwarded.
const HealthPath = "/_claudeproxy/health"

const routedPathPrefix = "/v1/messages"

// Handler routes every request to one provider handler. Only POST requests
// under /v1/messages are buffered to read their model; everything else streams
// straight to the fallback provider.
type Handler struct {
	routes       routeuc.RouteQueries
	upstreams    map[domain.ProviderName]http.Handler
	maxBodyBytes int64
	logger       *slog.Logger
}

// NewHandler wires the transport. upstreams must contain every provider the
// routes can return. maxBodyBytes bounds the buffered request body; larger
// requests are rejected with 413.
func NewHandler(routes routeuc.RouteQueries, upstreams map[domain.ProviderName]http.Handler, maxBodyBytes int64, logger *slog.Logger) *Handler {
	return &Handler{routes: routes, upstreams: upstreams, maxBodyBytes: maxBodyBytes, logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == HealthPath {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var body []byte
	var field jsonscan.Field
	if isRoutable(r) {
		var err error
		if body, err = h.readBody(w, r); err != nil {
			h.writeBodyError(w, r, err)
			return
		}
		field, _ = jsonscan.TopLevelString(body, "model")
	}

	r = r.WithContext(domain.WithAttribution(r.Context(), resolveAttribution(r, body)))
	decision, err := h.routes.ResolveRoute(r.Context(), domain.Model(field.Value))
	if err != nil {
		h.logger.LogAttrs(r.Context(), slog.LevelWarn, "no route", slog.String("model", field.Value), slog.String("path", r.URL.Path))
		apierror.Write(w, http.StatusNotFound, apierror.TypeNotFound, "claudeproxy: no provider configured for model "+field.Value)
		return
	}
	upstream, ok := h.upstreams[decision.Provider]
	if !ok {
		apierror.Write(w, http.StatusInternalServerError, apierror.TypeAPI, "claudeproxy: provider "+string(decision.Provider)+" is not configured")
		return
	}
	if body != nil {
		if decision.Rewritten() {
			body = field.Replace(body, string(decision.Model))
		}
		setBody(r, body)
	}

	recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	defer h.logRequest(r, decision, recorder, time.Now())
	upstream.ServeHTTP(recorder, r)
}

func isRoutable(r *http.Request) bool {
	return r.Method == http.MethodPost &&
		strings.HasPrefix(r.URL.Path, routedPathPrefix) &&
		r.Header.Get("Content-Encoding") == ""
}

func (h *Handler) readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r.ContentLength > h.maxBodyBytes {
		return nil, &http.MaxBytesError{Limit: h.maxBodyBytes}
	}
	var buf bytes.Buffer
	if r.ContentLength > 0 {
		buf.Grow(int(r.ContentLength) + bytes.MinRead)
	}
	if _, err := buf.ReadFrom(http.MaxBytesReader(w, r.Body, h.maxBodyBytes)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (h *Handler) writeBodyError(w http.ResponseWriter, r *http.Request, err error) {
	if maxErr, ok := errors.AsType[*http.MaxBytesError](err); ok {
		apierror.Write(w, http.StatusRequestEntityTooLarge, apierror.TypeRequestTooLarge, "claudeproxy: request body exceeds "+formatBytes(maxErr.Limit))
		return
	}
	h.logger.LogAttrs(r.Context(), slog.LevelDebug, "reading request body failed", slog.Any("error", err))
	apierror.Write(w, http.StatusBadRequest, apierror.TypeInvalidRequest, "claudeproxy: cannot read request body")
}

func setBody(r *http.Request, body []byte) {
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	r.TransferEncoding = nil
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
}

func (h *Handler) logRequest(r *http.Request, decision domain.Decision, recorder *statusRecorder, start time.Time) {
	attribution := domain.AttributionFrom(r.Context())
	h.logger.LogAttrs(r.Context(), slog.LevelInfo, "proxied",
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("model", string(decision.Requested)),
		slog.String("provider", string(decision.Provider)),
		slog.String("upstream_model", string(decision.Model)),
		slog.String("project", attribution.Project),
		slog.String("session", attribution.Session),
		slog.Int("status", recorder.status),
		slog.Duration("duration", time.Since(start)),
	)
}

// resolveAttribution reads the client project header and the Claude Code session
// id carried by the request body, then normalizes empty values to Unknown.
func resolveAttribution(r *http.Request, body []byte) domain.Attribution {
	return domain.Attribution{
		Project: r.Header.Get("x-project"),
		Session: sessionFromBody(body),
	}.Normalized()
}

func sessionFromBody(body []byte) string {
	if body == nil {
		return ""
	}
	metadata, err := jsonscan.TopLevelObject(body, "metadata")
	if err != nil {
		return ""
	}
	userID, err := jsonscan.TopLevelString(body[metadata.Start:metadata.End], "user_id")
	if err != nil {
		return ""
	}
	var identity struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(userID.Value), &identity); err != nil {
		return ""
	}
	return identity.SessionID
}
