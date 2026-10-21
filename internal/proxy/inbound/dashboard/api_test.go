package dashboard

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

type fakeQueries struct {
	totals    domain.MetricsTotals
	volume    []domain.MetricsPoint
	projects  []domain.ProjectStat
	models    []domain.ModelStat
	sessions  []domain.SessionStat
	providers []domain.ProviderStat
	rows      []domain.UsageRow
	total     int64
	snapshots []domain.RateLimitSnapshot

	lastRange  domain.MetricsRange
	lastFilter domain.UsageFilter
}

func (f *fakeQueries) Totals(_ context.Context, r domain.MetricsRange) (domain.MetricsTotals, error) {
	f.lastRange = r
	return f.totals, nil
}

func (f *fakeQueries) Volume(_ context.Context, _ domain.MetricsRange) ([]domain.MetricsPoint, error) {
	return f.volume, nil
}

func (f *fakeQueries) Projects(_ context.Context, _ domain.MetricsRange) ([]domain.ProjectStat, error) {
	return f.projects, nil
}

func (f *fakeQueries) Models(_ context.Context, _ domain.MetricsRange) ([]domain.ModelStat, error) {
	return f.models, nil
}

func (f *fakeQueries) Sessions(_ context.Context, _ domain.MetricsRange, _ int) ([]domain.SessionStat, error) {
	return f.sessions, nil
}

func (f *fakeQueries) Providers(_ context.Context, _ domain.MetricsRange) ([]domain.ProviderStat, error) {
	return f.providers, nil
}

func (f *fakeQueries) Requests(_ context.Context, r domain.MetricsRange, filter domain.UsageFilter) ([]domain.UsageRow, int64, error) {
	f.lastRange = r
	f.lastFilter = filter
	return f.rows, f.total, nil
}

func (f *fakeQueries) RateLimits(_ context.Context, r domain.MetricsRange) ([]domain.RateLimitSnapshot, error) {
	f.lastRange = r
	return f.snapshots, nil
}

func fixedNow() time.Time {
	return time.Date(2026, 10, 18, 15, 0, 0, 0, time.UTC)
}

func TestAPIOverviewReportsTotalsAndCost(t *testing.T) {
	queries := &fakeQueries{
		totals: domain.MetricsTotals{
			Requests:            3,
			InputTokens:         350,
			OutputTokens:        175,
			CacheReadTokens:     50,
			CacheCreationTokens: 10,
			Errors:              1,
			AvgLatencyMs:        1500,
			P95LatencyMs:        3000,
			CostUSD:             0.3,
			PricedRequests:      2,
		},
		projects: []domain.ProjectStat{{Project: "acme/gateway", Requests: 2, InputTokens: 300, OutputTokens: 150, CostUSD: 0.3}},
		models:   []domain.ModelStat{{Model: "claude-opus-4", Provider: "anthropic", Requests: 2, Tokens: 450, CostUSD: 0.3}},
	}
	handler := NewHandler(Config{Queries: queries, Now: fixedNow})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Prefix+"/api/overview?range=24h", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var body overviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Totals.Requests != 3 {
		t.Errorf("requests = %d, want 3", body.Totals.Requests)
	}
	if body.Totals.CostUSD != 0.3 || body.Totals.PricedRequests != 2 {
		t.Errorf("cost = %v / %d priced, want 0.3 / 2", body.Totals.CostUSD, body.Totals.PricedRequests)
	}
	if want := 50.0 / 410.0; math.Abs(body.Totals.CacheHitRate-want) > 1e-9 {
		t.Errorf("cache hit rate = %v, want %v", body.Totals.CacheHitRate, want)
	}
	if len(body.TopProjects) != 1 || body.TopProjects[0].Name != "acme/gateway" {
		t.Errorf("top projects = %+v", body.TopProjects)
	}
	if got := queries.lastRange.Until.Sub(queries.lastRange.Since); got != 24*time.Hour {
		t.Errorf("window = %v, want 24h", got)
	}
	if queries.lastRange.Bucket != time.Hour {
		t.Errorf("bucket = %v, want 1h", queries.lastRange.Bucket)
	}
}

