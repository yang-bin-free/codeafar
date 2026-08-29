package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCodexProcPrependsBinArgs(t *testing.T) {
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "wrapper")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + dir + "/args.txt\nexit 0\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	proc := NewCodexProc(CodexConfig{
		Bin: wrapper, BinArgs: []string{"codex"}, Cwd: dir, Permission: "readOnly",
	})
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	if err := proc.Send("hello"); err != nil {
		t.Fatal(err)
	}
	// Codex turns run asynchronously, so wait for the wrapper to record its
	// argv before Stop can kill the process. Poll until the content is
	// complete — the shell creates/truncates args.txt before printf finishes,
	// so a successful read alone can still observe a partial file.
	argsPath := filepath.Join(dir, "args.txt")
	var data []byte
	var err error
	for deadline := time.Now().Add(3 * time.Second); ; {
		data, err = os.ReadFile(argsPath)
		if err == nil && strings.HasSuffix(string(data), "hello\n") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("wrapper did not record complete args: read err=%v data=%q", err, data)
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = proc.Stop()
	got := strings.Fields(string(data))
	if len(got) < 3 || got[0] != "codex" {
		t.Fatalf("args=%v", got)
	}
	if got[1] != "-C" || got[2] != dir {
		t.Fatalf("args=%v", got)
	}
}

func TestCodexProcBuildsNewAndResumeCommands(t *testing.T) {
	fresh := NewCodexProc(CodexConfig{
		Bin: "codex", Cwd: "/repo", Permission: "workspaceWrite", Model: "gpt-test",
		AddDirs: []string{"/shared"},
	})
	got, err := fresh.buildArgs("fix it")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-C", "/repo", "-s", "workspace-write", "-a", "never", "-m", "gpt-test", "--add-dir", "/shared", "exec", "--json", "--color", "never", "--skip-git-repo-check", "fix it"} {
		if !slices.Contains(got, want) {
			t.Fatalf("fresh args %v missing %q", got, want)
		}
	}
	if slices.Contains(got, "resume") {
		t.Fatalf("fresh args unexpectedly resume: %v", got)
	}

	resumed := NewCodexProc(CodexConfig{
		Bin: "codex", Cwd: "/repo", ProviderSessionID: "thread-1", Permission: "readOnly",
	})
	got, err = resumed.buildArgs("continue")
	if err != nil {
		t.Fatal(err)
	}
	if !containsArgSequence(got, "exec", "resume") || !slices.Contains(got, "thread-1") || !slices.Contains(got, "read-only") {
		t.Fatalf("resume args = %v", got)
	}
	if slices.Contains(got, "--color") {
		t.Fatalf("resume args include unsupported --color flag: %v", got)
	}
}

func TestCodexProcMapsFullAccessAndRejectsUnknownPermission(t *testing.T) {
	full := NewCodexProc(CodexConfig{Cwd: "/repo", Permission: "fullAccess"})
	args, err := full.buildArgs("work")
	if err != nil || !slices.Contains(args, "danger-full-access") {
		t.Fatalf("full access args=%v err=%v", args, err)
	}
	unknown := NewCodexProc(CodexConfig{Cwd: "/repo", Permission: "default"})
	if _, err := unknown.buildArgs("work"); err == nil {
		t.Fatal("unknown Codex permission was accepted")
	}
}

func containsArgSequence(values []string, sequence ...string) bool {
	if len(sequence) == 0 || len(sequence) > len(values) {
		return false
	}
	for i := 0; i <= len(values)-len(sequence); i++ {
		if slices.Equal(values[i:i+len(sequence)], sequence) {
			return true
		}
	}
	return false
}

