// Package config is the "proxy" section of the configuration file: providers
// and model routes. It only parses and expands values; the composition root
// turns it into domain routes and upstream adapters.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// DefaultMaxBodyBytes bounds buffered Messages API bodies (images included).
const DefaultMaxBodyBytes = 64 << 20

// ErrUnsetEnv is returned when a value references an undefined environment variable.
var ErrUnsetEnv = errors.New("environment variable not set")

// Config is the "proxy" section.
type Config struct {
	MaxBodyBytes int64 `yaml:"max_body_bytes"`
	// DefaultProvider receives requests without a model and unmatched models.
	// Empty means unmatched models are rejected.
	DefaultProvider string              `yaml:"default_provider"`
	Providers       map[string]Provider `yaml:"providers"`
	// Routes are evaluated in order; the first match wins.
	Routes []Route `yaml:"routes"`
}

// Provider is an upstream exposing the Anthropic Messages API. APIKey,
// BaseURL and header values support ${VAR} environment expansion.
type Provider struct {
	BaseURL string `yaml:"base_url"`
	// Auth is passthrough, bearer or x-api-key.
	Auth        string            `yaml:"auth"`
	APIKey      string            `yaml:"api_key"`
	Headers     map[string]string `yaml:"headers"`
	DropHeaders []string          `yaml:"drop_headers"`
	// FlattenMidConversationUpdates moves the tool additions and reasoning
	// effort that Claude Code sends in mid-conversation system messages to
	// the request level, for models that reject such updates.
	FlattenMidConversationUpdates bool `yaml:"flatten_mid_conversation_updates"`
	// KeepClaudeMetadata keeps the metadata object and the billing attribution block in requests sent to a provider other than Anthropic. Leave it false so no Claude identifier leaks to a third party.
	KeepClaudeMetadata bool `yaml:"keep_claude_metadata"`
}

// Route matches the requested model with either Match (glob, '*' and '?') or
// Regex (RE2). Model, when set, renames the model upstream and may reference
// captures as $1 or ${name}.
type Route struct {
	Match    string `yaml:"match"`
	Regex    string `yaml:"regex"`
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// Resolve returns a copy with defaults applied and environment references
// expanded. It returns ErrUnsetEnv listing every missing variable.
func (c Config) Resolve() (Config, error) {
	if c.MaxBodyBytes <= 0 {
		c.MaxBodyBytes = DefaultMaxBodyBytes
	}
	var missing []string
	expand := func(value string) string {
		return os.Expand(value, func(name string) string {
			v, ok := os.LookupEnv(name)
			if !ok && !slices.Contains(missing, name) {
				missing = append(missing, name)
			}
			return v
		})
	}
	providers := make(map[string]Provider, len(c.Providers))
	for name, p := range c.Providers {
		p.BaseURL = expand(p.BaseURL)
		p.APIKey = expand(p.APIKey)
		headers := make(map[string]string, len(p.Headers))
		for k, v := range p.Headers {
			headers[k] = expand(v)
		}
		p.Headers = headers
		providers[name] = p
	}
	c.Providers = providers
	if len(missing) > 0 {
		return Config{}, errors.Join(ErrUnsetEnv, fmt.Errorf("missing: %s", strings.Join(missing, ", ")))
	}
	return c, nil
}

// Metrics configures request-usage recording. Recording is enabled by default;
// Disabled opts out.
type Metrics struct {
	Disabled bool   `yaml:"disabled"`
	Path     string `yaml:"path"`
}

func (m Metrics) Enabled() bool { return !m.Disabled }

func (m Metrics) ResolvePath() (string, error) {
	if m.Path != "" {
		return m.Path, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "claudeproxy", "usage.db"), nil
}
