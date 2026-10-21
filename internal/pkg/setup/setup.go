package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Result reports what Apply changed.
type Result struct {
	SettingsPath string
	BaseURLSet   bool
}

// Apply points a Claude Code config directory at a claudeproxy base URL. It is
// idempotent and leaves every other setting untouched.
func Apply(claudeDir, baseURL string) (Result, error) {
	settingsPath := filepath.Join(claudeDir, "settings.json")
	settings, err := load(settingsPath)
	if err != nil {
		return Result{}, err
	}

	result := Result{SettingsPath: settingsPath}

	env := object(settings, "env")
	if env["ANTHROPIC_BASE_URL"] != baseURL {
		result.BaseURLSet = true
	}
	env["ANTHROPIC_BASE_URL"] = baseURL

	if err := save(settingsPath, settings); err != nil {
		return Result{}, err
	}
	return result, nil
}

func load(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	settings := map[string]any{}
	if len(data) == 0 {
		return settings, nil
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings, nil
}

func save(path string, settings map[string]any) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// object returns settings[key] as a JSON object, creating an empty one if the
// key is absent or has another type.
func object(settings map[string]any, key string) map[string]any {
	if existing, ok := settings[key].(map[string]any); ok {
		return existing
	}
	created := map[string]any{}
	settings[key] = created
	return created
}
