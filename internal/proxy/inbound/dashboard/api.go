package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
	"github.com/JLugagne/claudeproxy/internal/proxy/domain/usecases/usageuc"
)

// ProviderInfo is a provider as the console may show it: the API key is
// reduced to whether one is configured, never to its value.
type ProviderInfo struct {
	Name       string
	BaseURL    string
	Auth       string
	KeyPresent bool
}

// RouteInfo is one configured route.
type RouteInfo struct {
	Match    string
	Regex    string
	Provider string
	Model    string
}

// Config wires the console: the metrics read side, and a redacted view of the
// proxy configuration. Queries is nil when metrics are disabled or the
// database could not be opened, and MetricsError then explains why.
type Config struct {
	Queries         usageuc.MetricsQueries
	MetricsError    string
	Providers       []ProviderInfo
	Routes          []RouteInfo
	DefaultProvider string
	// Now is the clock, overridable in tests.
	Now func() time.Time
}

func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// ErrUnknownRange is returned for a range the console does not offer.
var ErrUnknownRange = errors.New("unknown range")

func (h *handler) serveAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorDTO{Error: "method not allowed"})
		return
	}
	if h.cfg.Queries == nil {
		message := h.cfg.MetricsError
		if message == "" {
			message = "usage metrics are disabled; enable them in the configuration file"
		}
		writeJSON(w, http.StatusServiceUnavailable, errorDTO{Error: message})
		return
	}

	switch strings.TrimPrefix(r.URL.Path, Prefix+"/api/") {
	case "overview":
		h.overview(w, r)
	case "projects":
		h.projects(w, r)
	case "models":
		h.models(w, r)
	case "requests":
		h.requests(w, r)
	case "sessions":
		h.sessions(w, r)
	case "providers":
		h.providers(w, r)
	default:
		writeJSON(w, http.StatusNotFound, errorDTO{Error: "unknown endpoint"})
	}
}

func (h *handler) overview(w http.ResponseWriter, r *http.Request) {
	window, name, err := parseRange(r.URL.Query().Get("range"), h.cfg.now())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorDTO{Error: err.Error()})
		return
	}
	ctx := r.Context()

	totals, err := h.cfg.Queries.Totals(ctx, window)
	if err != nil {
		writeServerError(w, err)
		return
	}
	volume, err := h.cfg.Queries.Volume(ctx, window)
	if err != nil {
		writeServerError(w, err)
		return
	}
	projects, err := h.cfg.Queries.Projects(ctx, window)
	if err != nil {
		writeServerError(w, err)
		return
	}
	models, err := h.cfg.Queries.Models(ctx, window)
	if err != nil {
		writeServerError(w, err)
		return
	}
	recent, _, err := h.cfg.Queries.Requests(ctx, window, domain.UsageFilter{Limit: 8})
	if err != nil {
		writeServerError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, overviewResponse{
		Range:       name,
		Totals:      newTotals(totals),
		Volume:      newPoints(volume),
		TopProjects: newProjectBars(projects, 5),
		TopModels:   newModelBars(models, 5),
		Recent:      newRequests(recent),
	})
}

func (h *handler) projects(w http.ResponseWriter, r *http.Request) {
	window, name, err := parseRange(r.URL.Query().Get("range"), h.cfg.now())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorDTO{Error: err.Error()})
		return
	}
	ctx := r.Context()

	totals, err := h.cfg.Queries.Totals(ctx, window)
	if err != nil {
		writeServerError(w, err)
		return
	}
	volume, err := h.cfg.Queries.Volume(ctx, window)
	if err != nil {
		writeServerError(w, err)
		return
	}
	projects, err := h.cfg.Queries.Projects(ctx, window)
	if err != nil {
		writeServerError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, projectsResponse{
		Range:    name,
		Totals:   newTotals(totals),
		Volume:   newPoints(volume),
		Projects: newProjects(projects),
	})
}

func (h *handler) models(w http.ResponseWriter, r *http.Request) {
	window, name, err := parseRange(r.URL.Query().Get("range"), h.cfg.now())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorDTO{Error: err.Error()})
		return
	}
	ctx := r.Context()

	totals, err := h.cfg.Queries.Totals(ctx, window)
	if err != nil {
		writeServerError(w, err)
		return
	}
	volume, err := h.cfg.Queries.Volume(ctx, window)
	if err != nil {
		writeServerError(w, err)
		return
	}
	models, err := h.cfg.Queries.Models(ctx, window)
	if err != nil {
		writeServerError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, modelsResponse{
		Range:  name,
		Totals: newTotals(totals),
		Volume: newPoints(volume),
		Models: newModels(models),
	})
}

