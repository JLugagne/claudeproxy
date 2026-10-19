package setup

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestApplyWritesSettingsAndHook(t *testing.T) {
	dir := t.TempDir()

	result, err := Apply(dir, "http://127.0.0.1:8787")
	if err != nil {
		t.Fatal(err)
	}
	if !result.BaseURLSet || !result.HookAdded {
		t.Fatalf("result %+v, want both set", result)
	}

	settings := readSettings(t, result.SettingsPath)
	if got := settings["env"].(map[string]any)["ANTHROPIC_BASE_URL"]; got != "http://127.0.0.1:8787" {
		t.Fatalf("ANTHROPIC_BASE_URL = %v", got)
	}
	if countSessionStartHooks(t, settings) != 1 {
		t.Fatalf("expected one SessionStart hook in %v", settings["hooks"])
	}
	info, err := os.Stat(result.ScriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("script mode = %v, want 0700", info.Mode().Perm())
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
	if result.BaseURLSet || result.HookAdded {
		t.Fatalf("second apply changed %+v, want no changes", result)
	}
	if countSessionStartHooks(t, readSettings(t, result.SettingsPath)) != 1 {
		t.Fatal("hook was duplicated")
	}
}

func TestApplyPreservesOtherSettings(t *testing.T) {
	dir := t.TempDir()
	existing := `{"model":"opus","env":{"KEEP":"1"}}`
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

func countSessionStartHooks(t *testing.T, settings map[string]any) int {
	t.Helper()
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return 0
	}
	sessionStart, ok := hooks["SessionStart"].([]any)
	if !ok {
		return 0
	}
	return len(sessionStart)
}

func TestHookNamesTheProjectWithoutGitSuffix(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"remote", "add", "origin", "git@github.com:owner/repo.git"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	script := filepath.Join(t.TempDir(), "x-project.sh")
	if err := writeScript(script); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(t.TempDir(), "env")

	cmd := exec.Command("sh", script)
	cmd.Env = append(os.Environ(), "CLAUDE_ENV_FILE="+envFile, "CLAUDE_PROJECT_DIR="+repo)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook failed: %v: %s", err, out)
	}

	got, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if want := `export ANTHROPIC_CUSTOM_HEADERS="X-Project: owner/repo"` + "\n"; string(got) != want {
		t.Fatalf("env file = %q, want %q", got, want)
	}
}
