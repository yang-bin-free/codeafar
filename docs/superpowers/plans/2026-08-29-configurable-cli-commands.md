# Configurable CLI Commands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users configure the Claude/Codex launch command in the Mac app settings page, with support for parameterized commands (e.g. `dcc claude`), persisted to `~/.codeafar/config.yaml` and applied on restart.

**Architecture:** A new `ResolvedCommand{Path, PrependArgs}` flows from binary resolution through provider adapters into `session.ClaudeConfig`/`CodexConfig` as a `BinArgs []string` prefix before the existing CLI arguments. The runtime config gains `claudeCommand`/`codexCommand` fields validated at save time and read by `cmd/mac-app` at startup (flag > file > default).

**Tech Stack:** Go 1.26, `os/exec`, `strings.Fields`, `gopkg.in/yaml.v3`, vanilla JS admin UI, Playwright for UI acceptance.

**Spec:** `docs/superpowers/specs/2026-08-29-configurable-cli-commands-design.md`

## Global Constraints

- Repo stays wrapper-neutral: no detection or naming of any specific wrapper CLI anywhere in code, tests, or docs committed here.
- No runtime hot-switching: a saved command takes effect on app restart.
- No quoted arguments: commands are split on whitespace; quotes are rejected at validation.
- Backward compatibility: existing `config.yaml` files without the new fields keep working; single-word commands behave exactly as before (`PrependArgs` empty).
- Every task: write failing test first, verify RED, implement, verify GREEN, commit.

---

## File Map

- `pkg/desktop/command.go` (Create): `ResolvedCommand` type + `ResolveCommand(requested, defaultName, displayName string, includeClaudeLocal bool)` — multi-word splitting plus the existing search-path logic moved from `claude.go`.
- `pkg/desktop/claude.go` (Modify): `ResolveClaudeBinary`/`ResolveCodexBinary` return `ResolvedCommand`; delegate to `ResolveCommand`.
- `pkg/desktop/claude_test.go`, `pkg/desktop/command_test.go` (Modify/Create): resolution tests including multi-word cases.
- `pkg/engine/claude_version.go` (Modify): `DetectCLIVersion` accepts a command (path + prepend args).
- `pkg/session/claude.go` (Modify): `ClaudeConfig.BinArgs`, `Start()` prepends them.
- `pkg/session/codex.go` (Modify): `CodexConfig.BinArgs`, `Send()`/`Start()` prepend them.
- `pkg/session/claude_test.go`, `pkg/session/codex_test.go` (Modify): argument-order tests with a fake CLI script.
- `pkg/provider/provider.go` (Modify): `SessionConfig.BinArgs`; adapters carry `binArgs`.
- `pkg/provider/claude.go`, `pkg/provider/codex.go` (Modify): constructors take bin+args, `NewProcess` passes them through.
- `pkg/engine/config.go` (Modify): `Config.ClaudeBinArgs`/`CodexBinArgs`.
- `pkg/engine/engine.go` (Modify): registry construction passes args into adapters.
- `pkg/engine/runtime_config.go` (Modify): `runtimeConfig.ClaudeCommand`/`CodexCommand` + save-time validation.
- `pkg/engine/admin.go` (Modify): PATCH handler parses + validates commands.
- `pkg/engine/runtime_config_test.go`, `pkg/engine/admin_test.go` (Modify): validation and endpoint tests.
- `pkg/adminproto/adminproto.go` (Modify): `UpdateSettingsRequest.ClaudeCommand`/`CodexCommand`.
- `pkg/engine/settings_command.go` (Create): pure validation helpers shared by admin handler.
- `web/chat/index.html` (Modify): two inputs in settings form.
- `web/admin/admin.js` (Modify): load/save the two fields.
- `web/design_regression_test.go` (Modify): assert new controls exist.
- `cmd/mac-app/main.go` (Modify): read config.yaml commands when flags unset.
- `cmd/mac-app/application.go` (Modify): resolve/ detect against `ResolvedCommand`.
- `cmd/mac-app/application_test.go` (Modify): startup wiring tests.

---

### Task 1: ResolveCommand with multi-word support in pkg/desktop

**Files:**
- Create: `pkg/desktop/command.go`
- Create: `pkg/desktop/command_test.go`
- Modify: `pkg/desktop/claude.go`
- Modify: `pkg/desktop/claude_test.go`

**Interfaces:**
- Produces: `ResolvedCommand struct { Path string; PrependArgs []string }`, `func (c ResolvedCommand) String() string`, `func ResolveCommand(requested, defaultName, displayName string, includeClaudeLocal bool) (ResolvedCommand, error)`.
- `ResolveClaudeBinary`/`ResolveCodexBinary` change signature to `(ResolvedCommand, error)`.

- [ ] **Step 1: Write the failing tests**

Create `pkg/desktop/command_test.go`:

