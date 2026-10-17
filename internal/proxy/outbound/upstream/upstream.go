// Package upstream forwards requests to one provider exposing the Anthropic
// Messages API. Responses, including SSE streams, are relayed byte for byte
// and flushed as soon as they arrive.
package upstream

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/JLugagne/claudeproxy/internal/pkg/apierror"
	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// AuthMode selects how the provider is authenticated.
type AuthMode string

const (
	// AuthPassthrough forwards the client's credentials untouched. Use it only
	// for the provider the client actually holds credentials for (Anthropic).
	AuthPassthrough AuthMode = "passthrough"
	// AuthBearer sends the configured key as "Authorization: Bearer <key>".
	AuthBearer AuthMode = "bearer"
	// AuthAPIKey sends the configured key as "x-api-key: <key>".
	AuthAPIKey AuthMode = "x-api-key"
)

// StatusClientClosedRequest is recorded when the client disconnects before
// the upstream answered; it is never seen by the client.
const StatusClientClosedRequest = 499

const maxLoggedErrorBytes = 4 << 10

// ErrInvalidConfig is returned by New when the provider cannot be built.
var ErrInvalidConfig = errors.New("invalid upstream configuration")

var clientCredentialHeaders = []string{"Authorization", "X-Api-Key", "Cookie"}

// proxyMetaHeaders are set by claudeproxy for its own observability. They are
// stripped from every outbound request, Anthropic included, and never reach a
// provider.
var proxyMetaHeaders = []string{"X-Project"}

// claudeIdentityHeaders and claudeIdentityHeaderPrefixes identify the client as
// Claude Code. They carry no protocol meaning, so they are stripped before a
// provider that is not Anthropic (which holds the client's own credentials) can
// see them.
var claudeIdentityHeaders = []string{
	"User-Agent",
	"X-App",
	"Anthropic-Beta",
	"Anthropic-Dangerous-Direct-Browser-Access",
}

var claudeIdentityHeaderPrefixes = []string{"X-Stainless-", "X-Anthropic-"}

// Config describes one provider.
type Config struct {
	Name string
	// BaseURL is joined with the request path: "https://openrouter.ai/api"
	// turns "/v1/messages" into "https://openrouter.ai/api/v1/messages".
	BaseURL *url.URL
	Auth    AuthMode
	// APIKey is required unless Auth is AuthPassthrough.
	APIKey string
	// SetHeaders are set on every request after credentials are applied.
	SetHeaders map[string]string
	// DropHeaders are removed from every request before credentials are applied.
	DropHeaders []string
	// FlattenMidConversationUpdates rewrites mid-conversation system messages
	// for models that reject them: tool_addition blocks become entries of the
	// top-level tools array, and a message-level output_config replaces the
	// request-level one. Requests without such updates are forwarded untouched.
	FlattenMidConversationUpdates bool
	// KeepClaudeMetadata disables the stripping of Claude Code's metadata object and billing attribution block. They are stripped by default for every provider other than Anthropic.
	KeepClaudeMetadata bool
}

// NewTransport returns the transport shared by every provider. Compression is
// left to the client so encoded bodies are relayed without being decoded.
func NewTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		DisableCompression:    true,
	}
}

// New returns the handler forwarding to the provider. Outside passthrough mode
// the client's credentials are always stripped, so an Anthropic token never
// reaches a third-party provider. It returns ErrInvalidConfig.
func New(cfg Config, transport http.RoundTripper, logger *slog.Logger) (http.Handler, error) {
	if err := validate(cfg); err != nil {
		return nil, errors.Join(ErrInvalidConfig, fmt.Errorf("provider %q", cfg.Name), err)
	}
	logger = logger.With("provider", cfg.Name)
	proxy := &httputil.ReverseProxy{
		Rewrite:        rewriter(cfg),
		Transport:      transport,
		FlushInterval:  -1,
		ModifyResponse: errorBodyLogger(logger),
		ErrorLog:       slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
		ErrorHandler:   errorHandler(cfg.Name, logger),
	}
	handler := http.Handler(proxy)
	if cfg.Auth != AuthPassthrough && !cfg.KeepClaudeMetadata {
		handler = claudeMetadataStripper(handler)
	}
	if cfg.FlattenMidConversationUpdates {
		handler = midConversationFlattener(handler)
	}
	return handler, nil
}

