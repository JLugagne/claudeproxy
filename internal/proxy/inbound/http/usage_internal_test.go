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

func TestUsageParserMarksFirstTokenOnContentDelta(t *testing.T) {
	p := newUsageParser(true)
	p.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10}}}\n\n"))
	p.Write([]byte("event: ping\ndata: {\"type\":\"ping\"}\n\n"))
	if p.firstToken {
		t.Fatalf("first token marked before any content delta")
	}
	p.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n"))
	if !p.firstToken {
		t.Fatalf("first token not marked on content_block_delta")
	}
}

func TestUsageParserMarksFirstTokenOnJSONBody(t *testing.T) {
	p := newUsageParser(false)
	if p.firstToken {
		t.Fatalf("first token marked before the body")
	}
	p.Write([]byte(`{"usage":{"output_tokens":1}}`))
	if !p.firstToken {
		t.Fatalf("first token not marked on the first body byte")
	}
}
