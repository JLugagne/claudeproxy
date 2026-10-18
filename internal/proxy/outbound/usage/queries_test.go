package usage

import (
	"context"
	"database/sql"
	"log/slog"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

type seedRow struct {
	at          time.Time
	project     string
	session     string
	provider    string
	model       string
	input       int64
	output      int64
	cacheRead   int64
	cacheCreate int64
	status      int
	durationMs  int64
	stream      bool
	costUSD     float64
	hasCost     bool
}

func openSeedStore(t *testing.T, rows []seedRow) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "usage.db"), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for _, row := range rows {
		if _, err := store.db.Exec(insertUsage,
			row.at.UnixMilli(), row.project, row.session, row.provider, row.model, row.model,
			row.input, row.output, row.cacheRead, row.cacheCreate,
			row.status, row.durationMs, boolToInt(row.stream), row.costUSD, boolToInt(row.hasCost),
		); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return store
}

func testRange(now time.Time) domain.MetricsRange {
	until := now.Truncate(time.Hour).Add(time.Hour)
	return domain.MetricsRange{Since: until.Add(-24 * time.Hour), Until: until, Bucket: time.Hour}
}

func seedRows(now time.Time) []seedRow {
	return []seedRow{
		{
			at: now.Add(-3 * time.Hour), project: "acme/gateway", session: "s1", provider: "anthropic", model: "claude-opus-4",
			input: 100, output: 50, cacheRead: 30, cacheCreate: 10, status: 200, durationMs: 1000, stream: true,
			costUSD: 0.10, hasCost: true,
		},
		{
			at: now.Add(-2 * time.Hour), project: "acme/gateway", session: "s1", provider: "anthropic", model: "claude-opus-4",
			input: 200, output: 100, cacheRead: 20, status: 200, durationMs: 3000,
			costUSD: 0.20, hasCost: true,
		},
		{
			at: now.Add(-time.Hour), project: "acme/web", session: "s2", provider: "openrouter", model: "deepseek-v4.1-flash",
			input: 50, output: 25, status: 429, durationMs: 500, stream: true,
		},
	}
}

func TestTotalsAggregatesAndPrices(t *testing.T) {
	now := time.Now()
	store := openSeedStore(t, seedRows(now))

	totals, err := store.Totals(context.Background(), testRange(now))
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if totals.Requests != 3 {
		t.Errorf("requests = %d, want 3", totals.Requests)
	}
	if totals.InputTokens != 350 || totals.OutputTokens != 175 {
		t.Errorf("tokens = %d/%d, want 350/175", totals.InputTokens, totals.OutputTokens)
	}
	if totals.Tokens() != 525 {
		t.Errorf("Tokens() = %d, want 525", totals.Tokens())
	}
	if totals.CacheReadTokens != 50 || totals.CacheCreationTokens != 10 {
		t.Errorf("cache = %d/%d, want 50/10", totals.CacheReadTokens, totals.CacheCreationTokens)
	}
	if totals.Errors != 1 {
		t.Errorf("errors = %d, want 1", totals.Errors)
	}
	if math.Abs(totals.CostUSD-0.30) > 1e-9 || totals.PricedRequests != 2 {
		t.Errorf("cost = %v over %d priced, want 0.30 over 2", totals.CostUSD, totals.PricedRequests)
	}
	if totals.P95LatencyMs == 0 {
		t.Errorf("p95 was not computed")
	}
}

func TestVolumeZeroFillsEveryBucket(t *testing.T) {
	now := time.Now()
	store := openSeedStore(t, seedRows(now))

	points, err := store.Volume(context.Background(), testRange(now))
	if err != nil {
		t.Fatalf("volume: %v", err)
	}
	if len(points) != 24 {
		t.Fatalf("points = %d, want 24 buckets", len(points))
	}
	var total int64
	for _, point := range points {
		total += point.Requests
	}
	if total != 3 {
		t.Errorf("summed requests = %d, want 3", total)
	}
}

func TestProjectsAndModelsRankByTokens(t *testing.T) {
	now := time.Now()
	store := openSeedStore(t, seedRows(now))

	projects, err := store.Projects(context.Background(), testRange(now))
	if err != nil {
		t.Fatalf("projects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("projects = %d, want 2", len(projects))
	}
	if projects[0].Project != "acme/gateway" || projects[0].Tokens() != 450 {
		t.Errorf("first project = %+v, want acme/gateway with 450 tokens", projects[0])
	}
	if projects[0].Sessions != 1 {
		t.Errorf("sessions = %d, want 1", projects[0].Sessions)
	}
	if math.Abs(projects[0].CostUSD-0.30) > 1e-9 {
		t.Errorf("project cost = %v, want 0.30", projects[0].CostUSD)
	}

	models, err := store.Models(context.Background(), testRange(now))
	if err != nil {
		t.Fatalf("models: %v", err)
	}
	if len(models) != 2 || models[0].Model != "claude-opus-4" {
		t.Fatalf("models = %+v", models)
	}
	if models[1].Errors != 1 {
		t.Errorf("deepseek errors = %d, want 1", models[1].Errors)
	}
}

func TestSessionsReportTopModelAndDuration(t *testing.T) {
	now := time.Now()
	store := openSeedStore(t, seedRows(now))

	sessions, err := store.Sessions(context.Background(), testRange(now), 10)
	if err != nil {
		t.Fatalf("sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}
	if sessions[0].Session != "s2" {
		t.Errorf("most recent session = %q, want s2", sessions[0].Session)
	}
	if sessions[1].TopModel != "claude-opus-4" {
		t.Errorf("top model = %q, want claude-opus-4", sessions[1].TopModel)
	}
	if sessions[1].Duration() != time.Hour {
		t.Errorf("duration = %v, want 1h", sessions[1].Duration())
	}
}

func TestRequestsFiltersAndCounts(t *testing.T) {
	now := time.Now()
	store := openSeedStore(t, seedRows(now))
	window := testRange(now)

	all, total, err := store.Requests(context.Background(), window, domain.UsageFilter{})
	if err != nil {
		t.Fatalf("requests: %v", err)
	}
	if len(all) != 3 || total != 3 {
		t.Fatalf("got %d rows / %d total, want 3/3", len(all), total)
	}
	if !all[0].Time.After(all[1].Time) {
		t.Errorf("rows are not newest first")
	}

	streaming := false
	matching, total, err := store.Requests(context.Background(), window, domain.UsageFilter{Query: "ACME/GATE", Status: 200, Stream: &streaming})
	if err != nil {
		t.Fatalf("filtered: %v", err)
	}
	if total != 1 || len(matching) != 1 {
		t.Fatalf("filtered total = %d, want 1", total)
	}
	if matching[0].CostUSD != 0.20 || !matching[0].HasCost {
		t.Errorf("cost was not read back: %+v", matching[0])
	}
}

func TestEnsureColumnsMigratesAnOlderDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	store, err := Open(path, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = legacy.Close() }()
	if _, err := legacy.Exec("ALTER TABLE usage DROP COLUMN cost_usd"); err != nil {
		t.Skipf("this SQLite build cannot drop columns: %v", err)
	}
	if _, err := legacy.Exec("ALTER TABLE usage DROP COLUMN has_cost"); err != nil {
		t.Fatalf("drop has_cost: %v", err)
	}

	migrated, err := Open(path, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("reopen after migration: %v", err)
	}
	defer func() { _ = migrated.Close() }()

	columns, err := tableColumns(migrated.db, "usage")
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	if !columns["cost_usd"] || !columns["has_cost"] {
		t.Errorf("migration did not restore the columns: %v", columns)
	}
}
