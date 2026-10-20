// Package usageuc is the use case that records request usage. It is the only
// thing the inbound transport knows about metrics.
package usageuc

import "github.com/JLugagne/claudeproxy/internal/proxy/domain"

// Recorder accepts one request's usage. Implementations must not block the
// caller: recording is best-effort observability.
type Recorder interface {
	Record(usage domain.Usage)
}

// Publisher receives each usage once it is persisted. Implementations must not
// block the caller. Events published while nobody is subscribed are dropped:
// the feed is live-only, with no buffering and no backlog.
type Publisher interface {
	Publish(usage domain.Usage)
}

// Feed lets the console follow usage live. Subscribe returns a channel of usages
// and a cancel function that unsubscribes; cancel is safe to call more than
// once. The channel is closed when the feed shuts down. Events published while
// nobody is subscribed are dropped.
type Feed interface {
	Subscribe() (events <-chan domain.Usage, cancel func())
}

// Nop is a Recorder that discards everything, used when metrics are disabled.
type Nop struct{}

// Record implements Recorder.
func (Nop) Record(domain.Usage) {}