func (h *handler) requests(w http.ResponseWriter, r *http.Request) {
	window, name, err := parseRange(r.URL.Query().Get("range"), h.cfg.now())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorDTO{Error: err.Error()})
		return
	}

	filter := domain.UsageFilter{
		Query: r.URL.Query().Get("q"),
		Limit: atoiOr(r.URL.Query().Get("limit"), 0),
	}
	if raw := r.URL.Query().Get("status"); raw != "" && raw != "all" {
		filter.Status = atoiOr(raw, 0)
	}
	if raw := r.URL.Query().Get("stream"); raw != "" {
		stream := raw == "true"
		filter.Stream = &stream
	}

	rows, total, err := h.cfg.Queries.Requests(r.Context(), window, filter)
	if err != nil {
		writeServerError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, requestsResponse{
		Range: name,
		Total: total,
		Rows:  newRequests(rows),
	})
}

func (h *handler) sessions(w http.ResponseWriter, r *http.Request) {
	window, name, err := parseRange(r.URL.Query().Get("range"), h.cfg.now())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorDTO{Error: err.Error()})
		return
	}
	ctx := r.Context()

	totals, err := h.cfg.Queries.Totals(ctx, window)
	if err != nil {
		writeServerError(w, err)
		return
	}
	sessions, err := h.cfg.Queries.Sessions(ctx, window, atoiOr(r.URL.Query().Get("limit"), 100))
	if err != nil {
		writeServerError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sessionsResponse{
		Range:    name,
		Totals:   newTotals(totals),
		Sessions: newSessions(sessions),
	})
}

func (h *handler) providers(w http.ResponseWriter, r *http.Request) {
	window, name, err := parseRange(r.URL.Query().Get("range"), h.cfg.now())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorDTO{Error: err.Error()})
		return
	}
	stats, err := h.cfg.Queries.Providers(r.Context(), window)
	if err != nil {
		writeServerError(w, err)
		return
	}

	byName := make(map[string]domain.ProviderStat, len(stats))
	for _, stat := range stats {
		byName[stat.Provider] = stat
	}
	providers := make([]providerDTO, 0, len(h.cfg.Providers))
	for _, info := range h.cfg.Providers {
		providers = append(providers, newProvider(info, byName[info.Name]))
	}

	routes := make([]routeDTO, 0, len(h.cfg.Routes))
	for _, route := range h.cfg.Routes {
		routes = append(routes, routeDTO{
			Match:    route.Match,
			Regex:    route.Regex,
			Provider: route.Provider,
			Model:    route.Model,
		})
	}

	writeJSON(w, http.StatusOK, providersResponse{
		Range:           name,
		MetricsEnabled:  true,
		DefaultProvider: h.cfg.DefaultProvider,
		Providers:       providers,
		Routes:          routes,
	})
}

func parseRange(raw string, now time.Time) (domain.MetricsRange, string, error) {
	var (
		bucket time.Duration
		count  int
		name   string
	)
	switch raw {
	case "", "24h":
		bucket, count, name = time.Hour, 24, "24h"
	case "7d":
		bucket, count, name = 24*time.Hour, 7, "7d"
	case "30d":
		bucket, count, name = 24*time.Hour, 30, "30d"
	default:
		return domain.MetricsRange{}, "", ErrUnknownRange
	}
	// End on a bucket boundary so the chart always holds exactly count points,
	// including the partial current bucket.
	until := now.Truncate(bucket).Add(bucket)
	since := until.Add(-time.Duration(count) * bucket)
	return domain.MetricsRange{Since: since, Until: until, Bucket: bucket}, name, nil
}

