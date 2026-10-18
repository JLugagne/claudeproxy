package domain

import "time"

// MetricsRange is the window and bucket size a dashboard query reads. Since
// and Until are aligned on Bucket boundaries, so a bucketed series holds
// exactly (Until-Since)/Bucket points.
type MetricsRange struct {
	Since  time.Time
	Until  time.Time
	Bucket time.Duration
}

// MetricsTotals is the aggregate of every request in a range.
type MetricsTotals struct {
	Requests            int64
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	Errors              int64
	AvgLatencyMs        float64
	P95LatencyMs        int64
	// CostUSD is the sum of the prices providers reported; PricedRequests says
	// how many requests carried one.
	CostUSD        float64
	PricedRequests int64
}

// Tokens is the billable token count: input plus output.
func (t MetricsTotals) Tokens() int64 { return t.InputTokens + t.OutputTokens }

// CacheHitRate is the share of prompt tokens served from cache.
func (t MetricsTotals) CacheHitRate() float64 {
	denominator := t.CacheReadTokens + t.CacheCreationTokens + t.InputTokens
	if denominator == 0 {
		return 0
	}
	return float64(t.CacheReadTokens) / float64(denominator)
}

// MetricsPoint is one bucket of the volume chart.
type MetricsPoint struct {
	Start        time.Time
	Requests     int64
	InputTokens  int64
	OutputTokens int64
}

// MetricsBar is one entry of a top-N breakdown.
type MetricsBar struct {
	Name     string
	Requests int64
	Tokens   int64
}

// ProjectStat is the aggregate of one git project.
type ProjectStat struct {
	Project             string
	Sessions            int64
	Requests            int64
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	AvgLatencyMs        float64
	CostUSD             float64
}

// Tokens is the project's billable token count.
func (p ProjectStat) Tokens() int64 { return p.InputTokens + p.OutputTokens }

// CacheHitRate is the project's prompt-token cache share.
func (p ProjectStat) CacheHitRate() float64 {
	denominator := p.CacheReadTokens + p.CacheCreationTokens + p.InputTokens
	if denominator == 0 {
		return 0
	}
	return float64(p.CacheReadTokens) / float64(denominator)
}

// ModelStat is the aggregate of one requested model on one provider.
type ModelStat struct {
	Model        string
	Provider     string
	Requests     int64
	Tokens       int64
	AvgLatencyMs float64
	Errors       int64
	CostUSD      float64
}

// SessionStat is the aggregate of one Claude Code session.
type SessionStat struct {
	Session  string
	Project  string
	Started  time.Time
	LastSeen time.Time
	Requests int64
	Tokens   int64
	TopModel string
	CostUSD  float64
}

// Duration is the span between the first and last request of the session.
func (s SessionStat) Duration() time.Duration { return s.LastSeen.Sub(s.Started) }

// ProviderStat is the aggregate of one upstream provider.
type ProviderStat struct {
	Provider     string
	Requests     int64
	Tokens       int64
	AvgLatencyMs float64
	Errors       int64
	CostUSD      float64
}

// UsageRow is one recorded request.
type UsageRow struct {
	Time                time.Time
	Project             string
	Session             string
	Provider            ProviderName
	RequestedModel      Model
	UpstreamModel       Model
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	Status              int
	DurationMs          int64
	Stream              bool
	CostUSD             float64
	HasCost             bool
}

// UsageFilter narrows the request log. Zero values mean "no filter".
type UsageFilter struct {
	Query  string
	Status int
	Stream *bool
	Limit  int
}
