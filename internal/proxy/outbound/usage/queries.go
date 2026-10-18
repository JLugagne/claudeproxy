package usage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

// DefaultRequestLimit bounds the request log when the caller sets none.
const DefaultRequestLimit = 200

// MaxRequestLimit is the hard ceiling on the request log.
const MaxRequestLimit = 1000

const costExpr = "COALESCE(SUM(CASE WHEN has_cost = 1 THEN cost_usd ELSE 0 END),0)"

// Totals aggregates every request in the range.
func (s *Store) Totals(ctx context.Context, r domain.MetricsRange) (domain.MetricsTotals, error) {
	since, until := r.Since.UnixMilli(), r.Until.UnixMilli()
	var totals domain.MetricsTotals
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*),
       COALESCE(SUM(input_tokens),0),
       COALESCE(SUM(output_tokens),0),
       COALESCE(SUM(cache_read_tokens),0),
       COALESCE(SUM(cache_creation_tokens),0),
       COALESCE(SUM(CASE WHEN status >= 400 THEN 1 ELSE 0 END),0),
       COALESCE(SUM(CASE WHEN has_cost = 1 THEN 1 ELSE 0 END),0),
       `+costExpr+`,
       COALESCE(AVG(duration_ms),0)
FROM usage WHERE ts >= ? AND ts < ?`, since, until).
		Scan(
			&totals.Requests,
			&totals.InputTokens,
			&totals.OutputTokens,
			&totals.CacheReadTokens,
			&totals.CacheCreationTokens,
			&totals.Errors,
			&totals.PricedRequests,
			&totals.CostUSD,
			&totals.AvgLatencyMs,
		)
	if err != nil {
		return domain.MetricsTotals{}, err
	}
	if totals.Requests == 0 {
		return totals, nil
	}
	offset := int64(float64(totals.Requests) * 0.95)
	if offset >= totals.Requests {
		offset = totals.Requests - 1
	}
	err = s.db.QueryRowContext(ctx,
		`SELECT duration_ms FROM usage WHERE ts >= ? AND ts < ? ORDER BY duration_ms LIMIT 1 OFFSET ?`,
		since, until, offset,
	).Scan(&totals.P95LatencyMs)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.MetricsTotals{}, err
	}
	return totals, nil
}

// Volume returns one point per bucket, zero-filled across the whole range so
// the chart never has holes where nothing was recorded.
func (s *Store) Volume(ctx context.Context, r domain.MetricsRange) ([]domain.MetricsPoint, error) {
	bucket := r.Bucket
	if bucket <= 0 {
		bucket = time.Hour
	}
	bucketMs := bucket.Milliseconds()
	if bucketMs <= 0 {
		bucketMs = time.Hour.Milliseconds()
	}
	since, until := r.Since.UnixMilli(), r.Until.UnixMilli()

	rows, err := s.db.QueryContext(ctx, `
SELECT (ts / ?) * ? AS bucket,
       COUNT(*),
       COALESCE(SUM(input_tokens),0),
       COALESCE(SUM(output_tokens),0)
FROM usage WHERE ts >= ? AND ts < ?
GROUP BY bucket ORDER BY bucket`, bucketMs, bucketMs, since, until)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	byBucket := make(map[int64]domain.MetricsPoint)
	for rows.Next() {
		var (
			bucket int64
			point  domain.MetricsPoint
		)
		if err := rows.Scan(&bucket, &point.Requests, &point.InputTokens, &point.OutputTokens); err != nil {
			return nil, err
		}
		point.Start = time.UnixMilli(bucket)
		byBucket[bucket] = point
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	points := make([]domain.MetricsPoint, 0, 32)
	for ts := (since / bucketMs) * bucketMs; ts < until; ts += bucketMs {
		if point, ok := byBucket[ts]; ok {
			points = append(points, point)
			continue
		}
		points = append(points, domain.MetricsPoint{Start: time.UnixMilli(ts)})
	}
	return points, nil
}

// Projects aggregates usage per git project, most tokens first.
func (s *Store) Projects(ctx context.Context, r domain.MetricsRange) ([]domain.ProjectStat, error) {
	return queryAll(ctx, s.db, `
SELECT project,
       COUNT(DISTINCT session),
       COUNT(*),
       COALESCE(SUM(input_tokens),0),
       COALESCE(SUM(output_tokens),0),
       COALESCE(SUM(cache_read_tokens),0),
       COALESCE(SUM(cache_creation_tokens),0),
       COALESCE(AVG(duration_ms),0),
       `+costExpr+`
FROM usage WHERE ts >= ? AND ts < ?
GROUP BY project
ORDER BY SUM(input_tokens + output_tokens) DESC`,
		[]any{r.Since.UnixMilli(), r.Until.UnixMilli()},
		func(rows *sql.Rows) (domain.ProjectStat, error) {
			var stat domain.ProjectStat
			err := rows.Scan(
				&stat.Project,
				&stat.Sessions,
				&stat.Requests,
				&stat.InputTokens,
				&stat.OutputTokens,
				&stat.CacheReadTokens,
				&stat.CacheCreationTokens,
				&stat.AvgLatencyMs,
				&stat.CostUSD,
			)
			return stat, err
		})
}

// Models aggregates usage per requested model and provider.
func (s *Store) Models(ctx context.Context, r domain.MetricsRange) ([]domain.ModelStat, error) {
	return queryAll(ctx, s.db, `
SELECT requested_model,
       provider,
       COUNT(*),
       COALESCE(SUM(input_tokens + output_tokens),0),
       COALESCE(AVG(duration_ms),0),
       COALESCE(SUM(CASE WHEN status >= 400 THEN 1 ELSE 0 END),0),
       `+costExpr+`
