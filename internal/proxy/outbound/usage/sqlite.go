package usage

import (
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"

	_ "modernc.org/sqlite"
)

// schema is the usage table and its lookup index.
const schema = `
CREATE TABLE IF NOT EXISTS usage (
	ts INTEGER NOT NULL,
	project TEXT NOT NULL,
	session TEXT NOT NULL,
	provider TEXT NOT NULL,
	requested_model TEXT NOT NULL,
	upstream_model TEXT NOT NULL,
	input_tokens INTEGER NOT NULL,
	output_tokens INTEGER NOT NULL,
	cache_read_tokens INTEGER NOT NULL,
	cache_creation_tokens INTEGER NOT NULL,
	status INTEGER NOT NULL,
	duration_ms INTEGER NOT NULL,
	ttft_ms INTEGER,
	stream INTEGER NOT NULL,
	cost_usd REAL NOT NULL DEFAULT 0,
	has_cost INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS usage_project_ts ON usage(project, ts);
CREATE INDEX IF NOT EXISTS usage_ts ON usage(ts);
`

const insertUsage = `
INSERT INTO usage (
	ts, project, session, provider, requested_model, upstream_model,
	input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens,
	status, duration_ms, ttft_ms, stream, cost_usd, has_cost
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

const (
	insertBatch   = 64
	flushInterval = time.Second
	bufferSize    = 4096
)

// Store persists usage in a SQLite database. Record never blocks the caller:
// rows are buffered and written in batches by a background goroutine.
type Store struct {
	db     *sql.DB
	logger *slog.Logger
	ch     chan domain.Usage
	done   chan struct{}
}

// Open opens the database at path, creating it if needed, and applies the
// schema. Callers must Close it to flush pending records.
func Open(path string, logger *slog.Logger) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureColumns(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &Store{db: db, logger: logger, ch: make(chan domain.Usage, bufferSize), done: make(chan struct{})}
	go s.run()
	return s, nil
}

// Record buffers a usage. It never blocks: when the buffer is full the record
// is dropped, since metrics must not slow the proxy down.
func (s *Store) Record(usage domain.Usage) {
	select {
	case s.ch <- usage:
	default:
		s.logger.Warn("usage buffer full; dropping record")
	}
}

func (s *Store) run() {
	defer close(s.done)
	batch := make([]domain.Usage, 0, insertBatch)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case usage, ok := <-s.ch:
			if !ok {
				s.flush(batch)
				return
			}
			batch = append(batch, usage)
			if len(batch) == insertBatch {
				s.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			s.flush(batch)
			batch = batch[:0]
		}
	}
}

func (s *Store) flush(batch []domain.Usage) {
	if len(batch) == 0 {
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		s.logger.Warn("usage begin failed", "error", err)
		return
	}
	stmt, err := tx.Prepare(insertUsage)
	if err != nil {
		_ = tx.Rollback()
		s.logger.Warn("usage prepare failed", "error", err)
		return
	}
	for _, u := range batch {
		if _, err := stmt.Exec(
			u.Time.UnixMilli(),
			u.Project,
			u.Session,
			string(u.Provider),
			string(u.RequestedModel),
			string(u.UpstreamModel),
			u.InputTokens,
			u.OutputTokens,
			u.CacheReadInputTokens,
			u.CacheCreationInputTokens,
			u.Status,
			u.Duration.Milliseconds(),
			nullableMs(u.TimeToFirstToken.Milliseconds()),
			boolToInt(u.Stream),
			u.CostUSD,
			boolToInt(u.HasCost),
		); err != nil {
			s.logger.Warn("usage insert failed", "error", err)
		}
	}
	_ = stmt.Close()
	if err := tx.Commit(); err != nil {
		s.logger.Warn("usage commit failed", "error", err)
	}
}

// Close flushes pending records and closes the database.
func (s *Store) Close() error {
	close(s.ch)
	<-s.done
	return s.db.Close()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nullableMs stores an unmeasured duration as NULL, so AVG and the p95 skip it.
func nullableMs(ms int64) any {
	if ms <= 0 {
		return nil
	}
	return ms
}