func TestCodexProcCapturesThreadBeforeOutputAndReleasesBeforeTerminalEvent(t *testing.T) {
	proc := NewCodexProc(CodexConfig{
		Bin: "../../testdata/fake-codex.sh", Cwd: ".", Permission: "readOnly",
	})
	terminal := make(chan error, 1)
	proc.OnOutput(func(payload []byte) {
		var event struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(payload, &event) != nil {
			return
		}
		if event.Type == "thread.started" && proc.ProviderSessionID() != "thread-fake" {
			terminal <- fmt.Errorf("thread ID was not captured before callback")
		}
		if event.Type == "turn.completed" {
			terminal <- proc.Send("second turn")
		}
	})
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	if err := proc.Send("first turn"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-terminal:
		if err != nil {
			t.Fatalf("terminal callback could not start next turn: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for terminal event")
	}
	_ = proc.Stop()
}

func TestCodexProcRejectsConcurrentSendAndStopSuppressesExitError(t *testing.T) {
	proc := NewCodexProc(CodexConfig{
		Bin: "../../testdata/fake-codex.sh", Cwd: ".", Permission: "readOnly",
	})
	var mu sync.Mutex
	var output []string
	proc.OnOutput(func(payload []byte) {
		mu.Lock()
		output = append(output, string(payload))
		mu.Unlock()
	})
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	if err := proc.Send("SLOW"); err != nil {
		t.Fatal(err)
	}
	if err := proc.Send("overlap"); err == nil {
		t.Fatal("concurrent Codex turn was accepted")
	}
	if err := proc.Stop(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	for _, line := range output {
		if strings.Contains(line, "CODEX_ERROR") {
			t.Fatalf("Stop emitted a synthetic error: %v", output)
		}
	}
}

func TestCodexProcReportsBoundedProcessFailure(t *testing.T) {
	proc := NewCodexProc(CodexConfig{
		Bin: "../../testdata/fake-codex.sh", Cwd: ".", Permission: "readOnly",
	})
	result := make(chan string, 2)
	proc.OnOutput(func(payload []byte) {
		result <- string(payload)
	})
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	if err := proc.Send("FAIL"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-result:
		if strings.Contains(got, "simulated Codex failure") || !strings.Contains(got, "exit status 7") || len(got) > 2300 {
			t.Fatalf("process error = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Codex process error")
	}
	select {
	case got := <-result:
		if !strings.Contains(got, `"type":"done"`) {
			t.Fatalf("process failure did not finish turn: %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Codex failure completion")
	}
	_ = proc.Stop()
}

func TestReadBoundedStderrDrainsInputAfterCaptureLimit(t *testing.T) {
	reader := strings.NewReader(strings.Repeat("x\n", codexMaxStderrBytes))
	got := readBoundedStderr(reader)
	if len(got) > codexMaxStderrBytes {
		t.Fatalf("captured stderr length=%d", len(got))
	}
	if reader.Len() != 0 {
		t.Fatalf("stderr reader left %d bytes undrained", reader.Len())
	}
}

func TestCodexProcOversizedOutputFinishesTurnWithoutLeakingOrDeadlocking(t *testing.T) {
	for _, prompt := range []string{"HUGE_STDERR", "HUGE_STDOUT"} {
		t.Run(prompt, func(t *testing.T) {
			proc := NewCodexProc(CodexConfig{Bin: "../../testdata/fake-codex.sh", Cwd: ".", Permission: "readOnly"})
			messages := make(chan string, 2)
			proc.OnOutput(func(payload []byte) {
				if strings.Contains(string(payload), `"type":"error"`) || strings.Contains(string(payload), `"type":"done"`) {
					messages <- string(payload)
				}
			})
			if err := proc.Start(); err != nil {
				t.Fatal(err)
			}
			if err := proc.Send(prompt); err != nil {
				t.Fatal(err)
			}
			for index := 0; index < 2; index++ {
				select {
				case message := <-messages:
					if strings.Contains(message, strings.Repeat("s", 32)) {
						t.Fatalf("raw stderr leaked: %.100q", message)
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("%s deadlocked before terminal messages", prompt)
				}
			}
			if err := proc.Send("after oversized output"); err != nil {
				t.Fatalf("process did not release turn: %v", err)
			}
			_ = proc.Stop()
		})
	}
}

func TestClassifyCodexFailureReturnsActionableAllowlistedMessages(t *testing.T) {
	tests := []struct {
		stderr string
		want   string
	}{
		{"Not logged in. Run codex login", "codex login"},
		{"error: unexpected argument '--private-token' found", "CLI is incompatible"},
		{"session 019-secret not found", "conversation could not be resumed"},
		{"request timed out while connecting", "network connection"},
		{"api_key=top-secret", "exit status 7"},
	}
	for _, test := range tests {
		got := classifyCodexFailure(test.stderr, errors.New("exit status 7"))
		if !strings.Contains(got, test.want) {
			t.Errorf("classify %q = %q, want %q", test.stderr, got, test.want)
		}
		for _, secret := range []string{"private-token", "019-secret", "top-secret"} {
			if strings.Contains(got, secret) {
				t.Errorf("classified message leaked %q: %q", secret, got)
			}
		}
	}
}
