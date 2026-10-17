package domain

import "time"

// Usage is the consumption of one request, attributed to a project and a
// session. It is observability data: routing never depends on it.
type Usage struct {
	Time time.Time
	Attribution
	Provider                 ProviderName
	RequestedModel           Model
	UpstreamModel            Model
	InputTokens              int64
	OutputTokens             int64
	CacheReadInputTokens     int64
	CacheCreationInputTokens int64
	Status                   int
	Duration                 time.Duration
	Stream                   bool
}