func atoiOr(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func writeServerError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, errorDTO{Error: err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type errorDTO struct {
	Error string `json:"error"`
}

type totalsDTO struct {
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"inputTokens"`
	OutputTokens        int64   `json:"outputTokens"`
	CacheReadTokens     int64   `json:"cacheReadTokens"`
	CacheCreationTokens int64   `json:"cacheCreationTokens"`
	CacheHitRate        float64 `json:"cacheHitRate"`
	AvgLatencyMs        float64 `json:"avgLatencyMs"`
	P95LatencyMs        int64   `json:"p95LatencyMs"`
	AvgTTFTMs           float64 `json:"avgTtftMs"`
	P95TTFTMs           int64   `json:"p95TtftMs"`
	Errors              int64   `json:"errors"`
	CostUSD             float64 `json:"costUSD"`
	PricedRequests      int64   `json:"pricedRequests"`
}

type pointDTO struct {
	Start        time.Time `json:"start"`
	Requests     int64     `json:"requests"`
	InputTokens  int64     `json:"inputTokens"`
	OutputTokens int64     `json:"outputTokens"`
}

type barDTO struct {
	Name     string `json:"name"`
	Requests int64  `json:"requests"`
	Tokens   int64  `json:"tokens"`
}

type requestDTO struct {
	Time                time.Time `json:"time"`
	Project             string    `json:"project"`
	Session             string    `json:"session"`
	Provider            string    `json:"provider"`
	RequestedModel      string    `json:"requestedModel"`
	UpstreamModel       string    `json:"upstreamModel"`
	InputTokens         int64     `json:"inputTokens"`
	OutputTokens        int64     `json:"outputTokens"`
	CacheReadTokens     int64     `json:"cacheReadTokens"`
	CacheCreationTokens int64     `json:"cacheCreationTokens"`
	Status              int       `json:"status"`
	DurationMs          int64     `json:"durationMs"`
	TTFTMs              int64     `json:"ttftMs"`
	Stream              bool      `json:"stream"`
	CostUSD             float64   `json:"costUSD"`
	HasCost             bool      `json:"hasCost"`
}

type projectDTO struct {
	Name         string  `json:"name"`
	Sessions     int64   `json:"sessions"`
	Requests     int64   `json:"requests"`
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	CacheHitRate float64 `json:"cacheHitRate"`
	AvgLatencyMs float64 `json:"avgLatencyMs"`
	AvgTTFTMs    float64 `json:"avgTtftMs"`
	CostUSD      float64 `json:"costUSD"`
}

type modelDTO struct {
	Name         string  `json:"name"`
	Provider     string  `json:"provider"`
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	AvgLatencyMs float64 `json:"avgLatencyMs"`
	AvgTTFTMs    float64 `json:"avgTtftMs"`
	Errors       int64   `json:"errors"`
	CostUSD      float64 `json:"costUSD"`
}

type sessionDTO struct {
	Session    string    `json:"session"`
	Project    string    `json:"project"`
	Started    time.Time `json:"started"`
	DurationMs int64     `json:"durationMs"`
	Requests   int64     `json:"requests"`
	Tokens     int64     `json:"tokens"`
	TopModel   string    `json:"topModel"`
	CostUSD    float64   `json:"costUSD"`
}

type providerDTO struct {
	Name         string  `json:"name"`
	BaseURL      string  `json:"baseUrl"`
	Auth         string  `json:"auth"`
	KeyPresent   bool    `json:"keyPresent"`
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	AvgLatencyMs float64 `json:"avgLatencyMs"`
	AvgTTFTMs    float64 `json:"avgTtftMs"`
	Errors       int64   `json:"errors"`
	CostUSD      float64 `json:"costUSD"`
}

type routeDTO struct {
	Match    string `json:"match"`
	Regex    string `json:"regex"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type overviewResponse struct {
	Range       string       `json:"range"`
	Totals      totalsDTO    `json:"totals"`
	Volume      []pointDTO   `json:"volume"`
	TopProjects []barDTO     `json:"topProjects"`
	TopModels   []barDTO     `json:"topModels"`
	Recent      []requestDTO `json:"recent"`
}

type projectsResponse struct {
	Range    string       `json:"range"`
	Totals   totalsDTO    `json:"totals"`
	Volume   []pointDTO   `json:"volume"`
	Projects []projectDTO `json:"projects"`
}

type modelsResponse struct {
	Range  string     `json:"range"`
	Totals totalsDTO  `json:"totals"`
	Volume []pointDTO `json:"volume"`
	Models []modelDTO `json:"models"`
}

type requestsResponse struct {
	Range string       `json:"range"`
	Total int64        `json:"total"`
	Rows  []requestDTO `json:"rows"`
}

type sessionsResponse struct {
	Range    string       `json:"range"`
	Totals   totalsDTO    `json:"totals"`
	Sessions []sessionDTO `json:"sessions"`
}

type providersResponse struct {
	Range           string        `json:"range"`
	MetricsEnabled  bool          `json:"metricsEnabled"`
	DefaultProvider string        `json:"defaultProvider"`
	Providers       []providerDTO `json:"providers"`
	Routes          []routeDTO    `json:"routes"`
}

func newTotals(t domain.MetricsTotals) totalsDTO {
	return totalsDTO{
		Requests:            t.Requests,
		InputTokens:         t.InputTokens,
		OutputTokens:        t.OutputTokens,
		CacheReadTokens:     t.CacheReadTokens,
		CacheCreationTokens: t.CacheCreationTokens,
		CacheHitRate:        t.CacheHitRate(),
		AvgLatencyMs:        t.AvgLatencyMs,
		P95LatencyMs:        t.P95LatencyMs,
		AvgTTFTMs:           t.AvgTTFTMs,
		P95TTFTMs:           t.P95TTFTMs,
		Errors:              t.Errors,
		CostUSD:             t.CostUSD,
		PricedRequests:      t.PricedRequests,
	}
}

func newPoints(points []domain.MetricsPoint) []pointDTO {
	out := make([]pointDTO, 0, len(points))
	for _, point := range points {
		out = append(out, pointDTO{
			Start:        point.Start,
			Requests:     point.Requests,
			InputTokens:  point.InputTokens,
			OutputTokens: point.OutputTokens,
		})
	}
	return out
}

func newProjectBars(stats []domain.ProjectStat, limit int) []barDTO {
	out := make([]barDTO, 0, limit)
	for _, stat := range stats {
		if len(out) == limit {
			break
		}
		out = append(out, barDTO{Name: stat.Project, Requests: stat.Requests, Tokens: stat.Tokens()})
	}
	return out
}

func newModelBars(stats []domain.ModelStat, limit int) []barDTO {
	out := make([]barDTO, 0, limit)
	for _, stat := range stats {
		if len(out) == limit {
			break
		}
		out = append(out, barDTO{Name: stat.Model, Requests: stat.Requests, Tokens: stat.Tokens})
	}
	return out
}

func newProjects(stats []domain.ProjectStat) []projectDTO {
	out := make([]projectDTO, 0, len(stats))
	for _, stat := range stats {
		out = append(out, projectDTO{
			Name:         stat.Project,
			Sessions:     stat.Sessions,
			Requests:     stat.Requests,
			InputTokens:  stat.InputTokens,
			OutputTokens: stat.OutputTokens,
			CacheHitRate: stat.CacheHitRate(),
			AvgLatencyMs: stat.AvgLatencyMs,
			AvgTTFTMs:    stat.AvgTTFTMs,
			CostUSD:      stat.CostUSD,
		})
	}
	return out
}

func newModels(stats []domain.ModelStat) []modelDTO {
	out := make([]modelDTO, 0, len(stats))
	for _, stat := range stats {
		out = append(out, modelDTO{
			Name:         stat.Model,
			Provider:     stat.Provider,
			Requests:     stat.Requests,
			Tokens:       stat.Tokens,
			AvgLatencyMs: stat.AvgLatencyMs,
			AvgTTFTMs:    stat.AvgTTFTMs,
			Errors:       stat.Errors,
			CostUSD:      stat.CostUSD,
		})
	}
	return out
}

func newRequests(rows []domain.UsageRow) []requestDTO {
	out := make([]requestDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, requestDTO{
			Time:                row.Time,
			Project:             row.Project,
			Session:             row.Session,
			Provider:            string(row.Provider),
			RequestedModel:      string(row.RequestedModel),
			UpstreamModel:       string(row.UpstreamModel),
			InputTokens:         row.InputTokens,
			OutputTokens:        row.OutputTokens,
			CacheReadTokens:     row.CacheReadTokens,
			CacheCreationTokens: row.CacheCreationTokens,
			Status:              row.Status,
			DurationMs:          row.DurationMs,
			TTFTMs:              row.TTFTMs,
			Stream:              row.Stream,
			CostUSD:             row.CostUSD,
			HasCost:             row.HasCost,
		})
	}
	return out
}

func newSessions(stats []domain.SessionStat) []sessionDTO {
	out := make([]sessionDTO, 0, len(stats))
	for _, stat := range stats {
		out = append(out, sessionDTO{
			Session:    stat.Session,
			Project:    stat.Project,
			Started:    stat.Started,
			DurationMs: stat.Duration().Milliseconds(),
			Requests:   stat.Requests,
			Tokens:     stat.Tokens,
			TopModel:   stat.TopModel,
			CostUSD:    stat.CostUSD,
		})
	}
	return out
}

func newProvider(info ProviderInfo, stat domain.ProviderStat) providerDTO {
	return providerDTO{
		Name:         info.Name,
		BaseURL:      info.BaseURL,
		Auth:         info.Auth,
		KeyPresent:   info.KeyPresent,
		Requests:     stat.Requests,
		Tokens:       stat.Tokens,
		AvgLatencyMs: stat.AvgLatencyMs,
		AvgTTFTMs:    stat.AvgTTFTMs,
		Errors:       stat.Errors,
		CostUSD:      stat.CostUSD,
	}
}
