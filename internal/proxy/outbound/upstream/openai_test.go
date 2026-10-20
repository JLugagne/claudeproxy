package upstream_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/JLugagne/claudeproxy/internal/proxy/outbound/upstream"
)

func newOpenAIProxy(t *testing.T, serverURL string) *httptest.Server {
	t.Helper()
	return newProxy(t, upstream.Config{
		Name:    "opencode-go-openai",
		BaseURL: mustURL(t, serverURL+"/openai"),
		Auth:    upstream.AuthBearer,
		APIKey:  "sk-oc",
		Format:  upstream.FormatOpenAIChat,
	})
}

func newOpenAIServer(t *testing.T, respond http.HandlerFunc) (*httptest.Server, <-chan captured) {
	t.Helper()
	requests := make(chan captured, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- captured{path: r.URL.Path, query: r.URL.RawQuery, header: r.Header.Clone(), body: string(body)}
		respond(w, r)
	}))
	t.Cleanup(server.Close)
	return server, requests
}

func postMessages(t *testing.T, proxyURL, path, body string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, proxyURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return do(t, req)
}

func mustJSON(t *testing.T, raw string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("invalid JSON %q: %v", raw, err)
	}
	return v
}

func assertJSONEqual(t *testing.T, got, want string) {
	t.Helper()
	if g, w := mustJSON(t, got), mustJSON(t, want); !reflect.DeepEqual(g, w) {
		t.Fatalf("JSON mismatch\n got: %s\nwant: %s", got, want)
	}
}

const anthropicConversation = `{
  "model": "deepseek-v4.1-flash",
  "max_tokens": 1024,
  "temperature": 0.5,
  "stop_sequences": ["END"],
  "system": [{"type": "text", "text": "You are helpful."}, {"type": "text", "text": "Be brief."}],
  "thinking": {"type": "enabled", "budget_tokens": 1000},
  "metadata": {"user_id": "x"},
  "tools": [{"name": "get_weather", "description": "Weather", "input_schema": {"type": "object", "properties": {"city": {"type": "string"}}}}],
  "tool_choice": {"type": "auto"},
  "messages": [
    {"role": "user", "content": "Weather in Paris?"},
    {"role": "assistant", "content": [
      {"type": "thinking", "thinking": "hmm", "signature": "sig"},
      {"type": "text", "text": "Checking."},
      {"type": "tool_use", "id": "toolu_1", "name": "get_weather", "input": {"city": "Paris"}}
    ]},
    {"role": "user", "content": [
      {"type": "tool_result", "tool_use_id": "toolu_1", "content": [{"type": "text", "text": "Sunny"}]},
      {"type": "text", "text": "Thanks"}
    ]}
  ]
}`

func TestOpenAIChatTranslatesRequest(t *testing.T) {
	server, requests := newOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	})
	proxy := newOpenAIProxy(t, server.URL)

	resp := postMessages(t, proxy.URL, "/v1/messages?beta=true", anthropicConversation, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	got := <-requests
	if got.path != "/openai/v1/chat/completions" {
		t.Errorf("path = %q, want /openai/v1/chat/completions", got.path)
	}
	if got.query != "" {
		t.Errorf("query = %q, want none", got.query)
	}
	if got.header.Get("Authorization") != "Bearer sk-oc" {
		t.Errorf("Authorization = %q", got.header.Get("Authorization"))
	}
	assertJSONEqual(t, got.body, `{
	  "model": "deepseek-v4.1-flash",
	  "max_tokens": 1024,
	  "temperature": 0.5,
	  "stop": ["END"],
	  "tool_choice": "auto",
	  "tools": [{"type": "function", "function": {"name": "get_weather", "description": "Weather", "parameters": {"type": "object", "properties": {"city": {"type": "string"}}}}}],
	  "messages": [
	    {"role": "system", "content": "You are helpful.\n\nBe brief."},
	    {"role": "user", "content": "Weather in Paris?"},
	    {"role": "assistant", "content": "Checking.", "reasoning_content": "hmm", "tool_calls": [{"id": "toolu_1", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"Paris\"}"}}]},
	    {"role": "tool", "tool_call_id": "toolu_1", "content": "Sunny"},
	    {"role": "user", "content": "Thanks"}
	  ]
	}`)
}

