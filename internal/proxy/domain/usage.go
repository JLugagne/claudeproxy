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
	// TimeToFirstToken is the delay until the first generated content reached the
	// client: the first content delta of a stream, or the first body byte of a
	// non-streaming response. Zero means it was not measured, as on errors.
	TimeToFirstToken time.Duration
	Stream           bool
	// CostUSD is the upstream-reported price in USD, when the provider sent one.
	// HasCost distinguishes a free request from one the provider did not price.
	CostUSD float64
	HasCost bool
}
