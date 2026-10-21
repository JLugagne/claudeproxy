package usage

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	store, err := Open(path, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	store.Record(domain.Usage{
		Time:                     time.Unix(1700000000, 0),
		Attribution:              domain.Attribution{Project: "owner/repo", Session: "s1"},
		Provider:                 "openrouter",
		RequestedModel:           "claude-x",
		UpstreamModel:            "deepseek/chat",
		InputTokens:              11,
		OutputTokens:             22,
		CacheReadInputTokens:     3,
		CacheCreationInputTokens: 4,
		Status:                   200,
		Duration:                 1500 * time.Millisecond,
		TimeToFirstToken:         300 * time.Millisecond,
		Stream:                   true,
	})
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var (
		project, session, provider, requested, upstream   string
		in, out, cacheRead, cacheCreate, status, duration int
		stream, ttft                                      int
	)
	err = db.QueryRow(`SELECT project, session, provider, requested_model, upstream_model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, status, duration_ms, stream, ttft_ms FROM usage`).
		Scan(&project, &session, &provider, &requested, &upstream, &in, &out, &cacheRead, &cacheCreate, &status, &duration, &stream, &ttft)
	if err != nil {
		t.Fatal(err)
	}
	if project != "owner/repo" || session != "s1" || provider != "openrouter" || requested != "claude-x" || upstream != "deepseek/chat" || in != 11 || out != 22 || cacheRead != 3 || cacheCreate != 4 || status != 200 || duration != 1500 || stream != 1 || ttft != 300 {
		t.Fatalf("row = %s %s %s %s %s %d %d %d %d %d %d %d %d", project, session, provider, requested, upstream, in, out, cacheRead, cacheCreate, status, duration, stream, ttft)
	}
}

func TestStoreKeepsRateLimitHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	store, err := Open(path, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	store.Record(domain.Usage{
		Time:        time.Unix(1700000000, 0),
		Attribution: domain.Attribution{Project: "owner/repo", Session: "s1"},
		RateLimits:  map[string]string{"anthropic-ratelimit-unified-5h-utilization": "0.42"},
	})
	store.Record(domain.Usage{Time: time.Unix(1700000001, 0)})
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ratelimit`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("ratelimit rows = %d, want 1", rows)
	}
	var (
		ts               int64
		project, session string
		utilization      string
	)
	err = db.QueryRow(`SELECT ts, project, session, json_extract(headers, '$."anthropic-ratelimit-unified-5h-utilization"') FROM ratelimit`).
		Scan(&ts, &project, &session, &utilization)
	if err != nil {
		t.Fatal(err)
	}
	if ts != 1700000000000 || project != "owner/repo" || session != "s1" || utilization != "0.42" {
		t.Fatalf("row = %d %s %s %s", ts, project, session, utilization)
	}
}

func TestStoreReadsRateLimitSnapshots(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	store, err := Open(path, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	store.Record(domain.Usage{
		Time:        time.Unix(1700000100, 0),
		Attribution: domain.Attribution{Project: "owner/repo", Session: "s1"},
		RateLimits: map[string]string{
			"anthropic-ratelimit-unified-5h-utilization":       "0.42",
			"anthropic-ratelimit-unified-5h-reset":             "1700003600",
			"anthropic-ratelimit-unified-5h-status":            "allowed",
			"anthropic-ratelimit-unified-7d-utilization":       "0.1",
			"anthropic-ratelimit-unified-representative-claim": "five_hour",
		},
	})
	store.Record(domain.Usage{
		Time:       time.Unix(1700000200, 0),
		RateLimits: map[string]string{"anthropic-ratelimit-requests-limit": "50"},
	})
	store.Record(domain.Usage{
		Time:       time.Unix(1700009999, 0),
		RateLimits: map[string]string{"anthropic-ratelimit-unified-5h-utilization": "0.9"},
	})
	waitForRateLimits(t, store, 3)

	got, err := store.RateLimits(context.Background(), domain.MetricsRange{Since: time.Unix(1700000000, 0), Until: time.Unix(1700005000, 0)})
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 {
		t.Fatalf("snapshots = %+v, want 1 (one outside the range, one without unified headers)", got)
	}
	s := got[0]
	if s.Project != "owner/repo" || s.Session != "s1" || s.Claim != "five_hour" {
		t.Errorf("snapshot = %+v", s)
	}
	if !s.FiveHour.Present || s.FiveHour.Utilization != 0.42 || s.FiveHour.Status != "allowed" || !s.FiveHour.ResetsAt.Equal(time.Unix(1700003600, 0)) {
		t.Errorf("five hour = %+v", s.FiveHour)
	}
	if !s.SevenDay.Present || s.SevenDay.Utilization != 0.1 {
		t.Errorf("seven day = %+v", s.SevenDay)
	}
}

func waitForRateLimits(t *testing.T, store *Store, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM ratelimit`).Scan(&n); err == nil && n >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("ratelimit rows never reached %d", want)
}

func TestStoreKeepsOneRateLimitRowPerSessionAndHalfHour(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	store, err := Open(path, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	limits := func(utilization string) map[string]string {
		return map[string]string{"anthropic-ratelimit-unified-5h-utilization": utilization}
	}
	base := time.Unix(1700000000, 0).Truncate(30 * time.Minute)
	record := func(offset time.Duration, session, utilization string) {
		store.Record(domain.Usage{
			Time:        base.Add(offset),
			Attribution: domain.Attribution{Project: "owner/repo", Session: session},
			RateLimits:  limits(utilization),
		})
	}
	record(time.Minute, "s1", "0.10")
	record(10*time.Minute, "s1", "0.12")
	record(29*time.Minute, "s1", "0.15")
	record(20*time.Minute, "s2", "0.16")
	record(31*time.Minute, "s1", "0.20")
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT session, ts, json_extract(headers, '$."anthropic-ratelimit-unified-5h-utilization"') FROM ratelimit ORDER BY ts`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var (
			session, utilization string
			ts                   int64
		)
		if err := rows.Scan(&session, &ts, &utilization); err != nil {
			t.Fatal(err)
		}
		got = append(got, session+"@"+time.UnixMilli(ts).Sub(base).String()+"="+utilization)
	}
	want := []string{"s2@20m0s=0.16", "s1@29m0s=0.15", "s1@31m0s=0.20"}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %v, want %v", got, want)
		}
	}
}

func TestOpenCompactsRateLimitRowsWrittenPerRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`
CREATE TABLE ratelimit (ts INTEGER NOT NULL, project TEXT NOT NULL, session TEXT NOT NULL, headers TEXT NOT NULL);
INSERT INTO ratelimit VALUES (1700000000000, 'p', 's', '{"v":"old"}');
INSERT INTO ratelimit VALUES (1700000060000, 'p', 's', '{"v":"new"}');
INSERT INTO ratelimit VALUES (1700000070000, 'p', 'other', '{"v":"other"}');
INSERT INTO ratelimit VALUES (1700003600000, 'p', 's', '{"v":"later"}');`)
	if err != nil {
		t.Fatal(err)
	}
	_ = legacy.Close()

	store, err := Open(path, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var rows int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM ratelimit`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 3 {
		t.Fatalf("rows = %d, want 3 (two legacy rows of one session and half hour merged)", rows)
	}
	var headers string
	if err := store.db.QueryRow(`SELECT headers FROM ratelimit WHERE session = 's' AND ts = 1700000060000`).Scan(&headers); err != nil {
		t.Fatalf("the newest row of the half hour must survive: %v", err)
	}
}