func TestOpenAIChatRequestHeaders(t *testing.T) {
	server, requests := newOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	})
	proxy := newOpenAIProxy(t, server.URL)

	postMessages(t, proxy.URL, "/v1/messages", `{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`, map[string]string{
		"Accept-Encoding":          "gzip, br",
		"Anthropic-Version":        "2023-06-01",
		"X-Claude-Code-Session-Id": "sess-1",
	})

	got := <-requests
	if v := got.header.Get("Accept-Encoding"); v != "" {
		t.Errorf("Accept-Encoding = %q, want it removed so the body can be translated", v)
	}
	if v := got.header.Get("Anthropic-Version"); v != "" {
		t.Errorf("Anthropic-Version = %q, want it removed", v)
	}
	if v := got.header.Get("X-Opencode-Session"); v != "sess-1" {
		t.Errorf("X-Opencode-Session = %q, want sess-1", v)
	}
	if v := got.header.Get("X-Claude-Code-Session-Id"); v != "" {
		t.Errorf("X-Claude-Code-Session-Id = %q, want it removed", v)
	}
	if v := got.header.Get("Content-Type"); v != "application/json" {
		t.Errorf("Content-Type = %q", v)
	}
}

func TestOpenAIChatTranslatesResponse(t *testing.T) {
	server, _ := newOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","model":"deepseek-v4.1-flash","choices":[{"index":0,"message":{"role":"assistant","content":"Hi","reasoning_content":"thinking","tool_calls":[{"id":"call_1","type":"function","function":{"name":"f","arguments":"{\"a\":1}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":4}}}`))
	})
	proxy := newOpenAIProxy(t, server.URL)

	resp := postMessages(t, proxy.URL, "/v1/messages", `{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`, nil)
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not JSON: %s", body)
	}
	if id, _ := got["id"].(string); id == "" {
		t.Error("id is empty")
	}
	delete(got, "id")
	want := `{
	  "type": "message", "role": "assistant", "model": "deepseek-v4.1-flash",
	  "content": [{"type": "thinking", "thinking": "thinking", "signature": "claudeproxy"}, {"type": "text", "text": "Hi"}, {"type": "tool_use", "id": "call_1", "name": "f", "input": {"a": 1}}],
	  "stop_reason": "tool_use", "stop_sequence": null,
	  "usage": {"input_tokens": 6, "output_tokens": 3, "cache_read_input_tokens": 4}
	}`
	gotJSON, _ := json.Marshal(got)
	assertJSONEqual(t, string(gotJSON), want)
}

func TestOpenAIChatMapsFinishReasons(t *testing.T) {
	for finish, want := range map[string]string{"stop": "end_turn", "length": "max_tokens", "tool_calls": "tool_use", "content_filter": "end_turn"} {
		t.Run(finish, func(t *testing.T) {
			server, _ := newOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"x"},"finish_reason":"` + finish + `"}]}`))
			})
			proxy := newOpenAIProxy(t, server.URL)
			resp := postMessages(t, proxy.URL, "/v1/messages", `{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`, nil)
			var got struct {
				StopReason string `json:"stop_reason"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&got)
			if got.StopReason != want {
				t.Errorf("stop_reason = %q, want %q", got.StopReason, want)
			}
		})
	}
}

type sseEvent struct {
	name string
	data map[string]any
}

func readSSE(t *testing.T, body io.Reader) []sseEvent {
	t.Helper()
	var events []sseEvent
	var name string
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 1<<16), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			var data map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &data); err != nil {
				t.Fatalf("invalid SSE data %q: %v", line, err)
			}
			if typ, _ := data["type"].(string); typ != name {
				t.Errorf("event %q carries type %q", name, typ)
			}
			events = append(events, sseEvent{name: name, data: data})
		}
	}
	return events
}

func TestOpenAIChatTranslatesStream(t *testing.T) {
	chunks := []string{
		`{"id":"c1","model":"deepseek-v4.1-flash","choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"}}]}`,
		`{"id":"c1","choices":[{"index":0,"delta":{"content":"lo"}}]}`,
		`{"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`{"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`,
		`{"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}}]}}]}`,
		`{"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`{"id":"c1","choices":[],"usage":{"prompt_tokens":20,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":5}}}`,
	}
	server, requests := newOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range chunks {
			_, _ = io.WriteString(w, "data: "+chunk+"\n\n")
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	proxy := newOpenAIProxy(t, server.URL)

	resp := postMessages(t, proxy.URL, "/v1/messages", `{"model":"m","max_tokens":8,"stream":true,"messages":[{"role":"user","content":"hi"}]}`, nil)

	sent := <-requests
	var upstreamBody map[string]any
	_ = json.Unmarshal([]byte(sent.body), &upstreamBody)
	if upstreamBody["stream"] != true {
		t.Errorf("upstream stream = %v, want true", upstreamBody["stream"])
	}
	if !reflect.DeepEqual(upstreamBody["stream_options"], map[string]any{"include_usage": true}) {
		t.Errorf("stream_options = %v, want include_usage", upstreamBody["stream_options"])
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q", ct)
	}

	events := readSSE(t, resp.Body)
	var names []string
	for _, e := range events {
		names = append(names, e.name)
	}
	wantNames := []string{
		"message_start",
		"content_block_start", "content_block_delta", "content_block_delta", "content_block_stop",
		"content_block_start", "content_block_delta", "content_block_delta", "content_block_stop",
		"message_delta", "message_stop",
	}
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("events = %v\nwant     %v", names, wantNames)
	}

	if model := events[0].data["message"].(map[string]any)["model"]; model != "deepseek-v4.1-flash" {
		t.Errorf("message_start model = %v", model)
	}
	var text, partial strings.Builder
	for _, e := range events {
		delta, ok := e.data["delta"].(map[string]any)
		if !ok || e.name != "content_block_delta" {
			continue
		}
		switch delta["type"] {
		case "text_delta":
			text.WriteString(delta["text"].(string))
		case "input_json_delta":
			partial.WriteString(delta["partial_json"].(string))
		}
	}
	if text.String() != "Hello" {
		t.Errorf("text = %q, want Hello", text.String())
	}
	if partial.String() != `{"city":"Paris"}` {
		t.Errorf("tool arguments = %q", partial.String())
	}
	toolStart := events[5].data
	if toolStart["index"] != float64(1) {
		t.Errorf("tool block index = %v, want 1", toolStart["index"])
	}
	block := toolStart["content_block"].(map[string]any)
	if block["type"] != "tool_use" || block["id"] != "call_1" || block["name"] != "get_weather" {
		t.Errorf("tool block = %v", block)
	}

	final := events[9].data
	if got := final["delta"].(map[string]any)["stop_reason"]; got != "tool_use" {
		t.Errorf("stop_reason = %v, want tool_use", got)
	}
	usage := final["usage"].(map[string]any)
	if usage["input_tokens"] != float64(15) || usage["output_tokens"] != float64(7) || usage["cache_read_input_tokens"] != float64(5) {
		t.Errorf("usage = %v", usage)
	}
}

func TestOpenAIChatStreamWithoutDoneStillCloses(t *testing.T) {
	server, _ := newOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"},\"finish_reason\":\"stop\"}]}\n\n")
	})
	proxy := newOpenAIProxy(t, server.URL)

	resp := postMessages(t, proxy.URL, "/v1/messages", `{"model":"m","max_tokens":8,"stream":true,"messages":[{"role":"user","content":"hi"}]}`, nil)
	events := readSSE(t, resp.Body)

	if len(events) == 0 || events[len(events)-1].name != "message_stop" {
		t.Fatalf("stream must end with message_stop, got %v", events)
	}
}

