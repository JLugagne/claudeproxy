package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyWritesOnlyTheBaseURL(t *testing.T) {
	dir := t.TempDir()

	result, err := Apply(dir, "http://127.0.0.1:8787")
	if err != nil {
		t.Fatal(err)
	}
	if !result.BaseURLSet {
		t.Fatalf("result %+v, want the base URL set", result)
	}

	settings := readSettings(t, result.SettingsPath)
	if got := settings["env"].(map[string]any)["ANTHROPIC_BASE_URL"]; got != "http://127.0.0.1:8787" {
		t.Fatalf("ANTHROPIC_BASE_URL = %v", got)
	}
	if _, ok := settings["hooks"]; ok {
		t.Fatalf("hooks written: %v", settings["hooks"])
	}
	if _, err := os.Stat(filepath.Join(dir, "claudeproxy")); !os.IsNotExist(err) {
		t.Fatalf("claudeproxy directory created (stat error %v)", err)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := Apply(dir, "http://127.0.0.1:8787"); err != nil {
		t.Fatal(err)
	}

	result, err := Apply(dir, "http://127.0.0.1:8787")
	if err != nil {
		t.Fatal(err)
	}
	if result.BaseURLSet {
		t.Fatalf("second apply changed %+v, want no changes", result)
	}
}

func TestApplyPreservesOtherSettings(t *testing.T) {
	dir := t.TempDir()
	existing := `{"model":"opus","env":{"KEEP":"1"},"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"/mine.sh"}]}]}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Apply(dir, "http://127.0.0.1:8787"); err != nil {
		t.Fatal(err)
	}

	settings := readSettings(t, filepath.Join(dir, "settings.json"))
	if settings["model"] != "opus" {
		t.Fatalf("model lost: %v", settings)
	}
	env := settings["env"].(map[string]any)
	if env["KEEP"] != "1" || env["ANTHROPIC_BASE_URL"] != "http://127.0.0.1:8787" {
		t.Fatalf("env = %v", env)
	}
	hooks := settings["hooks"].(map[string]any)["SessionStart"].([]any)
	if len(hooks) != 1 {
		t.Fatalf("existing hooks altered: %v", settings["hooks"])
	}
}

func readSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	return settings
}
