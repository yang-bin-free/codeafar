package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveClaudeBinaryUsesExplicitPath(t *testing.T) {
	bin := writeExecutable(t, t.TempDir(), "claude")
	got, err := ResolveClaudeBinary(bin)
	if err != nil || got.Path != bin || len(got.PrependArgs) != 0 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestResolveClaudeBinaryUsesPATH(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "claude")
	t.Setenv("PATH", dir)
	got, err := ResolveClaudeBinary("claude")
	if err != nil || got.Path != bin {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestResolveClaudeBinaryMultiWordViaClaudeResolver(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, dir, "wrapper")
	t.Setenv("PATH", dir)
	got, err := ResolveClaudeBinary("wrapper claude")
	if err != nil || got.Path != filepath.Join(dir, "wrapper") || len(got.PrependArgs) != 1 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestResolveClaudeBinaryUsesFinderFallback(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := writeExecutable(t, binDir, "claude")
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	got, err := ResolveClaudeBinary("claude")
	if err != nil || got.Path != bin {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestResolveClaudeBinaryUsesNVMFallbackWithoutShellPATH(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".nvm", "versions", "node", "v22.23.1", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := writeExecutable(t, binDir, "claude")
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")
	got, err := ResolveClaudeBinary("claude")
	if err != nil || got.Path != bin {
		t.Fatalf("got=%+v want=%q err=%v", got, bin, err)
	}
}

func TestResolveClaudeBinaryRejectsNonExecutableAndReportsSearch(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	_, err := ResolveClaudeBinary("claude")
	if err == nil || !strings.Contains(err.Error(), ".local/bin/claude") {
		t.Fatalf("err=%v", err)
	}
}

func TestResolveCodexBinaryUsesNVMFallbackWithoutShellPATH(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".nvm", "versions", "node", "v22.23.1", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := writeExecutable(t, binDir, "codex")
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")
	got, err := ResolveCodexBinary("codex")
	if err != nil || got.Path != bin {
		t.Fatalf("got=%+v want=%q err=%v", got, bin, err)
	}
}
