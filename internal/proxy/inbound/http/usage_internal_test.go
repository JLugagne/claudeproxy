package http

import "testing"

func TestUsageParserJSON(t *testing.T) {
	p := newUsageParser(false)
	p.Write([]byte(`{"usage":{"input_tokens":12,"output_tokens":34,"cache_read_input_tokens":5,"cache_creation_input_tokens":6}}`))
	got := p.usage()
	if got.InputTokens != 12 || got.OutputTokens != 34 || got.CacheReadInputTokens != 5 || got.CacheCreationInputTokens != 6 {
		t.Fatalf("usage %+v", got)
	}
}

func TestUsageParserSSE(t *testing.T) {
	p := newUsageParser(true)
	p.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10,\"cache_read_input_tokens\":3}}}\n\n"))
	p.Write([]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":99}}\n\n"))

	got := p.usage()
	if got.InputTokens != 10 || got.CacheReadInputTokens != 3 || got.OutputTokens != 99 {
		t.Fatalf("usage %+v", got)
	}
}

func TestUsageParserSplitsChunks(t *testing.T) {
	p := newUsageParser(true)
	for _, chunk := range []string{"event: message_del", "ta\ndata: {\"us", "age\":{\"output_tokens\":7}}\n\n"} {
		p.Write([]byte(chunk))
	}
	if got := p.usage(); got.OutputTokens != 7 {
		t.Fatalf("usage %+v", got)
	}
}