func TestAPIIsUnavailableWhenMetricsAreOff(t *testing.T) {
	handler := NewHandler(Config{MetricsError: "usage metrics are disabled"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Prefix+"/api/projects", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var body errorDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "usage metrics are disabled" {
		t.Errorf("error = %q", body.Error)
	}
}

func TestAPIRejectsUnknownRange(t *testing.T) {
	handler := NewHandler(Config{Queries: &fakeQueries{}, Now: fixedNow})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Prefix+"/api/models?range=1y", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAPIRequestsForwardsFilters(t *testing.T) {
	queries := &fakeQueries{}
	handler := NewHandler(Config{Queries: queries, Now: fixedNow})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Prefix+"/api/requests?range=7d&q=acme&status=429&stream=true&limit=10", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if queries.lastFilter.Query != "acme" {
		t.Errorf("query = %q, want acme", queries.lastFilter.Query)
	}
	if queries.lastFilter.Status != 429 {
		t.Errorf("status filter = %d, want 429", queries.lastFilter.Status)
	}
	if queries.lastFilter.Stream == nil || !*queries.lastFilter.Stream {
		t.Errorf("stream filter = %v, want true", queries.lastFilter.Stream)
	}
	if queries.lastFilter.Limit != 10 {
		t.Errorf("limit = %d, want 10", queries.lastFilter.Limit)
	}
	if got := queries.lastRange.Until.Sub(queries.lastRange.Since); got != 7*24*time.Hour {
		t.Errorf("window = %v, want 7d", got)
	}
	if queries.lastRange.Bucket != 24*time.Hour {
		t.Errorf("bucket = %v, want 24h", queries.lastRange.Bucket)
	}
}

func TestAPIProvidersMergesConfigWithUsage(t *testing.T) {
	queries := &fakeQueries{
		providers: []domain.ProviderStat{{Provider: "anthropic", Requests: 2, Tokens: 450, CostUSD: 0.3}},
	}
	handler := NewHandler(Config{
		Queries: queries,
		Now:     fixedNow,
		Providers: []ProviderInfo{
			{Name: "anthropic", BaseURL: "https://api.anthropic.com", Auth: "passthrough"},
			{Name: "openrouter", BaseURL: "https://openrouter.ai/api", Auth: "bearer", KeyPresent: true},
		},
		Routes:          []RouteInfo{{Match: "claude-*", Provider: "anthropic"}},
		DefaultProvider: "anthropic",
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Prefix+"/api/providers", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var body providersResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(body.Providers))
	}
	if body.Providers[0].Requests != 2 || body.Providers[0].CostUSD != 0.3 {
		t.Errorf("anthropic = %+v", body.Providers[0])
	}
	if body.Providers[1].Requests != 0 {
		t.Errorf("openrouter should have no usage, got %+v", body.Providers[1])
	}
	if !body.Providers[1].KeyPresent {
		t.Errorf("openrouter key presence was lost")
	}
	if body.DefaultProvider != "anthropic" {
		t.Errorf("default provider = %q", body.DefaultProvider)
	}
}

func TestAPIQuotaReportsCurrentSeriesAndShares(t *testing.T) {
	at := func(minutes int) time.Time { return fixedNow().Add(time.Duration(minutes-60) * time.Minute) }
	window := func(utilization float64, resetMinutes int) domain.QuotaWindow {
		return domain.QuotaWindow{Utilization: utilization, ResetsAt: at(resetMinutes), Status: "allowed", Present: true}
	}
	queries := &fakeQueries{snapshots: []domain.RateLimitSnapshot{
		{Time: at(0), Attribution: domain.Attribution{Project: "acme/gateway", Session: "s1"}, FiveHour: window(0.10, 200), SevenDay: window(0.20, 5000)},
		{Time: at(30), Attribution: domain.Attribution{Project: "acme/gateway", Session: "s1"}, FiveHour: window(0.15, 200), SevenDay: window(0.22, 5000), Claim: "five_hour"},
	}}
	handler := NewHandler(Config{Queries: queries, Now: fixedNow})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Prefix+"/api/quota?range=24h", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var body quotaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Current == nil || body.Current.FiveHour.Utilization != 0.15 || body.Current.SevenDay.Utilization != 0.22 || body.Current.Claim != "five_hour" {
		t.Fatalf("current = %+v", body.Current)
	}
	if body.Current.FiveHour.ResetsAt == nil || !body.Current.FiveHour.ResetsAt.Equal(at(200)) {
		t.Errorf("five hour reset = %v, want %v", body.Current.FiveHour.ResetsAt, at(200))
	}
	if len(body.Series) != 2 {
		t.Errorf("series = %+v, want 2 points", body.Series)
	}
	if len(body.Projects) != 1 || body.Projects[0].Name != "acme/gateway" || math.Abs(body.Projects[0].SevenDay-0.02) > 1e-9 {
		t.Errorf("projects = %+v", body.Projects)
	}
	if len(body.Sessions) != 1 || body.Sessions[0].Name != "s1" || body.Sessions[0].Project != "acme/gateway" {
		t.Errorf("sessions = %+v", body.Sessions)
	}
}

func TestAPIQuotaIsEmptyWithoutSnapshots(t *testing.T) {
	handler := NewHandler(Config{Queries: &fakeQueries{}, Now: fixedNow})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Prefix+"/api/quota", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body quotaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Current != nil || len(body.Series) != 0 {
		t.Errorf("body = %+v, want no current and no series", body)
	}
	if !json.Valid(rec.Body.Bytes()) || !contains(rec.Body.String(), `"series":[]`) {
		t.Errorf("series must encode as an empty array: %s", rec.Body.String())
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