```go
package desktop

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveCommandSplitsMultiWordCommand(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, dir, "wrapper")
	t.Setenv("PATH", dir)
	got, err := ResolveCommand("wrapper claude extra", "claude", "Claude", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != filepath.Join(dir, "wrapper") {
		t.Fatalf("path=%q", got.Path)
	}
	if strings.Join(got.PrependArgs, " ") != "claude extra" {
		t.Fatalf("prepend=%v", got.PrependArgs)
	}
}

func TestResolveCommandSingleWordHasNoPrependArgs(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "claude")
	t.Setenv("PATH", dir)
	got, err := ResolveCommand("claude", "claude", "Claude", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != bin || len(got.PrependArgs) != 0 {
		t.Fatalf("got=%+v", got)
	}
}

func TestResolveCommandAbsolutePathWithArgs(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "wrapper")
	got, err := ResolveCommand(bin+" claude", "claude", "Claude", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != bin || len(got.PrependArgs) != 1 || got.PrependArgs[0] != "claude" {
		t.Fatalf("got=%+v", got)
	}
}

func TestResolveCommandMissingExecutableFails(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := ResolveCommand("nosuchwrapper claude", "claude", "Claude", false)
	if err == nil || !strings.Contains(err.Error(), "Claude") {
		t.Fatalf("err=%v", err)
	}
}

func TestResolvedCommandString(t *testing.T) {
	c := ResolvedCommand{Path: "/bin/wrapper", PrependArgs: []string{"claude"}}
	if c.String() != "/bin/wrapper claude" {
		t.Fatalf("string=%q", c.String())
	}
	if (ResolvedCommand{Path: "/bin/claude"}).String() != "/bin/claude" {
		t.Fatal("single-word String() broken")
	}
}
```

Update `pkg/desktop/claude_test.go` — the two existing tests and the Finder fallback test now expect `ResolvedCommand`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/desktop -run 'TestResolveCommand|TestResolveClaudeBinary' -count=1`
Expected: FAIL — `ResolveCommand` and `ResolvedCommand` undefined; existing tests fail to compile against the old signature.

- [ ] **Step 3: Implement ResolveCommand**

Create `pkg/desktop/command.go`:

```go
package desktop

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ResolvedCommand is an executable plus arguments that must precede the CLI's
// own arguments, supporting wrapper commands like "wrapper claude".
type ResolvedCommand struct {
	Path        string
	PrependArgs []string
}

func (c ResolvedCommand) String() string {
	if len(c.PrependArgs) == 0 {
		return c.Path
	}
	return c.Path + " " + strings.Join(c.PrependArgs, " ")
}

// ResolveCommand resolves the first word of a possibly multi-word command with
// the standard search paths and returns it with the remaining words as
// PrependArgs.
func ResolveCommand(requested, defaultName, displayName string, includeClaudeLocal bool) (ResolvedCommand, error) {
	words := strings.Fields(strings.TrimSpace(requested))
	if len(words) == 0 {
		words = []string{defaultName}
	}
	path, err := resolveCodingAgentBinary(words[0], defaultName, displayName, includeClaudeLocal)
	if err != nil {
		return ResolvedCommand{}, err
	}
	return ResolvedCommand{Path: path, PrependArgs: append([]string(nil), words[1:]...)}, nil
}
```

Rewrite `pkg/desktop/claude.go` — move the search logic into an unexported `resolveCodingAgentBinary(requested, defaultName, displayName string, includeClaudeLocal bool) (string, error)` with the exact body it has today (searched list, PATH, home candidates, nvm glob, homebrew), but resolving only the single first word. Keep the exported wrappers:

```go
// ResolveClaudeBinary finds a runnable Claude CLI even when the app was
// launched by Finder with a minimal PATH. Accepts wrapper commands such as
// "wrapper claude"; only the first word is resolved.
func ResolveClaudeBinary(requested string) (ResolvedCommand, error) {
	return ResolveCommand(requested, "claude", "Claude", true)
}

// ResolveCodexBinary finds Codex when Finder launches CodeAfar with a minimal
// PATH. Accepts wrapper commands such as "wrapper codex".
func ResolveCodexBinary(requested string) (ResolvedCommand, error) {
	return ResolveCommand(requested, "codex", "Codex", false)
}
```

The moved body keeps `os/exec`, `filepath`, `sort`, `strings` imports as needed; `claude.go` keeps only the two wrappers if the body now lives in `command.go` — put the moved search body in `command.go` and leave `claude.go` with just the wrappers and their doc comments.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/desktop -count=1`
Expected: PASS (whole package — other tests in `pkg/desktop` must still compile).

- [ ] **Step 5: Fix downstream compile errors, run full build**

Run: `go build ./...`
Expected: callers of the old signature fail to compile (`cmd/mac-app`, `cmd/mac-agent`, `pkg/engine` tests). Do NOT fix them here beyond what `go build ./pkg/...` needs — `cmd/` fixes are Task 6. `go build ./pkg/...` must pass.

Run: `go test ./pkg/desktop ./pkg/engine -count=1`
Expected: `pkg/desktop` PASS; `pkg/engine` compile errors are expected only if it references the resolvers — check `grep -rn "ResolveClaudeBinary" pkg/engine/` first; today only `cmd/` references them, so `pkg/engine` must PASS unchanged.

- [ ] **Step 6: Commit**

```bash
git add pkg/desktop/command.go pkg/desktop/command_test.go pkg/desktop/claude.go pkg/desktop/claude_test.go
git commit -m "feat: resolve multi-word CLI commands"
```

---

### Task 2: DetectCLIVersion accepts a command

**Files:**
- Modify: `pkg/engine/claude_version.go`
- Modify: `pkg/engine/claude_version_test.go` (create if absent)

**Interfaces:**
- `DetectCLIVersion(bin, productName string)` becomes `DetectCLIVersion(command []string, productName string) (string, error)` where `command[0]` is the executable and the rest are prepend args.

- [ ] **Step 1: Write the failing test**

Create/extend `pkg/engine/claude_version_test.go`:

```go
package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func writeVersionScript(t *testing.T, dir, name, version string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\necho '" + version "'\n"
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
	bin := writeVersionScript(t, dir, "wrapper", "2.1.245 (Claude Code)")
	got, err := DetectCLIVersion([]string{bin, "claude"}, "coding agent")
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
```