func validate(cfg Config) error {
	if cfg.BaseURL == nil || cfg.BaseURL.Scheme == "" || cfg.BaseURL.Host == "" {
		return errors.New("base_url must be an absolute URL")
	}
	switch cfg.Auth {
	case AuthPassthrough:
		return nil
	case AuthBearer, AuthAPIKey:
		if cfg.APIKey == "" {
			return fmt.Errorf("auth %q requires an api_key", cfg.Auth)
		}
		return nil
	case "":
		return errors.New("auth is required: passthrough, bearer or x-api-key")
	default:
		return fmt.Errorf("unknown auth mode %q", cfg.Auth)
	}
}

func rewriter(cfg Config) func(*httputil.ProxyRequest) {
	base := *cfg.BaseURL
	return func(pr *httputil.ProxyRequest) {
		pr.SetURL(&base)
		header := pr.Out.Header
		for _, name := range proxyMetaHeaders {
			header.Del(name)
		}
		if cfg.Auth != AuthPassthrough {
			for _, name := range claudeIdentityHeaders {
				header.Del(name)
			}
			deleteHeaderPrefixes(header, claudeIdentityHeaderPrefixes)
		}
		for _, name := range cfg.DropHeaders {
			header.Del(name)
		}
		switch cfg.Auth {
		case AuthBearer:
			stripCredentials(header)
			header.Set("Authorization", "Bearer "+cfg.APIKey)
		case AuthAPIKey:
			stripCredentials(header)
			header.Set("X-Api-Key", cfg.APIKey)
		}
		for name, value := range cfg.SetHeaders {
			header.Set(name, value)
		}
	}
}

func stripCredentials(header http.Header) {
	for _, name := range clientCredentialHeaders {
		header.Del(name)
	}
}

func errorHandler(name string, logger *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
			w.WriteHeader(StatusClientClosedRequest)
			return
		}
		logger.LogAttrs(r.Context(), slog.LevelWarn, "upstream request failed", slog.String("path", r.URL.Path), slog.Any("error", err))
		apierror.Write(w, http.StatusBadGateway, apierror.TypeAPI, "claudeproxy: provider "+name+" unreachable: "+err.Error())
	}
}

func errorBodyLogger(logger *slog.Logger) func(*http.Response) error {
	return func(resp *http.Response) error {
		if resp.StatusCode < http.StatusBadRequest {
			return nil
		}
		head, _ := io.ReadAll(io.LimitReader(resp.Body, maxLoggedErrorBytes))
		resp.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(head), resp.Body), resp.Body}
		logger.LogAttrs(resp.Request.Context(), slog.LevelWarn, "upstream error",
			slog.String("path", resp.Request.URL.Path),
			slog.Int("status", resp.StatusCode),
			slog.String("body", decodeForLog(head, resp.Header.Get("Content-Encoding"))),
		)
		return nil
	}
}

func decodeForLog(head []byte, encoding string) string {
	var decoder io.Reader
	switch encoding {
	case "", "identity":
		return string(head)
	case "gzip":
		zr, err := gzip.NewReader(bytes.NewReader(head))
		if err != nil {
			return undecodable(head, encoding)
		}
		decoder = zr
	case "br":
		decoder = brotli.NewReader(bytes.NewReader(head))
	case "zstd":
		zr, err := zstd.NewReader(bytes.NewReader(head), zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true))
		if err != nil {
			return undecodable(head, encoding)
		}
		defer zr.Close()
		decoder = zr
	default:
		return undecodable(head, encoding)
	}
	decoded, _ := io.ReadAll(io.LimitReader(decoder, maxLoggedErrorBytes))
	if len(decoded) == 0 {
		return undecodable(head, encoding)
	}
	return string(decoded)
}

func undecodable(head []byte, encoding string) string {
	return fmt.Sprintf("<%d bytes, content-encoding %q not decoded>", len(head), encoding)
}

func deleteHeaderPrefixes(header http.Header, prefixes []string) {
	for name := range header {
		for _, prefix := range prefixes {
			if strings.HasPrefix(name, prefix) {
				header.Del(name)
				break
			}
		}
	}
}