func TestOpenAIChatTranslatesErrors(t *testing.T) {
	for status, wantType := range map[int]string{
		http.StatusBadRequest:      "invalid_request_error",
		http.StatusUnauthorized:    "authentication_error",
		http.StatusTooManyRequests: "rate_limit_error",
		http.StatusBadGateway:      "api_error",
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server, _ := newOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":{"message":"nope","type":"whatever","code":"x"}}`))
			})
			proxy := newOpenAIProxy(t, server.URL)

			resp := postMessages(t, proxy.URL, "/v1/messages", `{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`, nil)
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, status)
			}
			assertJSONEqual(t, string(body), `{"type":"error","error":{"type":"`+wantType+`","message":"nope"}}`)
		})
	}
}

func TestOpenAIChatRejectsInvalidRequest(t *testing.T) {
	server, requests := newOpenAIServer(t, func(http.ResponseWriter, *http.Request) {})
	proxy := newOpenAIProxy(t, server.URL)

	resp := postMessages(t, proxy.URL, "/v1/messages", `not json`, nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	select {
	case got := <-requests:
		t.Errorf("an invalid request reached the provider: %+v", got)
	default:
	}
}

func TestOpenAIChatAnswersCountTokensLocally(t *testing.T) {
	server, requests := newOpenAIServer(t, func(http.ResponseWriter, *http.Request) {})
	proxy := newOpenAIProxy(t, server.URL)

	resp := postMessages(t, proxy.URL, "/v1/messages/count_tokens", anthropicConversation, nil)
	var got struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != http.StatusOK || got.InputTokens <= 0 {
		t.Errorf("status = %d, input_tokens = %d", resp.StatusCode, got.InputTokens)
	}
	select {
	case req := <-requests:
		t.Errorf("count_tokens reached the provider: %+v", req)
	default:
	}
}

func TestOpenAIChatTranslatesImagesAndToolChoice(t *testing.T) {
	server, requests := newOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	})
	proxy := newOpenAIProxy(t, server.URL)

	postMessages(t, proxy.URL, "/v1/messages", `{
	  "model": "m", "max_tokens": 8,
	  "system": "sys",
	  "tool_choice": {"type": "tool", "name": "f"},
	  "tools": [{"name": "f", "input_schema": {"type": "object"}}],
	  "messages": [{"role": "user", "content": [
	    {"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "AAAA"}},
	    {"type": "text", "text": "what is this?"}
	  ]}]
	}`, nil)

	assertJSONEqual(t, (<-requests).body, `{
	  "model": "m", "max_tokens": 8,
	  "tool_choice": {"type": "function", "function": {"name": "f"}},
	  "tools": [{"type": "function", "function": {"name": "f", "parameters": {"type": "object"}}}],
	  "messages": [
	    {"role": "system", "content": "sys"},
	    {"role": "user", "content": [
	      {"type": "image_url", "image_url": {"url": "data:image/png;base64,AAAA"}},
	      {"type": "text", "text": "what is this?"}
	    ]}
	  ]
	}`)
}

