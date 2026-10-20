package dashboard

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JLugagne/claudeproxy/internal/proxy/domain"
)

// fakeFeed exposes a channel the test controls, so it can push usages and end
// the stream deterministically.
type fakeFeed struct {
	events chan domain.Usage
}

func (f *fakeFeed) Subscribe() (<-chan domain.Usage, func()) {
	return f.events, func() {}
}

func TestEventsStreamsUsage(t *testing.T) {
	feed := &fakeFeed{events: make(chan domain.Usage, 4)}
	handler := NewHandler(Config{Queries: &fakeQueries{}, Feed: feed, Now: fixedNow})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := server.Client()
	client.Timeout = 5 * time.Second

	resp, err := client.Get(server.URL + Prefix + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("content type = %q, want text/event-stream", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("cache control = %q, want no-store", got)
	}
	if got := resp.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("x-accel-buffering = %q, want no", got)
	}

	reader := bufio.NewReader(resp.Body)
	if line, err := reader.ReadString('\n'); err != nil || line != ": connected\n" {
		t.Fatalf("connected comment = %q, %v", line, err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "\n" {
		t.Fatalf("connected blank line = %q, %v", line, err)
	}

	feed.events <- domain.Usage{
		Time:             fixedNow(),
		Attribution:      domain.Attribution{Project: "acme/gateway", Session: "s1"},
		Provider:         "anthropic",
		RequestedModel:   "claude-x",
		UpstreamModel:    "claude-x",
		Status:           200,
		Duration:         1500 * time.Millisecond,
		TimeToFirstToken: 300 * time.Millisecond,
	}

	if line, err := reader.ReadString('\n'); err != nil || line != "event: request\n" {
		t.Fatalf("event line = %q, %v", line, err)
	}
	data, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("data line: %v", err)
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(data, "data: "), "\n")
	var row requestDTO
	if err := json.Unmarshal([]byte(payload), &row); err != nil {
		t.Fatalf("decode %q: %v", payload, err)
	}
	if row.Project != "acme/gateway" || row.Status != 200 || row.TTFTMs != 300 {
		t.Errorf("row = %+v, want project acme/gateway, status 200, ttft 300", row)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "\n" {
		t.Fatalf("frame blank line = %q, %v", line, err)
	}

	// Closing the feed channel ends the response.
	close(feed.events)
	if _, err := reader.ReadString('\n'); err != io.EOF {
		t.Fatalf("stream did not end after feed close: %v", err)
	}
}

func TestEventsUnavailableWithoutFeed(t *testing.T) {
	handler := NewHandler(Config{Queries: &fakeQueries{}, Now: fixedNow})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Prefix+"/api/events", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body errorDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "live events are unavailable" {
		t.Errorf("error = %q", body.Error)
	}
}