If the version script must observe the prepend arg to prove ordering, use this variant for the second test instead:

```go
func TestDetectCLIVersionWithPrependArgs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wrapper")
	script := "#!/bin/sh\necho \"$1 -> 2.1.245\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := DetectCLIVersion([]string{path, "claude"}, "coding agent")
	if err != nil || got != "2.1.245" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/engine -run TestDetectCLIVersion -count=1`
Expected: FAIL — signature mismatch (compile error).

- [ ] **Step 3: Implement**

In `pkg/engine/claude_version.go` replace the function body:

```go
// DetectCLIVersion runs `<command> --version` and extracts the semantic
// version. command[0] is the executable; remaining words are prepend args for
// wrapper CLIs.
func DetectCLIVersion(command []string, productName string) (string, error) {
	if len(command) == 0 {
		return "", errors.New("empty " + productName + " command")
	}
	args := append(append([]string(nil), command[1:]...), "--version")
	output, err := exec.Command(command[0], args...).CombinedOutput()
	if err != nil {
		return "", err
	}
	match := cliVersionPattern.FindSubmatch(output)
	if len(match) != 2 {
		return "", errors.New("unable to parse " + productName + " CLI version")
	}
	return string(match[1]), nil
}
```

- [ ] **Step 4: Fix callers of the old signature inside pkg/engine only**

