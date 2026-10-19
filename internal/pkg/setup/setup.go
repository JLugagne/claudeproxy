package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// hookScript is written under <claude-dir>/claudeproxy and registered as a
// SessionStart hook. It exports X-Project so the proxy can attribute usage to
// the repository a session runs in.
const hookScript = `#!/bin/sh
[ -n "$CLAUDE_ENV_FILE" ] || exit 0
dir="${CLAUDE_PROJECT_DIR:-$PWD}"
proj=$(git -C "$dir" remote get-url origin 2>/dev/null | sed -E 's#\.git$##; s#.*[:/]([^/]+/[^/]+)$#\1#')
proj=${proj:-$(basename "$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null)")}
proj=${proj:-$dir}
printf 'export ANTHROPIC_CUSTOM_HEADERS="X-Project: %s"\n' "$proj" >> "$CLAUDE_ENV_FILE"
`

// Result reports what Apply changed.
type Result struct {
	SettingsPath string
	ScriptPath   string
	BaseURLSet   bool
	HookAdded    bool
}

// Apply points a Claude Code config directory at a claudeproxy base URL and
// installs the X-Project SessionStart hook. It is idempotent: running it again
// updates the values without duplicating the hook.
func Apply(claudeDir, baseURL string) (Result, error) {
	settingsPath := filepath.Join(claudeDir, "settings.json")
	settings, err := load(settingsPath)
	if err != nil {
		return Result{}, err
	}

	scriptPath := filepath.Join(claudeDir, "claudeproxy", "x-project.sh")
	if err := writeScript(scriptPath); err != nil {
		return Result{}, err
	}

	result := Result{SettingsPath: settingsPath, ScriptPath: scriptPath}

	env := object(settings, "env")
	if env["ANTHROPIC_BASE_URL"] != baseURL {
		result.BaseURLSet = true
	}
	env["ANTHROPIC_BASE_URL"] = baseURL

	if addSessionStartHook(settings, scriptPath) {
		result.HookAdded = true
	}

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

func writeScript(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(hookScript), 0o700)
}

// addSessionStartHook registers scriptPath as a SessionStart command hook
// unless it is already present, and reports whether it added it.
func addSessionStartHook(settings map[string]any, scriptPath string) bool {
	hooks := object(settings, "hooks")
	sessionStart, _ := hooks["SessionStart"].([]any)
	for _, entry := range sessionStart {
		if entryHasCommand(entry, scriptPath) {
			return false
		}
	}
	hooks["SessionStart"] = append(sessionStart, map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": scriptPath},
		},
	})
	return true
}

func entryHasCommand(entry any, command string) bool {
	entryObject, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	inner, ok := entryObject["hooks"].([]any)
	if !ok {
		return false
	}
	for _, hook := range inner {
		if hookObject, ok := hook.(map[string]any); ok && hookObject["command"] == command {
			return true
		}
	}
	return false
}
