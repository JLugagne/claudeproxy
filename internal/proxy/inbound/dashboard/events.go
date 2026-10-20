package dashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

// heartbeatInterval is how often an idle stream sends a comment line, so
// intermediaries do not treat the connection as dead.
const heartbeatInterval = 25 * time.Second

// events streams recorded usage to the console as Server-Sent Events. It ends
// when the client goes away, the feed is closed, or a write fails.
func (h *handler) events(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Feed == nil {
		writeJSON(w, http.StatusNotFound, errorDTO{Error: "live events are unavailable"})
		return
	}

	events, cancel := h.cfg.Feed.Subscribe()
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	if _, err := io.WriteString(w, ": connected\n\n"); err != nil {
		return
	}
	if err := rc.Flush(); err != nil {
		return
	}

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case usage, ok := <-events:
			if !ok {
				return
			}
			data, err := json.Marshal(newRequestFromUsage(usage))
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "event: request\ndata: %s\n\n", data); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}

// newRequestFromUsage maps a live usage onto the same shape as the requests
// log, so the console renders both identically.
func newRequestFromUsage(u domain.Usage) requestDTO {
	return requestDTO{
		Time:                u.Time,
		Project:             u.Project,
		Session:             u.Session,
		Provider:            string(u.Provider),
		RequestedModel:      string(u.RequestedModel),
		UpstreamModel:       string(u.UpstreamModel),
		InputTokens:         u.InputTokens,
		OutputTokens:        u.OutputTokens,
		CacheReadTokens:     u.CacheReadInputTokens,
		CacheCreationTokens: u.CacheCreationInputTokens,
		Status:              u.Status,
		DurationMs:          u.Duration.Milliseconds(),
		TTFTMs:              u.TimeToFirstToken.Milliseconds(),
		Stream:              u.Stream,
		CostUSD:             u.CostUSD,
		HasCost:             u.HasCost,
	}
}
