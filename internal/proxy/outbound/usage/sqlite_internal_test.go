package usage

import (
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
