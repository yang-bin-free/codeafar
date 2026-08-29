package session

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClaudeProcIncludesAllowedTools(t *testing.T) {
	args := NewClaudeProc(ClaudeConfig{SessionID: "s", Permission: "default", AllowedTools: []string{"Read", "Bash(git status:*)"}}).buildArgs()
	if !slices.Contains(args, "--allowedTools") || !slices.Contains(args, "Bash(git status:*)") {
		t.Fatalf("args missing allowed tools: %v", args)
	}
	if !slices.Contains(args, "--include-partial-messages") {
		t.Fatalf("args missing partial message streaming: %v", args)
	}
}

func TestClaudeProcStreamJSONEnablesVerbose(t *testing.T) {
	args := NewClaudeProc(ClaudeConfig{SessionID: "s", Permission: "default"}).buildArgs()
	if !slices.Contains(args, "--verbose") {
		t.Fatalf("stream-json args missing required --verbose: %v", args)
	}
}

func TestClaudeProcPrependsBinArgs(t *testing.T) {
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "wrapper")
	// The wrapper records the arguments it was invoked with, then exits.
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + dir + "/args.txt\nexit 0\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	proc := NewClaudeProc(ClaudeConfig{
		Bin: wrapper, BinArgs: []string{"claude"}, Cwd: dir,
		SessionID: "11111111-2222-3333-4444-555555555555", Permission: "default",
	})
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	if err := proc.Send("hello"); err != nil {
		t.Fatal(err)
	}
	_ = proc.Stop()
	data, err := os.ReadFile(filepath.Join(dir, "args.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(data))
	wantFirst := []string{"claude", "--print", "--session-id", "11111111-2222-3333-4444-555555555555"}
	if len(got) < len(wantFirst) {
		t.Fatalf("args=%v", got)
	}
	for i, want := range wantFirst {
		if got[i] != want {
			t.Fatalf("args[%d]=%q want %q (full: %v)", i, got[i], want, got)
		}
	}
}

func TestClaudeProc_StreamsTokens(t *testing.T) {
	proc := NewClaudeProc(ClaudeConfig{
		Bin:        "../../testdata/fake-claude.sh",
		Cwd:        ".",
		SessionID:  "sess1",
		Permission: "bypassPermissions",
	})

	var mu sync.Mutex
	var lines []string
	proc.OnOutput(func(payload []byte) {
		mu.Lock()
		lines = append(lines, string(payload))
		mu.Unlock()
	})

	if err := proc.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := proc.Send("检查并发"); err != nil {
		t.Fatalf("send: %v", err)
	}

	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		n := len(lines)
		mu.Unlock()
		if n >= 4 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout, got %d lines: %v", n, lines)
		case <-time.After(20 * time.Millisecond):
		}
	}
	_ = proc.Stop()

	if lines[0] != `{"type":"thinking"}` {
		t.Fatalf("first line = %s", lines[0])
	}
	if lines[len(lines)-1] != `{"type":"done"}` {
		t.Fatalf("last line = %s", lines[len(lines)-1])
	}
}
