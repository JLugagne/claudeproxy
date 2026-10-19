package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/JLugagne/claudeproxy/internal/pkg/apierror"
)

const (
	countTokensPath    = "/v1/messages/count_tokens"
	chatCompletionsURI = "/v1/chat/completions"
	claudeSessionHdr   = "X-Claude-Code-Session-Id"
	openCodeSessionHdr = "X-Opencode-Session"
)

// openAIChatTranslator lets a Messages API client talk to a chat-completions
// provider: it converts POST /v1/messages requests and their responses, and
// answers count_tokens locally with an estimate. Other requests are forwarded
// untouched.
func openAIChatTranslator(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Body == nil || r.Header.Get("Content-Encoding") != "" ||
			(r.URL.Path != messagesPathPrefix && r.URL.Path != countTokensPath) {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			apierror.Write(w, http.StatusBadRequest, apierror.TypeInvalidRequest, "claudeproxy: cannot read request body")
			return
		}
		if r.URL.Path == countTokensPath {
			writeTokenEstimate(w, body)
			return
		}
		converted, stream, err := messagesToChatRequest(body)
		if err != nil {
			apierror.Write(w, http.StatusBadRequest, apierror.TypeInvalidRequest, "claudeproxy: "+err.Error())
			return
		}

		out := r.Clone(r.Context())
		out.URL.Path, out.URL.RawPath, out.URL.RawQuery = chatCompletionsURI, "", ""
		out.Header.Set("Content-Type", "application/json")
		out.Header.Del("Anthropic-Version")
		// The response is translated, so it must not arrive compressed.
		out.Header.Del("Accept-Encoding")
		if out.Header.Get(openCodeSessionHdr) == "" {
			if session := r.Header.Get(claudeSessionHdr); session != "" {
				out.Header.Set(openCodeSessionHdr, session)
			}
		}
		out.Body = io.NopCloser(bytes.NewReader(converted))
		out.ContentLength = int64(len(converted))
		out.TransferEncoding = nil
		out.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(converted)), nil }

		translated := &chatResponseWriter{ResponseWriter: w, stream: stream, logger: logger, request: converted}
		next.ServeHTTP(translated, out)
		translated.finish()
	})
}

// writeTokenEstimate answers count_tokens without calling the provider, which
// has no such endpoint. Roughly four bytes make a token.
func writeTokenEstimate(w http.ResponseWriter, body []byte) {
	payload, _ := json.Marshal(map[string]int{"input_tokens": max(len(body)/4, 1)})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(payload)
}

type responseMode int

const (
	modeUndecided responseMode = iota
	modeJSON
	modeError
	modeStream
)

// chatResponseWriter translates the provider's response on its way to the
// client. Streams are converted event by event; JSON and error bodies are
// buffered and converted once the provider is done (see finish).
type chatResponseWriter struct {
	http.ResponseWriter
	stream bool
	mode   responseMode
	status int
	buf    bytes.Buffer
	events *streamTranslator
	logger *slog.Logger
	// request is the chat-completions body sent upstream, logged when the provider rejects it.
	request []byte
}

func (c *chatResponseWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

func (c *chatResponseWriter) WriteHeader(status int) {
	if c.mode != modeUndecided {
		return
	}
	c.status = status
	header := c.Header()
	header.Del("Content-Length")
	switch {
	case status >= http.StatusBadRequest:
		c.mode = modeError
	case c.stream:
		c.mode = modeStream
		header.Set("Content-Type", "text/event-stream")
		header.Set("Cache-Control", "no-cache")
		c.ResponseWriter.WriteHeader(status)
		c.events = newStreamTranslator(c.writeEvent)
	default:
		c.mode = modeJSON
	}
}

func (c *chatResponseWriter) Write(p []byte) (int, error) {
	if c.mode == modeUndecided {
		c.WriteHeader(http.StatusOK)
	}
	if c.mode == modeStream {
		c.events.write(p)
	} else {
		c.buf.Write(p)
	}
	return len(p), nil
}

// Flush forwards to the client only for streams; buffered bodies are sent by
// finish.
func (c *chatResponseWriter) Flush() {
	if c.mode == modeStream {
		_ = http.NewResponseController(c.ResponseWriter).Flush()
	}
}

func (c *chatResponseWriter) writeEvent(event string, payload map[string]any) {
	data, _ := json.Marshal(payload)
	var frame bytes.Buffer
	frame.WriteString("event: " + event + "\ndata: ")
	frame.Write(data)
	frame.WriteString("\n\n")
	_, _ = c.ResponseWriter.Write(frame.Bytes())
	_ = http.NewResponseController(c.ResponseWriter).Flush()
}

// finish sends what the provider's response was converted to. It must be called
// once the upstream handler has returned.
func (c *chatResponseWriter) finish() {
	c.logRejected()
	switch c.mode {
	case modeStream:
		c.events.finish()
	case modeError:
		c.send(c.status, chatErrorToMessagesError(c.status, c.buf.Bytes()))
	case modeJSON:
		converted, err := chatToMessagesResponse(c.buf.Bytes())
		if err != nil {
			apierror.Write(c.ResponseWriter, http.StatusBadGateway, apierror.TypeAPI, "claudeproxy: provider returned an invalid response: "+strings.TrimSpace(err.Error()))
			return
		}
		c.send(c.status, converted)
	}
}

func (c *chatResponseWriter) send(status int, body []byte) {
	c.Header().Set("Content-Type", "application/json")
	c.ResponseWriter.WriteHeader(status)
	_, _ = c.ResponseWriter.Write(body)
}

// maxLoggedRequestBytes bounds the rejected request body written to the log.
const maxLoggedRequestBytes = 256 << 10

// logRejected records what the provider refused. Its error body is often empty,
// so the request that was sent (never the credentials) is logged at debug level
// along with the provider's request id.
func (c *chatResponseWriter) logRejected() {
	if c.logger == nil {
		return
	}
	request := c.request
	if len(request) > maxLoggedRequestBytes {
		request = request[:maxLoggedRequestBytes]
	}
	c.logger.LogAttrs(context.Background(), slog.LevelDebug, "chat-completions request rejected",
		slog.Int("status", c.status),
		slog.String("opencode_log_id", c.Header().Get("X-Opencode-Log-Id")),
		slog.Int("request_bytes", len(c.request)),
		slog.String("request", string(request)),
	)
}
