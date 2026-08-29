package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuntimeConfigReloadsSafeFields(t *testing.T) {
	dir := t.TempDir()
	e := New(Config{DataDir: dir, DefaultWorkingDir: "/old", DefaultPermission: "default", MaxConcurrentSession: 5, ConfigPollInterval: 10 * time.Millisecond})
	defer e.Close()

	config := []byte("defaultWorkingDir: /new\ndefaultPermission: acceptEdits\nmaxConcurrentSessions: 2\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), config, 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got := e.Status()
		if got.DefaultWorkingDir == "/new" && got.DefaultPermission == "acceptEdits" && got.MaxConcurrentSession == 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("config did not reload: %+v", e.Status())
}

func TestUpdateRuntimeConfigPersistsCommands(t *testing.T) {
	dataDir := t.TempDir()
	workDir := t.TempDir()
	e := New(Config{DataDir: dataDir, DefaultWorkingDir: workDir, DefaultPermission: "default", MaxConcurrentSession: 2})
	defer e.Close()
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "wrapper")
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	command := wrapper + " claude"
	if _, err := ValidateCommandSetting(command, "Claude", func(s string) (string, error) { return s, nil }); err != nil {
		t.Fatal(err)
	}
	if err := e.updateRuntimeConfig(runtimeConfig{
		DefaultWorkingDir: workDir, DefaultPermission: "default",
		MaxConcurrentSessions: 2, ClaudeCommand: command, CodexCommand: "",
	}); err != nil {
		t.Fatal(err)
	}
	got := e.runtimeConfig()
	if got.ClaudeCommand != command {
		t.Fatalf("claudeCommand=%q", got.ClaudeCommand)
	}
	if got.CodexCommand != "" {
		t.Fatalf("codexCommand=%q", got.CodexCommand)
	}
	content, err := os.ReadFile(filepath.Join(dataDir, "config.yaml"))
	if err != nil || !strings.Contains(string(content), "claudeCommand: "+command) {
		t.Fatalf("config=%q err=%v", content, err)
	}
}

func TestReadPersistedCommands(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(
		"defaultWorkingDir: /tmp\ndefaultPermission: default\nmaxConcurrentSessions: 2\nclaudeCommand: wrapper claude\n"),
		0o600); err != nil {
		t.Fatal(err)
	}
	claude, codex := ReadPersistedCommands(dir)
	if claude != "wrapper claude" || codex != "" {
		t.Fatalf("claude=%q codex=%q", claude, codex)
	}
}

func TestReadPersistedCommandsReturnsEmptyWithoutFile(t *testing.T) {
	claude, codex := ReadPersistedCommands(t.TempDir())
	if claude != "" || codex != "" {
		t.Fatalf("claude=%q codex=%q", claude, codex)
	}
}

func TestReadPersistedCommandsIgnoresCorruptYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("::: not yaml [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if claude, codex := ReadPersistedCommands(dir); claude != "" || codex != "" {
		t.Fatalf("claude=%q codex=%q", claude, codex)
	}
}

func TestReadPersistedCommandsReturnsBothCommands(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("claudeCommand: wrapper claude\ncodexCommand:  wrapper codex \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	claude, codex := ReadPersistedCommands(dir)
	if claude != "wrapper claude" || codex != "wrapper codex" {
		t.Fatalf("claude=%q codex=%q", claude, codex)
	}
}

func TestInvalidRuntimeConfigKeepsLastValidValues(t *testing.T) {
	dir := t.TempDir()
	e := New(Config{DataDir: dir, DefaultWorkingDir: "/old", DefaultPermission: "default", MaxConcurrentSession: 3})
	defer e.Close()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("maxConcurrentSessions: nope\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.reloadRuntimeConfig(); err == nil {
		t.Fatal("expected invalid YAML value to fail")
	}
	got := e.Status()
	if got.DefaultWorkingDir != "/old" || got.DefaultPermission != "default" || got.MaxConcurrentSession != 3 {
		t.Fatalf("invalid config changed runtime: %+v", got)
	}
}