FROM usage WHERE ts >= ? AND ts < ?
GROUP BY requested_model, provider
ORDER BY SUM(input_tokens + output_tokens) DESC`,
		[]any{r.Since.UnixMilli(), r.Until.UnixMilli()},
		func(rows *sql.Rows) (domain.ModelStat, error) {
			var stat domain.ModelStat
			err := rows.Scan(
				&stat.Model,
				&stat.Provider,
				&stat.Requests,
				&stat.Tokens,
				&stat.AvgLatencyMs,
				&stat.Errors,
				&stat.CostUSD,
			)
			return stat, err
		})
}

// Sessions aggregates usage per Claude Code session, most recent first.
func (s *Store) Sessions(ctx context.Context, r domain.MetricsRange, limit int) ([]domain.SessionStat, error) {
	if limit <= 0 {
		limit = DefaultRequestLimit
	}
	return queryAll(ctx, s.db, `
SELECT u.session,
       u.project,
       MIN(u.ts),
       MAX(u.ts),
       COUNT(*),
       COALESCE(SUM(u.input_tokens + u.output_tokens),0),
       COALESCE((
           SELECT u2.requested_model FROM usage u2
           WHERE u2.session = u.session
           GROUP BY u2.requested_model
           ORDER BY SUM(u2.input_tokens + u2.output_tokens) DESC, COUNT(*) DESC
           LIMIT 1
       ), ''),
       `+costExpr+`
FROM usage u WHERE u.ts >= ? AND u.ts < ?
GROUP BY u.session, u.project
ORDER BY MAX(u.ts) DESC
LIMIT ?`,
		[]any{r.Since.UnixMilli(), r.Until.UnixMilli(), limit},
		func(rows *sql.Rows) (domain.SessionStat, error) {
			var (
				stat     domain.SessionStat
				started  int64
				lastSeen int64
			)
			err := rows.Scan(
				&stat.Session,
				&stat.Project,
				&started,
				&lastSeen,
				&stat.Requests,
				&stat.Tokens,
				&stat.TopModel,
				&stat.CostUSD,
			)
			stat.Started = time.UnixMilli(started)
			stat.LastSeen = time.UnixMilli(lastSeen)
			return stat, err
		})
}

// Providers aggregates usage per upstream provider.
func (s *Store) Providers(ctx context.Context, r domain.MetricsRange) ([]domain.ProviderStat, error) {
	return queryAll(ctx, s.db, `
SELECT provider,
       COUNT(*),
       COALESCE(SUM(input_tokens + output_tokens),0),
       COALESCE(AVG(duration_ms),0),
       COALESCE(SUM(CASE WHEN status >= 400 THEN 1 ELSE 0 END),0),
       `+costExpr+`
FROM usage WHERE ts >= ? AND ts < ?
GROUP BY provider
ORDER BY COUNT(*) DESC`,
		[]any{r.Since.UnixMilli(), r.Until.UnixMilli()},
		func(rows *sql.Rows) (domain.ProviderStat, error) {
			var stat domain.ProviderStat
			err := rows.Scan(
				&stat.Provider,
				&stat.Requests,
				&stat.Tokens,
				&stat.AvgLatencyMs,
				&stat.Errors,
				&stat.CostUSD,
			)
			return stat, err
		})
}

// Requests returns the request log, newest first, with the total number of
// rows the filter matches.
func (s *Store) Requests(ctx context.Context, r domain.MetricsRange, f domain.UsageFilter) ([]domain.UsageRow, int64, error) {
	where := []string{"ts >= ?", "ts < ?"}
	args := []any{r.Since.UnixMilli(), r.Until.UnixMilli()}

	if needle := strings.ToLower(strings.TrimSpace(f.Query)); needle != "" {
		where = append(where, `(instr(lower(project), ?) > 0 OR instr(lower(requested_model), ?) > 0 OR instr(lower(session), ?) > 0)`)
		args = append(args, needle, needle, needle)
	}
	if f.Status != 0 {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.Stream != nil {
		where = append(where, "stream = ?")
		args = append(args, boolToInt(*f.Stream))
	}
	clause := strings.Join(where, " AND ")

	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	switch {
	case limit <= 0:
		limit = DefaultRequestLimit
	case limit > MaxRequestLimit:
		limit = MaxRequestLimit
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT ts, project, session, provider, requested_model, upstream_model,
       input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens,
       status, duration_ms, stream, cost_usd, has_cost
FROM usage WHERE `+clause+`
ORDER BY ts DESC
LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	records := make([]domain.UsageRow, 0, 64)
	for rows.Next() {
		var (
			row     domain.UsageRow
			ts      int64
			stream  int
			hasCost int
		)
		if err := rows.Scan(
			&ts,
			&row.Project,
			&row.Session,
			&row.Provider,
			&row.RequestedModel,
			&row.UpstreamModel,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CacheReadTokens,
			&row.CacheCreationTokens,
			&row.Status,
			&row.DurationMs,
			&stream,
			&row.CostUSD,
			&hasCost,
		); err != nil {
			return nil, 0, err
		}
		row.Time = time.UnixMilli(ts)
		row.Stream = stream != 0
		row.HasCost = hasCost != 0
		records = append(records, row)
	}
	return records, total, rows.Err()
}

// queryAll runs a query and scans every row with scan.
func queryAll[T any](ctx context.Context, db *sql.DB, query string, args []any, scan func(*sql.Rows) (T, error)) ([]T, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]T, 0, 16)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
