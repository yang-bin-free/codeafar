package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func writeVersionScript(t *testing.T, dir, name, version string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\n[ \"$1\" = \"--version\" ] || { echo \"bad invocation: $*\" >&2; exit 1; }\necho '" + version + "'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDetectCLIVersionSingleCommand(t *testing.T) {
	dir := t.TempDir()
	bin := writeVersionScript(t, dir, "agent", "1.2.3 (Agent)")
	got, err := DetectCLIVersion([]string{bin}, "coding agent")
	if err != nil || got != "1.2.3" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestDetectCLIVersionWithPrependArgs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wrapper")
	script := "#!/bin/sh\n[ \"$1\" = \"claude\" ] && [ \"$2\" = \"--version\" ] || { echo \"bad invocation: $*\" >&2; exit 1; }\necho '2.1.245'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := DetectCLIVersion([]string{path, "claude"}, "coding agent")
	if err != nil || got != "2.1.245" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestDetectCLIVersionUnparsableOutputFails(t *testing.T) {
	dir := t.TempDir()
	bin := writeVersionScript(t, dir, "agent", "no version here")
	if _, err := DetectCLIVersion([]string{bin}, "coding agent"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestClaudeVersionPattern(t *testing.T) {
	for input, want := range map[string]string{"2.1.204 (Claude Code)": "2.1.204", "claude version 3.0.0-beta.1": "3.0.0-beta.1"} {
		match := claudeVersionPattern.FindStringSubmatch(input)
		if len(match) != 2 || match[1] != want {
			t.Fatalf("parse %q = %v, want %q", input, match, want)
		}
	}
}

func TestCLIVersionPatternSupportsCodex(t *testing.T) {
	match := cliVersionPattern.FindStringSubmatch("codex-cli 0.144.1")
	if len(match) != 2 || match[1] != "0.144.1" {
		t.Fatalf("match=%v", match)
	}
}