func TestOpenAIChatLogsRejectedRequest(t *testing.T) {
	server, _ := newOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Opencode-Log-Id", "log-123")
		w.WriteHeader(http.StatusBadRequest)
	})
	var logs bytes.Buffer
	handler, err := upstream.New(upstream.Config{
		Name:    "opencode-go-openai",
		BaseURL: mustURL(t, server.URL+"/openai"),
		Auth:    upstream.AuthBearer,
		APIKey:  "sk-oc",
		Format:  upstream.FormatOpenAIChat,
	}, upstream.NewTransport(), slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	t.Cleanup(proxy.Close)

	resp := postMessages(t, proxy.URL, "/v1/messages", `{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"hello debug"}]}`, nil)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	for _, want := range []string{"chat-completions request rejected", "log-123", "hello debug"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("logs do not contain %q:\n%s", want, logs.String())
		}
	}
	if strings.Contains(logs.String(), "sk-oc") {
		t.Errorf("logs leak the API key:\n%s", logs.String())
	}
}

func TestOpenAIChatHoistsMidConversationSystemMessages(t *testing.T) {
	server, requests := newOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	})
	proxy := newOpenAIProxy(t, server.URL)

	postMessages(t, proxy.URL, "/v1/messages", `{
	  "model": "m", "max_tokens": 8,
	  "system": "base prompt",
	  "messages": [
	    {"role": "user", "content": "do the task"},
	    {"role": "system", "content": "# Environment\nPlatform: linux"},
	    {"role": "assistant", "content": "working"},
	    {"role": "system", "content": [{"type": "text", "text": "extra rules"}]}
	  ]
	}`, nil)

	assertJSONEqual(t, (<-requests).body, `{
	  "model": "m", "max_tokens": 8,
	  "messages": [
	    {"role": "system", "content": "base prompt\n\n# Environment\nPlatform: linux\n\nextra rules"},
	    {"role": "user", "content": "do the task"},
	    {"role": "assistant", "content": "working"}
	  ]
	}`)
}

