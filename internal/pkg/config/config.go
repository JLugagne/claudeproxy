// Package config holds the process-level settings (listener, timeouts,
// logging) shared by every bounded context. It imports no internal package.
package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"
)

// ErrInvalid is returned when a server setting is not acceptable.
var ErrInvalid = errors.New("invalid server configuration")

// Server is the "server" section of the configuration file.
type Server struct {
	Listen    string     `yaml:"listen"`
	LogLevel  slog.Level `yaml:"log_level"`
	LogFormat string     `yaml:"log_format"`
	// ReadHeaderTimeout bounds how long a client may take to send headers.
	// There is deliberately no read/write timeout: SSE streams can last minutes.
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
	// ShutdownTimeout is how long in-flight streams may finish on SIGINT/SIGTERM.
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

// WithDefaults returns s with every unset field given its default.
func (s Server) WithDefaults() Server {
	if s.Listen == "" {
		s.Listen = "127.0.0.1:8787"
	}
	if s.LogFormat == "" {
		s.LogFormat = "text"
	}
	if s.ReadHeaderTimeout == 0 {
		s.ReadHeaderTimeout = 10 * time.Second
	}
	if s.IdleTimeout == 0 {
		s.IdleTimeout = 120 * time.Second
	}
	if s.ShutdownTimeout == 0 {
		s.ShutdownTimeout = 30 * time.Second
	}
	return s
}

// NewLogger builds the process logger writing to w. It returns ErrInvalid for
// an unknown log_format.
func (s Server) NewLogger(w io.Writer) (*slog.Logger, error) {
	opts := &slog.HandlerOptions{Level: s.LogLevel}
	switch s.LogFormat {
	case "text":
		return slog.New(slog.NewTextHandler(w, opts)), nil
	case "json":
		return slog.New(slog.NewJSONHandler(w, opts)), nil
	default:
		return nil, errors.Join(ErrInvalid, fmt.Errorf("log_format %q: want text or json", s.LogFormat))
	}
}
