package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// anthropicUsage mirrors the usage object of the Messages API.
type anthropicUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	// Cost is the upstream-reported price in USD, when the provider sends one
	// (OpenRouter does). It is a pointer so zero and absent stay distinct.
	Cost      *float64 `json:"cost"`
	TotalCost *float64 `json:"total_cost"`
}

// maxBufferedBodyBytes bounds what the parser keeps in memory.
const maxBufferedBodyBytes = 1 << 20

// usageRecorder wraps a statusRecorder, teeing the response body into a parser
// so the request's usage can be reported once it completes. It delegates Unwrap
// so the reverse proxy still sees the underlying Flusher.
type usageRecorder struct {
	*statusRecorder
	parser       *usageParser
	firstTokenAt time.Time
}

func newUsageRecorder(w http.ResponseWriter) *usageRecorder {
	return &usageRecorder{statusRecorder: &statusRecorder{ResponseWriter: w, status: http.StatusOK}}
}

func (u *usageRecorder) WriteHeader(status int) {
	u.statusRecorder.WriteHeader(status)
	u.ensureParser()
}

func (u *usageRecorder) Write(p []byte) (int, error) {
	u.ensureParser()
	u.parser.Write(p)
	if u.firstTokenAt.IsZero() && u.parser.firstToken {
		u.firstTokenAt = time.Now()
	}
	return u.statusRecorder.Write(p)
}

func (u *usageRecorder) ensureParser() {
	if u.parser == nil {
		u.parser = newUsageParser(strings.Contains(u.Header().Get("Content-Type"), "text/event-stream"))
	}
}

func (u *usageRecorder) parsed() anthropicUsage {
	if u.parser == nil {
		return anthropicUsage{}
	}
	return u.parser.usage()
}

func (u *usageRecorder) streaming() bool {
	return u.parser != nil && u.parser.stream
}

// usageParser extracts the Anthropic usage counters from a response body, in
// either the single-JSON form or the SSE stream form.
type usageParser struct {
	stream bool
	buf    []byte
	totals anthropicUsage
	// firstToken reports that generated content was seen: a content delta in a
	// stream, any byte of a single JSON body.
	firstToken bool
}

func newUsageParser(stream bool) *usageParser {
	return &usageParser{stream: stream}
}

func (p *usageParser) Write(chunk []byte) {
	if p.stream {
		p.writeStream(chunk)
		return
	}
	if len(chunk) > 0 {
		p.firstToken = true
	}
	if len(p.buf) < maxBufferedBodyBytes {
		p.buf = append(p.buf, chunk...)
	}
}

func (p *usageParser) usage() anthropicUsage {
	if p.stream {
		return p.totals
	}
	var body struct {
		Usage *anthropicUsage `json:"usage"`
	}
	if json.Unmarshal(p.buf, &body) == nil && body.Usage != nil {
		return *body.Usage
	}
	return anthropicUsage{}
}

func (p *usageParser) writeStream(chunk []byte) {
	p.buf = append(p.buf, chunk...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			break
		}
		line := p.buf[:i]
		p.buf = p.buf[i+1:]
		p.mergeEvent(line)
	}
	if len(p.buf) > maxBufferedBodyBytes {
		p.buf = p.buf[:0]
	}
}

func (p *usageParser) mergeEvent(line []byte) {
	data, ok := bytes.CutPrefix(line, []byte("data:"))
	if !ok {
		return
	}
	var event struct {
		Type    string          `json:"type"`
		Usage   *anthropicUsage `json:"usage"`
		Message *struct {
			Usage *anthropicUsage `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(bytes.TrimSpace(data), &event) != nil {
		return
	}
	if event.Type == "content_block_delta" {
		p.firstToken = true
	}
	if event.Message != nil && event.Message.Usage != nil {
		p.merge(*event.Message.Usage)
	}
	if event.Usage != nil {
		p.merge(*event.Usage)
	}
}

func (p *usageParser) merge(u anthropicUsage) {
	if u.InputTokens != 0 {
		p.totals.InputTokens = u.InputTokens
	}
	if u.OutputTokens != 0 {
		p.totals.OutputTokens = u.OutputTokens
	}
	if u.CacheReadInputTokens != 0 {
		p.totals.CacheReadInputTokens = u.CacheReadInputTokens
	}
	if cost := u.cost(); cost != nil {
		p.totals.Cost = cost
		p.totals.TotalCost = nil
	}
	if u.CacheCreationInputTokens != 0 {
		p.totals.CacheCreationInputTokens = u.CacheCreationInputTokens
	}
}

// cost returns the upstream-reported price in USD, when the provider sent one.
func (u anthropicUsage) cost() *float64 {
	if u.Cost != nil {
		return u.Cost
	}
	return u.TotalCost
}

func costValue(cost *float64) float64 {
	if cost == nil {
		return 0
	}
	return *cost
}

// timeToFirstToken is the delay between start and the first generated content,
// or zero when none was written.
func (u *usageRecorder) timeToFirstToken(start time.Time) time.Duration {
	if u.firstTokenAt.IsZero() {
		return 0
	}
	return u.firstTokenAt.Sub(start)
}

const rateLimitHeaderPrefix = "anthropic-ratelimit-"

func rateLimitHeaders(header http.Header) map[string]string {
	var limits map[string]string
	for name, values := range header {
		lower := strings.ToLower(name)
		if len(values) == 0 || !strings.HasPrefix(lower, rateLimitHeaderPrefix) {
			continue
		}
		if limits == nil {
			limits = make(map[string]string)
		}
		limits[lower] = values[0]
	}
	return limits
}