func TestOpenAIChatMidConversationSystemWithoutSystemPrompt(t *testing.T) {
	server, requests := newOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	})
	proxy := newOpenAIProxy(t, server.URL)

	postMessages(t, proxy.URL, "/v1/messages", `{
	  "model": "m", "max_tokens": 8,
	  "messages": [{"role": "user", "content": "hi"}, {"role": "system", "content": "env"}]
	}`, nil)

	assertJSONEqual(t, (<-requests).body, `{
	  "model": "m", "max_tokens": 8,
	  "messages": [{"role": "system", "content": "env"}, {"role": "user", "content": "hi"}]
	}`)
}

func TestOpenAIChatDropsRegexPatternsFromToolSchemas(t *testing.T) {
	server, requests := newOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	})
	proxy := newOpenAIProxy(t, server.URL)

	postMessages(t, proxy.URL, "/v1/messages", `{
	  "model": "m", "max_tokens": 8,
	  "messages": [{"role": "user", "content": "hi"}],
	  "tools": [{"name": "Grep", "input_schema": {
	    "type": "object",
	    "required": ["pattern"],
	    "properties": {
	      "pattern": {"type": "string", "description": "the regex to search for"},
	      "path": {"type": "string", "pattern": "^[^\\0]*$"},
	      "items": {"type": "array", "items": {"type": "string", "pattern": "^[a-z]+$"}}
	    }
	  }}]
	}`, nil)

	assertJSONEqual(t, (<-requests).body, `{
	  "model": "m", "max_tokens": 8,
	  "messages": [{"role": "user", "content": "hi"}],
	  "tools": [{"type": "function", "function": {"name": "Grep", "parameters": {
	    "type": "object",
	    "required": ["pattern"],
	    "properties": {
	      "pattern": {"type": "string", "description": "the regex to search for"},
	      "path": {"type": "string"},
	      "items": {"type": "array", "items": {"type": "string"}}
	    }
	  }}}]
	}`)
}

// Reasoning deltas are relayed as a signed thinking block so the client keeps
// receiving traffic while the model thinks and can send the reasoning back.
func TestOpenAIChatStreamsReasoningAsThinking(t *testing.T) {
	chunks := []string{
		`{"id":"c1","model":"deepseek-v4.1-flash","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"Let"}}]}`,
		`{"id":"c1","choices":[{"index":0,"delta":{"reasoning_content":" me think"}}]}`,
		`{"id":"c1","choices":[{"index":0,"delta":{"content":"Hi"}}]}`,
		`{"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	}
	server, _ := newOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range chunks {
			_, _ = io.WriteString(w, "data: "+chunk+"\n\n")
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	proxy := newOpenAIProxy(t, server.URL)

	resp := postMessages(t, proxy.URL, "/v1/messages", `{"model":"m","max_tokens":8,"stream":true,"messages":[{"role":"user","content":"hi"}]}`, nil)

	events := readSSE(t, resp.Body)
	var names []string
	for _, e := range events {
		names = append(names, e.name)
	}
	wantNames := []string{
		"message_start",
		"content_block_start", "content_block_delta", "content_block_delta", "content_block_delta", "content_block_stop",
		"content_block_start", "content_block_delta", "content_block_stop",
		"message_delta", "message_stop",
	}
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("events = %v\nwant     %v", names, wantNames)
	}

	thinkingStart := events[1].data
	if thinkingStart["index"] != float64(0) {
		t.Errorf("thinking block index = %v, want 0", thinkingStart["index"])
	}
	if block := thinkingStart["content_block"].(map[string]any); block["type"] != "thinking" {
		t.Errorf("first block = %v, want a thinking block", block)
	}
	var thinking strings.Builder
	for _, e := range events[2:4] {
		delta := e.data["delta"].(map[string]any)
		if delta["type"] != "thinking_delta" {
			t.Fatalf("delta = %v, want thinking_delta", delta)
		}
		thinking.WriteString(delta["thinking"].(string))
	}
	if thinking.String() != "Let me think" {
		t.Errorf("thinking = %q, want %q", thinking.String(), "Let me think")
	}
	if signature := events[4].data["delta"].(map[string]any); signature["type"] != "signature_delta" || signature["signature"] != "claudeproxy" {
		t.Errorf("signature delta = %v", signature)
	}

	if textStart := events[6].data; textStart["index"] != float64(1) || textStart["content_block"].(map[string]any)["type"] != "text" {
		t.Errorf("second block = %v, want text at index 1", textStart)
	}
	if delta := events[7].data["delta"].(map[string]any); delta["text"] != "Hi" {
		t.Errorf("text delta = %v", delta)
	}
}
