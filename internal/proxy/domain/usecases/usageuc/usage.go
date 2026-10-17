// Package usageuc is the use case that records request usage. It is the only
// thing the inbound transport knows about metrics.
package usageuc

import "github.com/JLugagne/claudeproxy/internal/proxy/domain"

// Recorder accepts one request's usage. Implementations must not block the
// caller: recording is best-effort observability.
type Recorder interface {
	Record(usage domain.Usage)
}

// Nop is a Recorder that discards everything, used when metrics are disabled.
type Nop struct{}

// Record implements Recorder.
func (Nop) Record(domain.Usage) {}
