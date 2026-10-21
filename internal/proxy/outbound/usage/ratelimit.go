package usage

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

const headerPrefix = "anthropic-ratelimit-unified-"

// RateLimits returns the quota snapshots recorded in the range, oldest first.
// Rows that carry no subscription window figure, such as the headers of an
// API-key account, are skipped.
func (s *Store) RateLimits(ctx context.Context, r domain.MetricsRange) ([]domain.RateLimitSnapshot, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ts, project, session, headers FROM ratelimit WHERE ts >= ? AND ts < ? ORDER BY ts`,
		r.Since.UnixMilli(), r.Until.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.RateLimitSnapshot
	for rows.Next() {
		var (
			ts                         int64
			project, session, rawValue string
		)
		if err := rows.Scan(&ts, &project, &session, &rawValue); err != nil {
			return nil, err
		}
		var headers map[string]string
		if json.Unmarshal([]byte(rawValue), &headers) != nil {
			continue
		}
		snapshot := domain.RateLimitSnapshot{
			Time:        time.UnixMilli(ts),
			Attribution: domain.Attribution{Project: project, Session: session},
			FiveHour:    quotaWindow(headers, "5h"),
			SevenDay:    quotaWindow(headers, "7d"),
			Claim:       headers[headerPrefix+"representative-claim"],
		}
		if snapshot.FiveHour.Present || snapshot.SevenDay.Present {
			out = append(out, snapshot)
		}
	}
	return out, rows.Err()
}

func quotaWindow(headers map[string]string, window string) domain.QuotaWindow {
	utilization, err := strconv.ParseFloat(headers[headerPrefix+window+"-utilization"], 64)
	if err != nil {
		return domain.QuotaWindow{}
	}
	quota := domain.QuotaWindow{
		Utilization: utilization,
		Status:      headers[headerPrefix+window+"-status"],
		Present:     true,
	}
	if reset, err := strconv.ParseInt(headers[headerPrefix+window+"-reset"], 10, 64); err == nil {
		quota.ResetsAt = time.Unix(reset, 0)
	}
	return quota
}
