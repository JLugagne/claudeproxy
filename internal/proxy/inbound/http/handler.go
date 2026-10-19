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
	"github.com/JLugagne/claudeproxy/internal/proxy/domain/usecases/usageuc"
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
	// usage records each request's consumption; it defaults to usageuc.Nop.
	usage usageuc.Recorder
}

// Option customizes the Handler.
type Option func(*Handler)

// WithUsage records the usage of every request the handler serves.
func WithUsage(recorder usageuc.Recorder) Option {
	return func(h *Handler) { h.usage = recorder }
}

// NewHandler wires the transport. upstreams must contain every provider the
// routes can return. maxBodyBytes bounds the buffered request body; larger
// requests are rejected with 413.
func NewHandler(routes routeuc.RouteQueries, upstreams map[domain.ProviderName]http.Handler, maxBodyBytes int64, logger *slog.Logger, opts ...Option) *Handler {
	h := &Handler{routes: routes, upstreams: upstreams, maxBodyBytes: maxBodyBytes, logger: logger, usage: usageuc.Nop{}}
	for _, opt := range opts {
		opt(h)
	}
	return h
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

	recorder := newUsageRecorder(w)
	start := time.Now()
	defer h.finish(r, decision, recorder, start)
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
		Project: projectOf(r, body),
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

func (h *Handler) finish(r *http.Request, decision domain.Decision, recorder *usageRecorder, start time.Time) {
	h.logRequest(r, decision, recorder.statusRecorder, start)
	if !isModelCall(r) {
		return
	}
	parsed := recorder.parsed()
	cost := parsed.cost()
	h.usage.Record(domain.Usage{
		Time:                     start,
		Attribution:              domain.AttributionFrom(r.Context()),
		Provider:                 decision.Provider,
		RequestedModel:           decision.Requested,
		UpstreamModel:            decision.Model,
		InputTokens:              parsed.InputTokens,
		OutputTokens:             parsed.OutputTokens,
		CacheReadInputTokens:     parsed.CacheReadInputTokens,
		CacheCreationInputTokens: parsed.CacheCreationInputTokens,
		Status:                   recorder.status,
		Duration:                 time.Since(start),
		Stream:                   recorder.streaming(),
		CostUSD:                  costValue(cost),
		HasCost:                  cost != nil,
	})
}

// workingDirectoryMarker precedes the project directory in the environment
// block Claude Code adds to its prompt.
var workingDirectoryMarker = []byte("Primary working directory: ")

// projectOf names the project of a request: the X-Project header when the
// client sends one, else the directory Claude Code reports in its prompt. The
// hook that sets the header only reaches commands run by the Bash tool, never
// the API client, so the prompt is the usual source.
func projectOf(r *http.Request, body []byte) string {
	if project := r.Header.Get("x-project"); project != "" {
		return project
	}
	return projectFromBody(body)
}

// projectFromBody returns the last two segments of the working directory found
// in the body ("owner/repo" for a checkout such as .../github.com/owner/repo),
// or an empty string when the body does not carry one.
func projectFromBody(body []byte) string {
	start := bytes.Index(body, workingDirectoryMarker)
	if start < 0 {
		return ""
	}
	rest := body[start+len(workingDirectoryMarker):]
	// The body is JSON, so a newline inside the prompt appears as the two
	// characters \n; the string itself ends at a quote.
	end := bytes.IndexAny(rest, "\"\\\r\n")
	if end < 0 {
		end = len(rest)
	}
	var segments []string
	for _, segment := range strings.Split(string(rest[:end]), "/") {
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	if len(segments) > 2 {
		segments = segments[len(segments)-2:]
	}
	return strings.Join(segments, "/")
}

// isModelCall reports whether the request is a Messages API call, the only
// requests that consume tokens. Others (the HEAD /api/hello connectivity check,
// model listings...) are proxied and logged but not counted as usage.
func isModelCall(r *http.Request) bool {
	return r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, routedPathPrefix)
}
