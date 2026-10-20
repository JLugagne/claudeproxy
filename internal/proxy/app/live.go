package app

import (
	"sync"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
	"github.com/JLugagne/claudeproxy/internal/proxy/domain/usecases/usageuc"
)

// subscriberBuffer is how many events a subscriber may lag behind. A slow
// listener must never block the proxy or the database writer, so once its
// buffer is full its events are dropped.
const subscriberBuffer = 64

// UsageHub broadcasts persisted usage to live dashboard listeners. It is safe
// for concurrent use. Publishing never blocks: with no subscribers, or with a
// subscriber that cannot keep up, the event is dropped.
type UsageHub struct {
	mu     sync.Mutex
	subs   map[chan domain.Usage]struct{}
	closed bool
}

var (
	_ usageuc.Publisher = (*UsageHub)(nil)
	_ usageuc.Feed      = (*UsageHub)(nil)
)

// NewUsageHub returns an empty, open hub.
func NewUsageHub() *UsageHub {
	return &UsageHub{subs: make(map[chan domain.Usage]struct{})}
}

// Publish implements usageuc.Publisher. It never blocks: each subscriber gets
// the usage only if its buffer has room, otherwise the event is dropped for
// that subscriber alone.
func (h *UsageHub) Publish(usage domain.Usage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- usage:
		default:
		}
	}
}

// Subscribe implements usageuc.Feed. The returned cancel unsubscribes and
// closes the channel; it is safe to call more than once. After Close, Subscribe
// returns an already-closed channel.
func (h *UsageHub) Subscribe() (<-chan domain.Usage, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		ch := make(chan domain.Usage)
		close(ch)
		return ch, func() {}
	}
	ch := make(chan domain.Usage, subscriberBuffer)
	h.subs[ch] = struct{}{}
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if _, ok := h.subs[ch]; !ok {
				return
			}
			delete(h.subs, ch)
			close(ch)
		})
	}
	return ch, cancel
}

// Close ends the feed: every subscriber channel is closed and later Subscribe
// calls return a closed channel. It is idempotent.
func (h *UsageHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for ch := range h.subs {
		close(ch)
		delete(h.subs, ch)
	}
}