Run: `grep -rn "DetectCLIVersion(" pkg/ --include="*.go" | grep -v claude_version`
Expected hits: `cmd/mac-agent/main.go`, `cmd/mac-app/application.go` (both fixed in Task 6) and any engine test helpers. Fix engine-internal callers by passing `[]string{bin}`. `go build ./pkg/...` and `go test ./pkg/engine -count=1` must pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/claude_version.go pkg/engine/claude_version_test.go
git commit -m "feat: detect CLI version for wrapper commands"
```

---

### Task 3: BinArgs flow through session and provider layers

**Files:**
- Modify: `pkg/session/claude.go`
- Modify: `pkg/session/codex.go`
- Modify: `pkg/session/claude_test.go`
- Modify: `pkg/session/codex_test.go`
- Modify: `pkg/provider/provider.go`
- Modify: `pkg/provider/claude.go`
- Modify: `pkg/provider/codex.go`
- Modify: `pkg/provider/provider_test.go`

**Interfaces:**
- `session.ClaudeConfig`/`CodexConfig` gain `BinArgs []string`.
- `provider.SessionConfig` gains `BinArgs []string`.
- `NewClaudeAdapter(bin string, args []string)` / `NewClaudeAdapterWithAvailability(bin string, args []string, available bool, reason string)` / `NewCodexAdapter(bin string, args []string, available bool, reason string)`.

- [ ] **Step 1: Write the failing session tests**

In `pkg/session/claude_test.go` add:

```go
func TestClaudeProcPrependsBinArgs(t *testing.T) {
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "wrapper")
	// The wrapper records the arguments it was invoked with, then execs nothing.
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
```

In `pkg/session/codex_test.go` add the equivalent:

```go
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
	_ = proc.Stop()
	data, err := os.ReadFile(filepath.Join(dir, "args.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(data))
	if len(got) < 3 || got[0] != "codex" {
		t.Fatalf("args=%v", got)
	}
}
```

In `pkg/provider/provider_test.go` add:

```go
func TestAdaptersPassBinArgsToSessionConfig(t *testing.T) {
	adapter := NewClaudeAdapter("/bin/wrapper", []string{"claude"})
	got := adapter.NewProcess(SessionConfig{Cwd: "/tmp", Permission: "default"})
	if got == nil {
		t.Fatal("nil process")
	}
}
```

(This compiles only after the signature change; it guards the constructor wiring.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/session ./pkg/provider -count=1`
Expected: FAIL — `BinArgs` undefined (compile error).

- [ ] **Step 3: Implement session layer**

`pkg/session/claude.go`:

```go
type ClaudeConfig struct {
	Bin          string   // claude 可执行文件路径（默认 "claude"）
	BinArgs      []string // 包装命令的前置参数（如 "dcc claude" 的 "claude"）
	Cwd          string
	SessionID    string
	Permission   string
	AddDirs      []string
	Resume       bool
	AllowedTools []string
}
```

`Start()`:

```go
func (p *ClaudeProc) Start() error {
	cmd := exec.Command(p.cfg.Bin, append(append([]string(nil), p.cfg.BinArgs...), p.buildArgs()...)...)
	cmd.Dir = p.cfg.Cwd
	// ... rest unchanged
}
```

`pkg/session/codex.go`: add `BinArgs []string` to `CodexConfig`; in `Send()` where `exec.Command(p.cfg.Bin, args...)` runs:

```go
cmd := exec.Command(p.cfg.Bin, append(append([]string(nil), p.cfg.BinArgs...), args...)...)
```

- [ ] **Step 4: Implement provider layer**

`pkg/provider/provider.go` — extend `SessionConfig`:

```go
type SessionConfig struct {
	Cwd               string
	SessionID         string
	ProviderSessionID string
	Permission        string
	Model             string
	Resume            bool
	AddDirs           []string
	AllowedTools      []string
	BinArgs           []string
}
```

`pkg/provider/claude.go`:

```go
type ClaudeAdapter struct {
	bin               string
	binArgs           []string
	available         bool
	unavailableReason string
}

func NewClaudeAdapter(bin string, binArgs []string) *ClaudeAdapter {
	if bin == "" {
		bin = "claude"
	}
	return &ClaudeAdapter{bin: bin, binArgs: binArgs, available: true}
}

func NewClaudeAdapterWithAvailability(bin string, binArgs []string, available bool, unavailableReason string) *ClaudeAdapter {
	adapter := NewClaudeAdapter(bin, binArgs)
	adapter.available = available
	adapter.unavailableReason = unavailableReason
	return adapter
}
```

`NewProcess`:

```go
func (a *ClaudeAdapter) NewProcess(config SessionConfig) Process {
	return session.NewClaudeProc(session.ClaudeConfig{
		Bin: a.bin, BinArgs: a.binArgs, Cwd: config.Cwd, SessionID: config.SessionID,
		Permission: config.Permission, AddDirs: config.AddDirs, Resume: config.Resume,
		AllowedTools: config.AllowedTools,
	})
}
```

`pkg/provider/codex.go`: same pattern — `CodexAdapter{bin, binArgs, available, unavailableReason}`, `NewCodexAdapter(bin string, binArgs []string, available bool, unavailableReason string)`, and `NewProcess` passes `BinArgs: a.binArgs`.

- [ ] **Step 5: Run tests, fix engine callers**

Run: `go test ./pkg/session ./pkg/provider -count=1`
Expected: PASS.

Run: `go build ./pkg/... && go test ./pkg/engine -count=1`
Expected: engine compile errors in `engine.go` where `NewClaudeAdapterWithAvailability`/`NewCodexAdapter` are called. Update `pkg/engine/engine.go` (line ~90):

```go
e.providers = provider.NewRegistry(
	provider.NewClaudeAdapterWithAvailability(cfg.ClaudeBin, cfg.ClaudeBinArgs, cfg.ClaudeUnavailableReason == "", cfg.ClaudeUnavailableReason),
	provider.NewCodexAdapter(cfg.CodexBin, cfg.CodexBinArgs, cfg.CodexUnavailableReason == "", cfg.CodexUnavailableReason),
)
```

and add to `pkg/engine/config.go`:

```go
ClaudeBinArgs           []string
CodexBinArgs            []string
```

Also update `claudeFactoryAdapter` in `engine.go` — its `NewProcess` must pass `BinArgs: c.BinArgs` into `session.ClaudeConfig` and `Descriptor()` uses `NewClaudeAdapter("claude", nil)`.

Run: `go test ./pkg/engine -count=1` — Expected: PASS (fix any remaining engine test callers the same way: `NewClaudeAdapter("claude", nil)`).

- [ ] **Step 6: Commit**

```bash
git add pkg/session pkg/provider pkg/engine
git commit -m "feat: pass wrapper args through session and provider layers"
```

---

### Task 4: Command validation and runtime config persistence

**Files:**
- Create: `pkg/engine/settings_command.go`
- Create: `pkg/engine/settings_command_test.go`
- Modify: `pkg/engine/runtime_config.go`
- Modify: `pkg/engine/runtime_config_test.go` (create if absent)
- Modify: `pkg/adminproto/adminproto.go`
- Modify: `pkg/engine/admin.go`
- Modify: `pkg/engine/admin_test.go`

**Interfaces:**
- `func ValidateCommandSetting(command, displayName string, resolve func(string) (string, error)) (string, error)` — returns the normalized command (empty stays empty); rejects >8 words, >200 chars, quotes, unresolvable executables.
- `runtimeConfig` gains `ClaudeCommand string` / `CodexCommand string`.

- [ ] **Step 1: Write the failing validation tests**

Create `pkg/engine/settings_command_test.go`:

```go
package engine

import (
	"errors"
	"strings"
	"testing"
)

func fakeResolve(ok bool) func(string) (string, error) {
	return func(string) (string, error) {
		if ok {
			return "/resolved/bin", nil
		}
		return "", errors.New("not found")
	}
}

func TestValidateCommandSettingAcceptsEmpty(t *testing.T) {
	got, err := ValidateCommandSetting("", "Claude", fakeResolve(true))
	if err != nil || got != "" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestValidateCommandSettingAcceptsResolvable(t *testing.T) {
	got, err := ValidateCommandSetting("wrapper claude", "Claude", fakeResolve(true))
	if err != nil || got != "wrapper claude" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestValidateCommandSettingRejectsQuotes(t *testing.T) {
	for _, bad := range []string{`wrapper "a b"`, "wrapper 'x'", "wrapper\ttab"} {
		if _, err := ValidateCommandSetting(bad, "Claude", fakeResolve(true)); err == nil {
			t.Fatalf("expected quote rejection for %q", bad)
		}
	}
}

func TestValidateCommandSettingRejectsTooManyWords(t *testing.T) {
	words := make([]string, 9)
	for i := range words {
		words[i] = "w"
	}
	if _, err := ValidateCommandSetting(strings.Join(words, " "), "Claude", fakeResolve(true)); err == nil {
		t.Fatal("expected word-count rejection")
	}
}

func TestValidateCommandSettingRejectsTooLong(t *testing.T) {
	long := strings.Repeat("a", 201)
	if _, err := ValidateCommandSetting(long, "Claude", fakeResolve(true)); err == nil {
		t.Fatal("expected length rejection")
	}
}

func TestValidateCommandSettingRejectsUnresolvable(t *testing.T) {
	if _, err := ValidateCommandSetting("nosuch bin", "Claude", fakeResolve(false)); err == nil {
		t.Fatal("expected resolution rejection")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/engine -run TestValidateCommandSetting -count=1`
Expected: FAIL — function undefined.

- [ ] **Step 3: Implement validation**

Create `pkg/engine/settings_command.go`:

```go
package engine

import (
	"errors"
	"strings"
)

const (
	commandMaxWords = 8
	commandMaxBytes = 200
)

// ValidateCommandSetting validates a user-entered CLI command. Empty means
// "use the default". Non-empty commands are whitespace-split, must not
// contain quotes, must stay within size limits, and their first word must
// resolve to an executable through the provided resolver.
func ValidateCommandSetting(command, displayName string, resolve func(string) (string, error)) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil
	}
	if strings.ContainsAny(command, "\"'") {
		return "", errors.New(displayName + " 命令不支持引号参数")
	}
	if strings.ContainsRune(command, '\t') || strings.ContainsRune(command, '\n') {
		return "", errors.New(displayName + " 命令包含非法空白字符")
	}
	if len(command) > commandMaxBytes {
		return "", errors.New(displayName + " 命令超过长度限制")
	}
	words := strings.Fields(command)
	if len(words) > commandMaxWords {
		return "", errors.New(displayName + " 命令参数过多")
	}
	if _, err := resolve(words[0]); err != nil {
		return "", errors.New(displayName + " 命令的第一个词不是可执行文件")
	}
	return command, nil
}
```

Note: `strings.Fields` already splits on tabs/newlines, so the explicit tab/newline check above makes the rejection message explicit — keep both.

- [ ] **Step 4: Run validation tests**

Run: `go test ./pkg/engine -run TestValidateCommandSetting -count=1`
Expected: PASS.

- [ ] **Step 5: Wire into runtime config + admin API (failing test first)**

In `pkg/engine/runtime_config_test.go` (create if absent) add:

```go
func TestUpdateRuntimeConfigPersistsCommands(t *testing.T) {
	e := newTestEngine(t) // reuse the existing test-engine helper in the package
	dir := t.TempDir()
	bin := writeExecutableScript(t, dir, "wrapper") // script that exits 0
	claude, err := ValidateCommandSetting(bin+" claude", "Claude", func(s string) (string, error) { return s, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := e.updateRuntimeConfig(runtimeConfig{
		DefaultWorkingDir: t.TempDir(), DefaultPermission: "default",
		MaxConcurrentSessions: 2, ClaudeCommand: claude, CodexCommand: "",
	}); err != nil {
		t.Fatal(err)
	}
	got := e.runtimeConfig()
	if got.ClaudeCommand != bin+" claude" {
		t.Fatalf("claudeCommand=%q", got.ClaudeCommand)
	}
	if got.CodexCommand != "" {
		t.Fatalf("codexCommand=%q", got.CodexCommand)
	}
}
```

Use whatever test-engine construction helper already exists in `pkg/engine` tests (`grep -n "func newTestEngine\|engine.New(" pkg/engine/*_test.go | head`); if none exists, construct `engine.New(Config{DataDir: t.TempDir(), DefaultWorkingDir: t.TempDir()})` and close it with the pattern used by existing engine tests. `writeExecutableScript` mirrors `pkg/desktop`'s helper — define locally.

In `pkg/engine/admin_test.go` add:

```go
func TestAdminSettingsRejectsUnresolvableCommand(t *testing.T) {
	// Follow the existing admin-handler test setup in this file; POST/PATCH
	// /admin/settings with {"defaultWorkingDir": <valid>, "defaultPermission":"default",
	// "maxConcurrentSessions":2, "claudeCommand":"nosuchwrapper claude"} and
	// assert HTTP 400 and that config.yaml was not written with the command.
}
```

Implement the assertion concretely once you see the existing handler test scaffolding (there are existing `TestAdmin...` tests using the engine's admin mux) — reuse their `httptest` pattern verbatim.

- [ ] **Step 6: Implement persistence + handler**

`runtimeConfig`:

```go
type runtimeConfig struct {
	DefaultWorkingDir     string `yaml:"defaultWorkingDir"`
	DefaultPermission     string `yaml:"defaultPermission"`
	MaxConcurrentSessions int    `yaml:"maxConcurrentSessions"`
	ClaudeCommand         string `yaml:"claudeCommand,omitempty"`
	CodexCommand          string `yaml:"codexCommand,omitempty"`
}
```

`updateRuntimeConfig` signature gains the two commands; validation happens in the admin handler (which owns the resolver), while `updateRuntimeConfig` persists what it is given. In `reloadRuntimeConfig` treat empty commands as "keep current" exactly like the other fields:

```go
if next.ClaudeCommand == "" {
	next.ClaudeCommand = current.ClaudeCommand
}
if next.CodexCommand == "" {
	next.CodexCommand = current.CodexCommand
}
```

Wait — this conflates "unset" with "user cleared the field". Decision (locked): `reloadRuntimeConfig` copies commands verbatim (empty means default); only the three legacy fields keep the fallback. Remove the two blocks above and do nothing for commands in reload.

`pkg/adminproto/adminproto.go`:

```go
type UpdateSettingsRequest struct {
	DefaultWorkingDir     string `json:"defaultWorkingDir"`
	DefaultPermission     string `json:"defaultPermission"`
	MaxConcurrentSessions int    `json:"maxConcurrentSessions"`
	ClaudeCommand         string `json:"claudeCommand,omitempty"`
	CodexCommand          string `json:"codexCommand,omitempty"`
}
```

`pkg/engine/admin.go` PATCH handler:

```go
mux.HandleFunc("PATCH /admin/settings", func(w http.ResponseWriter, r *http.Request) {
	var request adminproto.UpdateSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAdminBodyBytes)).Decode(&request); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	claudeCommand, err := e.validateCommand(request.ClaudeCommand, "Claude")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	codexCommand, err := e.validateCommand(request.CodexCommand, "Codex")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := e.updateRuntimeConfig(runtimeConfig{
		DefaultWorkingDir: request.DefaultWorkingDir, DefaultPermission: request.DefaultPermission,
		MaxConcurrentSessions: request.MaxConcurrentSessions,
		ClaudeCommand:         claudeCommand, CodexCommand: codexCommand,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
})
```

Add to `pkg/engine/settings_command.go`:

```go
// validateCommand validates a settings command with the production resolver.
func (e *Engine) validateCommand(command, displayName string) (string, error) {
	return ValidateCommandSetting(command, displayName, func(name string) (string, error) {
		return desktopResolveWord(name)
	})
}
```

`desktopResolveWord` is a package var (`var desktopResolveWord = func(name string) (string, error) { return desktop.ResolveCommandWord(name) }`) so tests can stub it. This requires one more small export in `pkg/desktop`:

```go
// ResolveCommandWord resolves a single executable word through the standard
// CLI search paths.
func ResolveCommandWord(requested string) (string, error) {
	return resolveCodingAgentBinary(requested, requested, requested, false)
}
```

Hmm — `resolveCodingAgentBinary`'s error message includes the displayName; passing `requested` as displayName keeps it generic. Tests stub `desktopResolveWord` anyway. Also expose the command fields on the snapshot: extend `adminproto.AgentStatus` with `ClaudeCommand`/`CodexCommand string` and set them in `adminStatus` from a new `runtimeConfig` read (NOT from `cfg.ClaudeBin`, which is the startup-time flag value). The engine needs an accessor:

```go
func (e *Engine) commands() (claude, codex string) {
	rc := e.runtimeConfig()
	return rc.ClaudeCommand, rc.CodexCommand
}
```

and `adminStatus` gains parameters or the snapshot handler reads `e.commands()` — implement by extending the existing snapshot handler where `adminStatus(e.Status())` is called, passing the two strings through.

- [ ] **Step 7: Run engine tests**

Run: `go test ./pkg/engine ./pkg/adminproto -count=1`
Expected: PASS, including the new persistence and 400 tests.

- [ ] **Step 8: Commit**

```bash
git add pkg/engine pkg/adminproto
git commit -m "feat: persist and validate CLI command settings"
```

---

### Task 5: Admin UI for the two commands

**Files:**
- Modify: `web/chat/index.html`
- Modify: `web/admin/admin.js`
- Modify: `web/design_regression_test.go`

**Interfaces:** Two new inputs `#settings-claude-command` / `#settings-codex-command`, loaded from `agent.claudeCommand`/`agent.codexCommand`, submitted in the PATCH body.

- [ ] **Step 1: Write the failing regression test**

In `web/design_regression_test.go` extend the id list check:

```go
for _, id := range []string{
	"device-name", "project-name", "project-path", "template-label",
	"template-prompt", "permission-tool", "permission-pattern",
	"settings-claude-command", "settings-codex-command",
} {
```

And add a JS-content check after the existing ones:

```go
	if !strings.Contains(js, `settings-claude-command`) || !strings.Contains(js, `settings-codex-command`) {
		t.Error("admin JS does not handle CLI command settings")
	}
```

(Read `admin/admin.js` via `fs.ReadFile(Assets, "admin/admin.js")` alongside the existing chat.js read.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./web -run TestDesktopAdminForms -count=1`
Expected: FAIL — ids missing.

- [ ] **Step 3: Implement HTML**

In `web/chat/index.html` settings form (after the concurrency input):

```html
<label>Claude 命令<input id="settings-claude-command" type="text" placeholder="claude"></label>
<label>Codex 命令<input id="settings-codex-command" type="text" placeholder="codex"></label>
```

The existing form labels have no `for` attribute but pass the persistent-label check because the label wraps the input — match that pattern exactly.

- [ ] **Step 4: Implement JS**

In `web/admin/admin.js` load section (next to the other three):

```js
document.querySelector("#settings-claude-command").value = agent.claudeCommand || "";
document.querySelector("#settings-codex-command").value = agent.codexCommand || "";
```

In the submit body add:

```js
claudeCommand: document.querySelector("#settings-claude-command").value.trim(),
codexCommand: document.querySelector("#settings-codex-command").value.trim(),
```

- [ ] **Step 5: Run tests**

Run: `go test ./web -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/chat/index.html web/admin/admin.js web/design_regression_test.go
git commit -m "feat: add CLI command fields to admin settings"
```

---

### Task 6: Startup wiring in cmd/mac-app and cmd/mac-agent

**Files:**
- Modify: `cmd/mac-app/main.go`
- Modify: `cmd/mac-app/application.go`
- Modify: `cmd/mac-app/application_test.go`
- Modify: `cmd/mac-agent/main.go`

**Interfaces:**
- `main.go`: when `--claude-bin`/`--codex-bin` are not explicitly set, read the commands from `~/.codeafar/config.yaml` (via a new exported engine helper `engine.ReadPersistedCommands(dataDir string) (claude, codex string)`).
- `application.go`: `resolveProvider` works with `desktop.ResolvedCommand`; `detectVersion` takes `[]string`.

- [ ] **Step 1: Write the failing test**

Add `ReadPersistedCommands` test in `pkg/engine/runtime_config_test.go`:

```go
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
```

In `cmd/mac-app/application_test.go` update existing tests that stub `resolveClaude`/`resolveCodex`/`detectVersion` to the new signatures (`resolveClaude func(string) (desktop.ResolvedCommand, error)`, `detectVersion func([]string, string) (string, error)`) and add:

```go
func TestStartUsesConfigFileCommandWhenFlagUnset(t *testing.T) {
	// newApplication with cfg.ClaudeBin pre-filled by main.go's file read
	// (simulate: ClaudeBin = "wrapper claude"); stub resolveClaude to return
	// ResolvedCommand{Path: "/bin/wrapper", PrependArgs: []string{"claude"}}
	// and detectVersion to return "2.1.245"; assert the started engine Config
	// carries ClaudeBin="/bin/wrapper" and ClaudeBinArgs=["claude"].
}
```

Implement following the existing `TestStart*` tests' scaffolding in that file (they already build the application with stub deps and inspect `engine.Config` via a fake `newEngine`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/engine -run TestReadPersistedCommands -count=1 && go test ./cmd/mac-app -count=1`
Expected: FAIL — helper undefined / signature compile errors.

- [ ] **Step 3: Implement ReadPersistedCommands**

In `pkg/engine/runtime_config.go`:

```go
// ReadPersistedCommands returns the persisted Claude/Codex launch commands
// from the data dir's config.yaml. Empty strings mean "use defaults".
func ReadPersistedCommands(dataDir string) (claude, codex string) {
	b, err := os.ReadFile(filepath.Join(dataDir, "config.yaml"))
	if err != nil {
		return "", ""
	}
	var cfg struct {
		ClaudeCommand string `yaml:"claudeCommand"`
		CodexCommand  string `yaml:"codexCommand"`
	}
	if yaml.Unmarshal(b, &cfg) != nil {
		return "", ""
	}
	return strings.TrimSpace(cfg.ClaudeCommand), strings.TrimSpace(cfg.CodexCommand)
}
```

(Add the `strings` import.)

- [ ] **Step 4: Implement main.go flag-vs-file precedence**

In `cmd/mac-app/main.go` after `fs.Parse`:

```go
claudeCommand := *claudeBin
codexCommand := *codexBin
claudeFlagSet := false
codexFlagSet := false
fs.Visit(func(f *flag.Flag) {
	switch f.Name {
	case "claude-bin":
		claudeFlagSet = true
	case "codex-bin":
		codexFlagSet = true
	}
})
if !claudeFlagSet || !codexFlagSet {
	fileClaude, fileCodex := engine.ReadPersistedCommands(resolvedDataDir)
	if !claudeFlagSet && fileClaude != "" {
		claudeCommand = fileClaude
	}
	if !codexFlagSet && fileCodex != "" {
		codexCommand = fileCodex
	}
}
```

then pass `ClaudeBin: claudeCommand, CodexBin: codexCommand` into `appConfig`.

- [ ] **Step 5: Implement application.go command flow**

`resolveProvider` becomes:

```go
func (a *application) resolveProvider(requested, name string, resolve func(string) (desktop.ResolvedCommand, error)) (desktop.ResolvedCommand, string, error) {
	command, err := resolve(requested)
	if err != nil {
		return desktop.ResolvedCommand{}, "", err
	}
	version, err := a.deps.detectVersion(command.Args())
	if err != nil {
		return desktop.ResolvedCommand{}, "", fmt.Errorf("%s CLI check failed: %w", name, err)
	}
	return command, version, nil
}
```

Add to `pkg/desktop/command.go`:

```go
// Args returns the command as an argv slice: executable followed by prepend
// args.
func (c ResolvedCommand) Args() []string {
	return append([]string{c.Path}, c.PrependArgs...)
}
```

Engine Config construction gains:

```go
ClaudeBin:               firstNonEmpty(claudeCommand.Path, a.cfg.ClaudeBin, "claude"),
ClaudeBinArgs:           claudeCommand.PrependArgs,
CodexBin:                firstNonEmpty(codexCommand.Path, a.cfg.CodexBin, "codex"),
CodexBinArgs:            codexCommand.PrependArgs,
```

and `AppStatus`/menu-state fields keep using `command.String()` for display. Update `cmd/mac-agent/main.go` `resolveServeProviders` the same way (it already receives the resolvers as parameters; change the param types and thread `ResolvedCommand` into `cfg.ClaudeBin`/`ClaudeBinArgs`).

- [ ] **Step 6: Run all Go tests**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS across all packages (this is the first task where `cmd/` compiles again).

- [ ] **Step 7: Commit**

```bash
git add cmd pkg/engine
git commit -m "feat: load persisted CLI commands at startup"
```

---

### Task 7: End-to-end verification with a fake wrapper CLI

**Files:**
- Create: `cmd/mac-app/fake_cli_e2e_test.go`

**Interfaces:** A black-box test that boots the mac-app application with a fake wrapper CLI (shell script emitting valid stream-json) configured via config.yaml, and drives one full chat turn.

- [ ] **Step 1: Write the failing e2e test**

```go
func TestApplicationEndToEndWithWrapperCommand(t *testing.T) {
	// 1. temp data dir; write config.yaml with claudeCommand: "<wrapper> claude"
	// 2. wrapper script: a shell script that, when invoked with first arg
	//    "claude", emits a minimal stream-json turn on stdout:
	//      {"type":"system","subtype":"init",...}
	//      {"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}}
	//      {"type":"result","subtype":"success","result":"hi","is_error":false}
	//    reading one user JSON line from stdin first.
	// 3. newApplication with ClaudeBin: "<wrapper-path> claude", stub deps where
	//    the existing application tests already stub them EXCEPT resolveClaude/
	//    detectVersion, which run for real against the wrapper.
	// 4. Drive the engine's WebSocket: create session, send first message,
	//    assert a token event with "hi" arrives.
	// Follow the existing real_claude_e2e_test.go engine-level pattern for the
	// WebSocket client; the application test only needs engine.Config
	// inspection plus one engine-level turn using the fake factory.
}
```

Model the WebSocket driving on `pkg/engine/real_claude_e2e_test.go` (which already builds a session and asserts token output); the difference is the Claude process comes from a wrapper script.

- [ ] **Step 2: Run to verify it fails, then implement the test fully**

Run: `go test ./cmd/mac-app -run TestApplicationEndToEndWithWrapperCommand -count=1`
Expected: initially FAIL (test not yet implemented beyond scaffold); implement it fully and reach PASS.

- [ ] **Step 3: Run the full suite**

Run: `go test ./... -count=1 && go test -race ./pkg/engine ./pkg/session ./pkg/provider ./pkg/desktop -count=1`
Expected: PASS everywhere, no race.

- [ ] **Step 4: Commit**

```bash
git add cmd/mac-app/fake_cli_e2e_test.go
git commit -m "test: cover wrapper CLI end to end"
```

---

### Task 8: Playwright UI acceptance + README note

**Files:**
- Create: `scripts/ui-test-cli-settings.py` (or `.js` — follow whatever Playwright convention the repo already uses; check `docs/testing/` first)
- Modify: `README.md` (one paragraph, wrapper-neutral)
- Modify: `docs/testing/mac-v1-acceptance-plan.md` (only if it enumerates settings fields)

**Interfaces:** Real-browser proof per the project's UI-testing rule: open the running Mac app's admin page, set the Claude command to a local fake wrapper, save, restart the app process, verify the chat uses the wrapper.

- [ ] **Step 1: Check existing Playwright conventions**

Run: `ls docs/testing/ && grep -rn "playwright" docs/ scripts/ --include="*.md" -l | head -5`
Read the existing acceptance plan to copy its launch/login/screenshot pattern.

- [ ] **Step 2: Write the Playwright script**

Script outline (adapt to repo conventions):

```python
# 1. Launch the mac app binary with a temp data dir (or reuse the script from
#    the existing acceptance plan).
# 2. page.goto(admin URL with token)
# 3. Fill #settings-claude-command with the fake wrapper path + " claude".
# 4. Click 保存设置; assert the success feedback and a 204 (page.on("response")).
# 5. Read ~/.codeafar/config.yaml (the temp dir) and assert the command line.
# 6. Reload the page; assert the input still shows the wrapper command.
# 7. Restart the app process; open chat; create a session in a temp project
#    dir; send "hi"; assert the assistant bubble shows the fake wrapper's
#    canned "hi from wrapper" token.
# 8. page.screenshot to docs/testing/artifacts or the plan's evidence dir.
# Negative: enter "nosuchcommand xyz", save, assert visible error, assert
# config.yaml unchanged.
```

- [ ] **Step 3: Run it against the real app and iterate until green**

Run: `python3 scripts/ui-test-cli-settings.py` (or the repo's runner)
Expected: all assertions pass, screenshots saved.

- [ ] **Step 4: README paragraph (wrapper-neutral)**

In `README.md` Mac V1 section, after the permission-modes part, add:

```markdown
### 自定义启动命令

设置页可以把 Claude/Codex 的启动命令改成任意「可执行文件 + 参数」的形式（例如本地的包装脚本）。命令在保存时校验：第一个词必须是可执行文件，最多 8 个参数、200 个字符，不支持引号参数。修改在重启应用后生效；会话历史与恢复不受影响。
```

- [ ] **Step 5: Commit**

```bash
git add scripts/ README.md docs/testing/
git commit -m "test: verify CLI settings through the browser"
```

---

### Task 9: Final verification and merge prep

- [ ] **Step 1: Full suite**

Run: `go build ./... && go test ./... -count=1 && go test -race ./... -count=1`
Expected: PASS.

- [ ] **Step 2: Manual smoke with a real wrapper (optional, local only)**

If a wrapper CLI is installed locally, run the Mac app with `--claude-bin "<wrapper> claude"`, send one message in the chat UI, confirm streaming works. Record the result in the worktree notes; do not commit wrapper-specific anything.

- [ ] **Step 3: Update plan checkboxes and merge**

Mark all boxes, squash-merge or merge `feat/cli-commands` into `master` per repo convention (check `git log --merges --oneline | head -3` for the usual style), push.

```bash
git checkout master && git merge --no-ff feat/cli-commands -m "feat: configurable CLI launch commands"
git push origin master
```

---

## Self-Review Notes

- Spec §2 (no hot-switch, no quotes, wrapper-neutral) → Tasks 4 (validation), 6 (restart-only), 8 (README text).
- Spec §3 (verified compat) → no code task needed; recorded as precondition.
- Spec §5 (precedence flag > file > default) → Task 6 Step 4.
- Spec §6 A/B/C/D → Tasks 1, 2, 3, 4, 5, 6.
- Spec §7 (startup failure path) → existing unavailable-provider path; Task 6 keeps `ClaudeUnavailableReason` wiring.
- Spec §8 (unit + UI + real-wrapper manual) → Tasks 3, 7, 8, 9.
- Type consistency: `ResolvedCommand{Path, PrependArgs}` with `String()`/`Args()` used in Tasks 1 and 6; `BinArgs []string` in Tasks 3 and 6; `ClaudeCommand`/`CodexCommand` in Tasks 4, 5, 6.
